package fsx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "f.md")
	if err := WriteAtomic(p, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, 0o640)
	if err := WriteAtomic(p, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Fatalf("content %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v, want existing 0640 kept", fi.Mode().Perm())
	}
	if es, _ := os.ReadDir(d); len(es) != 1 {
		t.Fatalf("temp files left behind: %v", es)
	}
}

func TestWriteAtomicFailureKeepsOriginal(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "f.md")
	os.WriteFile(p, []byte("keep"), 0o644)
	os.Chmod(d, 0o500) // temp file cannot be created
	defer os.Chmod(d, 0o700)
	if os.Geteuid() == 0 {
		t.Skip("root ignores dir perms")
	}
	if err := WriteAtomic(p, []byte("new"), 0o644); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(p); string(b) != "keep" {
		t.Fatalf("original damaged: %q", b)
	}
}
