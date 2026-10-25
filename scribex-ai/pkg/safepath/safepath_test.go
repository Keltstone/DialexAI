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
