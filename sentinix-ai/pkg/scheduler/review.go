// Package scheduler runs PR/MR reviews: shallow-fetch head and base, scan both, report only what the change introduced.
package scheduler

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/policy"
)

type Result struct {
	New []engines.Finding
	Ran []string
}

// Review scans ev.HeadSHA and ev.BaseRef without ever handing the token to a scan process.
// ponytail: engines run on the host, not in a per-scan container; add sandboxing before scanning untrusted repos with executable plugins.
func Review(ev *githost.Event, user, token string, extra ...engines.Engine) (*Result, error) {
	dir, err := os.MkdirTemp("", "sentinix-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // only findings leave this function

	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		s := strings.TrimSpace(string(out))
		if token != "" {
			s = strings.ReplaceAll(s, token, "***")
		}
		if err != nil {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, s)
		}
		return s, nil
	}
	fetch := func(_, url, ref string, auth bool) (string, error) {
		if auth && token != "" {
			url = withAuth(url, user, token)
		}
		if _, err := git("fetch", "--depth=1", "--no-tags", url, ref); err != nil {
			return "", err
		}
		return git("rev-parse", "FETCH_HEAD")
	}

	if _, err := git("init", "-q"); err != nil {
		return nil, err
	}
	// Fork heads are public clones: no token is sent to the fork's host.
	headRef := ev.HeadSHA
	if len(headRef) < 40 {
		headRef = ev.HeadRef
	}
	headSHA, err := fetch("head", ev.CloneURL, headRef, !ev.Fork)
	if err != nil {
		return nil, err
	}
	baseSHA, err := fetch("base", ev.BaseCloneURL, ev.BaseRef, true)
	if err != nil {
		return nil, err
	}
	wt := map[string]string{"head": filepath.Join(dir, "wt-head"), "base": filepath.Join(dir, "wt-base")}
	for k, sha := range map[string]string{"head": headSHA, "base": baseSHA} {
		if _, err := git("worktree", "add", "--detach", wt[k], sha); err != nil {
			return nil, err
		}
	}

	res := &Result{}
	base := policy.Baseline{}
	for _, e := range engines.Bundled(extra...) {
		hf, err := e.Scan(wt["head"])
		if err != nil {
			return nil, err // a broken engine must not look like a clean scan
		}
		bf, err := e.Scan(wt["base"])
		if err != nil {
			return nil, err
		}
		res.Ran = append(res.Ran, e.Name())
		for _, f := range bf {
			base[rel(f, wt["base"]).Fingerprint()] = true
		}
		for _, f := range hf {
			res.New = append(res.New, rel(f, wt["head"]))
		}
	}
	res.New = base.New(res.New)
	return res, nil
}

func rel(f engines.Finding, root string) engines.Finding {
	f.File = strings.TrimPrefix(strings.TrimPrefix(f.File, "file://"+root), root)
	f.File = strings.TrimPrefix(f.File, "/")
	return f
}

func withAuth(raw, user, token string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return raw // never put a token on a non-https URL
	}
	u.User = url.UserPassword(user, token)
	return u.String()
}
