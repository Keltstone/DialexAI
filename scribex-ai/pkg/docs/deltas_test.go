package docs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"scribex/pkg/gitx"
)

func newRepo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, a := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "a@b.c"},
		{"config", "user.name", "t"},
	} {
		if _, err := gitx.Run(d, a...); err != nil {
			t.Fatalf("git %v: %v", a, err)
		}
	}
	if err := os.WriteFile(filepath.Join(d, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitx.Run(d, "add", "-A")
	if _, err := gitx.Run(d, "commit", "-qm", "initial"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return d
}

// 1. Deltas(root, "--output=<tmp>/pwn") -> error, file <tmp>/pwn does not exist. Same for -x, a b, "" (empty), nonexistent ref (error mentions ref).
func TestGateSoundness_1_BadRefs(t *testing.T) {
	d := newRepo(t)
	pwn := filepath.Join(t.TempDir(), "pwn")

	for _, bad := range []string{"--output=" + pwn, "-x", "a b", ""} {
		_, err := Deltas(d, bad)
		if err == nil {
			t.Errorf("Deltas(%q) expected error, got nil", bad)
		}
	}
	if _, err := os.Stat(pwn); err == nil {
		t.Fatalf("option injection created file %s", pwn)
	}

	_, err := Deltas(d, "nosuch")
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
	if !strings.Contains(err.Error(), "nosuch") {
		t.Errorf("error %q should mention ref 'nosuch'", err.Error())
	}
}

// 2. Shallow clone fixture (git clone --depth 1 file://...): base = an ancestor that is not in the clone -> error containing "shallow clone".
func TestGateSoundness_2_ShallowClone(t *testing.T) {
	d := newRepo(t)
	initialShaBytes, err := gitx.Run(d, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	initialSha := strings.TrimSpace(string(initialShaBytes))

	// Commit again on main
	if err := os.WriteFile(filepath.Join(d, "commit2.txt"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitx.Run(d, "add", "-A")
	gitx.Run(d, "commit", "-qm", "second")

	cloneDir := filepath.Join(t.TempDir(), "clone")
	if _, err := gitx.Run(t.TempDir(), "clone", "-q", "--depth", "1", "file://"+d, cloneDir); err != nil {
		t.Skipf("shallow clone unavailable: %v", err)
	}

	_, err = Deltas(cloneDir, initialSha)
	if err == nil {
		t.Fatal("expected error on shallow clone with missing ancestor base")
	}
	if !strings.Contains(err.Error(), "shallow clone") {
		t.Errorf("expected error containing 'shallow clone', got: %v", err)
	}
}

// 3. Merge-base semantics: branch feature forked from main; commit a public-API change on main AFTER the fork; Deltas(root, "main") from feature reports only feature's own changes.
func TestGateSoundness_3_MergeBaseSemantics(t *testing.T) {
	d := newRepo(t)
	// Fork feature
	if _, err := gitx.Run(d, "checkout", "-b", "feature"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "feat.go"), []byte("package p\nfunc FeatureAPI() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitx.Run(d, "add", "-A")
	gitx.Run(d, "commit", "-qm", "feature commit")

	// Commit public API change on main after fork
	gitx.Run(d, "checkout", "main")
	if err := os.WriteFile(filepath.Join(d, "main_api.go"), []byte("package p\nfunc MainAPI() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitx.Run(d, "add", "-A")
	gitx.Run(d, "commit", "-qm", "main api commit")

	// Switch back to feature
	gitx.Run(d, "checkout", "feature")

	res, err := Deltas(d, "main")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d: %v", len(res.Deltas), res.Deltas)
	}
	if res.Deltas[0].Sym != "FeatureAPI" {
		t.Errorf("expected delta FeatureAPI, got %s", res.Deltas[0].Sym)
	}
}

// 4. New untracked file with func New() -> Delta{Kind:"added"}; ignored (.gitignore'd) file -> nothing.
func TestGateSoundness_4_UntrackedAndIgnored(t *testing.T) {
	d := newRepo(t)
	// .gitignore
	os.WriteFile(filepath.Join(d, ".gitignore"), []byte("ignored/\n"), 0o644)
	gitx.Run(d, "add", ".gitignore")
	gitx.Run(d, "commit", "-qm", "add gitignore")

	// Untracked file
	os.WriteFile(filepath.Join(d, "untracked.go"), []byte("package p\nfunc New() {}\n"), 0o644)
	// Ignored file
	os.MkdirAll(filepath.Join(d, "ignored"), 0o755)
	os.WriteFile(filepath.Join(d, "ignored", "secret.go"), []byte("package p\nfunc Ignored() {}\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d: %v", len(res.Deltas), res.Deltas)
	}
	if res.Deltas[0].Sym != "New" || res.Deltas[0].Kind != "added" {
		t.Errorf("expected Delta{Kind: 'added', Sym: 'New'}, got %v", res.Deltas[0])
	}
}

// 5. Staged-only change, unstaged-only change, both -> all detected.
func TestGateSoundness_5_StagedAndUnstaged(t *testing.T) {
	d := newRepo(t)
	// Staged file
	os.WriteFile(filepath.Join(d, "staged.go"), []byte("package p\nfunc Staged() {}\n"), 0o644)
	gitx.Run(d, "add", "staged.go")

	// Unstaged file
	os.WriteFile(filepath.Join(d, "unstaged.go"), []byte("package p\nfunc Unstaged() {}\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d: %v", len(res.Deltas), res.Deltas)
	}
}

// 6. Deleted file -> all its symbols removed; rename (git mv + edit) -> removed + added.
func TestGateSoundness_6_DeletedAndRenamed(t *testing.T) {
	d := newRepo(t)
	os.WriteFile(filepath.Join(d, "to_delete.go"), []byte("package p\nfunc Dead() {}\n"), 0o644)
	os.WriteFile(filepath.Join(d, "to_rename.go"), []byte("package p\nfunc Orig() {}\n"), 0o644)
	gitx.Run(d, "add", "-A")
	gitx.Run(d, "commit", "-qm", "add files")

	// Delete to_delete.go
	os.Remove(filepath.Join(d, "to_delete.go"))
	// Rename to_rename.go -> renamed.go and edit
	gitx.Run(d, "mv", "to_rename.go", "renamed.go")
	os.WriteFile(filepath.Join(d, "renamed.go"), []byte("package p\nfunc Renamed() {}\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}

	foundRemovedDead := false
	foundRemovedOrig := false
	foundAddedRenamed := false
	for _, d := range res.Deltas {
		if d.File == "to_delete.go" && d.Sym == "Dead" && d.Kind == "removed" {
			foundRemovedDead = true
		}
		if d.File == "to_rename.go" && d.Sym == "Orig" && d.Kind == "removed" {
			foundRemovedOrig = true
		}
		if d.File == "renamed.go" && d.Sym == "Renamed" && d.Kind == "added" {
			foundAddedRenamed = true
		}
	}

	if !foundRemovedDead {
		t.Errorf("missing removed Dead delta: %v", res.Deltas)
	}
	if !foundRemovedOrig {
		t.Errorf("missing removed Orig delta: %v", res.Deltas)
	}
	if !foundAddedRenamed {
		t.Errorf("missing added Renamed delta: %v", res.Deltas)
	}
}

// 7. File names: a b.go, ünï.go, and one containing a newline (skip on Windows) -> handled.
func TestGateSoundness_7_SpecialFilenames(t *testing.T) {
	d := newRepo(t)
	files := []string{"a b.go", "ünï.go"}
	if runtime.GOOS != "windows" {
		files = append(files, "new\nline.go")
	}

	for _, f := range files {
		os.WriteFile(filepath.Join(d, f), []byte("package p\nfunc F() {}\n"), 0o644)
	}

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Deltas) != len(files) {
		t.Fatalf("expected %d deltas, got %d: %v", len(files), len(res.Deltas), res.Deltas)
	}
}

// 8. Submodule path change -> Skipped, no error, no delta.
func TestGateSoundness_8_Submodule(t *testing.T) {
	d := newRepo(t)
	subRepo := newRepo(t)

	// Add submodule
	if _, err := gitx.Run(d, "-c", "protocol.file.allow=always", "submodule", "add", subRepo, "sub"); err != nil {
		t.Skipf("git submodule add not available: %v", err)
	}
	gitx.Run(d, "commit", "-qm", "add submodule")

	// Commit inside submodule
	os.WriteFile(filepath.Join(subRepo, "change.txt"), []byte("sub change\n"), 0o644)
	gitx.Run(subRepo, "add", "-A")
	gitx.Run(subRepo, "commit", "-qm", "sub change")

	// Update submodule reference in d
	gitx.Run(d, "-c", "protocol.file.allow=always", "submodule", "update", "--remote")
	gitx.Run(d, "add", "sub")

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Deltas) != 0 {
		t.Errorf("expected 0 deltas for submodule, got %v", res.Deltas)
	}
	if len(res.Skipped) == 0 || res.Skipped[0] != "sub" {
		t.Errorf("expected 'sub' in Skipped, got %v", res.Skipped)
	}
}

// 9. root = subdirectory of the repo: files outside root not reported.
func TestGateSoundness_9_SubdirectoryRoot(t *testing.T) {
	d := newRepo(t)
	sub := filepath.Join(d, "subdir")
	os.MkdirAll(sub, 0o755)

	// Outside file
	os.WriteFile(filepath.Join(d, "outside.go"), []byte("package p\nfunc Outside() {}\n"), 0o644)
	// Inside file
	os.WriteFile(filepath.Join(sub, "inside.go"), []byte("package p\nfunc Inside() {}\n"), 0o644)

	res, err := Deltas(sub, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error from subdir: %v", err)
	}
	if len(res.Deltas) != 1 {
		t.Fatalf("expected 1 delta from subdir, got %d: %v", len(res.Deltas), res.Deltas)
	}
	if res.Deltas[0].Sym != "Inside" {
		t.Errorf("expected delta Inside, got %s", res.Deltas[0].Sym)
	}
}

// 10. Hostile .git/config core.fsmonitor pointing to a script that touches a marker -> marker absent after Deltas.
func TestGateSoundness_10_HostileFsmonitor(t *testing.T) {
	d := newRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	hook := filepath.Join(t.TempDir(), "fsm.sh")
	os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755)
	gitx.Run(d, "config", "core.fsmonitor", hook)

	os.WriteFile(filepath.Join(d, "file.go"), []byte("package p\nfunc HostileTest() {}\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(res.Deltas))
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("hostile core.fsmonitor script was executed!")
	}
}

// 11. Changed .kt whose new version has an unterminated string -> Unknown non-empty, Deltas for that file empty; gate returns run=true.
func TestUnknownHandling_11_UnterminatedString(t *testing.T) {
	d := newRepo(t)
	os.WriteFile(filepath.Join(d, "bad.kt"), []byte("fun ok() = 1\n"), 0o644)
	gitx.Run(d, "add", "bad.kt")
	gitx.Run(d, "commit", "-qm", "initial kt")

	// Unterminated string
	os.WriteFile(filepath.Join(d, "bad.kt"), []byte("fun bad() = \"unterminated string\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Unknown) == 0 {
		t.Fatal("expected non-empty Unknown")
	}
	if res.Unknown[0].File != "bad.kt" {
		t.Errorf("expected Unknown file 'bad.kt', got %s", res.Unknown[0].File)
	}
	if len(res.Deltas) != 0 {
		t.Errorf("expected Deltas for bad.kt to be empty, got %v", res.Deltas)
	}
}

// 12. Changed .kt whose OLD version is unparsable and new is fine -> Unknown.
func TestUnknownHandling_12_OldVersionUnparsable(t *testing.T) {
	d := newRepo(t)
	// Commit unparsable Kotlin file
	os.WriteFile(filepath.Join(d, "fix.kt"), []byte("fun broken( = \n"), 0o644)
	gitx.Run(d, "add", "fix.kt")
	gitx.Run(d, "commit", "-qm", "broken kt")

	// Fix it in working tree
	os.WriteFile(filepath.Join(d, "fix.kt"), []byte("fun fixed() {}\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Unknown) == 0 {
		t.Fatal("expected Unknown for file whose old version was unparsable")
	}
	if res.Unknown[0].File != "fix.kt" {
		t.Errorf("expected Unknown file 'fix.kt', got %s", res.Unknown[0].File)
	}
}

// 13. Changed docs/img.png and notes.txt (unsupported, unreferenced) -> no Unknown, no delta, gate says no change.
func TestUnknownHandling_13_UnsupportedUnreferenced(t *testing.T) {
	d := newRepo(t)
	os.MkdirAll(filepath.Join(d, "docs"), 0o755)
	os.WriteFile(filepath.Join(d, "docs", "img.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(d, "notes.txt"), []byte("notes"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("expected 0 Unknown, got %v", res.Unknown)
	}
	if len(res.Deltas) != 0 {
		t.Errorf("expected 0 Deltas, got %v", res.Deltas)
	}
}

// 14. Changed lib.rs referenced by <!-- @scribex:anchor(lib.rs#f) --> -> Unknown("unsupported language referenced by an anchor").
func TestUnknownHandling_14_UnsupportedReferenced(t *testing.T) {
	d := newRepo(t)
	os.WriteFile(filepath.Join(d, "lib.rs"), []byte("pub fn f() {}\n"), 0o644)
	os.WriteFile(filepath.Join(d, "doc.md"), []byte("<!-- @scribex:anchor(lib.rs#f) -->\n"), 0o644)

	res, err := Deltas(d, "HEAD")
	if err != nil {
		t.Fatalf("Deltas error: %v", err)
	}
	if len(res.Unknown) != 1 {
		t.Fatalf("expected 1 Unknown, got %d: %v", len(res.Unknown), res.Unknown)
	}
	if res.Unknown[0].File != "lib.rs" {
		t.Errorf("expected Unknown file 'lib.rs', got %s", res.Unknown[0].File)
	}
	wantReason := "unsupported language referenced by an anchor"
	if res.Unknown[0].Reason != wantReason {
		t.Errorf("expected reason %q, got %q", wantReason, res.Unknown[0].Reason)
	}
}

// Regression (must still pass): body-only change -> zero deltas; formatting-only -> same; const value change -> delta; Kotlin multi-line param change -> delta.
func TestRegressionCases(t *testing.T) {
	d := newRepo(t)
	// Go body change
	os.WriteFile(filepath.Join(d, "func.go"), []byte("package p\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	// Go formatting change
	os.WriteFile(filepath.Join(d, "fmt.go"), []byte("package p\nfunc Formatted( x int ) {}\n"), 0o644)
	// Const value change
	os.WriteFile(filepath.Join(d, "const.go"), []byte("package p\nconst Version = 1\n"), 0o644)
	// Kotlin multiline param
	os.WriteFile(filepath.Join(d, "multi.kt"), []byte("fun process(\n    a: Int,\n    b: String\n): Unit = Unit\n"), 0o644)

	gitx.Run(d, "add", "-A")
	gitx.Run(d, "commit", "-qm", "baseline")

	// 1. Body-only change in Add
	os.WriteFile(filepath.Join(d, "func.go"), []byte("package p\nfunc Add(a, b int) int {\n\t// new body\n\treturn a + b\n}\n"), 0o644)
	res, err := Deltas(d, "HEAD")
	if err != nil || len(res.Deltas) != 0 {
		t.Fatalf("body change should have 0 deltas, got %v (err=%v)", res.Deltas, err)
	}

	// 2. Formatting-only change in Formatted
	os.WriteFile(filepath.Join(d, "fmt.go"), []byte("package p\nfunc Formatted(x int) {}\n"), 0o644)
	res, err = Deltas(d, "HEAD")
	if err != nil || len(res.Deltas) != 0 {
		t.Fatalf("formatting change should have 0 deltas, got %v (err=%v)", res.Deltas, err)
	}

	// 3. Const value change
	os.WriteFile(filepath.Join(d, "const.go"), []byte("package p\nconst Version = 2\n"), 0o644)
	res, err = Deltas(d, "HEAD")
	if err != nil || len(res.Deltas) != 1 || res.Deltas[0].Sym != "Version" || res.Deltas[0].Kind != "changed" {
		t.Fatalf("expected const value delta, got %v (err=%v)", res.Deltas, err)
	}
	// revert const
	gitx.Run(d, "checkout", "const.go")

	// 4. Kotlin multi-line param change
	os.WriteFile(filepath.Join(d, "multi.kt"), []byte("fun process(\n    a: Int,\n    b: String,\n    c: Boolean\n): Unit = Unit\n"), 0o644)
	res, err = Deltas(d, "HEAD")
	if err != nil || len(res.Deltas) != 1 || res.Deltas[0].Sym != "process" || res.Deltas[0].Kind != "changed" {
		t.Fatalf("expected kotlin param delta, got %v (err=%v)", res.Deltas, err)
	}
}
