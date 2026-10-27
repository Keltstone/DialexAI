package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"scribex/pkg/anchors"
)

func TestCheck_ReadOnlyRefused(t *testing.T) {
	root := t.TempDir()

	// Code file with a symbol
	code := "package main\n\nfunc MyFunc() {}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	// doc1: needs update, but is read-only (0444)
	doc1Content := "# Doc 1\n<!-- @scribex:anchor(main.go#MyFunc) -->\nold-content\n<!-- /@scribex:anchor -->\n"
	doc1Path := filepath.Join(root, "doc1.md")
	if err := os.WriteFile(doc1Path, []byte(doc1Content), 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(doc1Path, 0o644) // cleanup

	// doc2: needs update, normal permissions (0644)
	doc2Content := "# Doc 2\n<!-- @scribex:anchor(main.go#MyFunc) -->\nold-content\n<!-- /@scribex:anchor -->\n"
	doc2Path := filepath.Join(root, "doc2.md")
	if err := os.WriteFile(doc2Path, []byte(doc2Content), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := Check(root, true, nil)
	if err != nil {
		t.Fatalf("Check unexpected error: %v", err)
	}

	// 1. Returns issue with Code "write-refused"
	var foundRefused bool
	for _, issue := range report.Issues {
		if issue.Issue.Code == anchors.CodeWriteRefused {
			foundRefused = true
			if !strings.Contains(issue.File, "doc1.md") {
				t.Errorf("expected write-refused for doc1.md, got %s", issue.File)
			}
		}
	}
	if !foundRefused {
		t.Fatalf("expected Code %q in issues, got: %+v", anchors.CodeWriteRefused, report.Issues)
	}

	// 2. doc1 file content is untouched
	afterDoc1, err := os.ReadFile(doc1Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterDoc1) != doc1Content {
		t.Fatalf("read-only doc1 was modified:\nwant: %q\ngot:  %q", doc1Content, string(afterDoc1))
	}

	// 3. remaining docs still processed (doc2 updated)
	afterDoc2, err := os.ReadFile(doc2Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterDoc2) == doc2Content {
		t.Fatal("doc2.md should have been updated but remained unchanged")
	}

	// 4. Changed contains doc2.md but not doc1.md
	if len(report.Changed) != 1 || report.Changed[0] != "doc2.md" {
		t.Fatalf("expected Changed=[doc2.md], got: %v", report.Changed)
	}
}

func TestCheck_AtomicIntegrityAcrossDocs(t *testing.T) {
	root := t.TempDir()

	code := "package main\n\nfunc Hello() {}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create 50 docs
	for i := 0; i < 50; i++ {
		doc := fmt.Sprintf("# Doc %d\n<!-- @scribex:anchor(main.go#Hello) -->\nold\n<!-- /@scribex:anchor -->\n", i)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("doc_%02d.md", i)), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := Check(root, true, nil)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(report.Changed) != 50 {
		t.Fatalf("expected 50 changed docs, got %d", len(report.Changed))
	}

	// Check every doc is valid UTF-8 and no orphaned .tmp files exist
	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if strings.Contains(filepath.Base(p), ".tmp.") {
				t.Errorf("found orphaned tmp file: %s", p)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if !utf8.Valid(b) {
				t.Errorf("file %s is not valid UTF-8", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
