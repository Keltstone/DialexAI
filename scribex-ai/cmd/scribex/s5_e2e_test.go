package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"scribex/pkg/gitx"
)

var binaryPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "scribex-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "scribex")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "scribex/cmd/scribex")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\noutput: %s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

type stubLogger struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type recordedRequest struct {
	Method string
	URL    string
	Header http.Header
	Body   string
}

func (s *stubLogger) log(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := io.ReadAll(r.Body)
	s.requests = append(s.requests, recordedRequest{
		Method: r.Method,
		URL:    r.URL.String(),
		Header: r.Header.Clone(),
		Body:   string(b),
	})
}

func (s *stubLogger) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func setupTestRepo(t *testing.T, stubURL string) string {
	t.Helper()
	dir := t.TempDir()

	_, _ = gitx.Run(dir, "init")
	_, _ = gitx.Run(dir, "config", "user.name", "Test")
	_, _ = gitx.Run(dir, "config", "user.email", "test@example.com")

	code := "package main\n\nfunc Calculate(x int) int {\n\treturn x * 2\n}\n"
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(code), 0600)

	doc := "<!-- @scribex:anchor(main.go#Calculate) -->\nCalculates double the value.\n<!-- @scribex:end -->\n"
	_ = os.WriteFile(filepath.Join(dir, "doc.md"), []byte(doc), 0600)

	scribexDir := filepath.Join(dir, ".scribex")
	_ = os.MkdirAll(scribexDir, 0700)
	cfg := fmt.Sprintf("ai:\n  provider: anthropic\n  model: claude-3-haiku-20240307\n  api_key_env: ANTHROPIC_API_KEY\n  base_url: %s\n", stubURL)
	_ = os.WriteFile(filepath.Join(scribexDir, "config.yaml"), []byte(cfg), 0600)

	_, _ = gitx.Run(dir, "add", "-A")
	_, _ = gitx.Run(dir, "commit", "-m", "initial")

	// Run initial sync to populate anchor sig
	initCmd := exec.Command(binaryPath, "sync")
	initCmd.Dir = dir
	_ = initCmd.Run()

	_, _ = gitx.Run(dir, "add", "-A")
	_, _ = gitx.Run(dir, "commit", "-m", "synced baseline")

	// Now modify signature so sync -ai triggers
	codeChanged := "package main\n\nfunc Calculate(x int, factor int) int {\n\treturn x * factor\n}\n"
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(codeChanged), 0600)
	_, _ = gitx.Run(dir, "add", "main.go")
	_, _ = gitx.Run(dir, "commit", "-m", "update code")

	return dir
}

// 1. S5 Repro: Committed .scribex/config.yaml with untrusted base_url.
// sync -ai must fail with "not trusted", stub receives ZERO requests, and secret is never printed.
func TestS5ReproUntrustedEndpoint(t *testing.T) {
	key := "sk-" + strings.Repeat("a", 24)
	stubLog := &stubLogger{}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stubLog.log(r)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"Updated prose."}]}`))
	}))
	defer stub.Close()

	repo := setupTestRepo(t, stub.URL)

	cmd := exec.Command(binaryPath, "sync", "-ai", "-base", "HEAD~1")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_API_KEY="+key,
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"SCRIBEX_AI_ALLOWED_HOSTS=", // NOT allowlisted!
	)

	out, err := cmd.CombinedOutput()
	outStr := string(out)

	if err == nil {
		t.Fatalf("expected sync -ai to fail with untrusted host, but succeeded: %s", outStr)
	}
	if !strings.Contains(outStr, "not trusted") {
		t.Errorf("expected 'not trusted' in output, got: %s", outStr)
	}
	if stubLog.count() != 0 {
		t.Errorf("expected 0 requests to stub, got %d", stubLog.count())
	}
	if strings.Contains(outStr, key) {
		t.Errorf("secret key leaked in output: %s", outStr)
	}
}

// 2. Allowlisted host: With SCRIBEX_AI_ALLOWED_HOSTS set, stub receives exactly 1 request with the key.
func TestS5AllowedEndpoint(t *testing.T) {
	key := "sk-" + strings.Repeat("b", 24)
	stubLog := &stubLogger{}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stubLog.log(r)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"Calculates value multiplied by factor."}]}`))
	}))
	defer stub.Close()

	u, _ := url.Parse(stub.URL)
	repo := setupTestRepo(t, stub.URL)

	cmd := exec.Command(binaryPath, "sync", "-ai", "-base", "HEAD~1")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_API_KEY="+key,
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"SCRIBEX_AI_ALLOWED_HOSTS="+u.Host,
	)

	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err != nil {
		t.Fatalf("expected sync -ai to succeed with allowlisted host, failed: %v\noutput: %s", err, outStr)
	}

	if stubLog.count() != 1 {
		t.Fatalf("expected 1 request to stub, got %d", stubLog.count())
	}
	receivedKey := stubLog.requests[0].Header.Get("x-api-key")
	if receivedKey != key {
		t.Errorf("expected x-api-key=%s, got %s", key, receivedKey)
	}
}

// 3. Stub replies 302 to second stub -> second stub log empty; sync -ai fails.
func TestS5RedirectRefused(t *testing.T) {
	key := "sk-" + strings.Repeat("c", 24)
	var stub2Requests int32
	stub2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&stub2Requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer stub2.Close()

	stub1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, stub2.URL+"/steal", http.StatusFound)
	}))
	defer stub1.Close()

	u1, _ := url.Parse(stub1.URL)
	u2, _ := url.Parse(stub2.URL)
	repo := setupTestRepo(t, stub1.URL)

	cmd := exec.Command(binaryPath, "sync", "-ai", "-base", "HEAD~1")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_API_KEY="+key,
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"SCRIBEX_AI_ALLOWED_HOSTS="+u1.Host+","+u2.Host,
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected redirect to be refused, but sync -ai succeeded: %s", out)
	}

	if atomic.LoadInt32(&stub2Requests) != 0 {
		t.Errorf("stub2 received %d requests, expected 0", stub2Requests)
	}
}

// 4. Pull Request CI events refuse before any request; -force does not bypass.
func TestS5PullRequestEventRefusal(t *testing.T) {
	key := "sk-" + strings.Repeat("d", 24)
	stubLog := &stubLogger{}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stubLog.log(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer stub.Close()

	u, _ := url.Parse(stub.URL)
	repo := setupTestRepo(t, stub.URL)

	ciVars := []struct {
		k, v string
	}{
		{"GITHUB_EVENT_NAME", "pull_request"},
		{"GITHUB_EVENT_NAME", "pull_request_target"},
		{"CI_PIPELINE_SOURCE", "merge_request_event"},
		{"BITBUCKET_PR_ID", "123"},
		{"BUILD_REASON", "PullRequest"},
		{"SYSTEM_PULLREQUEST_PULLREQUESTID", "456"},
		{"CHANGE_ID", "PR-7"},
		{"TRAVIS_PULL_REQUEST", "89"},
	}

	for _, ciVar := range ciVars {
		t.Run(ciVar.k+"="+ciVar.v, func(t *testing.T) {
			// Normal run
			cmd := exec.Command(binaryPath, "sync", "-ai", "-base", "HEAD~1")
			cmd.Dir = repo
			cmd.Env = append(os.Environ(),
				"ANTHROPIC_API_KEY="+key,
				"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
				"SCRIBEX_AI_ALLOWED_HOSTS="+u.Host,
				ciVar.k+"="+ciVar.v,
			)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected refusal on PR event %s=%s, succeeded: %s", ciVar.k, ciVar.v, out)
			}
			if !strings.Contains(string(out), "refuses to run on pull-request events") {
				t.Errorf("unexpected error message: %s", out)
			}

			// -force does NOT bypass
			cmdForce := exec.Command(binaryPath, "sync", "-ai", "-force", "-base", "HEAD~1")
			cmdForce.Dir = repo
			cmdForce.Env = append(os.Environ(),
				"ANTHROPIC_API_KEY="+key,
				"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
				"SCRIBEX_AI_ALLOWED_HOSTS="+u.Host,
				ciVar.k+"="+ciVar.v,
			)
			outForce, errForce := cmdForce.CombinedOutput()
			if errForce == nil {
				t.Fatalf("expected refusal with -force on PR event %s=%s, succeeded: %s", ciVar.k, ciVar.v, outForce)
			}
			if !strings.Contains(string(outForce), "refuses to run on pull-request events") {
				t.Errorf("unexpected error message with -force: %s", outForce)
			}
		})
	}

	if stubLog.count() != 0 {
		t.Errorf("stub received %d requests on PR events, expected 0", stubLog.count())
	}
}

// 5. Confluence SSRF protection: 169.254.169.254 and unallowed 127.0.0.1 refused.
func TestS5ConfluenceSSRFRefused(t *testing.T) {
	repo := t.TempDir()

	// 1. Link-local metadata
	cmd := exec.Command(binaryPath, "confluence", "pull", "12345")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"ATLASSIAN_URL=http://169.254.169.254/",
		"ATLASSIAN_USER=user",
		"ATLASSIAN_TOKEN=token",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected 169.254.169.254 to be refused, succeeded: %s", out)
	}

	// 2. Localhost without SCRIBEX_ALLOW_INSECURE_LOCAL=1
	cmd2 := exec.Command(binaryPath, "confluence", "pull", "12345")
	cmd2.Dir = repo
	cmd2.Env = append(os.Environ(),
		"ATLASSIAN_URL=http://127.0.0.1:8080/",
		"ATLASSIAN_USER=user",
		"ATLASSIAN_TOKEN=token",
		"SCRIBEX_ALLOW_INSECURE_LOCAL=", // unset
	)
	out2, err2 := cmd2.CombinedOutput()
	if err2 == nil {
		t.Fatalf("expected http://127.0.0.1:8080 to be refused without flag, succeeded: %s", out2)
	}
}

// 6. Config test honours trust (untrusted host -> error, no request).
func TestS5ConfigTestHonoursTrust(t *testing.T) {
	var stubRequests int32
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&stubRequests, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`))
	}))
	defer stub.Close()

	repo := setupTestRepo(t, stub.URL)

	cmd := exec.Command(binaryPath, "config", "test")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_API_KEY=sk-"+strings.Repeat("e", 24),
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"SCRIBEX_AI_ALLOWED_HOSTS=", // NOT trusted!
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected config test to fail on untrusted host, succeeded: %s", out)
	}
	if !strings.Contains(string(out), "not trusted") {
		t.Errorf("expected 'not trusted' in output, got: %s", out)
	}
	if atomic.LoadInt32(&stubRequests) != 0 {
		t.Errorf("stub received %d requests, expected 0", stubRequests)
	}
}

// 7. Key hygiene: key never appears in config show, or error output of 401 echoing it.
func TestS5KeyHygiene(t *testing.T) {
	key := "sk-" + strings.Repeat("f", 24)

	// Stub echoes the key in 401 response
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"bad key %s"}`, key)))
	}))
	defer stub.Close()

	u, _ := url.Parse(stub.URL)
	repo := setupTestRepo(t, stub.URL)

	// 1. config show
	cmdShow := exec.Command(binaryPath, "config", "show")
	cmdShow.Dir = repo
	cmdShow.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+key)
	outShow, err := cmdShow.CombinedOutput()
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}
	if bytes.Contains(outShow, []byte(key)) {
		t.Errorf("key leaked in config show output: %s", outShow)
	}

	// 2. 401 response echoing the key during sync -ai
	cmdSync := exec.Command(binaryPath, "sync", "-ai", "-base", "HEAD~1")
	cmdSync.Dir = repo
	cmdSync.Env = append(os.Environ(),
		"ANTHROPIC_API_KEY="+key,
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"SCRIBEX_AI_ALLOWED_HOSTS="+u.Host,
	)
	outSync, _ := cmdSync.CombinedOutput()
	outSyncStr := string(outSync)
	if strings.Contains(outSyncStr, key) {
		t.Errorf("key leaked in 401 error output: %s", outSyncStr)
	}
	if !strings.Contains(outSyncStr, "[redacted]") {
		t.Errorf("expected [redacted] in error output, got: %s", outSyncStr)
	}
}
