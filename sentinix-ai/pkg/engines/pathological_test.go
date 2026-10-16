package engines

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPathologicalInputs_ContainedAndGapped(t *testing.T) {
	root := t.TempDir()

	// 1. External secret file outside root
	outsideDir := t.TempDir()
	outsideSecret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideSecret, []byte("AKIAIOSFODNN7EXAMPLE"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Symlink pointing outside checkout
	escapeLink := filepath.Join(root, "escape_link.txt")
	if err := os.Symlink(outsideSecret, escapeLink); err != nil {
		t.Fatal(err)
	}

	// 2. Directory loop (symlink to self or parent)
	loopLink := filepath.Join(root, "dir_loop")
	if err := os.Symlink(root, loopLink); err != nil {
		t.Fatal(err)
	}

	// 3. 2 GiB sparse file
	sparseFile := filepath.Join(root, "sparse.bin")
	f, err := os.Create(sparseFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(2 << 30); err != nil { // 2 GiB
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// 4. Pathological input
	pathoFile := filepath.Join(root, "patho.txt")
	pathoData := strings.Repeat("A", 200000) + "!"
	if err := os.WriteFile(pathoFile, []byte(pathoData), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var skipped []string
	secEngine := Secrets{SkippedFiles: &skipped}
	findings, err := secEngine.Scan(ctx, root)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// External secret must not be read or found
	for _, f := range findings {
		if strings.Contains(f.Evidence, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(f.File, "secret.txt") {
			t.Errorf("scanned external file through symlink: %+v", f)
		}
	}

	// Symlink and sparse file must be recorded in skipped
	hasEscapeSkipped := false
	hasSparseSkipped := false
	for _, sk := range skipped {
		if strings.Contains(sk, "escape_link.txt") {
			hasEscapeSkipped = true
		}
		if strings.Contains(sk, "sparse.bin") {
			hasSparseSkipped = true
		}
	}
	if !hasEscapeSkipped {
		t.Errorf("symlink escape was not recorded in skipped files: %v", skipped)
	}
	if !hasSparseSkipped {
		t.Errorf("sparse 2GiB file was not recorded in skipped files: %v", skipped)
	}
}

func TestExternalEngines_DoNotFollowSymlinksOutOfRoot(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outsideSecret := filepath.Join(outsideDir, "outside_key.txt")
	// Write a valid AWS key in outside file
	if err := os.WriteFile(outsideSecret, []byte("AKIAIOSFODNN7EXAMPLE\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Symlink pointing outside checkout
	escapeLink := filepath.Join(root, "leak_symlink.txt")
	if err := os.Symlink(outsideSecret, escapeLink); err != nil {
		t.Fatal(err)
	}

	// In-checkout regular file with clean text
	if err := os.WriteFile(filepath.Join(root, "clean.txt"), []byte("hello safe world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, bin := range []string{"gitleaks", "semgrep"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Logf("%s not installed on path, skipping", bin)
			continue
		}
		var ext External
		if bin == "gitleaks" {
			ext = External{Bin: "gitleaks", Args: []string{"detect", "--no-git", "--report-format", "sarif", "--report-path", "-", "--no-banner", "-s"}}
		} else {
			cfg, ok := resolveSemgrepConfig()
			if !ok {
				t.Logf("no semgrep config, skipping")
				continue
			}
			ext = External{Bin: "semgrep", Args: []string{"scan", "--config", cfg, "--sarif", "--quiet", "--no-git-ignore"}}
		}

		findings, err := ext.Scan(ctx, root)
		if err != nil && !errors.Is(err, ErrEngineUnavailable) {
			t.Fatalf("%s scan error: %v", bin, err)
		}
		for _, f := range findings {
			if strings.Contains(f.Evidence, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(f.File, "outside_key.txt") {
				t.Errorf("%s followed symlink outside root and reported secret: %+v", bin, f)
			}
		}
	}
}
