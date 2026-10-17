package engines

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

	err := copyTreeForScan(src, dst, nil)
	if err == nil {
		t.Fatal("expected copyTreeForScan to fail closed on unreadable file, but got nil")
	}
}

func TestExternal_EmptySemgrepignoreInIsolatedCopy(t *testing.T) {
	// Verify an empty .semgrepignore is written to the isolated copy
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "file.go"), []byte("package main"), 0o644)

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
}

func TestExternal_SemgrepScansTestsBuildVendorDist(t *testing.T) {
	// If semgrep is not on PATH, skip
	if _, err := exec.LookPath("semgrep"); err != nil {
		t.Skip("semgrep not installed")
	}

	d := t.TempDir()
	ruleContent := `
rules:
  - id: test-hardcoded-secret
    pattern: "HARDCODED_SECRET_TEST"
    message: "found secret"
    languages: [python]
    severity: ERROR
`
	ruleFile := filepath.Join(d, "rules.yaml")
	if err := os.WriteFile(ruleFile, []byte(ruleContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create files in tests/, build/, vendor/, dist/
	paths := []string{
		"tests/test.py",
		"build/out.py",
		"vendor/dep.py",
		"dist/bundle.py",
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
			"--no-git-ignore", "--disable-nosem",
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
		count := 0
		if foundPaths[p] {
			count = 1
		}
		t.Logf("%-22s semgrep=%d", p, count)
		if count != 1 {
			t.Errorf("expected finding in %s, but semgrep skipped it (found=%v)", p, foundPaths)
		}
	}
}
