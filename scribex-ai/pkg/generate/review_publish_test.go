package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fakeSocratix(t *testing.T, memo string) {
	dir := t.TempDir()
	script := "#!/bin/sh\nwhile [ \"$1\" != \"-out\" ]; do shift; done\ncat > /dev/null\nprintf '%s' '" + memo + "' > \"$2\"\n"
	if err := os.WriteFile(filepath.Join(dir, "socratix"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestSocratixDisputedAndClean(t *testing.T) {
	root := fixture(t)
	fakeSocratix(t, "memo\n- UNSUPPORTED: claims <b>X</b> @scribex end\n- UNSUPPORTED: it scales to 1M rps\n")
	s, err := NewSocratix(root, "redteam", "", "")
	if err != nil {
		t.Fatal(err)
	}
	v, dis, _, err := s.Review("runbook-ops", "repo", "draft", []string{"svc/svc.go"})
	if err != nil || v != "disputed" || len(dis) != 1 || strings.Contains(dis[0], "<") {
		t.Fatalf("%v %v %q", err, v, dis)
	}
	fakeSocratix(t, "draft reviewed: all fine")
	s, _ = NewSocratix(root, "redteam", "", "")
	if v, _, _, _ := s.Review("t", "u", "d", nil); v != "clean" {
		t.Fatal(v)
	}
}

func TestSocratixEmptyOrUnrelatedMemoErrors(t *testing.T) {
	root := fixture(t)
	// Memo empty -> error
	fakeSocratix(t, "")
	s, _ := NewSocratix(root, "redteam", "", "")
	if _, _, _, err := s.Review("t", "u", "d", nil); err == nil {
		t.Fatal("expected error on empty memo")
	}
	// Memo does not mention draft -> error
	fakeSocratix(t, "the weather is sunny today")
	if _, _, _, err := s.Review("t", "u", "d", nil); err == nil {
		t.Fatal("expected error on memo without draft reference")
	}
}

func TestSocratixValidation(t *testing.T) {
	root := t.TempDir()
	for _, c := range [][3]string{{"evil", "", ""}, {"redteam", "x;y", ""}, {"redteam", "", "-rf"}} {
		if _, err := NewSocratix(root, c[0], c[1], c[2]); err == nil {
			t.Fatalf("accepted %v", c)
		}
	}
}

func TestApplyDerivedWithReviewDisputed(t *testing.T) {
	root := fixture(t)
	fakeSocratix(t, "UNSUPPORTED: invented thing")
	rv, _ := NewSocratix(root, "redteam", "", "")
	write(t, root, ".scribex/doctypes.json", `{"types":[{"id":"runbook-ops","enabled":true,"review":"socratix"}]}`)
	gitCommit(t, root)
	f := &fake{reply: func(string) string { return "=== Components ===\nA tiny service.\n" }}
	_, r, err := run(t, root, derivedOpts(root), ApplyOptions{Model: f, ConfirmSend: true, Reviewer: rv})
	if err != nil || len(r.Failed) != 0 || r.Calls != 2 {
		t.Fatalf("%v %+v", err, r)
	}
	b, _ := os.ReadFile(filepath.Join(root, "docs/ops/runbook.md"))
	if !strings.Contains(string(b), "review=socratix:redteam:disputed") || !strings.Contains(string(b), "> Disputed: invented thing") {
		t.Fatalf("%s", b)
	}
}

func TestApplyDerivedReviewMissingIsSkipped(t *testing.T) {
	root := fixture(t)
	write(t, root, ".scribex/doctypes.json", `{"types":[{"id":"runbook-ops","enabled":true,"review":"socratix"}]}`)
	gitCommit(t, root)
	f := &fake{reply: func(string) string { return "=== Components ===\nTiny.\n" }}
	run(t, root, derivedOpts(root), ApplyOptions{Model: f, ConfirmSend: true})
	b, _ := os.ReadFile(filepath.Join(root, "docs/ops/runbook.md"))
	if !strings.Contains(string(b), "review=skipped:not-configured") {
		t.Fatalf("%s", b)
	}
}

func TestPublishBranchCommitOnly(t *testing.T) {
	root := fixture(t)
	run(t, root, opts(root), ApplyOptions{Model: &fake{reply: symReply}, ConfirmSend: true})
	res, err := Publish(root, "scribex", "r1", []string{"docs/reference/svc/svc.go.md"}, false)
	if err != nil || !strings.HasPrefix(res.Branch, "scribex/docs-") {
		t.Fatalf("%v %+v", err, res)
	}
	out, _ := exec.Command("git", "-C", root, "branch", "--show-current").Output()
	if strings.TrimSpace(string(out)) != res.Branch {
		t.Fatalf("on %s", out)
	}
	if st, _ := exec.Command("git", "-C", root, "status", "--porcelain").Output(); strings.TrimSpace(string(st)) != "" {
		t.Fatalf("dirty: %s", st)
	}
	if _, err := Publish(root, "scribex", "r1", []string{"x"}, false); err == nil {
		t.Fatal("expected existing-branch refusal")
	}
	if _, err := Publish(root, "Bad Prefix", "r1", []string{"x"}, false); err == nil {
		t.Fatal("bad prefix accepted")
	}
}

func TestCompareURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:o/r.git":      "https://github.com/o/r/compare/main...b?expand=1",
		"https://github.com/o/r.git":  "https://github.com/o/r/compare/main...b?expand=1",
		"https://example.com/o/r.git": "",
	}
	for in, want := range cases {
		if got := compareURL(in, "main", "b"); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}
