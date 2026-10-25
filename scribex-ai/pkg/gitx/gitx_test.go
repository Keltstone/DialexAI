package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "a@b.c"}, {"config", "user.name", "t"}} {
		if out, err := exec.Command("git", append([]string{"-C", d}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	os.WriteFile(filepath.Join(d, "f"), []byte("x"), 0o644)
	exec.Command("git", "-C", d, "add", "-A").Run()
	if out, err := exec.Command("git", "-C", d, "commit", "-qm", "c").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return d
}

func TestValidRef(t *testing.T) {
	for _, r := range []string{"", "-x", "--output=/tmp/x", "a b", "a\nb", "a\x00b", strings.Repeat("a", 300)} {
		if ValidRef(r) == nil {
			t.Errorf("ValidRef(%q) accepted", r)
		}
	}
	for _, r := range []string{"main", "origin/main", "HEAD~1", "v1.0.0", "a;b"} { // a;b is harmless: no shell, resolved by rev-parse
		if err := ValidRef(r); err != nil {
			t.Errorf("ValidRef(%q): %v", r, err)
		}
	}
}

func TestResolveCommit(t *testing.T) {
	d := repo(t)
	sha, err := ResolveCommit(d, "HEAD")
	if err != nil || len(sha) != 40 {
		t.Fatalf("sha=%q err=%v", sha, err)
	}
	for _, bad := range []string{"nosuch", "--output=" + filepath.Join(d, "pwn"), "a;b"} {
		if _, err := ResolveCommit(d, bad); err == nil {
			t.Errorf("ResolveCommit(%q) succeeded", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(d, "pwn")); err == nil {
		t.Fatal("option injection created a file")
	}
}

func TestHostileConfigNotExecuted(t *testing.T) {
	d := repo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	hook := filepath.Join(t.TempDir(), "fsm.sh")
	os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755)
	exec.Command("git", "-C", d, "config", "core.fsmonitor", hook).Run()
	os.WriteFile(filepath.Join(d, "g"), []byte("y"), 0o644)
	if _, err := Run(d, "status", "--porcelain"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("repo-controlled core.fsmonitor executed through gitx.Run")
	}
}

func TestRunErrorsAreErrors(t *testing.T) {
	if _, err := Run(t.TempDir(), "rev-parse", "HEAD"); err == nil {
		t.Fatal("non-repo must error")
	}
}

func TestIsShallow(t *testing.T) {
	d := repo(t)
	if s, err := IsShallow(d); err != nil || s {
		t.Fatalf("shallow=%v err=%v", s, err)
	}
	clone := filepath.Join(t.TempDir(), "c")
	if out, err := exec.Command("git", "clone", "-q", "--depth", "1", "file://"+d, clone).CombinedOutput(); err != nil {
		t.Skipf("shallow clone unavailable: %s", out)
	}
	if s, err := IsShallow(clone); err != nil || !s {
		t.Fatalf("shallow=%v err=%v, want true", s, err)
	}
}
