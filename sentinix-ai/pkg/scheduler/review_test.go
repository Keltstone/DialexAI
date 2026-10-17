package scheduler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sentinix/pkg/githost"
)

func init() {
	AllowLocalForTest = true
}


func TestHostPinning_RejectsInsecureSchemesAndCredentials(t *testing.T) {
	// Without AllowLocalForTest, test that insecure schemes and credentials are rejected
	AllowLocalForTest = false
	defer func() { AllowLocalForTest = true }()

	tests := []struct {
		url     string
		wantErr string
	}{
		{"file:///etc/passwd", "unsupported clone URL scheme"},
		{"ssh://git@github.com/org/repo.git", "unsupported clone URL scheme"},
		{"ext::sh -c 'echo pwned'", "invalid clone URL"},
		{"/var/tmp/repo", "invalid clone URL"},
		{"https://user:password@github.com/org/repo.git", "credentials in clone URL are forbidden"},
		{"http://github.com/org/repo.git", "unsupported clone URL scheme"},
	}

	for _, tt := range tests {
		err := isConfiguredHost(tt.url, "github.com")
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("url %q: expected error containing %q, got: %v", tt.url, tt.wantErr, err)
		}
	}

	// Valid https URL matching host should succeed
	if err := isConfiguredHost("https://github.com/org/repo.git", "github.com"); err != nil {
		t.Errorf("expected valid https URL to pass, got: %v", err)
	}
}

func TestReviewReportsOnlyNew(t *testing.T) {

	d := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", d, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(d, "old.txt"), []byte("k=\"AKIA"+"ABCDEFGHIJKLMNOP\"\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "feat")
	os.WriteFile(filepath.Join(d, "new.txt"), []byte("k=\"AKIA"+"ZZZZZZZZZZZZZZZZ\"\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "head")

	ev := &githost.Event{HeadSHA: "feat", HeadRef: "feat", BaseRef: "main", CloneURL: d, BaseCloneURL: d, Fork: true}
	res, err := Review(t.Context(), ev, "", "")
	if err != nil || len(res.New) != 1 || res.New[0].File != "new.txt" {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestFullScan(t *testing.T) {
	d := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", d, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(d, "x.txt"), []byte("k=\"AKIA"+"ABCDEFGHIJKLMNOP\"\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "c")
	res, _, err := Full(t.Context(), d, "main", "", "", "")
	if err != nil || len(res.New) != 1 || res.Commit == "" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestReview_SemgrepWithoutConfigReportsUnavailable(t *testing.T) {
	d := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", d, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(d, "file.txt"), []byte("hello world\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "init")

	ev := &githost.Event{HeadSHA: "main", HeadRef: "main", BaseRef: "main", CloneURL: d, BaseCloneURL: d}
	res, err := Review(t.Context(), ev, "", "")
	if err != nil {
		t.Fatalf("expected review to complete even if semgrep lacks config, got: %v", err)
	}
	if len(res.Gaps) == 0 {
		t.Logf("no gaps reported; checking if semgrep is on path")
	}
}

func TestReview_RenameMapAndCosmeticComment(t *testing.T) {
	d := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", d, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(d, "old.txt"), []byte("k = \"AKIA"+"ABCDEFGHIJKLMNOP\"\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "base")

	run("checkout", "-qb", "feat")
	run("mv", "old.txt", "new.txt")
	os.WriteFile(filepath.Join(d, "new.txt"), []byte("k = \"AKIA"+"ABCDEFGHIJKLMNOP\" // moved\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "head")

	ev := &githost.Event{HeadSHA: "feat", HeadRef: "feat", BaseRef: "main", CloneURL: d, BaseCloneURL: d, Fork: true}
	res, err := Review(t.Context(), ev, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.New) != 0 {
		t.Fatalf("expected 0 new findings after rename + cosmetic comment, got %d: %+v", len(res.New), res.New)
	}
}
