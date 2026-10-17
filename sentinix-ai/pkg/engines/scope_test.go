package engines

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScopedCopyAndDependencyFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for _, f := range []string{"a/changed.kt", "a/other.kt", "app/build.gradle.kts", "gradle/libs.versions.toml", "requirements-dev.txt"} {
		os.MkdirAll(filepath.Dir(filepath.Join(src, f)), 0o755)
		os.WriteFile(filepath.Join(src, f), []byte("x"), 0o644)
	}
	scope := map[string]bool{"a/changed.kt": true}
	if err := copyTreeForScan(src, dst, func(r string) bool { return scope[r] }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a/changed.kt")); err != nil {
		t.Fatal("changed file missing from scoped copy")
	}
	if _, err := os.Stat(filepath.Join(dst, "a/other.kt")); err == nil {
		t.Fatal("unchanged file leaked into scoped copy")
	}
	for _, f := range []string{"app/build.gradle.kts", "gradle/libs.versions.toml", "requirements-dev.txt"} {
		if !isDependencyFile(f) {
			t.Fatalf("%s must be kept for osv-scanner", f)
		}
	}
	if isDependencyFile("a/other.kt") {
		t.Fatal("source file treated as dependency file")
	}
}
