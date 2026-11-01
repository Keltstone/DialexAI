package llm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scribex/pkg/config"
)

// fakeCLI writes an executable named name that records argv/stdin/cwd/env under rec and prints body.
func fakeCLI(t *testing.T, name, body string) (rec string) {
	t.Helper()
	bin, rec := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> " + rec + "/args; done\n" +
		"cat > " + rec + "/stdin\n" +
		"pwd > " + rec + "/cwd\n" +
		"env > " + rec + "/env\n" +
		"touch leak.txt\n" +
		body + "\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rec
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

func cliAI(cmd, model string) config.AI {
	return config.AI{Provider: "cli", CLICommand: cmd, Model: model, MaxTokens: 10, MaxCalls: 1}
}

func TestCLIInvocation(t *testing.T) {
	rec := fakeCLI(t, "claude", "echo hello")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secretsecretsecret")
	t.Setenv("GITHUB_TOKEN", "ghp_secretsecretsecretsecret")
	t.Setenv("ATLASSIAN_TOKEN", "atl_secretsecretsecret")
	repoCwd, _ := os.Getwd()
	c, err := New(cliAI("claude", "claude-sonnet-5-5"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Complete("SYS", "USER")
	if err != nil || got != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if a := read(t, rec+"/args"); a != "-p\n--tools\n\n--no-session-persistence\n--output-format\ntext\n--model\nclaude-sonnet-5-5\n" {
		t.Errorf("argv = %q", a)
	}
	if in := read(t, rec+"/stdin"); !strings.Contains(in, "SYS") || !strings.Contains(in, "USER") {
		t.Errorf("stdin = %q", in)
	}
	cwd := strings.TrimSpace(read(t, rec+"/cwd"))
	if cwd == repoCwd || strings.HasPrefix(cwd, repoCwd) {
		t.Errorf("cwd is the repo: %s", cwd)
	}
	env := read(t, rec+"/env")
	for _, bad := range []string{"ANTHROPIC_API_KEY", "GITHUB_TOKEN", "ATLASSIAN_TOKEN", "secretsecret"} {
		if strings.Contains(env, bad) {
			t.Errorf("env leaked %s", bad)
		}
	}
	if _, err := os.Stat("leak.txt"); err == nil {
		os.Remove("leak.txt")
		t.Error("CLI wrote into the repo cwd")
	}
	if _, err := os.Stat(cwd); err == nil {
		t.Error("temp cwd not cleaned up")
	}
}

func TestCLIEmptyModelOmitsFlag(t *testing.T) {
	rec := fakeCLI(t, "claude", "echo ok")
	c, _ := New(cliAI("claude", ""))
	if _, err := c.Complete("s", "u"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, rec+"/args"), "--model") {
		t.Error("--model passed with empty model")
	}
}

func TestCLIPassEnvFromUserEnvOnly(t *testing.T) {
	rec := fakeCLI(t, "claude", "echo ok")
	t.Setenv("MY_CLI_AUTH", "visible-value")
	t.Setenv("SCRIBEX_CLI_PASS_ENV", "MY_CLI_AUTH")
	c, _ := New(cliAI("claude", ""))
	if _, err := c.Complete("s", "u"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, rec+"/env"), "MY_CLI_AUTH=visible-value") {
		t.Error("explicit pass-through env missing")
	}
}

func TestCLIAgyPromptAsSingleArgv(t *testing.T) {
	rec := fakeCLI(t, "agy", "echo from-agy")
	c, err := New(cliAI("agy", "gemini-3.8-flash-high"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Complete("SYS", "-rf; $(touch pwned) USER"); err != nil || got != "from-agy" {
		t.Fatal(got, err)
	}
	lines := strings.Split(strings.TrimSpace(read(t, rec+"/args")), "\n")
	var prompts int
	for _, l := range lines {
		if strings.HasPrefix(l, "-p=") {
			prompts++
		}
	}
	if prompts != 1 {
		t.Errorf("want exactly one -p= element, got %d in %q", prompts, lines)
	}
	if _, err := os.Stat("pwned"); err == nil {
		os.Remove("pwned")
		t.Error("shell injection")
	}
}

func TestCLIUnverifiedRefused(t *testing.T) {
	fakeCLI(t, "codex", "echo x")
	fakeCLI(t, "gemini", "echo x")
	if _, err := New(cliAI("codex", "")); err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("codex must be refused until verified: %v", err)
	}
	if _, err := New(cliAI("gemini", "")); err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("gemini must be refused until verified: %v", err)
	}
	t.Setenv("SCRIBEX_ALLOW_UNVERIFIED_CLI", "1")
	if _, err := New(cliAI("codex", "")); err != nil {
		t.Fatalf("explicit allow should work for codex: %v", err)
	}
	if _, err := New(cliAI("gemini", "")); err != nil {
		t.Fatalf("explicit allow should work for gemini: %v", err)
	}
}

func TestCLIAgenticRefused(t *testing.T) {
	cliTable = append(cliTable, CLISpec{Name: "agentic", Bin: "agentic", Agentic: true, Verified: "x", Checked: "2026-11-01"})
	defer func() { cliTable = cliTable[:len(cliTable)-1] }()
	fakeCLI(t, "agentic", "echo x")
	if _, err := newCLI(config.AI{Provider: "cli", CLICommand: "agentic"}); err == nil || !strings.Contains(err.Error(), "tool access") {
		t.Fatalf("agentic CLI must be refused: %v", err)
	}
	t.Setenv("SCRIBEX_ALLOW_AGENTIC_CLI", "1")
	if _, err := newCLI(config.AI{Provider: "cli", CLICommand: "agentic"}); err != nil {
		t.Fatal(err)
	}
}

func TestCLIRejectsBadCommandAndModel(t *testing.T) {
	fakeCLI(t, "claude", "echo x")
	for _, cmd := range []string{"/bin/sh", "bash", "claude --x", "../claude", ""} {
		if _, err := New(cliAI(cmd, "")); err == nil {
			t.Errorf("cli_command %q accepted", cmd)
		}
	}
	for _, m := range []string{"-rf", "--dangerously-skip-permissions", "a b", "a\nb", strings.Repeat("a", 101)} {
		if _, err := New(cliAI("claude", m)); err == nil {
			t.Errorf("model %q accepted", m)
		}
	}
	for _, m := range []string{"claude-sonnet-5-5", "sonnet", "llama3.2:3b", "gemini-3.8-flash-high"} {
		if _, err := New(cliAI("claude", m)); err != nil {
			t.Errorf("model %q rejected: %v", m, err)
		}
	}
}

func TestCLITimeoutOutputCapAndFailure(t *testing.T) {
	fakeCLI(t, "claude", "sleep 5")
	old := cliTimeout
	cliTimeout = 300 * time.Millisecond
	c, _ := New(cliAI("claude", ""))
	start := time.Now()
	if _, err := c.Complete("s", "u"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("want timeout, got %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Error("timeout did not kill promptly")
	}
	cliTimeout = old

	fakeCLI(t, "claude", "head -c 5000 /dev/zero | tr '\\0' x")
	oldMax := cliMaxOut
	cliMaxOut = 100
	c, _ = New(cliAI("claude", ""))
	if _, err := c.Complete("s", "u"); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("want output cap error, got %v", err)
	}
	cliMaxOut = oldMax

	fakeCLI(t, "claude", "echo 'boom sk-ant-api03-abcdefghijklmnopqrstuvwx' >&2; exit 3")
	c, _ = New(cliAI("claude", ""))
	_, err := c.Complete("s", "u")
	if err == nil || strings.Contains(err.Error(), "sk-ant") || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("want redacted failure, got %v", err)
	}

	fakeCLI(t, "claude", "true")
	c, _ = New(cliAI("claude", ""))
	if _, err := c.Complete("s", "u"); err == nil {
		t.Error("empty output must error")
	}
}

func TestCLITableComplete(t *testing.T) {
	for _, n := range config.CLINames {
		s, ok := CLISpecFor(n)
		if !ok || s.Bin == "" || len(s.Args) == 0 || s.Reviewer {
			t.Errorf("table entry %q missing or malformed: %+v", n, s)
		}
		ck, err := time.Parse("2006-01-02", s.Checked)
		if err != nil {
			t.Errorf("%s: bad Checked %q", n, s.Checked)
			continue
		}
		if time.Since(ck) > 120*24*time.Hour {
			t.Errorf("%s: CLI args/models last checked %s; re-verify flags with --help and update Checked", n, s.Checked)
		}
		for _, m := range s.Suggested {
			if !config.ValidModel(m) {
				t.Errorf("%s: suggested model %q is not a valid model string", n, m)
			}
		}
	}
	for _, n := range []string{"claude", "codex", "gemini", "agy"} {
		if _, ok := CLISpecFor(n); !ok {
			t.Errorf("missing %s", n)
		}
	}
}

func TestOllamaLoopback(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		b := make([]byte, 4096)
		n, _ := r.Body.Read(b)
		gotBody = string(b[:n])
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"pong"}}]}`))
	}))
	defer srv.Close()
	c, err := New(config.AI{Provider: "ollama", Model: "llama3.2:3b", BaseURL: srv.URL, MaxTokens: 50, MaxCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Complete("s", "u")
	if err != nil || got != "pong" {
		t.Fatal(got, err)
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "" || !strings.Contains(gotBody, `"llama3.2:3b"`) {
		t.Errorf("path=%q auth=%q body=%q", gotPath, gotAuth, gotBody)
	}
}

func TestOllamaRejectsNonLoopbackCleartextAndUntrusted(t *testing.T) {
	for _, u := range []string{"http://example.com", "http://10.0.0.5:11434", "https://example.com", "ftp://127.0.0.1", "http://user:pw@127.0.0.1"} {
		if _, err := New(config.AI{Provider: "ollama", Model: "m", BaseURL: u, MaxTokens: 5, MaxCalls: 1}); err == nil {
			t.Errorf("%s accepted", u)
		}
	}
	if _, err := New(config.AI{Provider: "ollama", Model: "", MaxTokens: 5, MaxCalls: 1}); err == nil {
		t.Error("empty model accepted")
	}
	t.Setenv("SCRIBEX_AI_ALLOWED_HOSTS", "ollama.corp.example")
	if _, err := New(config.AI{Provider: "ollama", Model: "m", BaseURL: "https://ollama.corp.example", MaxTokens: 5, MaxCalls: 1}); err != nil {
		t.Errorf("trusted https host rejected: %v", err)
	}
}

func TestCLIPromptTooLarge(t *testing.T) {
	fakeCLI(t, "claude", "echo ok")
	c, err := New(cliAI("claude", "claude-sonnet-5-5"))
	if err != nil {
		t.Fatal(err)
	}
	bigPrompt := strings.Repeat("x", cliMaxPrompt+100)
	_, err = c.Complete("system", bigPrompt)
	if err == nil || !strings.Contains(err.Error(), "prompt too large") {
		t.Fatalf("expected prompt too large error, got: %v", err)
	}
}
