package wiki

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_TitleValidation(t *testing.T) {
	root := t.TempDir()
	relDir := "docs/wiki"

	invalidTitles := []string{
		"../../evil",
		"a/b",
		"x\ny",
		": x",
		strings.Repeat("a", 121),
		"",
	}
	for _, title := range invalidTitles {
		_, err := New(root, relDir, title)
		if err == nil {
			t.Fatalf("expected title %q to be rejected", title)
		}
	}

	// Valid title with YAML special characters (#, ---, quotes)
	specTitle := "Notes #1 --- intro"
	path, err := New(root, relDir, specTitle)
	if err != nil {
		t.Fatalf("New failed for title with special chars: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	expectedFM := fmt.Sprintf("---\ntitle: %q\ntags: []\n---\n# %s\n\n", specTitle, specTitle)
	if content != expectedFM {
		t.Fatalf("front matter mismatch:\ngot:\n%s\nwant:\n%s", content, expectedFM)
	}

	// Title "---" also tests that YAML boundary delimiter cannot be injected
	dashPath, err := New(root, relDir, "---")
	if err != nil {
		t.Fatalf("New failed for title '---': %v", err)
	}
	bDash, _ := os.ReadFile(dashPath)
	if !strings.Contains(string(bDash), `title: "---"`) {
		t.Fatalf("expected quoted title for '---', got:\n%s", string(bDash))
	}
	w, err := Load(filepath.Join(root, relDir))
	if err != nil || w.Pages["---"] == nil || w.Pages["---"].Title != "---" {
		t.Fatalf("failed to load page with title '---': err=%v, page=%v", err, w.Pages["---"])
	}

	// Refuses to overwrite existing file
	_, err = New(root, relDir, specTitle)
	if err == nil {
		t.Fatalf("expected New to refuse overwriting existing page")
	}
}

func TestLoad_SymlinkSkippedAndLimits(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	// 1. Regular page
	if err := os.WriteFile(filepath.Join(dir, "Home.md"), []byte("# Home\nWelcome [[Getting Started]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Symlinked file and symlinked dir to outside
	outsideFile := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(outsideFile, []byte("# Secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(outsideFile, filepath.Join(dir, "sym_file.md"))

	outsideDir := filepath.Join(outside, "subdir")
	_ = os.Mkdir(outsideDir, 0o755)
	_ = os.WriteFile(filepath.Join(outsideDir, "subpage.md"), []byte("# Sub"), 0o644)
	_ = os.Symlink(outsideDir, filepath.Join(dir, "sym_dir"))

	w, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if w.Pages["secret"] != nil || w.Pages["subpage"] != nil {
		t.Fatalf("Load should have skipped symlinked files/dirs")
	}
	if w.Pages["home"] == nil {
		t.Fatalf("Home page not loaded")
	}

	// 3. Oversized file (>2 MiB)
	bigFile := filepath.Join(dir, "big.md")
	bigData := make([]byte, (2<<20)+1024)
	if err := os.WriteFile(bigFile, bigData, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Load(dir)
	if err == nil {
		t.Fatalf("expected error for file > 2 MiB")
	}
	_ = os.Remove(bigFile)

	// 4. Front matter parser does not hang on 1 MB single-line scalar (5s guard)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan bool)
	go func() {
		scalar := "---\ntitle: " + strings.Repeat("A", 1<<20) + "\n---\n# Large\n"
		p := parsePage("large.md", scalar)
		if p == nil {
			t.Errorf("parsePage returned nil")
		}
		done <- true
	}()

	select {
	case <-ctx.Done():
		t.Fatalf("front matter parser hung on 1 MB scalar")
	case <-done:
	}
}

func TestLoad_LargeTreePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 50k page tree performance test in -short mode")
	}
	dir := t.TempDir()
	const pageCount = 50000

	// Generate 50,000 small files
	pageData := []byte("# Page\nContent")
	for i := 0; i < pageCount; i++ {
		filename := filepath.Join(dir, fmt.Sprintf("page_%05d.md", i))
		if err := os.WriteFile(filename, pageData, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	w, err := Load(dir)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(w.Pages) != pageCount {
		t.Fatalf("loaded %d pages, want %d", len(w.Pages), pageCount)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("50,000 page tree took %v, want < 5s", elapsed)
	}
}

func TestRender_Safety(t *testing.T) {
	wikiDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wikiDir, "Home.md"), []byte("# Home\nLinks: [[Guide]] [[../../etc/passwd]] [[/abs]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wikiDir, "Guide.md"), []byte("# Guide\nBacklink to [[Home]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := Load(wikiDir)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Refuse render to root /
	if err := w.Render("/", "Home"); err == nil {
		t.Fatalf("expected error when rendering to /")
	}

	// 2. Refuse render inside wiki source dir
	inside := filepath.Join(wikiDir, "rendered")
	if err := w.Render(inside, "Home"); err == nil {
		t.Fatalf("expected error when rendering inside wiki source dir")
	}

	// 3. Refuse render to repository root (containing .git or .scribex)
	repoRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.Render(repoRoot, "Home"); err == nil {
		t.Fatalf("expected error when rendering to repository root (.git)")
	}

	// 4. render -out into dir with symlinked subdir and symlinked Home.md
	outDir := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "trap_home.md")
	if err := os.WriteFile(outsideFile, []byte("trap-orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(outDir, "Home.md")); err != nil {
		t.Fatal(err)
	}

	outsideSubdir := filepath.Join(outside, "sub")
	if err := os.Mkdir(outsideSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSubdir, filepath.Join(outDir, "sub_link")); err != nil {
		t.Fatal(err)
	}

	err = w.Render(outDir, "Home")
	if err == nil {
		t.Fatalf("expected render to fail when out has symlinks")
	}
	b, _ := os.ReadFile(outsideFile)
	if string(b) != "trap-orig" {
		t.Fatalf("outside file was modified: %q", b)
	}

	// 5. Successful render to a clean dir
	cleanOut := t.TempDir()
	if err := w.Render(cleanOut, "Home"); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	homeRendered, err := os.ReadFile(filepath.Join(cleanOut, "Home.md"))
	if err != nil {
		t.Fatal(err)
	}
	// [[Guide]] becomes [Guide](Guide.md)
	if !strings.Contains(string(homeRendered), "[Guide](Guide.md)") {
		t.Fatalf("wikilink not converted:\n%s", string(homeRendered))
	}
	// [[../../etc/passwd]] and [[/abs]] remain unknown (unchanged [[...]]) without filesystem access
	if !strings.Contains(string(homeRendered), "[[../../etc/passwd]]") || !strings.Contains(string(homeRendered), "[[/abs]]") {
		t.Fatalf("unknown links corrupted:\n%s", string(homeRendered))
	}
}

func TestWiki_LintAndSearch(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Home.md"), []byte("# Home\n[[Alpha]]"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Alpha.md"), []byte("# Alpha\n[[Broken]]"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Orphan.md"), []byte("# Orphan\nLonely"), 0o644)

	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	lint := w.Lint("Home")
	var foundBroken, foundOrphan bool
	for _, l := range lint {
		if strings.Contains(l, "broken link") {
			foundBroken = true
		}
		if strings.Contains(l, "orphan page") {
			foundOrphan = true
		}
	}
	if !foundBroken || !foundOrphan {
		t.Fatalf("Lint issues missing (broken=%v, orphan=%v): %v", foundBroken, foundOrphan, lint)
	}

	hits := w.Search("alpha")
	if len(hits) == 0 {
		t.Fatalf("Search failed to find Alpha")
	}
}
