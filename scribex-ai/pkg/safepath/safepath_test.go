package safepath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoin(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(root, "a"), 0o755)
	os.WriteFile(filepath.Join(outside, "x.go"), nil, 0o644)
	os.Symlink(filepath.Join(outside, "x.go"), filepath.Join(root, "link.go"))
	os.Symlink(outside, filepath.Join(root, "dir"))
	os.Symlink(filepath.Join(root, "loop"), filepath.Join(root, "loop"))

	bad := []string{
		"", "../x", "a/../../x", `..\x`, `a\..\..\x`, "/etc/passwd", `\etc`, "file:///etc/passwd",
		"a\x00b", "link.go", "dir/x.go", "dir/new.go", "loop", strings.Repeat("a/", 3000),
	}
	for _, rel := range bad {
		if got, err := Join(root, rel); err == nil {
			t.Errorf("Join(%q) = %q, want error", rel, got)
		}
	}
	good := []string{"a", "a/b.go", "a/new/deep/file.md", "./a/../a/c.go"}
	for _, rel := range good[:3] {
		if _, err := Join(root, rel); err != nil {
			t.Errorf("Join(%q): %v", rel, err)
		}
	}
	// ".." anywhere is rejected even when it would stay inside root: simpler to audit.
	if _, err := Join(root, good[3]); err == nil {
		t.Errorf("Join(%q) accepted a .. component", good[3])
	}
}

func TestValidPageID(t *testing.T) {
	for s, want := range map[string]bool{"123": true, "": false, "../1": false, "1;2": false, "1 2": false, strings.Repeat("9", 21): false, "０１": false} {
		if ValidPageID(s) != want {
			t.Errorf("ValidPageID(%q) != %v", s, want)
		}
	}
}

func TestCheckRel(t *testing.T) {
	bad := []string{
		"", "../x", "a/../../x", `..\x`, `a\..\..\x`, "/etc/passwd", `\etc`, "file:///etc/passwd",
		"a\x00b", strings.Repeat("a/", 3000),
	}
	for _, rel := range bad {
		if err := CheckRel(rel); err == nil {
			t.Errorf("CheckRel(%q) succeeded, want error", rel)
		}
	}
	good := []string{"a", "a/b.go", "a/new/deep/file.md"}
	for _, rel := range good {
		if err := CheckRel(rel); err != nil {
			t.Errorf("CheckRel(%q): %v", rel, err)
		}
	}
}

func TestScribexDir(t *testing.T) {
	// 1. Missing (create=false)
	t.Run("missing_no_create", func(t *testing.T) {
		root := t.TempDir()
		_, err := ScribexDir(root, false)
		if !os.IsNotExist(err) {
			t.Fatalf("expected os.ErrNotExist, got: %v", err)
		}
	})

	// 2. Missing (create=true)
	t.Run("missing_create", func(t *testing.T) {
		root := t.TempDir()
		dir, err := ScribexDir(root, true)
		if err != nil {
			t.Fatalf("ScribexDir create=true failed: %v", err)
		}
		fi, err := os.Stat(dir)
		if err != nil || !fi.IsDir() {
			t.Fatalf("expected created directory at %s", dir)
		}
	})

	// 3. Real dir
	t.Run("real_dir", func(t *testing.T) {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, ".scribex"), 0o755)
		dir, err := ScribexDir(root, false)
		if err != nil {
			t.Fatalf("ScribexDir on real dir failed: %v", err)
		}
		if !filepath.IsAbs(dir) {
			t.Fatalf("expected absolute path, got: %s", dir)
		}
	})

	// 4. File named .scribex
	t.Run("file_named_scribex", func(t *testing.T) {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, ".scribex"), []byte("file"), 0o644)
		_, err := ScribexDir(root, false)
		if err != ErrScribexDir {
			t.Fatalf("expected ErrScribexDir, got: %v", err)
		}
	})

	// 5. Symlinks (skip on Windows if not supported)
	t.Run("symlinks", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()

		// Symlink to dir outside
		outsideDir := filepath.Join(outside, "target")
		os.MkdirAll(outsideDir, 0o755)
		os.Symlink(outsideDir, filepath.Join(root, ".scribex"))
		if _, err := ScribexDir(root, false); err != ErrScribexDir {
			t.Fatalf("symlink to outside: expected ErrScribexDir, got: %v", err)
		}
		os.Remove(filepath.Join(root, ".scribex"))

		// Symlink to dir inside repo
		insideDir := filepath.Join(root, "real_scribex")
		os.MkdirAll(insideDir, 0o755)
		os.Symlink(insideDir, filepath.Join(root, ".scribex"))
		if _, err := ScribexDir(root, false); err != ErrScribexDir {
			t.Fatalf("symlink to inside: expected ErrScribexDir, got: %v", err)
		}
		os.Remove(filepath.Join(root, ".scribex"))

		// Symlink loop
		os.Symlink(filepath.Join(root, ".scribex"), filepath.Join(root, ".scribex"))
		if _, err := ScribexDir(root, false); err != ErrScribexDir {
			t.Fatalf("symlink loop: expected ErrScribexDir, got: %v", err)
		}
		os.Remove(filepath.Join(root, ".scribex"))
	})

	// 6. Root is a symlink
	t.Run("root_is_symlink", func(t *testing.T) {
		realRoot := t.TempDir()
		os.MkdirAll(filepath.Join(realRoot, ".scribex"), 0o755)

		symRoot := filepath.Join(t.TempDir(), "repo-symlink")
		if err := os.Symlink(realRoot, symRoot); err != nil {
			t.Skip("symlinks not supported")
		}

		dir, err := ScribexDir(symRoot, false)
		if err != nil {
			t.Fatalf("ScribexDir on symlinked root failed: %v", err)
		}
		if !strings.HasSuffix(dir, ".scribex") {
			t.Fatalf("expected .scribex path, got: %s", dir)
		}
	})
}
