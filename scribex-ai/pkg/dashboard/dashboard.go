// Package dashboard serves cross-repo doc health with team-scoped RBAC.
//
// repos file: `name path team` per line. rbac file: `sha256(token) role team,team` per line
// (role admin sees all repos, viewer only its teams). Clients send the token as the HTTP Basic password.
package dashboard

import (
	"container/list"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"scribex/pkg/config"
	"scribex/pkg/docs"
	"scribex/pkg/mcp"
	"scribex/pkg/wiki"
)

var (
	nameRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	hashRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	ErrBusy = errors.New("server busy; measure queue full")
)

const styleCSS = "body{font:14px system-ui;margin:2rem}td,th{padding:.3rem .8rem;text-align:left}.error{color:#b00}"

type Repo struct {
	Name string
	Path string
	Team string
}

type Principal struct {
	Hash  string
	Admin bool
	Teams map[string]bool
}

type Health struct {
	Repo       string  `json:"repo"`
	Team       string  `json:"team"`
	Total      int     `json:"total"`
	Stale      int     `json:"stale"`
	Dead       int     `json:"dead"`
	WikiIssues int     `json:"wiki_issues"`
	Drift      float64 `json:"drift"`
	Error      bool    `json:"error,omitempty"`
}

type Config struct {
	AllowedRoot    string
	CacheTTL       time.Duration
	BehindProxy    bool
	TrustedProxies []netip.Prefix
	Now            func() time.Time
	MeasureFunc    func(Repo) (Health, error)
	AccessLog      io.Writer
}

// HashToken is what goes in the rbac file.
func HashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// Authenticate performs a constant-time comparison against every principal in the list.
func Authenticate(tokenHash string, principals []Principal) (*Principal, int) {
	var match *Principal
	comparisons := 0
	tokenBytes := []byte(tokenHash)
	for i := range principals {
		comparisons++
		pBytes := []byte(principals[i].Hash)
		if subtle.ConstantTimeCompare(tokenBytes, pBytes) == 1 {
			match = &principals[i]
		}
	}
	return match, comparisons
}

type ipState struct {
	ip           string
	failures     []time.Time
	lockoutUntil time.Time
}

type ipLimiter struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List
	maxSize int
}

func newIPLimiter(maxSize int) *ipLimiter {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &ipLimiter{
		entries: make(map[string]*list.Element),
		lru:     list.New(),
		maxSize: maxSize,
	}
}

func (l *ipLimiter) get(ip string) *ipState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.entries[ip]; ok {
		l.lru.MoveToFront(el)
		s := el.Value.(*ipState)
		cp := *s
		return &cp
	}
	return nil
}

func (l *ipLimiter) checkLockout(ip string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.entries[ip]
	if !ok {
		return false, 0
	}
	l.lru.MoveToFront(el)
	s := el.Value.(*ipState)
	if now.Before(s.lockoutUntil) {
		rem := int(s.lockoutUntil.Sub(now).Seconds())
		if rem <= 0 {
			rem = 1
		}
		return true, rem
	}
	return false, 0
}

func (l *ipLimiter) recordFailure(ip string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var s *ipState
	if el, ok := l.entries[ip]; ok {
		l.lru.MoveToFront(el)
		s = el.Value.(*ipState)
	} else {
		if l.lru.Len() >= l.maxSize {
			oldest := l.lru.Back()
			if oldest != nil {
				l.lru.Remove(oldest)
				delete(l.entries, oldest.Value.(*ipState).ip)
			}
		}
		s = &ipState{ip: ip}
		el := l.lru.PushFront(s)
		l.entries[ip] = el
	}

	// Prune failures older than 60s
	var valid []time.Time
	for _, t := range s.failures {
		if now.Sub(t) <= 60*time.Second {
			valid = append(valid, t)
		}
	}
	valid = append(valid, now)
	s.failures = valid

	if len(s.failures) >= 5 {
		s.lockoutUntil = now.Add(15 * time.Minute)
		return true, 900
	}
	return false, 0
}

func lines(path string) ([][]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out [][]string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) > 0 {
			out = append(out, f)
		}
	}
	return out, nil
}

func checkPathConfinement(allowedRoot, targetPath string) (string, error) {
	cleanRoot, err := filepath.EvalSymlinks(allowedRoot)
	if err != nil {
		return "", fmt.Errorf("invalid allowed-root %s: %w", allowedRoot, err)
	}
	realTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return "", fmt.Errorf("invalid repo path %s: %w", targetPath, err)
	}
	fi, err := os.Stat(realTarget)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("repo path %s must be an existing directory", targetPath)
	}

	rel, err := filepath.Rel(cleanRoot, realTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("repo path %s resolves outside allowed-root", targetPath)
	}
	return realTarget, nil
}

// LoadRepos parses and strictly validates the repos file against allowedRoot.
func LoadRepos(path string, allowedRoot string) ([]Repo, error) {
	if allowedRoot == "" {
		return nil, errors.New("allowed-root is required")
	}
	ls, err := lines(path)
	if err != nil {
		return nil, err
	}
	var rs []Repo
	seen := make(map[string]bool)

	for lineNum, f := range ls {
		if len(f) != 3 {
			return nil, fmt.Errorf("repos line %d: expected exactly 3 fields (name path team), got %d", lineNum+1, len(f))
		}
		name, repoPath, team := f[0], f[1], f[2]
		if !nameRe.MatchString(name) {
			return nil, fmt.Errorf("repos line %d: invalid name %q", lineNum+1, name)
		}
		if !nameRe.MatchString(team) {
			return nil, fmt.Errorf("repos line %d: invalid team %q", lineNum+1, team)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate repo name %q", name)
		}
		seen[name] = true

		if _, err := checkPathConfinement(allowedRoot, repoPath); err != nil {
			return nil, err
		}

		rs = append(rs, Repo{Name: name, Path: repoPath, Team: team})
	}
	return rs, nil
}

// LoadRBAC parses and strictly validates the rbac file.
func LoadRBAC(path string) ([]Principal, error) {
	ls, err := lines(path)
	if err != nil {
		return nil, err
	}
	var ps []Principal
	seen := make(map[string]bool)

	for lineNum, f := range ls {
		if len(f) < 2 || len(f) > 3 {
			return nil, fmt.Errorf("rbac line %d: expected 2 or 3 fields (<hash> <admin|viewer> [teams]), got %d", lineNum+1, len(f))
		}
		h, role := f[0], f[1]
		if !hashRe.MatchString(h) {
			return nil, fmt.Errorf("rbac line %d: invalid sha256 hash %q", lineNum+1, h)
		}
		if role != "admin" && role != "viewer" {
			return nil, fmt.Errorf("rbac line %d: invalid role %q; must be admin or viewer", lineNum+1, role)
		}
		if seen[h] {
			return nil, fmt.Errorf("duplicate token hash in rbac: %q", h)
		}
		seen[h] = true

		p := Principal{
			Hash:  h,
			Admin: role == "admin",
			Teams: make(map[string]bool),
		}
		if len(f) == 3 && f[2] != "" {
			for _, t := range strings.Split(f[2], ",") {
				t = strings.TrimSpace(t)
				if !nameRe.MatchString(t) {
					return nil, fmt.Errorf("rbac line %d: invalid team name %q", lineNum+1, t)
				}
				p.Teams[t] = true
			}
		}
		ps = append(ps, p)
	}
	return ps, nil
}

type cacheEntry struct {
	health    Health
	expiresAt time.Time
}

type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*sfCall
}

type sfCall struct {
	wg  sync.WaitGroup
	val Health
	err error
}

func (g *singleflightGroup) Do(key string, fn func() (Health, error)) (Health, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*sfCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(sfCall)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}

// Server implements the hardened HTTP dashboard.
type Server struct {
	config     Config
	repos      []Repo
	principals []Principal
	cfgRoot    string
	limiter    *ipLimiter
	cache      map[string]*cacheEntry
	cacheMu    sync.RWMutex
	sf         singleflightGroup
	semaphore  chan struct{}
	styleHash  string
	handler    http.Handler
}

// Measure performs standard documentation health measurement on a repository.
func Measure(r Repo) (Health, error) {
	rep, err := docs.Check(r.Path, false, nil)
	h := Health{Repo: r.Name, Team: r.Team, Total: rep.Total}
	if cfg, cerr := config.Load(r.Path); cerr == nil {
		if w, werr := wiki.Load(filepath.Join(r.Path, cfg.Wiki.Dir)); werr == nil {
			h.WikiIssues = len(w.Lint(cfg.Wiki.Home))
		}
	}
	for _, i := range rep.Issues {
		if i.Stale {
			h.Stale++
		} else {
			h.Dead++
		}
	}
	if h.Total > 0 {
		h.Drift = float64(h.Stale+h.Dead) / float64(h.Total)
	}
	return h, err
}

func computeStyleHash() string {
	h := sha256.Sum256([]byte(styleCSS))
	return "sha256-" + base64.StdEncoding.EncodeToString(h[:])
}

// NewServer constructs a hardened dashboard Server.
func NewServer(cfg Config, repos []Repo, principals []Principal, cfgRoot string) (*Server, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 30 * time.Second
	}
	if cfg.MeasureFunc == nil {
		cfg.MeasureFunc = Measure
	}
	if cfg.AccessLog == nil {
		cfg.AccessLog = os.Stderr
	}

	s := &Server{
		config:     cfg,
		repos:      repos,
		principals: principals,
		cfgRoot:    cfgRoot,
		limiter:    newIPLimiter(10000),
		cache:      make(map[string]*cacheEntry),
		semaphore:  make(chan struct{}, 4),
		styleHash:  computeStyleHash(),
	}
	s.handler = s.buildHandler()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if s.config.BehindProxy && len(s.config.TrustedProxies) > 0 {
		if directAddr, err := netip.ParseAddr(host); err == nil {
			isTrusted := false
			for _, prefix := range s.config.TrustedProxies {
				if prefix.Contains(directAddr) {
					isTrusted = true
					break
				}
			}
			if isTrusted {
				xff := r.Header.Get("X-Forwarded-For")
				if xff != "" {
					parts := strings.Split(xff, ",")
					last := strings.TrimSpace(parts[len(parts)-1])
					if _, err := netip.ParseAddr(last); err == nil {
						return last
					}
				}
			}
		}
	}
	return host
}

func (s *Server) measureRepo(r Repo) (Health, error) {
	now := s.config.Now()

	s.cacheMu.RLock()
	if e, ok := s.cache[r.Name]; ok && now.Before(e.expiresAt) {
		val := e.health
		s.cacheMu.RUnlock()
		return val, nil
	}
	s.cacheMu.RUnlock()

	return s.sf.Do(r.Name, func() (Health, error) {
		s.cacheMu.RLock()
		if e, ok := s.cache[r.Name]; ok && s.config.Now().Before(e.expiresAt) {
			val := e.health
			s.cacheMu.RUnlock()
			return val, nil
		}
		s.cacheMu.RUnlock()

		select {
		case s.semaphore <- struct{}{}:
			defer func() { <-s.semaphore }()
		case <-time.After(5 * time.Second):
			return Health{Repo: r.Name, Team: r.Team, Error: true}, ErrBusy
		}

		if s.config.AllowedRoot != "" {
			if _, err := checkPathConfinement(s.config.AllowedRoot, r.Path); err != nil {
				return Health{Repo: r.Name, Team: r.Team, Error: true}, err
			}
		}

		h, err := s.config.MeasureFunc(r)
		if err != nil {
			h.Error = true
		}

		s.cacheMu.Lock()
		s.cache[r.Name] = &cacheEntry{
			health:    h,
			expiresAt: s.config.Now().Add(s.config.CacheTTL),
		}
		s.cacheMu.Unlock()

		return h, nil
	})
}

func (s *Server) sendError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, msg)
}

func (s *Server) setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	csp := fmt.Sprintf("default-src 'none'; style-src '%s'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'", s.styleHash)
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	if s.config.BehindProxy {
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

type statusLoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusLoggingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) logAccess(remote, method, path string, status int, dur time.Duration, principal string) {
	if s.config.AccessLog == nil {
		return
	}
	type logEntry struct {
		TS        string `json:"ts"`
		Remote    string `json:"remote"`
		Method    string `json:"method"`
		Path      string `json:"path"`
		Status    int    `json:"status"`
		DurMS     int64  `json:"dur_ms"`
		Principal string `json:"principal"`
	}
	cleanPath := path
	if idx := strings.Index(cleanPath, "?"); idx >= 0 {
		cleanPath = cleanPath[:idx]
	}
	ent := logEntry{
		TS:        s.config.Now().UTC().Format(time.RFC3339),
		Remote:    remote,
		Method:    method,
		Path:      cleanPath,
		Status:    status,
		DurMS:     dur.Milliseconds(),
		Principal: principal,
	}
	b, _ := json.Marshal(ent)
	s.config.AccessLog.Write(append(b, '\n'))
}

func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()

	// 1. Unauthenticated healthz
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		s.setSecurityHeaders(w)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Authenticated endpoints
	authenticate := func(w http.ResponseWriter, r *http.Request) (*Principal, string) {
		clientIP := s.clientIP(r)
		now := s.config.Now()

		if locked, retryAfter := s.limiter.checkLockout(clientIP, now); locked {
			s.setSecurityHeaders(w)
			w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintln(w, "too many authentication failures; lockout in effect")
			return nil, "-"
		}

		_, pass, ok := r.BasicAuth()
		if !ok || pass == "" {
			if locked, retryAfter := s.limiter.recordFailure(clientIP, now); locked {
				s.setSecurityHeaders(w)
				w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintln(w, "too many authentication failures; lockout in effect")
				return nil, "-"
			}
			s.setSecurityHeaders(w)
			w.Header().Set("WWW-Authenticate", `Basic realm="scribex"`)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "unauthorized")
			return nil, "-"
		}

		tokHash := HashToken(pass)
		p, _ := Authenticate(tokHash, s.principals)
		if p == nil {
			if locked, retryAfter := s.limiter.recordFailure(clientIP, now); locked {
				s.setSecurityHeaders(w)
				w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintln(w, "too many authentication failures; lockout in effect")
				return nil, "-"
			}
			s.setSecurityHeaders(w)
			w.Header().Set("WWW-Authenticate", `Basic realm="scribex"`)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "unauthorized")
			return nil, "-"
		}

		principalPrefix := p.Hash
		if len(principalPrefix) > 8 {
			principalPrefix = principalPrefix[:8]
		}
		return p, principalPrefix
	}

	collectHealth := func(p *Principal) []Health {
		var visible []Repo
		for _, r := range s.repos {
			if p.Admin || p.Teams[r.Team] {
				visible = append(visible, r)
			}
		}

		hs := make([]Health, 0, len(visible))
		for _, rp := range visible {
			h, err := s.measureRepo(rp)
			if err != nil {
				h.Error = true
			}
			hs = append(hs, h)
		}
		sort.Slice(hs, func(i, j int) bool { return hs[i].Drift > hs[j].Drift })
		return hs
	}

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		p, _ := authenticate(w, r)
		if p == nil {
			return
		}
		hs := collectHealth(p)
		s.setSecurityHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(hs)
	})

	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		p, _ := authenticate(w, r)
		if p == nil {
			return
		}
		if !p.Admin {
			s.sendError(w, r, http.StatusForbidden, "forbidden: admin only")
			return
		}

		settings := SafeSettings(s.cfgRoot)
		s.setSecurityHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		var b strings.Builder
		b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Scribex Config</title>`)
		b.WriteString(fmt.Sprintf("<style>%s</style></head><body>", styleCSS))
		b.WriteString("<h1>Scribex configuration</h1><p>Read-only. Change config via <code>scribex config set</code>; secrets via <code>scribex secrets set</code> or CI secrets.</p>")
		b.WriteString("<table><tr><th>Setting</th><th>Value</th><th>Status</th></tr>")
		for _, st := range settings {
			statusHtml := "<b>unset</b>"
			if st.Set {
				statusHtml = "ok"
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td></tr>", html.EscapeString(st.Name), html.EscapeString(st.Value), statusHtml))
		}
		b.WriteString("</table></body></html>")
		w.Write([]byte(b.String()))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			s.sendError(w, r, http.StatusNotFound, "not found")
			return
		}
		p, _ := authenticate(w, r)
		if p == nil {
			return
		}
		hs := collectHealth(p)
		s.setSecurityHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		var b strings.Builder
		b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Scribex</title>`)
		b.WriteString(fmt.Sprintf("<style>%s</style></head><body>", styleCSS))
		b.WriteString("<h1>Scribex doc health</h1><table><tr><th>Repo</th><th>Team</th><th>Anchors</th><th>Stale</th><th>Dead</th><th>Wiki issues</th><th>Drift</th></tr>")
		for _, h := range hs {
			if h.Error {
				b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td colspan=\"5\" class=\"error\">unavailable</td></tr>", html.EscapeString(h.Repo), html.EscapeString(h.Team)))
			} else {
				driftStr := fmt.Sprintf("%.0f%%", h.Drift*100)
				b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td></tr>",
					html.EscapeString(h.Repo), html.EscapeString(h.Team), h.Total, h.Stale, h.Dead, h.WikiIssues, html.EscapeString(driftStr)))
			}
		}
		b.WriteString("</table></body></html>")
		w.Write([]byte(b.String()))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.config.Now()
		srw := &statusLoggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		principal := "-"

		defer func() {
			if rec := recover(); rec != nil {
				s.sendError(srw, r, http.StatusInternalServerError, "internal server error")
				fmt.Fprintf(os.Stderr, "panic in dashboard handler: %v\n%s\n", rec, debug.Stack())
			}
			dur := s.config.Now().Sub(start)
			s.logAccess(s.clientIP(r), r.Method, r.URL.Path, srw.statusCode, dur, principal)
		}()

		// Method restriction: only GET and HEAD
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			srw.Header().Set("Allow", "GET, HEAD")
			s.sendError(srw, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		mux.ServeHTTP(srw, r)
	})
}

// Setting is one configuration item with the state of any secret it needs.
type Setting struct {
	Name  string
	Value string
	Set   bool
}

// SafeSettings summarizes the config for admin view with URLs stripped to host-only and no env values leaked.
func SafeSettings(root string) []Setting {
	c, _ := config.Load(root)
	out := []Setting{
		{"ai.provider", c.AI.Provider, true},
		{"ai.model", c.AI.Model, true},
		{"ai.max_calls", fmt.Sprint(c.AI.MaxCalls), true},
		{"wiki.dir", c.Wiki.Dir, true},
	}
	f, _ := mcp.Load(root)
	for n, s := range f.Servers {
		val := s.Command
		if s.URL != "" {
			if u, err := url.Parse(s.URL); err == nil {
				if val != "" {
					val += " " + u.Host
				} else {
					val = u.Host
				}
			}
		}
		out = append(out, Setting{Name: "mcp." + n, Value: val, Set: len(s.Missing()) == 0})
	}
	names := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GITHUB_TOKEN", "ATLASSIAN_URL", "ATLASSIAN_USER", "ATLASSIAN_TOKEN"}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, Setting{Name: "secret " + n, Value: "", Set: os.Getenv(n) != ""})
	}
	return out
}

// Settings maintains backward compatibility.
func Settings(root string) []Setting {
	return SafeSettings(root)
}

// Handler maintains backward compatibility.
func Handler(repos []Repo, ps []Principal, cfgRoot string) http.Handler {
	s, _ := NewServer(Config{AllowedRoot: cfgRoot}, repos, ps, cfgRoot)
	return s.Handler()
}

// IsLoopbackHost reports whether host represents a loopback interface.
func IsLoopbackHost(host string) bool {
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateServeConfig enforces mandatory allowed-root and TLS/proxy flags for non-loopback bindings.
func ValidateServeConfig(addr, allowedRoot, tlsCert, tlsKey string, behindProxy bool) error {
	if allowedRoot == "" {
		return errors.New("serve: -allowed-root is required")
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	if host == "" || !IsLoopbackHost(host) {
		hasTLS := tlsCert != "" && tlsKey != ""
		if !hasTLS && !behindProxy {
			return errors.New("binding non-loopback address requires either -tls-cert/-tls-key or -behind-proxy")
		}
	}
	return nil
}
