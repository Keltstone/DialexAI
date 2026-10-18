package scheduler

import (
	"os"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/githost"
)

func TestCheckRefRejectsOptionLikeRefs(t *testing.T) {
	for _, bad := range []string{"", "-x", "--upload-pack=id", "a b", "a..b", "a:b", "a\nb"} {
		if checkRef(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, ok := range []string{"main", "feature/x-1", "refs/pull/12/head", "0123456789abcdef0123456789abcdef01234567"} {
		if err := checkRef(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
}

func TestSweepStaleTempAndFileLimit(t *testing.T) {
	d, _ := os.MkdirTemp("", "sentinix-stale-*")
	old := time.Now().Add(-3 * time.Hour)
	os.Chtimes(d, old, old)
	fresh, _ := os.MkdirTemp("", "sentinix-fresh-*")
	defer os.RemoveAll(fresh)
	SweepStaleTemp(time.Hour)
	if _, err := os.Stat(d); err == nil {
		os.RemoveAll(d)
		t.Fatal("stale dir not swept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh dir wrongly swept")
	}
}

func TestReviewRefusesFileCountBomb(t *testing.T) {
	old := MaxTreeFiles
	MaxTreeFiles = 3
	defer func() { MaxTreeFiles = old }()
	d := t.TempDir()
	run := perfGit(t, d)
	run("init", "-q", "-b", "main")
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		os.WriteFile(d+"/"+n+".kt", nil, 0o644)
	}
	run("add", ".")
	run("commit", "-qm", "x")
	_, err := Review(t.Context(), &githost.Event{HeadRef: "main", BaseRef: "main", CloneURL: d, BaseCloneURL: d, Fork: true}, "", "")
	if err == nil || !strings.Contains(err.Error(), "file count") {
		t.Fatalf("want file count refusal, got %v", err)
	}
}
