package generate

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"scribex/pkg/gitx"
)

var prefixRe = regexp.MustCompile(`^[a-z0-9][a-z0-9/_-]{0,40}$`)

// PublishResult describes what Publish created.
type PublishResult struct {
	Branch     string
	Commit     string
	Pushed     bool
	CompareURL string
}

// Publish commits exactly the written files to a new branch <prefix>/docs-<shortsha>. The engine never
// opens a PR and never touches the branch it started on. Hooks are neutralised by gitx.
func Publish(root, prefix, runID string, written []string, push bool) (*PublishResult, error) {
	if !prefixRe.MatchString(prefix) {
		return nil, errors.New("invalid branch prefix")
	}
	if len(written) == 0 {
		return nil, errors.New("nothing to publish")
	}
	base, err := gitx.Run(root, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return nil, errors.New("detached HEAD: check out a branch first")
	}
	baseBranch := strings.TrimSpace(string(base))
	for _, k := range []string{"user.email", "user.name"} {
		if v, err := gitx.Run(root, "config", k); err != nil || strings.TrimSpace(string(v)) == "" {
			return nil, fmt.Errorf("git %s is not set", k)
		}
	}
	short, err := gitx.Run(root, "rev-parse", "--short=8", "HEAD")
	if err != nil {
		return nil, err
	}
	branch := prefix + "/docs-" + strings.TrimSpace(string(short))
	if err := gitx.ValidRef(branch); err != nil {
		return nil, err
	}
	if _, err := gitx.Run(root, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err == nil {
		return nil, fmt.Errorf("branch %s already exists", branch)
	}
	if _, err := gitx.Run(root, "checkout", "-q", "-b", branch); err != nil {
		return nil, err
	}
	args := append([]string{"add", "--"}, written...)
	if _, err := gitx.Run(root, args...); err != nil {
		return nil, err
	}
	msg := "docs: scribex generate " + runID + "\n\nAI-drafted documentation. Review before merging."
	if _, err := gitx.Run(root, "commit", "-q", "--no-verify", "-m", msg); err != nil {
		return nil, err
	}
	c, _ := gitx.Run(root, "rev-parse", "HEAD")
	res := &PublishResult{Branch: branch, Commit: strings.TrimSpace(string(c))}
	if remote, err := gitx.Run(root, "remote", "get-url", "origin"); err == nil {
		res.CompareURL = compareURL(strings.TrimSpace(string(remote)), baseBranch, branch)
	}
	if push {
		if _, err := gitx.Run(root, "push", "-u", "origin", branch); err != nil {
			return res, err
		}
		res.Pushed = true
	}
	return res, nil
}

var scpRe = regexp.MustCompile(`^[A-Za-z0-9._-]+@([A-Za-z0-9.-]+):(.+)$`)

// compareURL builds a PR/MR compare link for github.com and gitlab hosts, "" otherwise.
func compareURL(remote, base, branch string) string {
	var host, path string
	if m := scpRe.FindStringSubmatch(remote); m != nil {
		host, path = m[1], m[2]
	} else if u, err := url.Parse(remote); err == nil && (u.Scheme == "https" || u.Scheme == "ssh") {
		host, path = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" {
		return ""
	}
	switch {
	case host == "github.com":
		return fmt.Sprintf("https://github.com/%s/compare/%s...%s?expand=1", path, url.PathEscape(base), url.PathEscape(branch))
	case strings.Contains(host, "gitlab"):
		return fmt.Sprintf("https://%s/%s/-/merge_requests/new?merge_request[source_branch]=%s&merge_request[target_branch]=%s", host, path, url.QueryEscape(branch), url.QueryEscape(base))
	}
	return ""
}
