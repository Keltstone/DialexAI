package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
