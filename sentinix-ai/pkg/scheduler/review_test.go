package scheduler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"sentinix/pkg/githost"
)

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
	res, err := Review(ev, "", "")
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
	res, _, err := Full(d, "main", "", "")
	if err != nil || len(res.New) != 1 || res.Commit == "" {
		t.Fatalf("%+v %v", res, err)
	}
}
