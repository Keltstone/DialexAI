package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctypesCmd(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"list"}, {"validate"}, {"show", "requirements"}} {
		if err := doctypesCmd(root, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if err := doctypesCmd(root, []string{"show", "nope"}); err == nil {
		t.Error("unknown id accepted")
	}
	if err := doctypesCmd(root, nil); err == nil {
		t.Error("no subcommand accepted")
	}
	if ents, _ := os.ReadDir(root); len(ents) != 0 {
		t.Errorf("read-only commands wrote files: %v", ents)
	}
	if err := os.MkdirAll(filepath.Join(root, ".scribex"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := `{"types":[{"id":"requirements","enabled":true,"output":"../x.md"},{"id":"zz"}]}`
	if err := os.WriteFile(filepath.Join(root, ".scribex", "doctypes.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	err := doctypesCmd(root, []string{"validate"})
	if err == nil {
		t.Fatal("bad file validated")
	}
	for _, want := range []string{"escapes root", "title is required", "features map"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing problem %q in:\n%v", want, err)
		}
	}
}
