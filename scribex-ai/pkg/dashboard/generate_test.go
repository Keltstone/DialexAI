package dashboard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var csrfRe = regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`)

type genEnv struct {
	t     *testing.T
	srv   *httptest.Server
	repo  string
	calls [][]string
	mu    sync.Mutex
	gate  chan struct{}
}

func newGenEnv(t *testing.T, enable bool) *genEnv {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	os.MkdirAll(filepath.Join(repo, "svc"), 0o755)
	os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "svc/svc.go"), []byte("package svc\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"}, {"add", "-A"}, {"commit", "-q", "-m", "i"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	e := &genEnv{t: t, repo: repo, gate: make(chan struct{})}
	ps := []Principal{
		{Hash: HashToken("admintok"), Admin: true},
		{Hash: HashToken("usertok"), Teams: map[string]bool{"t": true}},
	}
	s, err := NewServer(Config{
		EnableGenerate: enable,
		Now:            time.Now,
		AccessLog:      &strings.Builder{},
		GenerateRun: func(r Repo, args []string) (string, error) {
			e.mu.Lock()
			e.calls = append(e.calls, args)
			e.mu.Unlock()
			<-e.gate
			return "done", nil
		},
	}, []Repo{{Name: "app", Path: repo, Team: "t"}, {Name: "app2", Path: repo, Team: "t"}}, ps, root)
	if err != nil {
		t.Fatal(err)
	}
	e.srv = httptest.NewServer(s.Handler())
	t.Cleanup(e.srv.Close)
	return e
}

func (e *genEnv) do(method, path, tok string, form url.Values, hdr map[string]string) (*http.Response, string) {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, body)
	req.SetBasicAuth("u", tok)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp, sb.String()
}

func (e *genEnv) csrf(path string) string {
	_, body := e.do("GET", path, "admintok", nil, nil)
	m := csrfRe.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no csrf on %s: %s", path, body)
	}
	return m[1]
}

func TestGenerateDisabledByDefault(t *testing.T) {
	e := newGenEnv(t, false)
	if r, _ := e.do("GET", "/ai?repo=app", "admintok", nil, nil); r.StatusCode != 404 {
		t.Fatalf("GET /ai = %d", r.StatusCode)
	}
	if r, _ := e.do("POST", "/ai", "admintok", url.Values{}, nil); r.StatusCode != 405 {
		t.Fatalf("POST /ai = %d", r.StatusCode)
	}
}

func TestGenerateAdminOnlyAndCSRF(t *testing.T) {
	e := newGenEnv(t, true)
	if r, _ := e.do("GET", "/ai?repo=app", "usertok", nil, nil); r.StatusCode != 403 {
		t.Fatalf("non-admin = %d", r.StatusCode)
	}
	if r, _ := e.do("GET", "/ai?repo=app", "", nil, nil); r.StatusCode != 401 {
		t.Fatalf("anon = %d", r.StatusCode)
	}
	f := url.Values{"repo": {"app"}, "provider": {"ollama"}, "model": {"llama3"}}
	if r, _ := e.do("POST", "/ai", "admintok", f, nil); r.StatusCode != 403 {
		t.Fatalf("no csrf = %d", r.StatusCode)
	}
	f.Set("csrf", e.csrf("/ai?repo=app"))
	if r, _ := e.do("POST", "/ai", "admintok", f, map[string]string{"Origin": "http://evil.example"}); r.StatusCode != 403 {
		t.Fatalf("cross origin = %d", r.StatusCode)
	}
	if r, _ := e.do("POST", "/doctypes", "admintok", f, nil); r.StatusCode != 403 {
		t.Fatalf("csrf token must be bound to the action: %d", r.StatusCode)
	}
}

func TestGenerateAISettingsSave(t *testing.T) {
	e := newGenEnv(t, true)
	t.Setenv("ANTHROPIC_API_KEY", "sekret-value-123456")
	_, page := e.do("GET", "/ai?repo=app", "admintok", nil, nil)
	if strings.Contains(page, "sekret-value") {
		t.Fatal("secret leaked")
	}
	f := url.Values{"repo": {"app"}, "csrf": {e.csrf("/ai?repo=app")}, "provider": {"ollama"}, "cli_command": {"claude"}, "model": {"llama3"},
		"max_calls": {"5"}, "max_tokens": {"512"}, "review": {"off"}, "review_mode": {"redteam"}, "branch_prefix": {"scribex"}}
	if r, _ := e.do("POST", "/ai", "admintok", f, nil); r.StatusCode != 303 {
		t.Fatalf("save = %d", r.StatusCode)
	}
	b, _ := os.ReadFile(filepath.Join(e.repo, ".scribex/config.yaml"))
	if !strings.Contains(string(b), "provider: ollama") || !strings.Contains(string(b), "model: llama3") {
		t.Fatalf("%s", b)
	}
	f.Set("provider", "cli")
	f.Set("cli_command", "rm -rf /")
	if _, body := e.do("POST", "/ai", "admintok", f, nil); !strings.Contains(body, "cli_command") {
		t.Fatalf("bad cli accepted: %s", body)
	}
	b2, _ := os.ReadFile(filepath.Join(e.repo, ".scribex/config.yaml"))
	if string(b2) != string(b) {
		t.Fatal("config changed despite validation error")
	}
}

func TestGenerateDoctypesToggle(t *testing.T) {
	e := newGenEnv(t, true)
	f := url.Values{"repo": {"app"}, "csrf": {e.csrf("/doctypes?repo=app")}, "seen_runbook-ops": {"1"}, "en_runbook-ops": {"on"}, "rev_runbook-ops": {"off"}}
	if r, _ := e.do("POST", "/doctypes", "admintok", f, nil); r.StatusCode != 303 {
		t.Fatalf("%d", r.StatusCode)
	}
	b, _ := os.ReadFile(filepath.Join(e.repo, ".scribex/doctypes.json"))
	if !strings.Contains(string(b), "runbook-ops") || !strings.Contains(string(b), `"enabled": true`) {
		t.Fatalf("%s", b)
	}
}

func TestGeneratePlanApplyJob(t *testing.T) {
	e := newGenEnv(t, true)
	pf := url.Values{"repo": {"app"}, "csrf": {e.csrf("/generate?repo=app")}, "type": {"tech-reference"}}
	r, _ := e.do("POST", "/generate/plan", "admintok", pf, nil)
	if r.StatusCode != 303 {
		t.Fatalf("plan = %d", r.StatusCode)
	}
	loc := r.Header.Get("Location")
	id := regexp.MustCompile(`id=(r[0-9a-f]+)`).FindStringSubmatch(loc)[1]
	if _, err := os.Stat(filepath.Join(e.repo, "docs")); err == nil {
		t.Fatal("planning wrote docs")
	}
	csrf := e.csrf(loc)
	af := url.Values{"repo": {"app"}, "csrf": {csrf}, "id": {id}}
	if r, _ := e.do("POST", "/generate/apply", "admintok", af, nil); r.StatusCode != 400 {
		t.Fatalf("cloud backend without confirm = %d", r.StatusCode)
	}
	af.Set("confirm", "yes")
	if r, _ := e.do("POST", "/generate/apply", "admintok", af, nil); r.StatusCode != 303 {
		t.Fatalf("apply = %d", r.StatusCode)
	}
	if r, _ := e.do("POST", "/generate/apply", "admintok", af, nil); r.StatusCode != 409 {
		t.Fatalf("second concurrent job = %d", r.StatusCode)
	}
	_, js := e.do("GET", "/api/runs/"+id+"?repo=app", "admintok", nil, nil)
	if !strings.Contains(js, `"job":"running"`) {
		t.Fatalf("%s", js)
	}
	close(e.gate)
	for i := 0; i < 50; i++ {
		if _, js = e.do("GET", "/api/runs/"+id+"?repo=app", "admintok", nil, nil); strings.Contains(js, "finished") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(js, "finished") {
		t.Fatalf("%s", js)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.calls) != 1 || strings.Join(e.calls[0], " ") != "-resume "+id+" -apply -confirm-send" {
		t.Fatalf("args %v", e.calls)
	}
}

func TestGenerateRejectsBadRunID(t *testing.T) {
	e := newGenEnv(t, true)
	if r, _ := e.do("GET", "/generate/run?repo=app&id=../../etc", "admintok", nil, nil); r.StatusCode != 404 {
		t.Fatalf("%d", r.StatusCode)
	}
	if r, _ := e.do("GET", "/ai?repo=nope", "admintok", nil, nil); r.StatusCode != 200 {
		t.Fatalf("unknown repo should show picker, got %d", r.StatusCode)
	}
}

func TestDashboardSecurityHardening(t *testing.T) {
	e := newGenEnv(t, true)

	// 1. XSS checks on every page: repo name, run id, doc-type id with HTML payloads
	xssPayloads := []string{
		`<script>alert("xss")</script>`,
		`"><img src=x onerror=alert(1)>`,
		`'<svg/onload=alert(1)>`,
	}
	pages := []string{"/ai", "/doctypes", "/generate", "/generate/run"}
	for _, page := range pages {
		for _, xss := range xssPayloads {
			q := url.Values{"repo": {xss}, "id": {xss}}
			_, body := e.do("GET", page+"?"+q.Encode(), "admintok", nil, nil)
			if strings.Contains(body, xss) {
				t.Fatalf("unescaped XSS payload on %s: %s", page, xss)
			}
		}
	}

	// 2. POST with wrong Content-Type (e.g. application/json or text/plain) rejected
	form := url.Values{"repo": {"app"}, "csrf": {e.csrf("/ai?repo=app")}, "provider": {"ollama"}}
	req, _ := http.NewRequest("POST", e.srv.URL+"/ai", strings.NewReader(form.Encode()))
	req.SetBasicAuth("u", "admintok")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 403 {
		t.Fatalf("POST with wrong Content-Type expected 403, got %d", resp.StatusCode)
	}

	// 3. Oversized body (>64 KB) rejected
	bigForm := url.Values{"repo": {"app"}, "csrf": {e.csrf("/ai?repo=app")}, "model": {strings.Repeat("a", 70<<10)}}
	if r, _ := e.do("POST", "/ai", "admintok", bigForm, nil); r.StatusCode != 400 {
		t.Fatalf("oversized body (>64 KB) expected 400, got %d", r.StatusCode)
	}

	// 4. Non-admin POST rejected
	if r, _ := e.do("POST", "/ai", "usertok", form, nil); r.StatusCode != 403 {
		t.Fatalf("non-admin POST expected 403, got %d", r.StatusCode)
	}

	// 5. Token for repo A rejected on repo B
	tokA := e.csrf("/ai?repo=app")
	formRepoB := url.Values{"repo": {"app2"}, "csrf": {tokA}, "provider": {"ollama"}}
	if r, _ := e.do("POST", "/ai", "admintok", formRepoB, nil); r.StatusCode != 403 {
		t.Fatalf("token for repo A on repo B expected 403, got %d", r.StatusCode)
	}
}
