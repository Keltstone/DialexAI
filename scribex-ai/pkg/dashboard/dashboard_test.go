package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Helper to hash token
func hash(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func setupTestDashboard(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()

	repoDir1 := filepath.Join(root, "repo1")
	repoDir2 := filepath.Join(root, "repo2")
	_ = os.MkdirAll(repoDir1, 0o755)
	_ = os.MkdirAll(repoDir2, 0o755)

	repos := []Repo{
		{Name: "repo-alpha", Path: repoDir1, Team: "team-a"},
		{Name: "repo-beta", Path: repoDir2, Team: "team-b"},
	}

	principals := []Principal{
		{Hash: hash("admin-token"), Admin: true, Teams: map[string]bool{}},
		{Hash: hash("viewer-a-token"), Admin: false, Teams: map[string]bool{"team-a": true}},
		{Hash: hash("viewer-b-token"), Admin: false, Teams: map[string]bool{"team-b": true}},
		{Hash: hash("empty-token"), Admin: false, Teams: map[string]bool{}},
	}

	srv, err := NewServer(Config{
		AllowedRoot:    root,
		CacheTTL:       30 * time.Second,
		BehindProxy:    false,
		TrustedProxies: nil,
		Now:            time.Now,
		AccessLog:      io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			return Health{
				Repo:       r.Name,
				Team:       r.Team,
				Total:      10,
				Stale:      1,
				Dead:       0,
				WikiIssues: 0,
				Drift:      0.1,
			}, nil
		},
	}, repos, principals, root)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	return srv, root
}

// 1. Headers present on all status codes
func TestSecurityHeaders(t *testing.T) {
	srv, _ := setupTestDashboard(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	checkHeaders := func(resp *http.Response) {
		headers := []string{
			"Content-Security-Policy",
			"X-Content-Type-Options",
			"X-Frame-Options",
			"Referrer-Policy",
			"Cache-Control",
			"Permissions-Policy",
			"Cross-Origin-Resource-Policy",
		}
		for _, h := range headers {
			if resp.Header.Get(h) == "" {
				t.Errorf("missing security header %q for status %d", h, resp.StatusCode)
			}
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("X-Content-Type-Options = %q, want nosniff", resp.Header.Get("X-Content-Type-Options"))
		}
		if resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("X-Frame-Options = %q, want DENY", resp.Header.Get("X-Frame-Options"))
		}
	}

	// 200 OK
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("user", "admin-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	checkHeaders(resp)

	// 401 Unauthorized
	req, _ = http.NewRequest("GET", ts.URL+"/", nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	checkHeaders(resp)

	// 403 Forbidden
	req, _ = http.NewRequest("GET", ts.URL+"/config", nil)
	req.SetBasicAuth("user", "viewer-a-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 403 {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	checkHeaders(resp)

	// 404 Not Found
	req, _ = http.NewRequest("GET", ts.URL+"/non-existent", nil)
	req.SetBasicAuth("user", "admin-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	checkHeaders(resp)

	// 405 Method Not Allowed
	req, _ = http.NewRequest("POST", ts.URL+"/", strings.NewReader("bad"))
	req.SetBasicAuth("user", "admin-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 405 {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Allow") != "GET, HEAD" {
		t.Errorf("expected Allow: GET, HEAD, got %q", resp.Header.Get("Allow"))
	}
	checkHeaders(resp)
}

// 2. Methods: only GET and HEAD permitted
func TestMethodRestriction(t *testing.T) {
	srv, _ := setupTestDashboard(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"} {
		req, _ := http.NewRequest(method, ts.URL+"/api/health", strings.NewReader("{}"))
		req.SetBasicAuth("user", "admin-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, resp.StatusCode)
		}
	}
}

// 3. Content-Type explicit formatting
func TestContentType(t *testing.T) {
	srv, _ := setupTestDashboard(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// JSON endpoint
	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.SetBasicAuth("user", "admin-token")
	resp, _ := http.DefaultClient.Do(req)
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("JSON content-type = %q, want application/json", ct)
	}

	// HTML endpoint
	req, _ = http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("user", "admin-token")
	resp, _ = http.DefaultClient.Do(req)
	ct = resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("HTML content-type = %q, want text/html", ct)
	}

	// /healthz
	req, _ = http.NewRequest("GET", ts.URL+"/healthz", nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Errorf("healthz status = %d, want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(b)) != "ok" {
		t.Errorf("healthz body = %q, want ok", string(b))
	}
}

// 4. Constant-work authentication
func TestConstantWorkAuthentication(t *testing.T) {
	principals := []Principal{
		{Hash: hash("tok1"), Admin: false},
		{Hash: hash("tok2"), Admin: false},
		{Hash: hash("tok3"), Admin: true},
	}

	// Unit test constant work: Authenticate compares against ALL principals
	p, comparisons := Authenticate(hash("tok1"), principals)
	if p == nil || p.Hash != hash("tok1") {
		t.Fatal("expected match for tok1")
	}
	if comparisons != len(principals) {
		t.Errorf("expected %d comparisons, got %d", len(principals), comparisons)
	}

	p, comparisons = Authenticate(hash("non-existent"), principals)
	if p != nil {
		t.Fatal("expected no match")
	}
	if comparisons != len(principals) {
		t.Errorf("expected %d comparisons, got %d", len(principals), comparisons)
	}

	// Token rejected when sent in query, cookie, or X-Api-Key
	srv, _ := setupTestDashboard(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Query token -> 401
	resp, _ := http.Get(ts.URL + "/api/health?token=admin-token")
	if resp.StatusCode != 401 {
		t.Errorf("token in query got %d, want 401", resp.StatusCode)
	}

	// Cookie token -> 401
	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "admin-token"})
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("token in cookie got %d, want 401", resp.StatusCode)
	}

	// X-Api-Key -> 401
	req, _ = http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("X-Api-Key", "admin-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("token in X-Api-Key got %d, want 401", resp.StatusCode)
	}
}

// 5. Lockout and failure rate limiter with injectable clock
func TestLockoutAndRateLimiter(t *testing.T) {
	fakeNow := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	var nowMu sync.Mutex
	getNow := func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return fakeNow
	}
	addTime := func(d time.Duration) {
		nowMu.Lock()
		defer nowMu.Unlock()
		fakeNow = fakeNow.Add(d)
	}

	root := t.TempDir()
	principals := []Principal{
		{Hash: hash("correct-token"), Admin: true, Teams: map[string]bool{}},
	}

	srv, err := NewServer(Config{
		AllowedRoot: root,
		CacheTTL:    30 * time.Second,
		Now:         getNow,
		AccessLog:   io.Discard,
	}, nil, principals, root)
	if err != nil {
		t.Fatal(err)
	}

	handler := srv.Handler()

	doReq := func(remoteAddr, pass string) int {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.RemoteAddr = remoteAddr
		if pass != "" {
			req.SetBasicAuth("u", pass)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	ip1 := "192.0.2.1:1234"
	ip2 := "192.0.2.2:5678"

	// 4 bad attempts -> 401
	for i := 0; i < 4; i++ {
		code := doReq(ip1, "bad-pass")
		if code != 401 {
			t.Fatalf("attempt %d: got code %d, want 401", i+1, code)
		}
		addTime(1 * time.Second)
	}

	// 5th bad attempt triggers lockout -> 429
	code := doReq(ip1, "bad-pass")
	if code != 429 {
		t.Fatalf("5th attempt: got %d, want 429", code)
	}

	// Even with correct password, ip1 receives 429 during lockout
	code = doReq(ip1, "correct-token")
	if code != 429 {
		t.Fatalf("during lockout with correct pass: got %d, want 429", code)
	}

	// Independent ip2 is NOT locked out and can authenticate with correct pass
	code = doReq(ip2, "correct-token")
	if code != 200 {
		t.Fatalf("ip2 got %d, want 200", code)
	}

	// Advance time past 15-minute lockout
	addTime(16 * time.Minute)

	// ip1 can now authenticate
	code = doReq(ip1, "correct-token")
	if code != 200 {
		t.Fatalf("after lockout expiry: got %d, want 200", code)
	}
}

// 6. Proxy trust & X-Forwarded-For handling
func TestProxyTrust(t *testing.T) {
	root := t.TempDir()
	trustedPrefix := netip.MustParsePrefix("10.0.0.0/8")

	srv, err := NewServer(Config{
		AllowedRoot:    root,
		BehindProxy:    true,
		TrustedProxies: []netip.Prefix{trustedPrefix},
		Now:            time.Now,
		AccessLog:      io.Discard,
	}, nil, []Principal{{Hash: hash("tok"), Admin: true}}, root)
	if err != nil {
		t.Fatal(err)
	}

	handler := srv.Handler()

	// Trusted proxy forwarding client 198.51.100.5
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.RemoteAddr = "10.1.2.3:8080"
	req.Header.Set("X-Forwarded-For", "198.51.100.5")
	req.SetBasicAuth("u", "bad")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify failure recorded for 198.51.100.5, not 10.1.2.3
	state := srv.limiter.get("198.51.100.5")
	if state == nil || len(state.failures) != 1 {
		t.Errorf("expected failure logged for forwarded IP 198.51.100.5")
	}

	// Untrusted peer forwarding client -> XFF must be ignored
	req = httptest.NewRequest("GET", "/api/health", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.SetBasicAuth("u", "bad")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if srv.limiter.get("1.2.3.4") != nil {
		t.Errorf("untrusted peer's XFF header was honoured!")
	}
	if srv.limiter.get("203.0.113.10") == nil {
		t.Errorf("expected direct peer 203.0.113.10 to be rate-limited")
	}
}

// 7. RBAC isolation & no leaked filesystem paths
func TestRBACIsolationAndNoLeakedPaths(t *testing.T) {
	srv, root := setupTestDashboard(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Viewer A should see repo-alpha but NOT repo-beta
	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.SetBasicAuth("u", "viewer-a-token")
	resp, _ := http.DefaultClient.Do(req)
	var hs []Health
	json.NewDecoder(resp.Body).Decode(&hs)
	if len(hs) != 1 || hs[0].Repo != "repo-alpha" {
		t.Errorf("viewer-a saw: %+v, want only repo-alpha", hs)
	}

	// Check response body does not leak root path or other repo
	req, _ = http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("u", "viewer-a-token")
	resp, _ = http.DefaultClient.Do(req)
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)
	if strings.Contains(bodyStr, root) {
		t.Errorf("filesystem path %q leaked in HTML body!", root)
	}
	if strings.Contains(bodyStr, "repo-beta") {
		t.Errorf("viewer-a saw repo-beta in HTML body!")
	}

	// Viewer with no teams sees empty list, not error
	req, _ = http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.SetBasicAuth("u", "empty-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Errorf("empty-token status = %d, want 200", resp.StatusCode)
	}
	var emptyHs []Health
	json.NewDecoder(resp.Body).Decode(&emptyHs)
	if len(emptyHs) != 0 {
		t.Errorf("expected 0 repos for empty-token, got %d", len(emptyHs))
	}

	// Viewer cannot access /config
	req, _ = http.NewRequest("GET", ts.URL+"/config", nil)
	req.SetBasicAuth("u", "viewer-a-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 403 {
		t.Errorf("expected 403 on /config for viewer, got %d", resp.StatusCode)
	}
}

// 8. Stored XSS escaping in HTML and JSON
func TestStoredXSS(t *testing.T) {
	root := t.TempDir()
	xssRepo := filepath.Join(root, "xss")
	_ = os.MkdirAll(xssRepo, 0o755)

	repos := []Repo{
		{Name: "<script>alert(1)</script>", Path: xssRepo, Team: `"><img src=x onerror=alert(1)>`},
	}
	principals := []Principal{
		{Hash: hash("admin"), Admin: true, Teams: map[string]bool{}},
	}

	srv, err := NewServer(Config{
		AllowedRoot: root,
		CacheTTL:    30 * time.Second,
		Now:         time.Now,
		AccessLog:   io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			return Health{Repo: r.Name, Team: r.Team, Total: 1}, nil
		},
	}, repos, principals, root)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// HTML View
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("u", "admin")
	resp, _ := http.DefaultClient.Do(req)
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "<script>alert(1)</script>") {
		t.Errorf("raw <script> tag found in HTML response!")
	}
	if !strings.Contains(string(body), "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("expected HTML escaped script tag in body")
	}

	// JSON View
	req, _ = http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.SetBasicAuth("u", "admin")
	resp, _ = http.DefaultClient.Do(req)
	var hs []Health
	json.NewDecoder(resp.Body).Decode(&hs)
	if len(hs) != 1 || hs[0].Repo != "<script>alert(1)</script>" {
		t.Errorf("JSON did not preserve uncorrupted string: %+v", hs)
	}
}

// 9. LoadRepos & LoadRBAC strictness checks
func TestStrictLoading(t *testing.T) {
	dir := t.TempDir()
	allowedRoot := filepath.Join(dir, "allowed")
	_ = os.MkdirAll(allowedRoot, 0o755)

	// Valid repo inside allowedRoot
	validDir := filepath.Join(allowedRoot, "my-repo")
	_ = os.MkdirAll(validDir, 0o755)

	// Path traversal outside allowedRoot
	outsideDir := filepath.Join(dir, "outside")
	_ = os.MkdirAll(outsideDir, 0o755)

	reposF := filepath.Join(dir, "repos.txt")
	// Malformed LINE syntax (fatal error with line number, not content)
	_ = os.WriteFile(reposF, []byte("bad line with too many fields here\n"), 0o644)
	_, err := LoadRepos(reposF, allowedRoot)
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected fatal error with line number for malformed line, got: %v", err)
	}

	// RBAC strictness: invalid role
	rbacF := filepath.Join(dir, "rbac.txt")
	_ = os.WriteFile(rbacF, []byte(strings.Repeat("a", 64)+" superuser team1\n"), 0o644)
	_, err = LoadRBAC(rbacF)
	if err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("expected error on invalid role, got %v", err)
	}

	// Duplicate hash
	_ = os.WriteFile(rbacF, []byte(strings.Repeat("b", 64)+" admin\n"+strings.Repeat("b", 64)+" viewer team1\n"), 0o644)
	_, err = LoadRBAC(rbacF)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected error on duplicate hash, got %v", err)
	}
}

func TestN37_ReposFileHandling(t *testing.T) {
	dir := t.TempDir()
	allowedRoot := filepath.Join(dir, "allowed")
	_ = os.MkdirAll(allowedRoot, 0o755)

	// Two good repos (one with relative path, one with absolute path)
	good1Dir := filepath.Join(allowedRoot, "good1")
	_ = os.MkdirAll(good1Dir, 0o755)
	good2Dir := filepath.Join(allowedRoot, "good2")
	_ = os.MkdirAll(good2Dir, 0o755)

	// Outside dir for symlink escape
	outsideDir := filepath.Join(dir, "outside")
	_ = os.MkdirAll(outsideDir, 0o755)
	symlinkRepo := filepath.Join(allowedRoot, "symlink-esc")
	_ = os.Symlink(outsideDir, symlinkRepo)

	reposContent := fmt.Sprintf(
		"good1 good1 team1\n"+ // relative path
			"good2 %s team1\n"+ // absolute path
			"bad-sym %s team1\n"+ // symlink escape
			"missing missing-repo team1\n", // missing
		good2Dir, symlinkRepo,
	)

	reposF := filepath.Join(dir, "repos.txt")
	_ = os.WriteFile(reposF, []byte(reposContent), 0o644)

	repos, err := LoadRepos(reposF, allowedRoot)
	if err != nil {
		t.Fatalf("LoadRepos should not fail on bad entries (non-fatal): %v", err)
	}
	if len(repos) != 4 {
		t.Fatalf("expected 4 repo entries, got %d", len(repos))
	}

	// Verify relative path resolved under allowedRoot
	if repos[0].Path != good1Dir {
		t.Errorf("relative path resolved to %s, want %s", repos[0].Path, good1Dir)
	}
	if repos[0].Unavailable {
		t.Errorf("good1 should not be unavailable")
	}
	if repos[1].Unavailable {
		t.Errorf("good2 should not be unavailable")
	}
	if !repos[2].Unavailable {
		t.Errorf("symlink escape should be marked unavailable")
	}
	if !repos[3].Unavailable {
		t.Errorf("missing repo should be marked unavailable")
	}

	// Start server with these repos and verify dashboard HTML & JSON
	srv, err := NewServer(Config{
		AllowedRoot: allowedRoot,
		CacheTTL:    30 * time.Second,
		Now:         time.Now,
		AccessLog:   io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			return Health{Repo: r.Name, Team: r.Team, Total: 10, Drift: 0.1}, nil
		},
	}, repos, []Principal{{Hash: hash("tok"), Admin: true}}, allowedRoot)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Dashboard HTML
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("", "tok")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	htmlBody, _ := io.ReadAll(resp.Body)
	htmlStr := string(htmlBody)

	// Check 2 ok rows and 2 unavailable rows
	unavailableCount := strings.Count(htmlStr, "class=\"error\">unavailable<")
	if unavailableCount != 2 {
		t.Errorf("expected 2 unavailable rows in HTML, got %d", unavailableCount)
	}

	// Verify NO filesystem path appears anywhere in HTML response
	if strings.Contains(htmlStr, dir) || strings.Contains(htmlStr, allowedRoot) || strings.Contains(htmlStr, outsideDir) {
		t.Errorf("filesystem path leaked in HTML response: %s", htmlStr)
	}

	// 2. /api/health JSON
	reqJSON, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	reqJSON.SetBasicAuth("", "tok")
	respJSON, err := ts.Client().Do(reqJSON)
	if err != nil {
		t.Fatal(err)
	}
	defer respJSON.Body.Close()
	var hs []Health
	_ = json.NewDecoder(respJSON.Body).Decode(&hs)
	if len(hs) != 4 {
		t.Fatalf("expected 4 health entries in JSON, got %d", len(hs))
	}
	errCount := 0
	for _, h := range hs {
		if h.Error {
			errCount++
		}
	}
	if errCount != 2 {
		t.Errorf("expected 2 error entries in JSON, got %d", errCount)
	}

	// 3. Duplicate name test: second is marked unavailable
	dupContent := fmt.Sprintf("dup-repo good1 team1\ndup-repo good1 team1\n")
	dupF := filepath.Join(dir, "dup.txt")
	_ = os.WriteFile(dupF, []byte(dupContent), 0o644)
	dupRepos, err := LoadRepos(dupF, allowedRoot)
	if err != nil {
		t.Fatalf("duplicate names should not abort server: %v", err)
	}
	if len(dupRepos) != 2 || !dupRepos[1].Unavailable {
		t.Errorf("expected second duplicate repo to be marked unavailable")
	}

	// 4. Two lines, only second bad line syntax: error must name line 2, not line 1!
	syntaxContent := "good1 good1 team1\nbad-line-missing-team\n"
	syntaxF := filepath.Join(dir, "syntax.txt")
	_ = os.WriteFile(syntaxF, []byte(syntaxContent), 0o644)
	_, err = LoadRepos(syntaxF, allowedRoot)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected error naming line 2, got: %v", err)
	}
}

// 10. Cache hit/miss and singleflight
func TestCacheAndSingleflight(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "myrepo")
	_ = os.MkdirAll(repoDir, 0o755)

	var measureCalls int64
	srv, err := NewServer(Config{
		AllowedRoot: root,
		CacheTTL:    30 * time.Second,
		Now:         time.Now,
		AccessLog:   io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			atomic.AddInt64(&measureCalls, 1)
			time.Sleep(50 * time.Millisecond) // simulate work
			return Health{Repo: r.Name, Team: r.Team, Total: 100}, nil
		},
	}, []Repo{{Name: "myrepo", Path: repoDir, Team: "t"}}, []Principal{{Hash: hash("tok"), Admin: true}}, root)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 50 concurrent requests simultaneously
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
			req.SetBasicAuth("u", "tok")
			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != 200 {
				t.Errorf("concurrent request failed: %v, status %d", err, resp.StatusCode)
			}
		}()
	}
	wg.Wait()

	calls := atomic.LoadInt64(&measureCalls)
	if calls != 1 {
		t.Errorf("expected exactly 1 measure call via singleflight, got %d", calls)
	}
}

// 11. Unreadable repo shows Error: true (HTML: unavailable)
func TestUnreadableRepo(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "broken")
	_ = os.MkdirAll(repoDir, 0o755)

	srv, err := NewServer(Config{
		AllowedRoot: root,
		CacheTTL:    30 * time.Second,
		Now:         time.Now,
		AccessLog:   io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			return Health{Repo: r.Name, Team: r.Team}, fmt.Errorf("permission denied reading repo")
		},
	}, []Repo{{Name: "broken", Path: repoDir, Team: "t"}}, []Principal{{Hash: hash("tok"), Admin: true}}, root)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.SetBasicAuth("u", "tok")
	resp, _ := http.DefaultClient.Do(req)
	var hs []Health
	json.NewDecoder(resp.Body).Decode(&hs)
	if len(hs) != 1 || !hs[0].Error {
		t.Errorf("expected Error: true in JSON for failed measure, got: %+v", hs)
	}

	req, _ = http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("u", "tok")
	resp, _ = http.DefaultClient.Do(req)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "unavailable") {
		t.Errorf("expected 'unavailable' in HTML row for broken repo, got: %s", string(body))
	}
}

// 12. Panic recovery middleware
func TestPanicRecovery(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "p"), 0o755)
	srv, err := NewServer(Config{
		AllowedRoot: root,
		CacheTTL:    30 * time.Second,
		Now:         time.Now,
		AccessLog:   io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			panic("unexpected internal explosion")
		},
	}, []Repo{{Name: "panic-repo", Path: filepath.Join(root, "p"), Team: "t"}}, []Principal{{Hash: hash("tok"), Admin: true}}, root)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.SetBasicAuth("u", "tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("client error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("expected 500 on panic, got %d", resp.StatusCode)
	}

	// Verify server is still serving requests afterwards
	req, _ = http.NewRequest("GET", ts.URL+"/healthz", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Errorf("server failed after panic recovery: %v, status %d", err, resp.StatusCode)
	}
}

// 13. ValidateServeConfig TLS and proxy requirements
func TestValidateServeConfig(t *testing.T) {
	// Missing allowed-root
	if err := ValidateServeConfig("127.0.0.1:8080", "", "", "", false); err == nil {
		t.Error("expected error when -allowed-root is missing")
	}

	// Loopback address without TLS or proxy is allowed
	if err := ValidateServeConfig("127.0.0.1:8080", "/tmp", "", "", false); err != nil {
		t.Errorf("unexpected error on loopback: %v", err)
	}
	if err := ValidateServeConfig("localhost:8080", "/tmp", "", "", false); err != nil {
		t.Errorf("unexpected error on localhost: %v", err)
	}

	// Non-loopback (:8080, 0.0.0.0, or public IP) without TLS or behind-proxy MUST fail
	if err := ValidateServeConfig(":8080", "/tmp", "", "", false); err == nil {
		t.Error("expected error for :8080 without TLS/proxy")
	}
	if err := ValidateServeConfig("0.0.0.0:8080", "/tmp", "", "", false); err == nil {
		t.Error("expected error for 0.0.0.0:8080 without TLS/proxy")
	}
	if err := ValidateServeConfig("192.168.1.50:8080", "/tmp", "", "", false); err == nil {
		t.Error("expected error for non-loopback without TLS/proxy")
	}

	// Non-loopback WITH behind-proxy is allowed
	if err := ValidateServeConfig("0.0.0.0:8080", "/tmp", "", "", true); err != nil {
		t.Errorf("unexpected error when -behind-proxy is true: %v", err)
	}

	// Non-loopback WITH TLS cert and key is allowed
	if err := ValidateServeConfig("0.0.0.0:8080", "/tmp", "cert.pem", "key.pem", false); err != nil {
		t.Errorf("unexpected error when TLS cert/key provided: %v", err)
	}
}

// 13. Comprehensive RBAC & IDOR sweep across all routes and roles
func TestRBACAndIDORSweep(t *testing.T) {
	srv, _ := setupTestDashboard(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	type rbacResult struct {
		role   string
		route  string
		status int
		access string
	}
	var results []rbacResult

	routes := []struct {
		desc string
		path string
	}{
		{"Root HTML", "/"},
		{"Health API", "/api/health"},
		{"Config HTML", "/config"},
		{"Healthz Probe", "/healthz"},
		{"Direct IDOR (repo-beta)", "/repo-beta"},
		{"Direct IDOR (repo-alpha)", "/repo-alpha"},
		{"Traversal DotDot", "/.."},
		{"Encoded Traversal", "/%2e%2e"},
		{"Encoded Slash Traversal", "/%2e%2e/%2e%2e/etc/passwd"},
		{"Query IDOR Param", "/api/health?repo=repo-beta"},
	}

	roles := []struct {
		name  string
		token string
	}{
		{"Viewer Team A", "viewer-a-token"},
		{"Viewer Team B", "viewer-b-token"},
		{"Admin", "admin-token"},
		{"Anonymous", ""},
	}

	for _, r := range roles {
		for _, rt := range routes {
			req, _ := http.NewRequest("GET", ts.URL+rt.path, nil)
			if r.token != "" {
				req.SetBasicAuth("", r.token)
			}
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			access := "Denied"
			if resp.StatusCode == http.StatusOK {
				// Evaluate visible repos
				bodyStr := string(body)
				sawAlpha := strings.Contains(bodyStr, "repo-alpha")
				sawBeta := strings.Contains(bodyStr, "repo-beta")

				if sawAlpha && sawBeta {
					access = "All Repos (alpha+beta)"
				} else if sawAlpha {
					access = "Team A Only (alpha)"
				} else if sawBeta {
					access = "Team B Only (beta)"
				} else {
					access = "Allowed (no repos)"
				}
			} else if resp.StatusCode == http.StatusForbidden {
				access = "403 Forbidden"
			} else if resp.StatusCode == http.StatusNotFound {
				access = "404 Not Found"
			} else if resp.StatusCode == http.StatusUnauthorized {
				access = "401 Unauthorized"
			}

			results = append(results, rbacResult{
				role:   r.name,
				route:  rt.desc + " (" + rt.path + ")",
				status: resp.StatusCode,
				access: access,
			})

			// Security assertions:
			if r.name == "Viewer Team A" {
				if strings.Contains(string(body), "repo-beta") {
					t.Errorf("IDOR VULNERABILITY: Viewer Team A saw repo-beta on route %s!", rt.path)
				}
				if rt.path == "/config" && resp.StatusCode != http.StatusForbidden {
					t.Errorf("Viewer Team A got status %d on /config, want 403", resp.StatusCode)
				}
			}
			if r.name == "Viewer Team B" {
				if strings.Contains(string(body), "repo-alpha") {
					t.Errorf("IDOR VULNERABILITY: Viewer Team B saw repo-alpha on route %s!", rt.path)
				}
				if rt.path == "/config" && resp.StatusCode != http.StatusForbidden {
					t.Errorf("Viewer Team B got status %d on /config, want 403", resp.StatusCode)
				}
			}
			if strings.Contains(rt.path, "%2e") {
				if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusUnauthorized {
					t.Errorf("Traversal attempt on %s was not 404/401, got %d", rt.path, resp.StatusCode)
				}
			}
		}
	}

	// Print RBAC & IDOR Matrix Table
	fmt.Println("\n====================== RBAC & IDOR ROUTE SWEEP MATRIX ======================")
	fmt.Printf("%-15s | %-32s | %-6s | %s\n", "Role", "Route", "Status", "Access Level / Data Scope")
	fmt.Println("----------------+----------------------------------+--------+-----------------------------------")
	for _, res := range results {
		fmt.Printf("%-15s | %-32s | %-6d | %s\n", res.role, res.route, res.status, res.access)
	}
	fmt.Println("=============================================================================")
}

// 14. Comprehensive XSS payloads in doc text, symbol names, and repo names
func TestComprehensiveXSSPayloads(t *testing.T) {
	root := t.TempDir()
	xssDir := filepath.Join(root, "xss-repo")
	_ = os.MkdirAll(xssDir, 0o755)

	xssPayloads := []string{
		`<script>alert("xss")</script>`,
		`"><svg/onload=alert(1)>`,
		`' onfocus='alert(1)'`,
		`<img src=x onerror=alert(document.cookie)>`,
		`<iframe src="javascript:alert(1)">`,
	}

	for idx, payload := range xssPayloads {
		repo := Repo{
			Name: fmt.Sprintf("repo-xss-%d", idx),
			Path: xssDir,
			Team: payload,
		}
		principals := []Principal{
			{Hash: hash("admin"), Admin: true},
		}

		srv, err := NewServer(Config{
			AllowedRoot: root,
			CacheTTL:    30 * time.Second,
			Now:         time.Now,
			AccessLog:   io.Discard,
			MeasureFunc: func(r Repo) (Health, error) {
				return Health{Repo: payload, Team: r.Team, Total: 5, Stale: 1}, nil
			},
		}, []Repo{repo}, principals, root)
		if err != nil {
			t.Fatalf("NewServer failed for payload %q: %v", payload, err)
		}

		ts := httptest.NewServer(srv.Handler())
		defer ts.Close()

		req, _ := http.NewRequest("GET", ts.URL+"/", nil)
		req.SetBasicAuth("", "admin")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(body)

		if strings.Contains(bodyStr, "<script>") || strings.Contains(bodyStr, "<svg/onload") ||
			strings.Contains(bodyStr, "<img src=x") || strings.Contains(bodyStr, "<iframe") {
			t.Fatalf("Unescaped XSS payload executed in HTML response for payload: %s", payload)
		}
	}
}

// 15. Load scale test: 100 repositories, 50 concurrent viewers, bounded RSS memory
func TestLoadScale100Repos50Viewers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	root := t.TempDir()
	var repos []Repo
	for i := 0; i < 100; i++ {
		rDir := filepath.Join(root, fmt.Sprintf("repo_%d", i))
		_ = os.MkdirAll(rDir, 0o755)
		repos = append(repos, Repo{
			Name: fmt.Sprintf("repo_%d", i),
			Path: rDir,
			Team: fmt.Sprintf("team_%d", i%10),
		})
	}

	var principals []Principal
	for i := 0; i < 50; i++ {
		tName := fmt.Sprintf("team_%d", i%10)
		principals = append(principals, Principal{
			Hash:  hash(fmt.Sprintf("viewer_tok_%d", i)),
			Admin: false,
			Teams: map[string]bool{tName: true},
		})
	}

	srv, err := NewServer(Config{
		AllowedRoot: root,
		CacheTTL:    30 * time.Second,
		Now:         time.Now,
		AccessLog:   io.Discard,
		MeasureFunc: func(r Repo) (Health, error) {
			return Health{Repo: r.Name, Team: r.Team, Total: 20, Drift: 0.05}, nil
		},
	}, repos, principals, root)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Measure baseline memory
	runtime.GC()
	var mBefore runtime.MemStats
	runtime.ReadMemStats(&mBefore)

	var wg sync.WaitGroup
	var completedReqs int64

	start := time.Now()
	// 50 concurrent goroutines, each making 20 requests = 1000 requests total
	for i := 0; i < 50; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			token := fmt.Sprintf("viewer_tok_%d", workerID)
			for j := 0; j < 20; j++ {
				path := "/"
				if j%2 == 1 {
					path = "/api/health"
				}
				req, _ := http.NewRequest("GET", ts.URL+path, nil)
				req.SetBasicAuth("", token)
				resp, err := client.Do(req)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						atomic.AddInt64(&completedReqs, 1)
					}
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	runtime.GC()
	var mAfter runtime.MemStats
	runtime.ReadMemStats(&mAfter)

	heapGrowthMB := float64(mAfter.HeapAlloc-mBefore.HeapAlloc) / (1024 * 1024)
	t.Logf("Completed %d requests across 100 repos / 50 concurrent viewers in %v", completedReqs, elapsed)
	t.Logf("Heap before: %.2f MB, Heap after: %.2f MB (Net growth: %.2f MB)",
		float64(mBefore.HeapAlloc)/(1024*1024), float64(mAfter.HeapAlloc)/(1024*1024), heapGrowthMB)

	if completedReqs != 1000 {
		t.Errorf("expected 1000 successful requests, got %d", completedReqs)
	}
	// Verify memory remains strictly bounded (< 50 MB growth for 1000 requests on 100 repos)
	if heapGrowthMB > 50.0 {
		t.Errorf("excessive memory growth during load: %.2f MB", heapGrowthMB)
	}
}
