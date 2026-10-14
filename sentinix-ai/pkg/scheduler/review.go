// Package scheduler runs scans: PR/MR reviews (shallow-fetch head and base, report only what the change
// introduced) and full scans of a branch, plus cron timing for scheduled runs.
package scheduler

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/policy"
)

type Result struct {
	New     []engines.Finding
	Ran     []string
	Surface []diffmap.Item
	Commit  string
}

// repo is a throwaway git dir. The token is used only for fetches and never reaches a scan process.
type repo struct {
	dir, user, token string
}

func newRepo(user, token string) (*repo, error) {
	dir, err := os.MkdirTemp("", "sentinix-*")
	if err != nil {
		return nil, err
	}
	r := &repo{dir, user, token}
	if _, err := r.git("init", "-q"); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return r, nil
}

func (r *repo) close() { os.RemoveAll(r.dir) } // only findings leave the scan

func (r *repo) git(args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", r.dir}, args...)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if r.token != "" {
		s = strings.ReplaceAll(s, r.token, "***")
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, s)
	}
	return s, nil
}

// fetch brings one ref in at depth 1 and returns its commit. Tokens are sent only over https.
func (r *repo) fetch(rawURL, ref string, auth bool) (string, error) {
	if u, err := url.Parse(rawURL); auth && r.token != "" && err == nil && u.Scheme == "https" {
		u.User = url.UserPassword(r.user, r.token)
		rawURL = u.String()
	}
	if _, err := r.git("fetch", "--depth=1", "--no-tags", rawURL, ref); err != nil {
		return "", err
	}
	return r.git("rev-parse", "FETCH_HEAD")
}

func (r *repo) worktree(name, sha string) (string, error) {
	p := filepath.Join(r.dir, name)
	_, err := r.git("worktree", "add", "--detach", p, sha)
	return p, err
}

func scanAll(root string, extra []engines.Engine) ([]engines.Finding, []string, error) {
	var all []engines.Finding
	var ran []string
	for _, e := range engines.Bundled(extra...) {
		fs, err := e.Scan(root)
		if err != nil {
			return nil, nil, err // a broken engine must not look like a clean scan
		}
		ran = append(ran, e.Name())
		for _, f := range fs {
			all = append(all, rel(f, root))
		}
	}
	return all, ran, nil
}

// Review scans ev.HeadSHA and ev.BaseRef.
// ponytail: engines run on the host, not in a per-scan container; use sandbox.Docker before scanning untrusted repos with external engines.
func Review(ev *githost.Event, user, token string, extra ...engines.Engine) (*Result, error) {
	r, err := newRepo(user, token)
	if err != nil {
		return nil, err
	}
	defer r.close()
	// Fork heads are public clones: no token is sent to the fork's host.
	headRef := ev.HeadSHA
	if len(headRef) < 40 {
		headRef = ev.HeadRef
	}
	headSHA, err := r.fetch(ev.CloneURL, headRef, !ev.Fork)
	if err != nil {
		return nil, err
	}
	baseSHA, err := r.fetch(ev.BaseCloneURL, ev.BaseRef, true)
	if err != nil {
		return nil, err
	}
	head, err := r.worktree("wt-head", headSHA)
	if err != nil {
		return nil, err
	}
	base, err := r.worktree("wt-base", baseSHA)
	if err != nil {
		return nil, err
	}
	hf, ran, err := scanAll(head, extra)
	if err != nil {
		return nil, err
	}
	bf, _, err := scanAll(base, extra)
	if err != nil {
		return nil, err
	}
	known := policy.Baseline{}
	for _, f := range bf {
		known[f.Fingerprint()] = true
	}
	res := &Result{New: known.New(hf), Ran: ran, Commit: headSHA}
	if d, err := r.git("diff", "-U0", baseSHA, headSHA); err == nil {
		res.Surface, _ = diffmap.Parse(strings.NewReader(d))
	}
	return res, nil
}

// Full scans a whole branch. Findings are everything the engines report; state tracking is the store's job.
func Full(cloneURL, branch, user, token string, extra ...engines.Engine) (*Result, time.Duration, error) {
	start := time.Now()
	r, err := newRepo(user, token)
	if err != nil {
		return nil, 0, err
	}
	defer r.close()
	sha, err := r.fetch(cloneURL, branch, true)
	if err != nil {
		return nil, 0, err
	}
	wt, err := r.worktree("wt", sha)
	if err != nil {
		return nil, 0, err
	}
	fs, ran, err := scanAll(wt, extra)
	if err != nil {
		return nil, 0, err
	}
	return &Result{New: fs, Ran: ran, Commit: sha}, time.Since(start), nil
}

func rel(f engines.Finding, root string) engines.Finding {
	f.File = strings.TrimPrefix(strings.TrimPrefix(f.File, "file://"+root), root)
	f.File = strings.TrimPrefix(f.File, "/")
	return f
}
