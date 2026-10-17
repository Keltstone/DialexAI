package engines

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyTreeForScan_FailsClosedOnUnreadableFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	unreadable := filepath.Join(src, "secret.key")
	if err := os.WriteFile(unreadable, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Skip("cannot chmod 0000 on this filesystem")
	}
	defer os.Chmod(unreadable, 0o600)

	err := copyTreeForScan(src, dst)
	if err == nil {
		t.Fatal("expected copyTreeForScan to fail closed on unreadable file, but got nil")
	}
}

func TestExternal_EmptySemgrepignoreAndNoDefaultIgnore(t *testing.T) {
	// 1. Verify semgrep external engine includes --no-default-ignore
	var semgrepArgs []string
	for _, e := range Bundled() {
		if e.Name() == "semgrep" {
			if ext, ok := e.(External); ok {
				semgrepArgs = ext.Args
			}
		}
	}
	argsStr := strings.Join(semgrepArgs, " ")
	if !strings.Contains(argsStr, "--no-default-ignore") {
		t.Fatalf("expected semgrep args to contain --no-default-ignore, got %v", semgrepArgs)
	}

	// 2. Verify an empty .semgrepignore is written to the isolated copy
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "file.go"), []byte("package main"), 0o644)

	var capturedIsolatedDir string
	captureExt := External{
		Bin: "sh",
		Args: []string{"-c", `
			if [ ! -f "$1/.semgrepignore" ]; then
				echo 'missing .semgrepignore'
				exit 1
			fi
			if [ -s "$1/.semgrepignore" ]; then
				echo '.semgrepignore is not empty'
				exit 1
			fi
			echo '{"runs":[]}'
		`, "--"},
	}
	fs, err := captureExt.ScanContext(context.Background(), d)
	if err != nil {
		t.Fatalf("ScanContext failed: %v", err)
	}
	_ = fs
	_ = capturedIsolatedDir
}

func TestExternal_SemgrepScansTestsBuildVendorDist(t *testing.T) {
	// If semgrep is not on PATH, skip
	if _, err := os.Stat("/opt/homebrew/bin/semgrep"); err != nil {
		t.Skip("semgrep not installed")
	}

	d := t.TempDir()
	ruleContent := `
rules:
  - id: test-hardcoded-secret
    pattern: "HARDCODED_SECRET_TEST"
    message: "found secret"
    languages: [generic]
    severity: ERROR
`
	ruleFile := filepath.Join(d, "rules.yaml")
	if err := os.WriteFile(ruleFile, []byte(ruleContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create files in tests/, build/, vendor/, dist/
	paths := []string{
		"tests/test.txt",
		"build/out.txt",
		"vendor/dep.txt",
		"dist/bundle.txt",
	}
	for _, p := range paths {
		fp := filepath.Join(d, p)
		os.MkdirAll(filepath.Dir(fp), 0o755)
		os.WriteFile(fp, []byte("HARDCODED_SECRET_TEST = 1\n"), 0o644)
	}

	ext := External{
		Bin: "semgrep",
		Args: []string{
			"scan", "--config", ruleFile, "--sarif", "--quiet",
			"--no-git-ignore", "--no-default-ignore", "--disable-nosem",
		},
	}
	fs, err := ext.ScanContext(context.Background(), d)
	if err != nil {
		t.Fatalf("semgrep scan failed: %v", err)
	}

	foundPaths := map[string]bool{}
	for _, f := range fs {
		foundPaths[filepath.ToSlash(f.File)] = true
	}

	for _, p := range paths {
		if !foundPaths[p] {
			t.Errorf("expected finding in %s, but semgrep skipped it (found=%v)", p, foundPaths)
		}
	}
}
