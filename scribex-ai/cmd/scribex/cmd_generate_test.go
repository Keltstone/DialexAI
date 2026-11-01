package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaude answers like a model: one section per "- id:" line (symbols) or per requested "- Section" line.
const fakeClaude = `#!/bin/sh
in=$(cat)
echo "$in" | grep '^- id: ' | sed 's/^- id: //' | while read id; do printf '=== %s ===\nDoes something useful.\n\n' "$id"; done
echo "$in" | grep -q 'Use exactly these sections' && printf '=== Components ===\nA small service.\n'
exit 0
`

func e2eRepo(t *testing.T) (repo, bin string, env []string) {
	t.Helper()
	tmp := t.TempDir()
	bin = filepath.Join(tmp, "scribex")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	fb := filepath.Join(tmp, "fakebin")
	os.MkdirAll(fb, 0o755)
	os.WriteFile(filepath.Join(fb, "claude"), []byte(fakeClaude), 0o755)
	repo = filepath.Join(tmp, "repo")
	os.MkdirAll(filepath.Join(repo, "svc"), 0o755)
	os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "svc/svc.go"), []byte("package svc\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	env = append(os.Environ(), "PATH="+fb+":"+os.Getenv("PATH"), "HOME="+tmp)
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"}, {"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	return
}

func runGen(t *testing.T, bin, repo string, env []string, args ...string) (string, error) {
	c := exec.Command(bin, append([]string{"generate", "-C", repo}, args...)...)
	c.Env = env
	out, err := c.CombinedOutput()
	return string(out), err
}

func TestGenerateE2E(t *testing.T) {
	repo, bin, env := e2eRepo(t)
	out, err := runGen(t, bin, repo, env)
	if err != nil || !strings.Contains(out, "plan only") {
		t.Fatalf("plan: %v %s", err, out)
	}
	if _, serr := os.Stat(filepath.Join(repo, "docs")); serr == nil {
		t.Fatal("plan wrote docs")
	}
	if out, err = runGen(t, bin, repo, env, "-apply"); err == nil || !strings.Contains(out, "confirm-send") {
		t.Fatalf("cloud apply without confirm must fail: %v %s", err, out)
	}
	if out, err = runGen(t, bin, repo, env, "-apply", "-confirm-send", "-branch"); err != nil {
		t.Fatalf("apply: %v %s", err, out)
	}
	if !strings.Contains(out, "branch scribex/docs-") {
		t.Fatalf("%s", out)
	}
	if out, err := exec.Command(bin, "check", "-C", repo).CombinedOutput(); err != nil {
		t.Fatalf("check after generate: %v %s", err, out)
	}
	// second run on the new branch is a no-op
	if out, err = runGen(t, bin, repo, env, "-apply", "-confirm-send"); err != nil || !strings.Contains(out, "wrote 0 doc(s), 0 model call(s)") {
		t.Fatalf("second run: %v %s", err, out)
	}
}

func TestGenerateTypesFlagDerived(t *testing.T) {
	repo, bin, env := e2eRepo(t)
	out, err := runGen(t, bin, repo, env, "-types", "tech-reference,runbook-ops", "-apply", "-confirm-send")
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	b, _ := os.ReadFile(filepath.Join(repo, "docs/ops/runbook.md"))
	if !strings.Contains(string(b), "@scribex:derived(id=runbook-ops:repo)") {
		t.Fatalf("%s", b)
	}
}

func TestGeneratePushOnlyNewBranchAndNothingToPublish(t *testing.T) {
	repo, bin, env := e2eRepo(t)
	tmp := filepath.Dir(repo)
	bare := filepath.Join(tmp, "bare.git")
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "remote", "add", "origin", bare).CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v %s", err, out)
	}
	// Initial push of main to bare remote
	if out, err := exec.Command("git", "-C", repo, "push", "origin", "main").CombinedOutput(); err != nil {
		t.Fatalf("git push main: %v %s", err, out)
	}
	mainSha, _ := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/main").Output()

	// 1. Run generate with -branch -push
	out, err := runGen(t, bin, repo, env, "-apply", "-confirm-send", "-branch", "-push")
	if err != nil {
		t.Fatalf("generate -branch -push: %v %s", err, out)
	}
	if !strings.Contains(out, "branch scribex/docs-") {
		t.Fatalf("expected new branch output: %s", out)
	}

	// Verify origin/main was NOT touched
	mainShaAfter, _ := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/main").Output()
	if string(mainSha) != string(mainShaAfter) {
		t.Fatalf("push modified the current/main branch on remote: %s vs %s", mainSha, mainShaAfter)
	}

	// Verify the new branch exists on the remote
	branches, _ := exec.Command("git", "--git-dir", bare, "branch", "-a").Output()
	if !strings.Contains(string(branches), "scribex/docs-") {
		t.Fatalf("expected remote bare to have scribex/docs- branch, got: %s", branches)
	}

	// 2. Second run when nothing was written: verify "nothing to publish"
	out2, err := runGen(t, bin, repo, env, "-apply", "-confirm-send", "-branch")
	if err != nil {
		t.Fatalf("second run: %v %s", err, out2)
	}
	if !strings.Contains(out2, "nothing to publish") {
		t.Fatalf("expected 'nothing to publish' on no-op run, got: %s", out2)
	}
}
