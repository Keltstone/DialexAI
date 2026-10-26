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
	_ = os.WriteFile(reposF, []byte(fmt.Sprintf("repo-bad %s team1\n", outsideDir)), 0o644)

	_, err := LoadRepos(reposF, allowedRoot)
	if err == nil || !strings.Contains(err.Error(), "allowed-root") {
		t.Fatalf("expected error for path outside allowed-root, got: %v", err)
	}

	// Symlink escape
	symlinkRepo := filepath.Join(allowedRoot, "symlink-repo")
	_ = os.Symlink(outsideDir, symlinkRepo)
	_ = os.WriteFile(reposF, []byte(fmt.Sprintf("repo-sym %s team1\n", symlinkRepo)), 0o644)
	_, err = LoadRepos(reposF, allowedRoot)
	if err == nil {
		t.Fatalf("expected error for symlink resolving outside allowed-root")
	}

	// Duplicate repo names
	_ = os.WriteFile(reposF, []byte(fmt.Sprintf("repo1 %s team1\nrepo1 %s team2\n", validDir, validDir)), 0o644)
	_, err = LoadRepos(reposF, allowedRoot)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected error for duplicate repo name, got %v", err)
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
