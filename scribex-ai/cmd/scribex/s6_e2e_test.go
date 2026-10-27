package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"scribex/pkg/gitx"
)

type s6Request struct {
	Method string
	URL    string
	Header http.Header
	Body   string
}

type s6Stub struct {
	mu       sync.Mutex
	requests []s6Request
	handler  func(w http.ResponseWriter, r *http.Request, reqCount int)
}

func (s *s6Stub) log(r *http.Request) s6Request {
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(b))
	req := s6Request{
		Method: r.Method,
		URL:    r.URL.String(),
		Header: r.Header.Clone(),
		Body:   string(b),
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	return req
}

func (s *s6Stub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *s6Stub) getRequests() []s6Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]s6Request, len(s.requests))
	copy(copied, s.requests)
	return copied
}

func newS6Stub(h func(w http.ResponseWriter, r *http.Request, reqCount int)) (*s6Stub, *httptest.Server) {
	stub := &s6Stub{handler: h}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.log(r)
		count := stub.count()
		if stub.handler != nil {
			stub.handler(w, r, count)
		}
	}))
	return stub, srv
}

func setupS6Repo(t *testing.T, parent string) string {
	t.Helper()
	repoDir := filepath.Join(parent, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	for _, a := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
	} {
		if _, err := gitx.Run(repoDir, a...); err != nil {
			t.Fatalf("git %v: %v", a, err)
		}
	}
	// Initial file
	code := "package main\n\nfunc Foo() {}\n"
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if _, err := gitx.Run(repoDir, "add", "main.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := gitx.Run(repoDir, "commit", "-qm", "initial commit"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return repoDir
}

// 1. S6 repro: manifest page ../../../pwned -> confluence pull exits non-zero,
// stub saw 0 requests, no file named pwned* anywhere under the temp parent;
// message names the manifest entry.
func TestS6_ManifestPageTraversal_PullBlocked(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)

	stub, srv := newS6Stub(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"pwned","body":{"storage":{"value":"pwned content"}}}`))
	})
	defer srv.Close()

	scribexDir := filepath.Join(repo, ".scribex")
	if err := os.MkdirAll(scribexDir, 0o755); err != nil {
		t.Fatalf("mkdir .scribex: %v", err)
	}

	manifestYAML := "- page: \"../../../pwned\"\n  space: \"ENG\"\n  title: \"Exploit Page\"\n  symbols:\n    - \"main.go#Foo\"\n"
	if err := os.WriteFile(filepath.Join(scribexDir, "manifest.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := exec.Command(binaryPath, "confluence", "pull")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"ATLASSIAN_URL="+srv.URL,
		"ATLASSIAN_USER=user@example.com",
		"ATLASSIAN_TOKEN=token123",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected confluence pull to fail with invalid manifest page, but succeeded. Output:\n%s", out)
	}

	// Stub must receive zero requests
	if stub.count() != 0 {
		t.Fatalf("expected stub to receive 0 requests, got %d", stub.count())
	}

	// Error output must name entry 1 and field page
	outStr := string(out)
	if !strings.Contains(outStr, "entry 1") || !strings.Contains(outStr, "page") {
		t.Errorf("expected error message to mention entry 1 and field page, got: %s", outStr)
	}

	// No file named pwned* anywhere under the temp parent
	err = filepath.WalkDir(parent, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), "pwned") {
			return fmt.Errorf("found disallowed pwned file at %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pwned file check failed: %v", err)
	}
}

// 2. Valid page -> file at .scribex/pages/123.html, content identical to stub body;
// second pull idempotent; push twice -> exactly one draft POST (state hash), corrupt state.json -> error, no POST.
func TestS6_Confluence_ValidPage_PullPushIdempotence(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)

	var draftPosts int
	var mu sync.Mutex

	stub, srv := newS6Stub(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/wiki/rest/api/content/123") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"123","body":{"storage":{"value":"<p>Valid Content</p>"}}}`))
			return
		}
		if r.Method == "POST" && r.URL.Path == "/wiki/rest/api/content" {
			mu.Lock()
			draftPosts++
			mu.Unlock()
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["status"] != "draft" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"expected status draft"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"draft-999"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	scribexDir := filepath.Join(repo, ".scribex")
	if err := os.MkdirAll(scribexDir, 0o755); err != nil {
		t.Fatalf("mkdir .scribex: %v", err)
	}

	manifestYAML := "- page: \"123\"\n  space: \"ENG\"\n  title: \"Valid Docs Page\"\n  symbols:\n    - \"main.go#Foo\"\n"
	if err := os.WriteFile(filepath.Join(scribexDir, "manifest.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runCmd := func(args ...string) ([]byte, error) {
		c := exec.Command(binaryPath, args...)
		c.Dir = repo
		c.Env = append(os.Environ(),
			"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
			"ATLASSIAN_URL="+srv.URL,
			"ATLASSIAN_USER=user@example.com",
			"ATLASSIAN_TOKEN=token123",
		)
		return c.CombinedOutput()
	}

	// 1st pull
	out, err := runCmd("confluence", "pull")
	if err != nil {
		t.Fatalf("confluence pull 1 failed: %v\nOutput: %s", err, out)
	}

	pageFile := filepath.Join(scribexDir, "pages", "123.html")
	b, err := os.ReadFile(pageFile)
	if err != nil {
		t.Fatalf("read page file: %v", err)
	}
	if string(b) != "<p>Valid Content</p>" {
		t.Fatalf("page file content mismatch: got %q, want %q", string(b), "<p>Valid Content</p>")
	}

	// 2nd pull (idempotent)
	out, err = runCmd("confluence", "pull")
	if err != nil {
		t.Fatalf("confluence pull 2 failed: %v\nOutput: %s", err, out)
	}
	b2, _ := os.ReadFile(pageFile)
	if string(b2) != "<p>Valid Content</p>" {
		t.Fatalf("page file content changed after 2nd pull")
	}

	// 1st push -> creates draft
	out, err = runCmd("confluence", "push")
	if err != nil {
		t.Fatalf("confluence push 1 failed: %v\nOutput: %s", err, out)
	}

	mu.Lock()
	firstDraftPosts := draftPosts
	mu.Unlock()
	if firstDraftPosts != 1 {
		t.Fatalf("expected exactly 1 draft POST, got %d", firstDraftPosts)
	}

	// 2nd push -> idempotent, state hash unchanged -> no new draft POST
	out, err = runCmd("confluence", "push")
	if err != nil {
		t.Fatalf("confluence push 2 failed: %v\nOutput: %s", err, out)
	}

	mu.Lock()
	secondDraftPosts := draftPosts
	mu.Unlock()
	if secondDraftPosts != 1 {
		t.Fatalf("expected draft POST count to remain 1 after 2nd push, got %d", secondDraftPosts)
	}

	// Corrupt state.json -> error, 0 new POSTs
	stateFile := filepath.Join(scribexDir, "state.json")
	if err := os.WriteFile(stateFile, []byte("invalid json {{{"), 0o644); err != nil {
		t.Fatalf("corrupt state.json: %v", err)
	}

	out, err = runCmd("confluence", "push")
	if err == nil {
		t.Fatalf("expected push with corrupt state.json to fail, but succeeded. Output: %s", out)
	}

	mu.Lock()
	finalDraftPosts := draftPosts
	mu.Unlock()
	if finalDraftPosts != 1 {
		t.Fatalf("expected 0 new POSTs after corrupt state.json, got total %d", finalDraftPosts)
	}
	if stub.count() == 0 {
		t.Fatalf("expected stub requests, got 0")
	}
}

// 3. jira -key 'PROJ-1/../../x' -> error, 0 requests; -post twice -> 1 POST.
func TestS6_Jira_InvalidKeyAndPostIdempotence(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)

	var jiraPosts int
	var postedComments []string
	var mu sync.Mutex

	stub, srv := newS6Stub(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/PROJ-123/comment") {
			mu.Lock()
			var comms []map[string]string
			for _, c := range postedComments {
				comms = append(comms, map[string]string{"body": c})
			}
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(map[string]any{"comments": comms})
			w.Write(resp)
			return
		}
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/PROJ-123") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"fields":{"description":"Ticket only talks about database schema and login","summary":"Ticket summary"}}`))
			return
		}
		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/PROJ-123/comment") {
			b, _ := io.ReadAll(r.Body)
			var comm struct {
				Body string `json:"body"`
			}
			_ = json.Unmarshal(b, &comm)
			mu.Lock()
			jiraPosts++
			postedComments = append(postedComments, comm.Body)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"comm-100"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	// Commit a branch with contract change vs main
	if _, err := gitx.Run(repo, "checkout", "-b", "feature/PROJ-123"); err != nil {
		t.Fatalf("git checkout: %v", err)
	}
	newCode := "package main\n\nfunc Foo(a int) {}\n"
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(newCode), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if _, err := gitx.Run(repo, "commit", "-am", "change Foo signature"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	runJira := func(args ...string) ([]byte, error) {
		c := exec.Command(binaryPath, append([]string{"jira", "-base", "main"}, args...)...)
		c.Dir = repo
		c.Env = append(os.Environ(),
			"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
			"ATLASSIAN_URL="+srv.URL,
			"ATLASSIAN_USER=user@example.com",
			"ATLASSIAN_TOKEN=token123",
		)
		return c.CombinedOutput()
	}

	// 1. Invalid key -> error, 0 requests
	out, err := runJira("-key", "PROJ-1/../../x")
	if err == nil {
		t.Fatalf("expected invalid key to fail, got success. Output: %s", out)
	}
	if !strings.Contains(string(out), "invalid Jira key") {
		t.Errorf("expected 'invalid Jira key' error message, got: %s", out)
	}
	if stub.count() != 0 {
		t.Fatalf("expected stub to receive 0 requests, got %d", stub.count())
	}

	// 2. Valid key with -post twice -> 1 POST
	out, err = runJira("-key", "PROJ-123", "-post")
	// Since Foo changed and ticket does not mention Foo(int), it reports mismatch and exits non-zero, but should have posted.
	mu.Lock()
	p1 := jiraPosts
	mu.Unlock()
	if p1 != 1 {
		t.Fatalf("expected 1 jira POST after first run, got %d. Output: %s", p1, out)
	}

	out, err = runJira("-key", "PROJ-123", "-post")
	mu.Lock()
	p2 := jiraPosts
	mu.Unlock()
	if p2 != 1 {
		t.Fatalf("expected jira POST count to remain 1 after second run (idempotent), got %d. Output: %s", p2, out)
	}
}

// 4. comment -repo ../../orgs/x -pr 1 -> error, 0 requests.
func TestS6_Comment_InvalidRepo_ZeroRequests(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)

	stub, srv := newS6Stub(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	cmd := exec.Command(binaryPath, "comment", "-repo", "../../orgs/x", "-pr", "1")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"GITHUB_API_URL="+srv.URL,
		"GITHUB_TOKEN=gh_token123",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected comment with invalid repo to fail, got success. Output: %s", out)
	}
	if !strings.Contains(string(out), "invalid -repo") {
		t.Errorf("expected 'invalid -repo' in error message, got: %s", out)
	}
	if stub.count() != 0 {
		t.Fatalf("expected stub to receive 0 requests, got %d", stub.count())
	}
}

// 5. .scribex symlink to an outside dir: secrets set, config set, confluence pull, init each fail;
// outside dir stays empty.
func TestS6_ScribexSymlink_OutsideDirProtected(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)
	outside := t.TempDir()

	// Create symlink repo/.scribex -> outside
	scribexLink := filepath.Join(repo, ".scribex")
	if err := os.Symlink(outside, scribexLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	runCli := func(stdinStr string, args ...string) ([]byte, error) {
		c := exec.Command(binaryPath, args...)
		c.Dir = repo
		if stdinStr != "" {
			c.Stdin = strings.NewReader(stdinStr)
		}
		c.Env = append(os.Environ(),
			"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
			"ATLASSIAN_URL=http://127.0.0.1:9999",
			"ATLASSIAN_USER=user@example.com",
			"ATLASSIAN_TOKEN=token123",
		)
		return c.CombinedOutput()
	}

	expectedErr := ".scribex must be a real directory inside the repository"

	// 1. secrets set
	out, err := runCli("secretval\n", "secrets", "set", "FOO")
	if err == nil || !strings.Contains(string(out), expectedErr) {
		t.Errorf("secrets set expected '%s', got err: %v, out: %s", expectedErr, err, out)
	}

	// 2. config set
	out, err = runCli("", "config", "set", "ai.provider", "anthropic")
	if err == nil || !strings.Contains(string(out), expectedErr) {
		t.Errorf("config set expected '%s', got err: %v, out: %s", expectedErr, err, out)
	}

	// 3. confluence pull
	out, err = runCli("", "confluence", "pull")
	if err == nil || !strings.Contains(string(out), expectedErr) {
		t.Errorf("confluence pull expected '%s', got err: %v, out: %s", expectedErr, err, out)
	}

	// 4. init
	out, err = runCli("", "init")
	if err == nil || !strings.Contains(string(out), expectedErr) {
		t.Errorf("init expected '%s', got err: %v, out: %s", expectedErr, err, out)
	}

	// Assert outside dir remains completely empty!
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("readdir outside: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was expected to be empty, but has %d entries", len(entries))
	}
}

// 6. Token never appears in any stdout/stderr (stub echoes it in a 401).
func TestS6_TokenScrubbing_NeverInOutput(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)

	secretToken := "secret-super-sensitive-token-12345"

	stub, srv := newS6Stub(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		// Stub echoes back the secret token
		w.Write([]byte(fmt.Sprintf(`{"error":"unauthorized access with token %s"}`, secretToken)))
	})
	defer srv.Close()

	scribexDir := filepath.Join(repo, ".scribex")
	_ = os.MkdirAll(scribexDir, 0o755)
	manifestYAML := "- page: \"123\"\n  space: \"ENG\"\n  title: \"Valid Docs Page\"\n  symbols:\n    - \"main.go#Foo\"\n"
	_ = os.WriteFile(filepath.Join(scribexDir, "manifest.yaml"), []byte(manifestYAML), 0o644)

	cmd := exec.Command(binaryPath, "confluence", "pull")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"SCRIBEX_ALLOW_INSECURE_LOCAL=1",
		"ATLASSIAN_URL="+srv.URL,
		"ATLASSIAN_USER=user@example.com",
		"ATLASSIAN_TOKEN="+secretToken,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected confluence pull to fail with 401, but succeeded")
	}

	outStr := string(out)
	if strings.Contains(outStr, secretToken) {
		t.Fatalf("token leaked into CLI output: %s", outStr)
	}
	if stub.count() == 0 {
		t.Fatalf("expected stub to receive 401 request, got 0")
	}
}

// 7. ATLASSIAN_URL=https://u:p@host, …?x=1, …#f -> rejected before any request.
func TestS6_AtlassianURL_StrictCleaning(t *testing.T) {
	parent := t.TempDir()
	repo := setupS6Repo(t, parent)

	scribexDir := filepath.Join(repo, ".scribex")
	_ = os.MkdirAll(scribexDir, 0o755)
	manifestYAML := "- page: \"123\"\n  space: \"ENG\"\n  title: \"Valid Docs Page\"\n  symbols:\n    - \"main.go#Foo\"\n"
	_ = os.WriteFile(filepath.Join(scribexDir, "manifest.yaml"), []byte(manifestYAML), 0o644)

	stub, srv := newS6Stub(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"123"}`))
	})
	defer srv.Close()

	badURLs := []struct {
		url       string
		errSubstr string
	}{
		{"https://user:pass@atlassian.example.com", "userinfo not allowed"},
		{"https://atlassian.example.com?query=1", "query and fragment not allowed"},
		{"https://atlassian.example.com#fragment", "query and fragment not allowed"},
		{"http://atlassian.example.com", "scheme must be https"},
	}

	for _, tc := range badURLs {
		cmd := exec.Command(binaryPath, "confluence", "pull")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"ATLASSIAN_URL="+tc.url,
			"ATLASSIAN_USER=user@example.com",
			"ATLASSIAN_TOKEN=token123",
		)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("url %q: expected error, got success", tc.url)
		}
		if !strings.Contains(string(out), tc.errSubstr) {
			t.Errorf("url %q: expected error containing %q, got: %s", tc.url, tc.errSubstr, out)
		}
	}

	if stub.count() != 0 {
		t.Fatalf("expected stub to receive 0 requests, got %d", stub.count())
	}
}
