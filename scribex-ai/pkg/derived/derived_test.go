package derived

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, c := range files {
		fp := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fp, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func block(t *testing.T, root string, srcs ...string) Block {
	t.Helper()
	b := Block{ID: "feature-design:login", Body: "Handles login.\n\nSecond paragraph."}
	for _, s := range srcs {
		h, err := HashFile(root, s)
		if err != nil {
			t.Fatal(err)
		}
		b.Sources = append(b.Sources, Source{s, h})
	}
	return b
}

func TestRoundTripCanonical(t *testing.T) {
	root := repo(t, map[string]string{"a/Login.kt": "x", "b c/Odd,File@1.kt": "y"})
	b := block(t, root, "a/Login.kt", "b c/Odd,File@1.kt")
	b.Review = "socratix:redteam:disputed"
	b.Disputed = []string{"claim one", "claim two"}
	md := "# T\n\n" + Render(b) + "\n\ntail\n"
	got := Parse(md)
	if len(got) != 1 {
		t.Fatalf("parsed %d blocks", len(got))
	}
	g := got[0]
	if g.ID != b.ID || len(g.Sources) != 2 || g.Sources[1].Path != "b c/Odd,File@1.kt" || g.Review != b.Review || g.Body != b.Body || len(g.Disputed) != 2 {
		t.Fatalf("round trip mismatch: %+v", g)
	}
	g.Line, g.End = 0, 0
	if Render(g) != Render(b) {
		t.Fatalf("not canonical:\n%s\n---\n%s", Render(g), Render(b))
	}
	if !strings.Contains(Render(b), Banner) {
		t.Fatal("banner missing on unconfirmed block")
	}
}

func TestParseIgnoresFencedAndRejectsForged(t *testing.T) {
	md := "```\n<!-- @scribex:derived(id=a:b) -->\nx\n<!-- @scribex:end -->\n```\n"
	if len(Parse(md)) != 0 {
		t.Fatal("fenced block parsed")
	}
	// unknown key, bad hash, bad date, missing end => invalid, never trusted
	for _, h := range []string{
		"<!-- @scribex:derived(id=a:b) evil=1 -->\nx\n<!-- @scribex:end -->",
		"<!-- @scribex:derived(id=a:b) sources=f@zzzz -->\nx\n<!-- @scribex:end -->",
		"<!-- @scribex:derived(id=a:b) confirmed=yesterday -->\nx\n<!-- @scribex:end -->",
		"<!-- @scribex:derived(id=a:b) -->\nx without end",
	} {
		bs := Parse(h)
		if len(bs) != 1 || bs[0].Review != "invalid" {
			t.Errorf("not flagged invalid: %q -> %+v", h, bs)
		}
	}
	// nested opener stops the first block
	md = "<!-- @scribex:derived(id=a:b) -->\nx\n<!-- @scribex:derived(id=c:d) -->\ny\n<!-- @scribex:end -->"
	bs := Parse(md)
	if len(bs) != 2 || bs[0].Review != "invalid" || bs[1].ID != "c:d" {
		t.Fatalf("nested handling: %+v", bs)
	}
}

func TestCheckStaleDeadClean(t *testing.T) {
	root := repo(t, map[string]string{"s/a.kt": "one", "s/b.kt": "two"})
	b := block(t, root, "s/a.kt", "s/b.kt")
	md := Render(b)
	if iss := Check(root, md); len(iss) != 0 {
		t.Fatalf("clean block flagged: %+v", iss)
	}
	os.WriteFile(filepath.Join(root, "s/a.kt"), []byte("changed"), 0o644)
	iss := Check(root, md)
	if len(iss) != 1 || iss[0].Code != CodeStaleSource || !strings.Contains(iss[0].Msg, "s/a.kt") || !strings.Contains(iss[0].Msg, "regenerate or confirm") {
		t.Fatalf("stale: %+v", iss)
	}
	os.Remove(filepath.Join(root, "s/b.kt"))
	iss = Check(root, md)
	codes := []string{}
	for _, i := range iss {
		codes = append(codes, i.Code)
	}
	if strings.Join(codes, ",") != CodeDeadSource+","+CodeStaleSource {
		t.Fatalf("codes = %v", codes)
	}
}

func TestCheckConfinesSourcePaths(t *testing.T) {
	root := repo(t, map[string]string{"ok.kt": "x"})
	outside := repo(t, map[string]string{"secret.txt": "s"})
	md := "<!-- @scribex:derived(id=a:b) sources=../" + filepath.Base(outside) + "/secret.txt@000000000000 -->\nx\n<!-- @scribex:end -->"
	iss := Check(root, md)
	if len(iss) != 1 || iss[0].Code != CodeDeadSource {
		t.Fatalf("traversal source should be dead, got %+v", iss)
	}
}

func TestDisputedIssue(t *testing.T) {
	root := repo(t, map[string]string{"a.kt": "x"})
	b := block(t, root, "a.kt")
	b.Review, b.Disputed = "socratix:redteam:disputed", []string{"c1"}
	iss := Check(root, Render(b))
	if len(iss) != 1 || iss[0].Code != CodeDisputed {
		t.Fatalf("%+v", iss)
	}
}

func TestConfirm(t *testing.T) {
	root := repo(t, map[string]string{"a.kt": "x"})
	b := block(t, root, "a.kt")
	md := "intro\n" + Render(b) + "\nouter"
	os.WriteFile(filepath.Join(root, "a.kt"), []byte("y"), 0o644)
	out, changed, err := Confirm(root, md, b.ID, "2026-11-01")
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	if strings.Contains(out, Banner) || !strings.Contains(out, "confirmed=2026-11-01") || !strings.HasPrefix(out, "intro\n") || !strings.HasSuffix(out, "\nouter") {
		t.Fatalf("confirm output wrong:\n%s", out)
	}
	if iss := Check(root, out); len(iss) != 0 {
		t.Fatalf("still flagged after confirm: %+v", iss)
	}
	if _, _, err := Confirm(root, md, "nope:x", "2026-11-01"); err == nil {
		t.Error("unknown id accepted")
	}
	if _, _, err := Confirm(root, md, b.ID, "tomorrow"); err == nil {
		t.Error("bad date accepted")
	}
}

func TestCheckBody(t *testing.T) {
	for _, bad := range []string{"", "  ", "x <!-- @scribex:end --> y", "see @Scribex:anchor", "a --> b", Banner, "> Disputed: x"} {
		if CheckBody(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if CheckBody("Plain paragraph.\n\n- item") != nil {
		t.Error("rejected plain markdown")
	}
}

func TestEncodingRoundTrip(t *testing.T) {
	for _, p := range []string{"a/b.kt", "a b/c,d@e.kt", "ünï/cødé.go", "100%/x.kt"} {
		got, ok := decPath(encPath(p))
		if !ok || got != p {
			t.Errorf("%q -> %q (%v)", p, got, ok)
		}
	}
	if _, ok := decPath("bad%2"); ok {
		t.Error("truncated escape accepted")
	}
	if _, ok := decPath("bad%ZZ"); ok {
		t.Error("non-hex escape accepted")
	}
}
