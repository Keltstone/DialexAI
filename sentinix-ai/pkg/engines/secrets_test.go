package engines

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecrets(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a.kt"), []byte("val k = \"AKIA"+"ABCDEFGHIJKLMNOP\"\nval ok = 1\n"), 0o644)
	fs, err := Secrets{}.Scan(t.Context(), d)
	if err != nil || len(fs) != 1 || fs[0].Line != 1 || fs[0].Evidence != "AKIA****" {
		t.Fatalf("got %+v %v", fs, err)
	}
}

func TestTestFixturesAreHighSeverity(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "commonTest"), 0o755)
	os.WriteFile(filepath.Join(d, "commonTest", "A.kt"), []byte("val apiKey = \"abcdefgh12345678abcd\"\n"), 0o644)
	fs, _ := Secrets{}.Scan(t.Context(), d)
	if len(fs) != 1 || fs[0].Severity != High {
		t.Fatalf("%+v", fs)
	}
}

func TestPlaceholdersNotSkipped(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a.go"), []byte("a := \"token = \\\"fake-access-token-12345678\\\"\"\nb := Cfg{Token: \"fake-access-token-12345678\"}\nk = \"AKIA"+"ABCDEFGHIJKLMNOP\"\n"), 0o644)
	fs, _ := Secrets{}.Scan(t.Context(), d)
	if len(fs) < 2 {
		t.Fatalf("expected placeholders to be detected, got %+v", fs)
	}
}
