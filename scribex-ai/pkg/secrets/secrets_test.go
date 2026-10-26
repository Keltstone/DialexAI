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
