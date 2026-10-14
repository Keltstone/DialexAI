package engines

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecrets(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a.kt"), []byte("val k = \"AKIA"+"ABCDEFGHIJKLMNOP\"\nval ok = 1\n"), 0o644)
	fs, err := Secrets{}.Scan(d)
	if err != nil || len(fs) != 1 || fs[0].Line != 1 || fs[0].Evidence != "AKIA****" {
		t.Fatalf("got %+v %v", fs, err)
	}
}
