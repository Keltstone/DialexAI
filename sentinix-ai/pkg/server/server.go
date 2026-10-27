// Package server is the HTTP API behind the dashboard, the CLI and CI: auth, projects, findings,
// approvals, webhooks and the scan job runner.
package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/ai"
	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/integrations"
	"sentinix/pkg/packs"
	"sentinix/pkg/report"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

type App struct {
	Store          *store.Store
	Hosts          map[string]githost.Host
	Sess           auth.Sessions
	OIDC           *auth.OIDC // nil: SSO not configured, only API tokens and existing sessions work
	Packs          packs.Engine
	OSV            *advisories.Client // nil: no dependency advisory lookups (default; nothing leaves the network)
	FailOn         engines.Severity
	Enforcement    string       // default for projects and unregistered repos: "block" or "report"
	OwnerEmail     string       // this identity gets the Owner role on first login
	TrustedProxies []string     // IP or CIDR strings of trusted reverse proxies
	Static         http.Handler // dashboard
	Workers         int
	RequiredEngines []string
	AIModels        []ai.Model // empty: AI add-on unavailable
	HuntMaxCalls    int        // >0: AI-enabled projects also run the llm-audit engine, capped per scan
	TestRunner     ai.Runner         // sandboxed runner for model-written tests; nil disables test generation
	Integrations   *integrations.Set // nil: no chat, webhooks or tickets
	Brand          report.Brand      // white-label name and accent for HTML reports
	TrustedKeys    []ed25519.PublicKey
	// OnScan is called after a recorded full scan or reviewed PR (integrations hook).
	OnScan func(ctx context.Context, p store.Project, sc store.Scan, newFindings []store.Finding)

	sem         chan struct{}
	webhookSem  chan struct{}
	InstanceID  string
	SyncForTest bool
}

type principal struct {
	Email, Role, Project string // Project is set for API tokens, which have no Role
	Groups               []string
}

type ctxKey struct{}

func (a *App) Handler() http.Handler {
	workers := a.Workers
	if workers < 1 && a.Workers != -1 {
		workers = 2
	}
	semWorkers := workers
	if semWorkers < 1 {
		semWorkers = 1
	}
	scheduler.OnBaseCacheHit = RecordBaseCacheHit
	a.sem = make(chan struct{}, semWorkers)
	a.webhookSem = make(chan struct{}, 32)
	if a.Store != nil && a.Workers != -1 {
		a.Store.ReclaimStaleJobs()
		go func() {
			for range time.Tick(30 * time.Second) {
				a.Store.ReclaimStaleJobs()
			}
		}()
		for i := 0; i < workers; i++ {
			go func() {
				for {
					jobID, hostName, payload, err := a.Store.DequeueJob()
					if err != nil {
						time.Sleep(50 * time.Millisecond)
						continue
					}
					hbDone := make(chan struct{})
					go func() { // heartbeat: a dead replica stops renewing and its job gets reclaimed
						t := time.NewTicker(30 * time.Second)
						defer t.Stop()
						for {
							select {
							case <-hbDone:
								return
							case <-t.C:
								a.Store.ExtendJobLease(jobID)
							}
						}
					}()
					if h, ok := a.Hosts[hostName]; ok {
						var ev githost.Event
						if json.Unmarshal([]byte(payload), &ev) == nil {
							a.review(context.Background(), h, &ev)
						}
					}
					close(hbDone)
					a.Store.FinishJob(jobID)
				}
			}()
		}
	}
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })
	m.HandleFunc("POST /webhook/{name}", a.webhook)
	if a.OIDC != nil {
		m.HandleFunc("GET /auth/login", a.OIDC.Login)
		m.HandleFunc("GET /auth/callback", a.OIDC.Callback)
	}
	m.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") != "sentinix" { // blocks cross-site logout CSRF
			http.Error(w, "missing X-Requested-With", http.StatusForbidden)
			return
		}
		a.Sess.Clear(w)
	})
	m.HandleFunc("GET /readyz", a.readyz)
	m.HandleFunc("GET /metrics", a.metrics)
	m.HandleFunc("GET /.well-known/security.txt", a.securityTxt)
	m.HandleFunc("GET /security.txt", a.securityTxt)

	api := func(perm string, h http.HandlerFunc) http.HandlerFunc { return a.guard(perm, h) }
	m.HandleFunc("GET /api/me", api("read", a.me))
	m.HandleFunc("GET /api/posture", api("read", a.posture))
	m.HandleFunc("GET /api/projects", api("read", a.projects))
	m.HandleFunc("POST /api/projects", api("manage", a.createProject))
	m.HandleFunc("PATCH /api/projects/{id}", api("manage", a.updateProject))
	m.HandleFunc("GET /api/projects/{id}/policy", api("read", a.getProjectPolicy))
	m.HandleFunc("PATCH /api/projects/{id}/policy", api("manage", a.updateProjectPolicy))
	m.HandleFunc("GET /api/projects/{id}/rule-policies", api("read", a.getProjectRulePolicies))
	m.HandleFunc("PUT /api/projects/{id}/rule-policies", api("manage", a.setProjectRulePolicy))
	m.HandleFunc("DELETE /api/projects/{id}/rule-policies/{rule...}", api("manage", a.deleteProjectRulePolicy))
	m.HandleFunc("POST /api/projects/{id}/scan", api("scan", a.scanNow))
	m.HandleFunc("GET /api/projects/{id}/scans", api("read", a.scans))
	m.HandleFunc("GET /api/projects/{id}/hunt/runs", api("read", a.listHuntRuns))
	m.HandleFunc("GET /api/projects/{id}/hunt/runs/{runID}", api("read", a.getHuntRun))
	m.HandleFunc("POST /api/projects/{id}/hunt/run", api("scan", a.triggerHuntRun))
	m.HandleFunc("GET /api/projects/{id}/surface", api("read", a.surface))
	m.HandleFunc("POST /api/advisories/rescan", api("scan", a.rescanAdvisories))
	m.HandleFunc("GET /api/projects/{id}/sbom", api("read", a.sbom))
	m.HandleFunc("POST /api/projects/{id}/tokens", api("manage", a.createToken))
	m.HandleFunc("POST /api/projects/{id}/tokens/revoke", api("manage", a.revokeToken))
	m.HandleFunc("POST /api/projects/{id}/tokens/{token}/revoke", api("manage", a.revokeToken))
	m.HandleFunc("DELETE /api/projects/{id}/tokens/{token}", api("manage", a.deleteToken))
	m.HandleFunc("DELETE /api/projects/{id}/tokens", api("manage", a.deleteToken))
	m.HandleFunc("POST /api/projects/{id}/ingest", a.ingest) // API token only
	m.HandleFunc("GET /api/findings", api("read", a.findings))
	m.HandleFunc("GET /api/findings/{id}", api("read", a.finding))
	m.HandleFunc("POST /api/findings/{id}/triage", api("triage", a.triage))
	m.HandleFunc("POST /api/findings/{id}/ticket", api("triage", a.ticket))
	m.HandleFunc("POST /api/findings/{id}/ai/triage", api("triage", a.aiTriage))
	m.HandleFunc("POST /api/findings/{id}/ai/patch", api("triage", a.aiPatch))
	m.HandleFunc("POST /api/findings/{id}/ai/test", api("owner", a.aiTest))
	m.HandleFunc("GET /api/models", api("read", a.models))
	m.HandleFunc("GET /api/approvals", api("read", a.approvals))
	m.HandleFunc("POST /api/approvals", api("triage", a.propose))
	m.HandleFunc("POST /api/approvals/{id}/decide", api("approve", a.decide))
	m.HandleFunc("GET /api/audit", api("audit", a.audit))
	m.HandleFunc("GET /api/users", api("manage", a.users))
	m.HandleFunc("PUT /api/users/role", api("manage", a.setRole))
	m.HandleFunc("POST /api/users/revoke", api("manage", a.revokeUser))
	m.HandleFunc("GET /api/packs", api("read", a.packList))
	m.HandleFunc("GET /api/projects/{id}/report/{fmt}", api("read", a.report))
	if a.Static != nil {
		m.Handle("/", a.Static)
	}
	return secureHeaders(newRateLimiter(a).limit(m))
}

// secureHeaders sets baseline hardening on every response; handlers may override (the SPA sets its own CSP).
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu        sync.Mutex
	clients   map[string]*clientRate
	app       *App
	lastPrune time.Time
}

type clientRate struct {
	count     int
	resetTime time.Time
}

func newRateLimiter(a *App) *rateLimiter {
	return &rateLimiter{
		clients:   make(map[string]*clientRate),
		app:       a,
		lastPrune: time.Now(),
	}
}

func (rl *rateLimiter) prune(now time.Time) {
	for k, v := range rl.clients {
		if now.After(v.resetTime) {
			delete(rl.clients, k)
		}
	}
	rl.lastPrune = now
}

func (rl *rateLimiter) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/healthz" || strings.HasPrefix(path, "/webhook/") {
			next.ServeHTTP(w, r)
			return
		}
		ip := r.RemoteAddr
		if rl.app != nil {
			ip = rl.app.clientIP(r)
		} else if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}
		rl.mu.Lock()
		now := time.Now()
		if len(rl.clients) > 1000 || now.Sub(rl.lastPrune) > time.Minute {
			rl.prune(now)
		}
		c, ok := rl.clients[ip]
		if !ok || now.After(c.resetTime) {
			rl.clients[ip] = &clientRate{count: 1, resetTime: now.Add(time.Minute)}
		} else {
			c.count++
			if c.count > 300 {
				rl.mu.Unlock()
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// guard authenticates (session cookie or Bearer token), checks the permission, and stores the principal.
// Mutating cookie requests must carry X-Requested-With, which a cross-site form cannot set (CSRF).
func (a *App) guard(perm string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.who(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if p.Project != "" { // tokens only reach token routes, not guarded ones, except read of their project
			if perm != "read" || (r.PathValue("id") != p.Project && !strings.HasPrefix(r.URL.Path, "/api/findings")) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		} else if !auth.Can(p.Role, perm) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != "GET" && p.Project == "" && r.Header.Get("X-Requested-With") != "sentinix" {
			http.Error(w, "missing X-Requested-With", http.StatusForbidden)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	}
}

func (a *App) who(r *http.Request) (principal, bool) {
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		proj, ok := a.Store.TokenProject(tok)
		return principal{Project: proj, Email: "token:" + proj}, ok
	}
	email, iat, ok := a.Sess.Issued(r)
	if !ok {
		return principal{}, false
	}
	u, err := a.Store.UserByEmail(email)
	if err != nil {
		return principal{}, false
	}
	if iat < u.RevokedBefore { // revoked (also rejects pre-revocation cookies with no iat once a cutoff exists)
		return principal{}, false
	}
	return principal{Email: email, Role: u.Role}, true
}

func who(r *http.Request) principal { return r.Context().Value(ctxKey{}).(principal) }

func (a *App) canReadProject(r *http.Request, projectID string) bool {
	p := who(r)
	if p.Project != "" {
		return p.Project == projectID
	}
	if p.Role != auth.Developer {
		return true
	}
	if a.Store == nil {
		return true
	}
	has, err := a.Store.HasProjectAccess(p.Email, p.Groups, projectID)
	if err != nil {
		return false
	}
	return has
}

// Login is the OIDC OnLogin hook: new users start as developers; the configured owner email becomes owner.
func (a *App) Login(email, name string) error {
	role := auth.Developer
	if a.OwnerEmail != "" && strings.EqualFold(email, a.OwnerEmail) {
		role = auth.Owner
	}
	_, err := a.Store.UpsertUser(email, name, role)
	return err
}

func (a *App) clientIP(r *http.Request) string {
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}
	if len(a.TrustedProxies) == 0 {
		return remoteIP
	}
	trusted := false
	for _, p := range a.TrustedProxies {
		if remoteIP == p {
			trusted = true
			break
		}
		if _, ipnet, err := net.ParseCIDR(p); err == nil {
			if ip := net.ParseIP(remoteIP); ip != nil && ipnet.Contains(ip) {
				trusted = true
				break
			}
		}
	}
	if trusted {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	return remoteIP
}

func (a *App) audit_(r *http.Request, action, target string, beforeAfter ...string) {
	before, after := "", ""
	if len(beforeAfter) > 0 {
		before = beforeAfter[0]
	}
	if len(beforeAfter) > 1 {
		after = beforeAfter[1]
	}
	src := a.clientIP(r)
	if err := a.Store.AddAudit(store.Audit{Actor: who(r).Email, Action: action, Target: target, Source: src, Before: before, After: after}); err != nil {
		log.Printf("audit: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return false
	}
	return true
}

func fail(w http.ResponseWriter, err error, code int) {
	http.Error(w, err.Error(), code)
}

func (a *App) readyz(w http.ResponseWriter, r *http.Request) {
	if a.Store != nil && a.Store.DB() != nil {
		if err := a.Store.DB().PingContext(r.Context()); err != nil {
			http.Error(w, "not ready: db ping failed", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready\n"))
}

func (a *App) securityTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	contact := "mailto:security@sentinix.internal"
	if a.OwnerEmail != "" {
		contact = "mailto:" + a.OwnerEmail
	}
	policyURL := "/.well-known/security.txt"
	content := fmt.Sprintf("Contact: %s\nExpires: %s\nPreferred-Languages: en\nPolicy: %s\nCanonical: %s\n",
		contact,
		time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339),
		policyURL,
		policyURL,
	)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content))
}

var (
	baseCacheHits      atomic.Int64
	reviewLatencies    []float64
	latenciesMu        sync.Mutex
	failuresTotal      atomic.Int64
	redactionFallbacks atomic.Int64
)

func RecordBaseCacheHit() {
	baseCacheHits.Add(1)
}

func RecordReviewLatency(d time.Duration) {
	latenciesMu.Lock()
	defer latenciesMu.Unlock()
	reviewLatencies = append(reviewLatencies, d.Seconds())
}

func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	latenciesMu.Lock()
	var p50, p95 float64
	n := len(reviewLatencies)
	if n > 0 {
		sorted := make([]float64, n)
		copy(sorted, reviewLatencies)
		sort.Float64s(sorted)
		p50 = sorted[int(float64(n)*0.5)]
		p95 = sorted[int(float64(n)*0.95)]
	}
	latenciesMu.Unlock()

	var qDepth int
	if a.Store != nil && a.Store.DB() != nil {
		row := a.Store.DB().QueryRowContext(r.Context(), "select count(*) from durable_queue where status='pending' or status='running'")
		_ = row.Scan(&qDepth)
	}

	ok := 0
	if a.Store != nil && a.Store.AuditChainOK() {
		ok = 1
	}
	fmt.Fprintf(w, "# HELP sentinix_audit_chain_ok 1 if the last scheduled audit-chain verification passed.\n# TYPE sentinix_audit_chain_ok gauge\nsentinix_audit_chain_ok %d\n", ok)
	fmt.Fprintf(w, "# HELP sentinix_queue_depth Current depth of the scan job queue.\n")
	fmt.Fprintf(w, "# TYPE sentinix_queue_depth gauge\n")
	fmt.Fprintf(w, "sentinix_queue_depth %d\n", qDepth)

	fmt.Fprintf(w, "# HELP sentinix_review_latency_seconds_p50 p50 review latency in seconds.\n")
	fmt.Fprintf(w, "# TYPE sentinix_review_latency_seconds_p50 gauge\n")
	fmt.Fprintf(w, "sentinix_review_latency_seconds_p50 %f\n", p50)

	fmt.Fprintf(w, "# HELP sentinix_review_latency_seconds_p95 p95 review latency in seconds.\n")
	fmt.Fprintf(w, "# TYPE sentinix_review_latency_seconds_p95 gauge\n")
	fmt.Fprintf(w, "sentinix_review_latency_seconds_p95 %f\n", p95)

	fmt.Fprintf(w, "# HELP sentinix_base_cache_hits_total Number of base branch scan cache hits.\n")
	fmt.Fprintf(w, "# TYPE sentinix_base_cache_hits_total counter\n")
	fmt.Fprintf(w, "sentinix_base_cache_hits_total %d\n", baseCacheHits.Load())

	fmt.Fprintf(w, "# HELP sentinix_failures_total Failures by cause.\n")
	fmt.Fprintf(w, "# TYPE sentinix_failures_total counter\n")
	fmt.Fprintf(w, "sentinix_failures_total %d\n", failuresTotal.Load())

	fmt.Fprintf(w, "# HELP sentinix_redaction_fallback_total Redaction fallback count.\n")
	fmt.Fprintf(w, "# TYPE sentinix_redaction_fallback_total counter\n")
	fmt.Fprintf(w, "sentinix_redaction_fallback_total %d\n", redactionFallbacks.Load())

	var revQueueDepth int
	var oldestSec float64
	var slaBreaches int
	nowUnix := time.Now().Unix()
	if a.Store != nil && a.Store.DB() != nil {
		rows, err := a.Store.DB().QueryContext(r.Context(), "select ts from approvals where status='pending' order by ts asc")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var ts int64
				if rows.Scan(&ts) == nil {
					revQueueDepth++
					age := float64(nowUnix - ts)
					if age > oldestSec {
						oldestSec = age
					}
					if age > 86400 { // 24h SLA
						slaBreaches++
					}
				}
			}
		}
	}

	fmt.Fprintf(w, "# HELP sentinix_reviewer_queue_depth Depth of pending approvals in the reviewer queue.\n")
	fmt.Fprintf(w, "# TYPE sentinix_reviewer_queue_depth gauge\n")
	fmt.Fprintf(w, "sentinix_reviewer_queue_depth %d\n", revQueueDepth)

	fmt.Fprintf(w, "# HELP sentinix_reviewer_queue_oldest_seconds Age of oldest pending approval in seconds.\n")
	fmt.Fprintf(w, "# TYPE sentinix_reviewer_queue_oldest_seconds gauge\n")
	fmt.Fprintf(w, "sentinix_reviewer_queue_oldest_seconds %f\n", oldestSec)

	fmt.Fprintf(w, "# HELP sentinix_reviewer_queue_sla_breaches_total Number of pending approvals exceeding SLA.\n")
	fmt.Fprintf(w, "# TYPE sentinix_reviewer_queue_sla_breaches_total counter\n")
	fmt.Fprintf(w, "sentinix_reviewer_queue_sla_breaches_total %d\n", slaBreaches)
}

func (a *App) ProposeInlineSuppression(projectID string, s engines.InlineSuppression) (string, error) {
	if a.Store == nil {
		return "", errors.New("no store configured")
	}
	s.Project = projectID
	payload := map[string]any{
		"project":     s.Project,
		"file":        s.File,
		"line":        s.Line,
		"rule":        s.Rule,
		"until":       s.Until,
		"owner":       s.Owner,
		"reason":      s.Reason,
		"fingerprint": s.Fingerprint,
	}

	// Check if an approval already exists for this exact suppression
	if apps, err := a.Store.Approvals("pending"); err == nil {
		for _, ap := range apps {
			if ap.Kind == "sentinix:ignore" || ap.Kind == "ignore" {
				var p struct {
					Project string `json:"project"`
					File    string `json:"file"`
					Line    int    `json:"line"`
					Rule    string `json:"rule"`
				}
				if json.Unmarshal(ap.Payload, &p) == nil {
					if p.Project == s.Project && p.File == s.File && p.Line == s.Line && (p.Rule == s.Rule || s.Rule == "*") {
						return ap.ID, nil
					}
				}
			}
		}
	}

	return a.Store.Propose("sentinix:ignore", payload, s.Owner)
}

func randID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Tick runs scheduled full scans for projects whose cron fires in the current minute. Call once a minute.
func (a *App) Tick(ctx context.Context, now time.Time) {
	if a.InstanceID == "" {
		a.InstanceID = randID()
	}
	if a.Store != nil {
		leader, err := a.Store.TryAcquireLeader(ctx, "cron", a.InstanceID, 30*time.Second)
		if err != nil || !leader {
			return // only the leader runs the cron
		}
	}
	ps, err := a.Store.Projects()
	if err != nil {
		log.Printf("tick: %v", err)
		return
	}
	for _, p := range ps {
		if p.Schedule == "" {
			continue
		}
		due, err := dueFn(p.Schedule, p.TZ, now)
		if err != nil {
			log.Printf("project %s: schedule: %v", p.Name, err)
			continue
		}
		if due {
			if a.SyncForTest {
				a.queueFull(ctx, p, "")
			} else {
				go a.queueFull(ctx, p, "")
			}
		}
	}
}
