package docs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"scribex/pkg/anchors"
)

func TestAnchorPathConfinement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	secretFile := filepath.Join(outside, "secret.go")
	secretContent := "package secret\nfunc OutsideSecret() string { return \"classified\" }\n"
	if err := os.WriteFile(secretFile, []byte(secretContent), 0o644); err != nil {
		t.Fatal(err)
	}

	symlinkSupported := runtime.GOOS != "windows"
	if symlinkSupported {
		if err := os.Symlink(secretFile, filepath.Join(root, "link.go")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "outside_dir")); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name string
		path string
		sym  bool
	}{
		{"parent", "../outside.go", false},
		{"nested_parent", "a/../../outside.go", false},
		{"absolute", filepath.Join(outside, "secret.go"), false},
		{"file_url", "file:///etc/passwd", false},
		{"nul", "a\x00b.go", false},
		{"long_path", strings.Repeat("a", 4097) + ".go", false},
		{"backslash", `..\outside.go`, false},
	}
	if symlinkSupported {
		cases = append(cases,
			struct {
				name string
				path string
				sym  bool
			}{"symlink_file", "link.go", true},
			struct {
				name string
				path string
				sym  bool
			}{"symlink_dir", "outside_dir/secret.go", true},
		)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docName := filepath.Join(root, tc.name+".md")
			docContent := fmt.Sprintf("<!-- @scribex:anchor(%s#OutsideSecret) -->\n```signature\nold\n```\nprose\n", tc.path)
			if err := os.WriteFile(docName, []byte(docContent), 0o644); err != nil {
				t.Fatal(err)
			}

			rep, err := Check(root, true, nil)
			if err != nil {
				t.Fatalf("Check failed: %v", err)
			}

			found := false
			for _, iss := range rep.Issues {
				if iss.File == docName {
					found = true
					if !strings.Contains(iss.Msg, tc.path) {
						t.Errorf("issue msg %q should mention path %q", iss.Msg, tc.path)
					}
					if iss.Stale {
						t.Errorf("stale should be false for confined path issue")
					}
					if strings.Contains(iss.Msg, "classified") {
						t.Errorf("secret content leaked in issue msg: %s", iss.Msg)
					}
				}
			}
			if !found {
				t.Fatalf("expected issue for path %s, got issues: %v", tc.path, rep.Issues)
			}

			after, err := os.ReadFile(docName)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != docContent {
				t.Fatalf("doc was modified: got %q, want %q", string(after), docContent)
			}
			if strings.Contains(string(after), "OutsideSecret") && strings.Contains(string(after), "classified") {
				t.Fatalf("secret leaked into doc")
			}
		})
	}
}

func TestDeltasConfinement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping symlink test on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()

	secretFile := filepath.Join(outside, "secret.go")
	secretContent := "package secret\nfunc TopSecretExported() string { return \"secret\" }\n"
	if err := os.WriteFile(secretFile, []byte(secretContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Init git repo
	cmds := [][]string{
		{"init"},
		{"config", "user.name", "Test"},
		{"config", "user.email", "test@example.com"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Create initial file & commit
	initFile := filepath.Join(root, "init.go")
	if err := os.WriteFile(initFile, []byte("package initpkg\nfunc Init() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c1 := exec.Command("git", "add", "init.go")
	c1.Dir = root
	if out, err := c1.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	c2 := exec.Command("git", "commit", "-m", "initial")
	c2.Dir = root
	if out, err := c2.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Create git-tracked symlink pointing outside
	evilSymlink := filepath.Join(root, "evil.go")
	if err := os.Symlink(secretFile, evilSymlink); err != nil {
		t.Fatal(err)
	}
	c3 := exec.Command("git", "add", "evil.go")
	c3.Dir = root
	if out, err := c3.CombinedOutput(); err != nil {
		t.Fatalf("git add evil.go: %v\n%s", err, out)
	}

	res, err := Deltas(root, "HEAD")
	if err != nil {
		t.Fatalf("Deltas failed: %v", err)
	}

	foundUnknown := false
	for _, u := range res.Unknown {
		if u.File == "evil.go" {
			foundUnknown = true
			if !strings.Contains(u.Reason, "safepath") {
				t.Errorf("expected safepath reason for evil.go unknown, got: %s", u.Reason)
			}
		}
	}
	if !foundUnknown {
		t.Fatalf("evil.go not found in Unknown: %+v", res.Unknown)
	}

	for _, d := range res.Deltas {
		if d.File == "evil.go" || strings.Contains(d.Sym, "TopSecret") {
			t.Fatalf("secret leaked into Deltas: %+v", d)
		}
	}
}

func TestFilesSymlinkSkip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping symlink test on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()

	// 1. Outside markdown file with a stale anchor
	outsideMD := filepath.Join(outside, "target.md")
	targetAnchor := "<!-- @scribex:anchor(target.go#Target) -->\n"
	if err := os.WriteFile(outsideMD, []byte(targetAnchor), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Symlink inside repo pointing to outsideMD
	symlinkMD := filepath.Join(root, "x.md")
	if err := os.Symlink(outsideMD, symlinkMD); err != nil {
		t.Fatal(err)
	}

	// 3. Symlinked dir with .md inside
	outsideDir := filepath.Join(outside, "docs_outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "nested.md"), []byte("# Nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(root, "sym_dir")
	if err := os.Symlink(outsideDir, symlinkDir); err != nil {
		t.Fatal(err)
	}

	// 4. Normal md file
	normalMD := filepath.Join(root, "normal.md")
	if err := os.WriteFile(normalMD, []byte("# Normal"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, skipped := FilesWithSkips(root)
	if len(files) != 1 || files[0] != normalMD {
		t.Fatalf("FilesWithSkips files = %v, want [%s]", files, normalMD)
	}

	foundX := false
	foundDir := false
	for _, s := range skipped {
		if s == symlinkMD {
			foundX = true
		}
		if s == symlinkDir {
			foundDir = true
		}
	}
	if !foundX || !foundDir {
		t.Fatalf("expected x.md and sym_dir in skipped, got: %v", skipped)
	}

	// N16 repro: run sync and verify outside target.md is byte-identical
	_, err := Check(root, true, nil)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	afterTarget, err := os.ReadFile(outsideMD)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterTarget) != targetAnchor {
		t.Fatalf("outside target.md was modified by sync: got %q, want %q", string(afterTarget), targetAnchor)
	}
}

func TestLinksConfinement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Real files
	docsDir := filepath.Join(root, "docs")
	os.MkdirAll(docsDir, 0o755)
	os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A"), 0o644)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("# A root"), 0o644)
	os.WriteFile(filepath.Join(root, "a b.md"), []byte("# A B"), 0o644)

	if runtime.GOOS != "windows" {
		os.Symlink(filepath.Join(outside, "out.md"), filepath.Join(root, "link-out.md"))
	}

	testDoc := filepath.Join(root, "links.md")
	content := `
[escape](../../etc/passwd)
[rooted](/docs/a.md)
[query](a.md?x=1#frag)
[percent](a%20b.md)
[web](https://example.com)
[cdn](//cdn.example.com/asset.js)
[mail](mailto:user@example.com)
` + "```markdown\n[broken inside](nonexistent_in_fence.md)\n```\n" + `
[pure-fragment](#heading)
[missing](missing.md)
`
	if runtime.GOOS != "windows" {
		content += "[symlink-out](link-out.md)\n"
	}

	if err := os.WriteFile(testDoc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	issues := Links(root)

	escapeFound := false
	missingFound := false
	symlinkEscapeFound := false

	for _, iss := range issues {
		if strings.Contains(iss.Ref, "../../etc/passwd") {
			if iss.Msg == "link escapes repository" {
				escapeFound = true
			} else {
				t.Errorf("expected 'link escapes repository' for etc/passwd, got: %s", iss.Msg)
			}
		}
		if strings.Contains(iss.Ref, "missing.md") {
			if iss.Msg == "broken link" {
				missingFound = true
			} else {
				t.Errorf("expected 'broken link' for missing.md, got: %s", iss.Msg)
			}
		}
		if strings.Contains(iss.Ref, "link-out.md") {
			if iss.Msg == "link escapes repository" {
				symlinkEscapeFound = true
			} else {
				t.Errorf("expected 'link escapes repository' for link-out.md, got: %s", iss.Msg)
			}
		}
		if strings.Contains(iss.Ref, "nonexistent_in_fence.md") {
			t.Errorf("link inside fence was not ignored: %v", iss)
		}
		if strings.Contains(iss.Ref, "example.com") || strings.Contains(iss.Ref, "#heading") || strings.Contains(iss.Ref, "a%20b.md") || strings.Contains(iss.Ref, "/docs/a.md") {
			t.Errorf("unexpected issue for valid/skipped link: %v", iss)
		}
	}

	if !escapeFound {
		t.Errorf("expected escape issue for ../../etc/passwd")
	}
	if !missingFound {
		t.Errorf("expected broken link issue for missing.md")
	}
	if runtime.GOOS != "windows" && !symlinkEscapeFound {
		t.Errorf("expected link escapes repository for link-out.md")
	}

	// Case mismatch test (skipped on case-insensitive filesystems)
	t.Run("case_mismatch", func(t *testing.T) {
		caseDoc := filepath.Join(root, "case.md")
		if err := os.WriteFile(caseDoc, []byte("[case](A.MD)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(caseDoc)
		// Check if filesystem is case-insensitive
		if _, err := os.Stat(filepath.Join(root, "A.MD")); err == nil {
			t.Skip("case-insensitive filesystem detected; skipping case mismatch test")
		}
		iss := Links(root)
		foundCase := false
		for _, i := range iss {
			if i.File == caseDoc && i.Msg == "broken link" {
				foundCase = true
			}
		}
		if !foundCase {
			t.Errorf("expected broken link for case mismatch on case-sensitive FS")
		}
	})
}

func TestSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping symlink test on Windows")
	}
	realRoot := t.TempDir()
	outside := t.TempDir()
	symRoot := filepath.Join(t.TempDir(), "repo-symlink")
	if err := os.Symlink(realRoot, symRoot); err != nil {
		t.Fatal(err)
	}

	// Healthy file
	os.WriteFile(filepath.Join(realRoot, "code.go"), []byte("package p\nfunc Sym() {}\n"), 0o644)
	doc := filepath.Join(symRoot, "doc.md")
	os.WriteFile(doc, []byte("<!-- @scribex:anchor(code.go#Sym) -->\n"), 0o644)

	// Check on symlink root
	rep, err := Check(symRoot, false, nil)
	if err != nil {
		t.Fatalf("Check on symRoot failed: %v", err)
	}
	if len(rep.Issues) != 1 || !rep.Issues[0].Stale {
		t.Fatalf("expected 1 stale issue, got: %v", rep.Issues)
	}
	// Verify sync repairs it
	repSync, err := Check(symRoot, true, nil)
	if err != nil {
		t.Fatalf("Check sync failed: %v", err)
	}
	if len(repSync.Issues) != 0 {
		t.Fatalf("expected 0 issues after sync, got: %v", repSync.Issues)
	}

	// Outside anchor
	badDoc := filepath.Join(symRoot, "bad.md")
	os.WriteFile(badDoc, []byte("<!-- @scribex:anchor(../outside.go#X) -->\n"), 0o644)
	rep2, _ := Check(symRoot, false, nil)
	foundBad := false
	for _, i := range rep2.Issues {
		if i.File == badDoc && strings.Contains(i.Msg, "../outside.go") {
			foundBad = true
		}
	}
	if !foundBad {
		t.Fatalf("expected outside anchor rejected on symRoot")
	}
	_ = outside
}

func TestPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf test in short mode")
	}

	// 1. 20,000 .md files tree -> Files < 2s
	t.Run("files_20k", func(t *testing.T) {
		root := t.TempDir()
		numDirs := 200
		filesPerDir := 100
		for i := 0; i < numDirs; i++ {
			d := filepath.Join(root, fmt.Sprintf("d%d", i))
			if err := os.Mkdir(d, 0o755); err != nil {
				t.Fatal(err)
			}
			for j := 0; j < filesPerDir; j++ {
				f := filepath.Join(d, fmt.Sprintf("f%d.md", j))
				if err := os.WriteFile(f, []byte("# Test"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}

		start := time.Now()
		files := Files(root)
		elapsed := time.Since(start)
		if len(files) != numDirs*filesPerDir {
			t.Fatalf("Files count = %d, want %d", len(files), numDirs*filesPerDir)
		}
		if elapsed > 3*time.Second {
			t.Fatalf("Files took %v (> 3s)", elapsed)
		}
	})

	// 2. 1 MB doc with 10 000 anchors -> Process < 1s
	t.Run("process_10k_anchors", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; i < 10000; i++ {
			fmt.Fprintf(&sb, "<!-- @scribex:anchor(pkg/file.go#Sym%d) sig=1234abcd -->\n```signature\nfunc Sym%d()\n```\nprose %d\n", i, i, i)
		}
		md := sb.String()
		if len(md) < 1024*1024 {
			// Pad if under 1 MB
			md += strings.Repeat("a\n", (1024*1024-len(md))/2)
		}

		look := func(file, sym string) (string, bool, error) {
			return "func " + sym + "()", true, nil
		}

		start := time.Now()
		_, _ = anchors.Process(md, look, false, nil)
		elapsed := time.Since(start)
		if elapsed > 1*time.Second {
			t.Fatalf("Process took %v (> 1s)", elapsed)
		}
	})
}

func TestCLIS8Repro(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI-level test in short mode")
	}

	bin := filepath.Join(t.TempDir(), "scribex")
	buildCmd := exec.Command("go", "build", "-o", bin, "scribex/cmd/scribex")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build scribex failed: %v\n%s", err, out)
	}

	outside := t.TempDir()
	secretFile := filepath.Join(outside, "secret.go")
	secretContent := "package secret\nfunc SuperSecretPassword() {}\n"
	if err := os.WriteFile(secretFile, []byte(secretContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Case 1: ../outside.go#Secret
	t.Run("parent_path_cli", func(t *testing.T) {
		root := t.TempDir()
		docPath := filepath.Join(root, "readme.md")
		docContent := "<!-- @scribex:anchor(../outside.go#Secret) -->\n"
		if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(bin, "check", "-C", root)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected check to fail (rc 1), succeeded with output:\n%s", out)
		}
		outStr := string(out)
		if !strings.Contains(outStr, "anchor path") || !strings.Contains(outStr, "escapes") {
			t.Fatalf("output missing 'anchor path ... escapes':\n%s", outStr)
		}
		if strings.Contains(outStr, "SuperSecretPassword") {
			t.Fatalf("secret leaked into output:\n%s", outStr)
		}
		after, _ := os.ReadFile(docPath)
		if string(after) != docContent {
			t.Fatalf("doc was modified: %q != %q", string(after), docContent)
		}
	})

	// Case 2: symlink link.go
	if runtime.GOOS != "windows" {
		t.Run("symlink_cli", func(t *testing.T) {
			root := t.TempDir()
			if err := os.Symlink(secretFile, filepath.Join(root, "link.go")); err != nil {
				t.Fatal(err)
			}
			docPath := filepath.Join(root, "readme.md")
			docContent := "<!-- @scribex:anchor(link.go#Secret) -->\n"
			if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(bin, "check", "-C", root)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected check to fail (rc 1), succeeded with output:\n%s", out)
			}
			outStr := string(out)
			if !strings.Contains(outStr, "anchor path") || (!strings.Contains(outStr, "outside") && !strings.Contains(outStr, "escapes")) {
				t.Fatalf("output missing anchor path confinement message:\n%s", outStr)
			}
			if strings.Contains(outStr, "SuperSecretPassword") {
				t.Fatalf("secret leaked into output:\n%s", outStr)
			}
			after, _ := os.ReadFile(docPath)
			if string(after) != docContent {
				t.Fatalf("doc was modified: %q != %q", string(after), docContent)
			}
		})
	}
}
