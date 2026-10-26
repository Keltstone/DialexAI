package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"scribex/pkg/anchors"
	"scribex/pkg/docs"
	"scribex/pkg/gitx"
)

func TestIncompleteErrorHelper(t *testing.T) {
	cases := []struct {
		unknowns []docs.Unknown
		want     string
	}{
		{
			unknowns: []docs.Unknown{
				{File: "a.kt", Reason: "unterminated string"},
				{File: "b.ts", Reason: "syntax error"},
			},
			want: "incomplete: cannot analyse 2 file(s): a.kt, b.ts",
		},
		{
			unknowns: []docs.Unknown{
				{File: "main.go", Reason: "unparsable"},
			},
			want: "incomplete: cannot analyse 1 file(s): main.go",
		},
	}

	for _, c := range cases {
		err := incompleteError(c.unknowns)
		if err == nil || err.Error() != c.want {
			t.Errorf("incompleteError() = %v, want %q", err, c.want)
		}
	}
}

func TestGateWithUnknownReturnsTrue(t *testing.T) {
	d := t.TempDir()
	for _, a := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "a@b.c"},
		{"config", "user.name", "t"},
	} {
		if _, err := gitx.Run(d, a...); err != nil {
			t.Fatalf("git %v: %v", a, err)
		}
	}
	os.WriteFile(filepath.Join(d, "init.txt"), []byte("init\n"), 0o644)
	gitx.Run(d, "add", "-A")
	gitx.Run(d, "commit", "-qm", "initial")

	// Add an unparsable kotlin file
	os.WriteFile(filepath.Join(d, "bad.kt"), []byte("fun bad() = \"unterminated string\n"), 0o644)

	run, err := gate(d, "HEAD")
	if err != nil {
		t.Fatalf("gate error: %v", err)
	}
	if !run {
		t.Fatal("expected gate to return run=true when unknown files are present")
	}
}

func TestCLIRound1Repros(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI binary tests in -short mode")
	}

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "scribex")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// Helper to create test repo
	setupRepo := func(t *testing.T) string {
		t.Helper()
		d := t.TempDir()
		for _, a := range [][]string{
			{"init", "-q", "-b", "main"},
			{"config", "user.email", "a@b.c"},
			{"config", "user.name", "t"},
		} {
			if _, err := gitx.Run(d, a...); err != nil {
				t.Fatalf("git %v: %v", a, err)
			}
		}
		os.WriteFile(filepath.Join(d, "file.go"), []byte("package p\nconst X = 1\n"), 0o644)
		os.WriteFile(filepath.Join(d, "doc.md"), []byte("<!-- @scribex:anchor(file.go#X) -->\n```signature\nconst X = 1\n```\n"), 0o644)
		gitx.Run(d, "add", "-A")
		gitx.Run(d, "commit", "-qm", "initial")
		return d
	}

	// Repro 1: check -base '--output=/tmp/x' exits non-zero and creates nothing
	t.Run("OptionInjectionRef", func(t *testing.T) {
		repoDir := setupRepo(t)
		pwnFile := filepath.Join(t.TempDir(), "pwn")
		cmd := exec.Command(binPath, "check", "-base", "--output="+pwnFile, "-C", repoDir)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected check with injected option ref to exit non-zero")
		}
		if _, statErr := os.Stat(pwnFile); statErr == nil {
			t.Fatalf("pwn file %s was created by injected option ref!", pwnFile)
		}
	})

	// Repro 2: check -base nosuch non-zero
	t.Run("NonexistentRef", func(t *testing.T) {
		repoDir := setupRepo(t)
		cmd := exec.Command(binPath, "check", "-base", "nosuch", "-C", repoDir)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected check with nonexistent ref to exit non-zero")
		}
	})

	// Repro 3: const value change exits 1 with "1 contract delta"
	t.Run("ConstValueChange", func(t *testing.T) {
		repoDir := setupRepo(t)
		// Change const X = 2 in working tree
		os.WriteFile(filepath.Join(repoDir, "file.go"), []byte("package p\nconst X = 2\n"), 0o644)
		cmd := exec.Command(binPath, "check", "-base", "HEAD", "-C", repoDir)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected check with const value change to exit non-zero")
		}
		outStr := out.String()
		if !strings.Contains(outStr, "1 contract delta") {
			t.Errorf("expected output to contain '1 contract delta', got:\n%s", outStr)
		}
	})
}

func TestN34_WriteReport(t *testing.T) {
	dir := t.TempDir()

	// 1. Symlink refusal & victim unchanged
	victimFile := filepath.Join(dir, "victim.txt")
	_ = os.WriteFile(victimFile, []byte("safe content"), 0o644)
	symReport := filepath.Join(dir, "sym-report.md")
	_ = os.Symlink(victimFile, symReport)

	err := writeReport(symReport, []byte("pwned"))
	if err == nil {
		t.Fatal("expected writeReport to refuse symlink target")
	}
	victimContent, _ := os.ReadFile(victimFile)
	if string(victimContent) != "safe content" {
		t.Fatalf("victim file modified despite symlink refusal: %s", string(victimContent))
	}

	// 2. Directory refusal
	dirTarget := filepath.Join(dir, "target_dir")
	_ = os.MkdirAll(dirTarget, 0o755)
	err = writeReport(dirTarget, []byte("data"))
	if err == nil {
		t.Fatal("expected writeReport to refuse directory target")
	}

	// 3. Missing parent refusal (must not create parent)
	missingParentTarget := filepath.Join(dir, "missing_subdir", "report.md")
	err = writeReport(missingParentTarget, []byte("data"))
	if err == nil {
		t.Fatal("expected writeReport to refuse missing parent directory")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "missing_subdir")); !os.IsNotExist(statErr) {
		t.Fatal("writeReport created missing parent directory!")
	}

	// 4. Success: file mode 0644, no .tmp left behind
	cleanReport := filepath.Join(dir, "report.md")
	err = writeReport(cleanReport, []byte("valid report content"))
	if err != nil {
		t.Fatalf("writeReport failed: %v", err)
	}
	fi, statErr := os.Stat(cleanReport)
	if statErr != nil {
		t.Fatalf("stat report: %v", statErr)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("file mode %v, expected 0644", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}

	// 5. Write failure makes check() exit non-zero
	repoDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(repoDir, "doc.md"), []byte("# Doc\n"), 0o644)
	checkErr := check(repoDir, "", symReport)
	if checkErr == nil {
		t.Fatal("expected check to exit non-zero on report write failure")
	}
}

func TestN35_CommentUntrustedBodyFile(t *testing.T) {
	var requestCount int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		if r.URL.Path == "/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"login":"test-bot"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/comments") {
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
				return
			}
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":1001,"body":"created"}`))
				return
			}
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	dir := t.TempDir()
	summaryFile := filepath.Join(dir, "step_summary.md")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", ts.URL)
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryFile)

	// 1. Genuine Report.Markdown() passes byte-for-byte and sends request
	t.Run("GenuineRoundTrip", func(t *testing.T) {
		atomic.StoreInt64(&requestCount, 0)
		_ = os.Remove(summaryFile)

		rep := docs.Report{
			Total:  5,
			Issues: []docs.Issue{{File: "main.go", Issue: anchors.Issue{Line: 1, Ref: "main.go#Main", Msg: "doc stale"}}},
		}
		genuineBody := rep.Markdown()
		bodyFile := filepath.Join(dir, "genuine.md")
		_ = os.WriteFile(bodyFile, []byte(genuineBody), 0o644)

		err := comment(dir, "", "owner/repo", 42, bodyFile, "test-bot")
		if err != nil {
			t.Fatalf("comment failed on genuine report: %v", err)
		}
		if atomic.LoadInt64(&requestCount) == 0 {
			t.Fatal("expected HTTP requests for genuine report comment")
		}
	})

	// 2. Table-driven hostile input rejection: stub receives 0 requests
	hostileCases := []struct {
		name    string
		content string
	}{
		{"mention_team", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: @org/team alert\n"},
		{"mention_user", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: @user please review\n"},
		{"html_img", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: <img src=x>\n"},
		{"html_script", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: <script>alert(1)</script>\n"},
		{"markdown_image", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: ![x](http://t/p.png)\n"},
		{"markdown_link", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: [click](http://evil)\n"},
		{"bare_url", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: http://evil.com/leak\n"},
		{"forged_marker", docs.ReportHeader + "\n\n<!-- scribex-report:v1 -->\nScribex found 1 doc issue(s) (1 anchors):\n"},
		{"heading_approved", docs.ReportHeader + "\n\n# Approved by Security Team\n"},
		{"nul_byte", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: bad\x00byte\n"},
		{"bidi_char", docs.ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: \u202Eoverride\n"},
		{"oversized_70kb", docs.ReportHeader + "\n\n" + strings.Repeat("- `a.go` line 1: issue\n", 4000)},
	}

	for _, tc := range hostileCases {
		t.Run(tc.name, func(t *testing.T) {
			atomic.StoreInt64(&requestCount, 0)
			_ = os.Remove(summaryFile)

			hPath := filepath.Join(dir, "hostile_"+tc.name+".md")
			_ = os.WriteFile(hPath, []byte(tc.content), 0o644)

			err := comment(dir, "", "owner/repo", 42, hPath, "test-bot")
			if err == nil {
				t.Fatalf("expected error for hostile input %s, got nil", tc.name)
			}
			if reqs := atomic.LoadInt64(&requestCount); reqs > 0 {
				t.Fatalf("hostile input %s made %d HTTP requests to stub!", tc.name, reqs)
			}

			// Verify step summary received degraded message
			summaryBytes, _ := os.ReadFile(summaryFile)
			summaryStr := string(summaryBytes)
			if !strings.Contains(summaryStr, "report failed validation:") {
				t.Errorf("step summary does not contain validation failure message: %s", summaryStr)
			}
		})
	}

	// 3. Symlink body-file refusal: 0 requests
	t.Run("SymlinkBodyFile", func(t *testing.T) {
		atomic.StoreInt64(&requestCount, 0)
		_ = os.Remove(summaryFile)

		realFile := filepath.Join(dir, "real_body.md")
		_ = os.WriteFile(realFile, []byte(docs.ReportHeader+"\n\nScribex: docs in sync (0 anchors checked).\n"), 0o644)
		symFile := filepath.Join(dir, "sym_body.md")
		_ = os.Symlink(realFile, symFile)

		err := comment(dir, "", "owner/repo", 42, symFile, "test-bot")
		if err == nil {
			t.Fatal("expected comment to refuse symlinked body-file")
		}
		if reqs := atomic.LoadInt64(&requestCount); reqs > 0 {
			t.Fatalf("symlinked body-file made %d HTTP requests to stub!", reqs)
		}
	})

	// 4. FIFO body-file refusal: 0 requests
	t.Run("FifoBodyFile", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping FIFO test on Windows")
		}
		atomic.StoreInt64(&requestCount, 0)
		_ = os.Remove(summaryFile)

		fifoPath := filepath.Join(dir, "body.fifo")
		if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
			t.Skipf("mkfifo failed: %v", err)
		}

		err := comment(dir, "", "owner/repo", 42, fifoPath, "test-bot")
		if err == nil {
			t.Fatal("expected comment to refuse FIFO body-file")
		}
		if reqs := atomic.LoadInt64(&requestCount); reqs > 0 {
			t.Fatalf("FIFO body-file made %d HTTP requests to stub!", reqs)
		}
	})
}

func TestEnterpriseAuth(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		if r.URL.Path == "/api/v3/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"login":"enterprise-bot"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/comments") {
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
				return
			}
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":1001,"body":"created"}`))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "report.md")
	_ = os.WriteFile(bodyFile, []byte(docs.ReportHeader+"\n\nScribex: docs in sync (0 anchors checked).\n"), 0o644)

	// Configure GitHub Enterprise Server URL
	t.Setenv("GITHUB_TOKEN", "enterprise-token")
	t.Setenv("GITHUB_API_URL", ts.URL+"/api/v3")
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_ALLOW_PRIVATE_HOSTS", "1")

	err := comment(dir, "", "enterprise-org/repo", 10, bodyFile, "enterprise-bot")
	if err != nil {
		t.Fatalf("enterprise comment failed: %v", err)
	}
	if !strings.HasPrefix(requestedPath, "/api/v3") {
		t.Errorf("enterprise API path prefix missing, got %s", requestedPath)
	}
}
