package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"sentinix/pkg/githost"
)

// TestPerfReview times one PR review over an N-file repo. Run: bench/perf.sh [N...]
// Skipped unless PERF_FILES is set. Needs the real scanners on PATH.
func TestPerfReview(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("PERF_FILES"))
	if n == 0 {
		t.Skip("set PERF_FILES")
	}
	d := t.TempDir()
	run := perfGit(t, d)
	run("init", "-q", "-b", "main")
	for i := 0; i < n; i++ {
		p := filepath.Join(d, fmt.Sprintf("m%03d/p%02d/F%d.kt", i%200, i%37, i))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(fmt.Sprintf("package a%d\nclass F%d { fun f(x:Int) = x + %d }\n", i, i, i)), 0o644)
	}
	run("add", ".")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "feat")
	os.WriteFile(filepath.Join(d, "m000/New.kt"), []byte("val k = \"AKIA"+"ABCDEFGHIJKLMNOP\"\n"), 0o644)
	run("add", ".")
	run("commit", "-qm", "head")
	ev := &githost.Event{HeadSHA: "feat", HeadRef: "feat", BaseRef: "main", CloneURL: d, BaseCloneURL: d, Fork: true}
	if c, _ := strconv.Atoi(os.Getenv("PERF_CONC")); c > 1 { // c simultaneous PRs, no queue limit: worst case for the host
		w, _ := strconv.Atoi(os.Getenv("PERF_WORKERS")) // sentinixd -workers default is 2
		if w == 0 {
			w = 2
		}
		sem := make(chan struct{}, w)
		walls := make([]float64, c)
		var wg sync.WaitGroup
		for i := 0; i < c; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s0 := time.Now() // latency includes queue wait
				sem <- struct{}{}
				defer func() { <-sem }()
				if _, err := Review(t.Context(), ev, "", ""); err != nil {
					t.Error(err)
				}
				walls[i] = time.Since(s0).Seconds()
			}()
		}
		wg.Wait()
		sort.Float64s(walls)
		t.Logf("PERF files=%d workers=%d concurrent=%d p50=%.1fs p95=%.1fs max=%.1fs", n, w, c, walls[c/2], walls[c*95/100], walls[c-1])
		return
	}
	st := time.Now()
	res, err := Review(t.Context(), ev, "", "")
	wall := time.Since(st)
	if err != nil {
		t.Fatal(err)
	}
	var self, kids syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &self)
	syscall.Getrusage(syscall.RUSAGE_CHILDREN, &kids)
	// Maxrss is bytes on darwin, KiB on linux; PERF line states the raw value.
	t.Logf("PERF files=%d wall=%s new=%d maxrss_self=%d maxrss_largest_child=%d cpu_user_s=%.1f", n, wall.Round(time.Millisecond), len(res.New), self.Maxrss, kids.Maxrss,
		time.Duration(kids.Utime.Nano()+self.Utime.Nano()).Seconds())
}

func perfGit(t *testing.T, d string) func(args ...string) {
	return func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", d, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}
