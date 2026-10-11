package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"artix/pkg/policy"
)

func TestConfinementBlocksNetworkAndOutsideWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			if os.Getenv("CI") != "" || os.Getenv("ARTIX_ENTERPRISE") != "" {
				t.Fatalf("bwrap OS sandbox is required in CI / Enterprise environments, but was not found: %v", err)
			}
			t.Skip("no OS sandbox available")
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	ws := t.TempDir()
	outside := filepath.Join(os.Getenv("HOME"), ".artix-sandbox-test-should-not-exist")
	defer os.Remove(outside)
	box := NewSandbox(ws)
	ctx := context.Background()

	if r := box.Run(ctx, "echo hi > inside.txt && cat inside.txt", nil); !r.Success() || r.Isolation != IsolationOSSandbox {
		t.Fatalf("write inside workspace must work under os-sandbox: %+v", r)
	}
	if r := box.Run(ctx, "touch "+outside, nil); r.Success() {
		t.Fatal("write outside the workspace must be denied")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("file was created outside the workspace")
	}
	if r := box.Run(ctx, "curl -s -m 3 "+srv.URL, nil); r.Success() {
		t.Fatal("network must be denied")
	}
	t.Setenv("ARTIX_SANDBOX_NETWORK", "1")
	if r := box.Run(ctx, "curl -s -m 3 "+srv.URL, nil); !r.Success() || r.Stdout != "ok" {
		t.Fatalf("network opt-in must work: %+v", r)
	}
}

func TestBuildBwrapArgs(t *testing.T) {
	writable := []string{"/tmp", "/var/tmp"}
	argsNoNet := BuildBwrapArgs(writable, false, "echo hello")
	
	hasRoBind := false
	hasUnshareNet := false
	for _, a := range argsNoNet {
		if a == "--ro-bind" {
			hasRoBind = true
		}
		if a == "--unshare-net" {
			hasUnshareNet = true
		}
	}
	if !hasRoBind {
		t.Errorf("expected --ro-bind in bwrap args: %v", argsNoNet)
	}
	if !hasUnshareNet {
		t.Errorf("expected --unshare-net when network disabled: %v", argsNoNet)
	}

	argsWithNet := BuildBwrapArgs(writable, true, "echo hello")
	for _, a := range argsWithNet {
		if a == "--unshare-net" {
			t.Errorf("did not expect --unshare-net when network allowed: %v", argsWithNet)
		}
	}
}

func TestEnterpriseModeBlocksSandboxEscape(t *testing.T) {
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_SANDBOX", "off")

	// Even with ARTIX_SANDBOX=off, enterprise mode must prevent plain unconfined fallback
	// on platforms where os sandbox exists
	_, iso := confine(t.TempDir(), "echo hi")
	if runtime.GOOS == "darwin" {
		if iso != IsolationOSSandbox {
			t.Errorf("enterprise mode must ignore ARTIX_SANDBOX=off on darwin, got isolation: %s", iso)
		}
	}
}

func TestR13_6_EnterpriseMode_ViaPolicyIsEnterprise_BlocksSandboxEscape(t *testing.T) {
	policy.EnforceSignedPolicy()
	defer func() {
		policy.RequireSignedPolicyFlag = "false"
		policy.ResetCachedPolicy()
	}()
	policy.ResetCachedPolicy()

	t.Setenv("ARTIX_ENTERPRISE", "")
	t.Setenv("KRITIX_ENTERPRISE", "")
	t.Setenv("ARTIX_SANDBOX", "off")

	if !policy.IsEnterprise() {
		t.Fatalf("policy.IsEnterprise() must be true when EnforceSignedPolicy() is active")
	}

	_, iso := confine(t.TempDir(), "echo hi")
	if runtime.GOOS == "darwin" {
		if iso != IsolationOSSandbox {
			t.Errorf("R13-6 VIOLATION: policy.IsEnterprise() must ignore ARTIX_SANDBOX=off on darwin, got isolation: %s", iso)
		}
	}
}

func TestEnterpriseModeFailClosedRefusal(t *testing.T) {
	t.Setenv("ARTIX_ENTERPRISE", "1")
	// Clear PATH so LookPath fails for sandbox-exec and bwrap
	t.Setenv("PATH", "")

	box := NewSandbox(t.TempDir())
	res := box.Run(context.Background(), "echo fail-closed", nil)
	if res.Isolation != IsolationRefused {
		t.Fatalf("expected IsolationRefused in enterprise mode when no sandbox tool is available, got: %s", res.Isolation)
	}
	if res.ExitCode != 126 {
		t.Errorf("expected exit code 126 for refused sandbox execution, got %d", res.ExitCode)
	}
	if res.Success() {
		t.Errorf("refused sandbox execution must not succeed")
	}
}

func TestSandboxBlocksSensitiveCredentialsRead(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			t.Skip("skipping credential read deny test: no sandbox available on this OS")
		}
	}

	fakeHome := t.TempDir()
	sshDir := filepath.Join(fakeHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(sshDir, "id_ed25519")
	if err := os.WriteFile(secretFile, []byte("SUPER_SECRET_KEY"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", fakeHome)
	ws := t.TempDir()
	box := NewSandbox(ws)

	res := box.Run(context.Background(), "cat "+secretFile, nil)
	if res.Success() {
		t.Fatalf("sandbox must deny reading sensitive ssh credential from %s, but read succeeded: %s", secretFile, res.Stdout)
	}
}

func TestSandboxBlocksArtixHomeDirectoryReadAndWrite(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			t.Skip("skipping artix home sandbox test: no sandbox available on this OS")
		}
	}

	fakeHome := t.TempDir()
	artixDir := filepath.Join(fakeHome, ".artix")
	if err := os.MkdirAll(artixDir, 0700); err != nil {
		t.Fatal(err)
	}
	personaFile := filepath.Join(artixDir, "persona.json")
	if err := os.WriteFile(personaFile, []byte("SENSITIVE_STEERING_DATA"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", fakeHome)
	ws := t.TempDir()
	box := NewSandbox(ws)

	// 1. Read must be denied
	resRead := box.Run(context.Background(), "cat "+personaFile, nil)
	if resRead.Success() {
		t.Fatalf("sandbox must deny reading ~/.artix files, but succeeded: %s", resRead.Stdout)
	}

	// 2. Write to ~/.artix must be denied (prevent poisoning of global state)
	poisonFile := filepath.Join(artixDir, "poison.json")
	resWrite := box.Run(context.Background(), "echo poisoned > "+poisonFile, nil)
	if resWrite.Success() {
		t.Fatalf("sandbox must deny writing to ~/.artix, but succeeded")
	}
	if _, err := os.Stat(poisonFile); err == nil {
		t.Fatalf("poison file was created inside ~/.artix by sandboxed code")
	}
}

func TestSandbox_GitDirectoryMountedReadOnly_BlocksGitHookWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			t.Skip("skipping sandbox test: no sandbox available on this OS")
		}
	}

	ws := t.TempDir()

	// Initialize a valid git repo in the workspace
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = ws
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}
	exec.Command("git", "-C", ws, "config", "user.name", "Test User").Run()
	exec.Command("git", "-C", ws, "config", "user.email", "test@example.com").Run()

	// Create an initial commit
	initFile := filepath.Join(ws, "file.txt")
	if err := os.WriteFile(initFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", ws, "add", "file.txt").Run()
	exec.Command("git", "-C", ws, "commit", "-m", "initial commit").Run()

	canaryFile := filepath.Join(ws, "hook_canary.txt")
	defer os.Remove(canaryFile)

	box := NewSandbox(ws)
	ctx := context.Background()

	// Attempt to write a malicious pre-commit hook inside the sandbox
	payload := `mkdir -p .git/hooks && printf '#!/bin/sh\necho PWNED > hook_canary.txt\nexit 0\n' > .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit`
	res := box.Run(ctx, payload, nil)

	// Sandboxed execution MUST fail when trying to write to .git
	if res.Success() {
		t.Fatalf("sandbox must block writing to .git / .git/hooks/pre-commit, but command succeeded: %+v", res)
	}

	// Verify hook file was not created
	hookFile := filepath.Join(ws, ".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookFile); err == nil {
		t.Fatalf(".git/hooks/pre-commit was created by sandboxed execution")
	}

	// Create another commit outside sandbox to verify hook does not fire
	secondFile := filepath.Join(ws, "second.txt")
	_ = os.WriteFile(secondFile, []byte("second"), 0644)
	exec.Command("git", "-C", ws, "add", "second.txt").Run()
	if out, err := exec.Command("git", "-C", ws, "commit", "-m", "second commit").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v (%s)", err, string(out))
	}

	// Canary file must NOT exist
	if _, err := os.Stat(canaryFile); err == nil {
		t.Fatalf("malicious pre-commit hook executed during git commit outside sandbox!")
	}
}

func TestBuildBwrapArgs_MountsGitReadOnly(t *testing.T) {
	ws := t.TempDir()
	gitDir := filepath.Join(ws, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	writable := []string{ws}
	args := BuildBwrapArgs(writable, false, "echo hello")

	hasRoBindGit := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == gitDir && args[i+2] == gitDir {
			hasRoBindGit = true
			break
		}
	}

	if !hasRoBindGit {
		t.Fatalf("expected bwrap args to include --ro-bind %s %s, got: %v", gitDir, gitDir, args)
	}
}
