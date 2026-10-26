package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsSymlinkedOutside(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Symlink root/.scribex -> outside
	if err := os.Symlink(outside, filepath.Join(root, ".scribex")); err != nil {
		t.Skip("symlinks not supported")
	}

	err := Set(root, "TEST_TOKEN", "secret123")
	if err == nil {
		t.Fatalf("Set should fail when .scribex is symlinked outside")
	}

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside dir should remain empty, got %d entries", len(entries))
	}
}

func TestSecretsNormalFlowAndGitignore(t *testing.T) {
	root := t.TempDir()

	// Run Set three times
	if err := Set(root, "KEY_ONE", "val1"); err != nil {
		t.Fatalf("Set 1 failed: %v", err)
	}
	if err := Set(root, "KEY_TWO", "val2"); err != nil {
		t.Fatalf("Set 2 failed: %v", err)
	}
	if err := Set(root, "KEY_ONE", "val1_updated"); err != nil {
		t.Fatalf("Set 3 failed: %v", err)
	}

	// Mode 0600
	secFile := File(root)
	fi, err := os.Stat(secFile)
	if err != nil {
		t.Fatalf("Stat secrets.env failed: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("secrets.env perm = %o, want 0600", fi.Mode().Perm())
	}

	// Read & Load
	m := read(root)
	if m["KEY_ONE"] != "val1_updated" || m["KEY_TWO"] != "val2" {
		t.Fatalf("unexpected secrets content: %+v", m)
	}
	os.Unsetenv("KEY_ONE")
	Load(root)
	if os.Getenv("KEY_ONE") != "val1_updated" {
		t.Fatalf("Load did not set KEY_ONE in env")
	}

	// .gitignore has exactly one entry
	gi, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile .gitignore failed: %v", err)
	}
	count := strings.Count(string(gi), ".scribex/secrets.env")
	if count != 1 {
		t.Fatalf("expected exactly 1 entry for .scribex/secrets.env in .gitignore, got %d:\n%s", count, string(gi))
	}
}

func TestSecretsPartF_OrderingAndGitignore(t *testing.T) {
	// 1. CRLF .gitignore stays CRLF
	root1 := t.TempDir()
	giPath1 := filepath.Join(root1, ".gitignore")
	if err := os.WriteFile(giPath1, []byte("node_modules/\r\n.env\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Set(root1, "KEY_CRLF", "val"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	giContent1, _ := os.ReadFile(giPath1)
	if strings.Contains(string(giContent1), "\r\n.scribex/secrets.env\r\n") == false &&
		strings.HasSuffix(string(giContent1), ".scribex/secrets.env\r\n") == false {
		t.Fatalf(".gitignore CRLF was not preserved:\n%q", string(giContent1))
	}

	// 2. Symlinked .gitignore -> error and .scribex/secrets.env absent
	root2 := t.TempDir()
	outsideGI := filepath.Join(t.TempDir(), "outside_gi")
	if err := os.WriteFile(outsideGI, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideGI, filepath.Join(root2, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := Set(root2, "KEY_SYM", "val"); err == nil {
		t.Fatalf("expected error when .gitignore is symlinked")
	}
	secPath2 := filepath.Join(root2, ".scribex", "secrets.env")
	if _, err := os.Stat(secPath2); !os.IsNotExist(err) {
		t.Fatalf(".scribex/secrets.env should NOT be created when .gitignore fails")
	}

	// 3. Read-only .gitignore -> error, no secret stored
	root3 := t.TempDir()
	giPath3 := filepath.Join(root3, ".gitignore")
	if err := os.WriteFile(giPath3, []byte("node_modules/\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := Set(root3, "KEY_RO", "val"); err == nil {
		t.Fatalf("expected error when .gitignore is read-only")
	}
	secPath3 := filepath.Join(root3, ".scribex", "secrets.env")
	if _, err := os.Stat(secPath3); !os.IsNotExist(err) {
		t.Fatalf(".scribex/secrets.env should NOT be created when .gitignore is read-only")
	}

	// 4. Group/world-readable check in CI
	root4 := t.TempDir()
	if err := Set(root4, "KEY_PERM", "secret_val"); err != nil {
		t.Fatal(err)
	}
	secFile4 := File(root4)
	if err := os.Chmod(secFile4, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CI", "true")
	os.Unsetenv("KEY_PERM")
	Load(root4)
	if os.Getenv("KEY_PERM") != "" {
		t.Fatalf("Load should NOT load secrets when group/world-readable in CI")
	}
}
