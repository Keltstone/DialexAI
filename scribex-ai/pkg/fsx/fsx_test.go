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

func TestWriteFileIn(t *testing.T) {
	root := t.TempDir()

	// 1. new file
	if err := WriteFileIn(root, "sub/new.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFileIn new file: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "sub/new.txt"))
	if err != nil || string(content) != "hello" {
		t.Fatalf("content = %q, err = %v", content, err)
	}

	// 2. overwrite keeps mode
	if err := os.Chmod(filepath.Join(root, "sub/new.txt"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIn(root, "sub/new.txt", []byte("updated"), 0o600); err != nil {
		t.Fatalf("WriteFileIn overwrite: %v", err)
	}
	fi, err := os.Stat(filepath.Join(root, "sub/new.txt"))
	if err != nil || fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", fi.Mode().Perm())
	}
	content, _ = os.ReadFile(filepath.Join(root, "sub/new.txt"))
	if string(content) != "updated" {
		t.Fatalf("updated content = %q", content)
	}

	// 3. symlink target refused (target file unchanged, link still a link)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "link_to_outside.txt")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIn(root, "link_to_outside.txt", []byte("hacked"), 0o644); err == nil {
		t.Fatalf("expected error for symlink target, got nil")
	}
	if b, _ := os.ReadFile(outside); string(b) != "outside-orig" {
		t.Fatalf("outside file was modified: %q", b)
	}
	lfi, err := os.Lstat(linkPath)
	if err != nil || lfi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: %v, mode=%v", err, lfi.Mode())
	}

	// 4. dangling symlink refused
	dangling := filepath.Join(root, "dangling.txt")
	if err := os.Symlink(filepath.Join(root, "nonexistent.txt"), dangling); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIn(root, "dangling.txt", []byte("dangle"), 0o644); err == nil {
		t.Fatalf("expected error for dangling symlink target, got nil")
	}

	// 5. read-only (0444) refused, unchanged
	ro := filepath.Join(root, "readonly.txt")
	if err := os.WriteFile(ro, []byte("ro-orig"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIn(root, "readonly.txt", []byte("ro-mod"), 0o644); err == nil {
		t.Fatalf("expected error for read-only target, got nil")
	}
	if b, _ := os.ReadFile(ro); string(b) != "ro-orig" {
		t.Fatalf("read-only file was modified: %q", b)
	}

	// 6. directory at target refused
	dirTarget := filepath.Join(root, "some_dir")
	if err := os.Mkdir(dirTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIn(root, "some_dir", []byte("dir-data"), 0o644); err == nil {
		t.Fatalf("expected error when target is a directory, got nil")
	}

	// 7. .. and absolute refused
	if err := WriteFileIn(root, "../escaped.txt", []byte("esc"), 0o644); err == nil {
		t.Fatalf("expected error for .., got nil")
	}
	if err := WriteFileIn(root, "/abs/path.txt", []byte("abs"), 0o644); err == nil {
		t.Fatalf("expected error for absolute path, got nil")
	}

	// 8. parent symlink pointing outside refused (via safepath)
	outsideDir := t.TempDir()
	parentLink := filepath.Join(root, "sym_parent")
	if err := os.Symlink(outsideDir, parentLink); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileIn(root, "sym_parent/escaped.txt", []byte("esc"), 0o644); err == nil {
		t.Fatalf("expected error for parent symlink pointing outside, got nil")
	}

	// 9. no leftover .tmp files in success and failure paths
	var leftover []string
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && filepath.Ext(p) == ".tmp" {
			leftover = append(leftover, p)
		}
		return nil
	})
	if len(leftover) > 0 {
		t.Fatalf("found leftover temp files: %v", leftover)
	}
}

func TestWriteFileInConcurrent(t *testing.T) {
	root := t.TempDir()
	rel := "concurrent.txt"
	const writers = 20
	const lineLen = 1000

	done := make(chan []byte, writers)
	for i := 0; i < writers; i++ {
		char := byte('A' + i)
		go func(c byte) {
			data := make([]byte, lineLen)
			for j := range data {
				data[j] = c
			}
			err := WriteFileIn(root, rel, data, 0o644)
			if err != nil {
				t.Errorf("concurrent writer error: %v", err)
			}
			done <- data
		}(char)
	}

	for i := 0; i < writers; i++ {
		<-done
	}

	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read concurrent file: %v", err)
	}
	if len(content) != lineLen {
		t.Fatalf("content len %d, want %d", len(content), lineLen)
	}
	first := content[0]
	for idx, b := range content {
		if b != first {
			t.Fatalf("interleaved bytes at pos %d: %c != %c", idx, b, first)
		}
	}
}

func TestWriteAtomicRenameFailureCleansTemp(t *testing.T) {
	d := t.TempDir()
	targetDir := filepath.Join(d, "nonempty_dir")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "keep.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Calling WriteAtomic on a path that is a non-empty directory should fail during rename
	err := WriteAtomic(targetDir, []byte("fail"), 0o644)
	if err == nil {
		t.Fatalf("expected rename failure when target is a non-empty directory")
	}

	entries, err := os.ReadDir(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "nonempty_dir" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
