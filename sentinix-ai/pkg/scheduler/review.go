// Package scheduler runs scans: PR/MR reviews (shallow-fetch head and base, report only what the change
// introduced) and full scans of a branch, plus cron timing for scheduled runs.
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/policy"
)

var (
	FetchTimeout          = 5 * time.Minute
	MaxFetchBytes int64   = 500 << 20 // 500 MiB
)

type Result struct {
	New     []engines.Finding
	Ran     []string
	Surface []diffmap.Item
	Commit  string
	// Components is the SBOM of a full scan.
	Components []advisories.Component
	Skipped    []string
}

func isConfiguredHost(rawURL, configuredHost string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil // local directory/path (used in tests)
	}
	host := u.Hostname()
	if configuredHost == "" {
		return fmt.Errorf("clone URL host %q: no configured host provided", host)
	}
	ch := configuredHost
	if strings.Contains(ch, "://") {
		if pu, err := url.Parse(ch); err == nil && pu.Hostname() != "" {
			ch = pu.Hostname()
		}
	}
	if strings.EqualFold(host, ch) {
		return nil
	}
	return fmt.Errorf("clone URL host %q differs from configured host %q", host, ch)
}

// repo is a throwaway git dir. The token is used only for fetches and never reaches a scan process.
type repo struct {
	dir, user, token string
	lastFetchArgs    []string
}

func newRepo(ctx context.Context, user, token string) (*repo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "sentinix-*")
	if err != nil {
		return nil, err
	}
	r := &repo{dir: dir, user: user, token: token}
	if _, err := r.gitContext(ctx, "init", "-q"); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if token != "" {
		credHelper := filepath.Join(dir, "cred-helper.sh")
		script := "#!/bin/sh\n" +
			"if [ $# -gt 0 ]; then\n" +
			"  case \"$1\" in\n" +
			"    *[Uu]sername*) echo \"$SENTINIX_GIT_USER\" ;;\n" +
			"    *[Pp]assword*) echo \"$SENTINIX_GIT_TOKEN\" ;;\n" +
			"    *) echo \"$SENTINIX_GIT_TOKEN\" ;;\n" +
			"  esac\n" +
			"  exit 0\n" +
			"fi\n" +
			"echo \"username=$SENTINIX_GIT_USER\"\n" +
			"echo \"password=$SENTINIX_GIT_TOKEN\"\n"
		if err := os.WriteFile(credHelper, []byte(script), 0o700); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
		if _, err := r.gitContext(ctx, "config", "credential.helper", "!"+credHelper); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
	}
	return r, nil
}

func (r *repo) close() { os.RemoveAll(r.dir) } // only findings leave the scan

func (r *repo) gitContext(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.dir}, args...)...)
	cmd.WaitDelay = 100 * time.Millisecond
	credHelper := filepath.Join(r.dir, "cred-helper.sh")
	cmd.Env = append(os.Environ(),
		"SENTINIX_GIT_USER="+r.user,
		"SENTINIX_GIT_TOKEN="+r.token,
		"GIT_ASKPASS="+credHelper,
		"SSH_ASKPASS="+credHelper,
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if r.token != "" {
		s = strings.ReplaceAll(s, r.token, "***")
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, s)
	}
	return s, nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// fetch brings one ref in at depth 1 and returns its commit. Tokens are sent only over https.
func (r *repo) fetch(ctx context.Context, rawURL, ref string, auth bool) (string, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()

	maxBytes := MaxFetchBytes

	done := make(chan struct{})
	monitorDone := make(chan struct{})
	var exceeded atomic.Bool
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if sz, _ := dirSize(r.dir); sz > maxBytes {
					exceeded.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	var args []string
	if !auth {
		args = append(args, "-c", "credential.helper=")
	}
	args = append(args, "-c", fmt.Sprintf("pack.packSizeLimit=%d", maxBytes), "fetch", "--depth=1", "--no-tags", rawURL, ref)
	r.lastFetchArgs = args
	_, err := r.gitContext(fetchCtx, args...)
	close(done)
	<-monitorDone

	if exceeded.Load() {
		return "", fmt.Errorf("fetch size exceeded limit (%d bytes)", maxBytes)
	}
	if err != nil {
		if sz, _ := dirSize(r.dir); sz > maxBytes {
			return "", fmt.Errorf("fetch size exceeded limit (%d bytes)", sz)
		}
		return "", err
	}
	if sz, _ := dirSize(r.dir); sz > maxBytes {
		return "", fmt.Errorf("fetch size exceeded limit (%d bytes)", sz)
	}
	return r.gitContext(fetchCtx, "rev-parse", "FETCH_HEAD")
}

func (r *repo) fetchDeepen(ctx context.Context, rawURL, ref string, auth bool, depth int) error {
	fetchCtx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()

	maxBytes := MaxFetchBytes

	done := make(chan struct{})
	monitorDone := make(chan struct{})
	var exceeded atomic.Bool
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if sz, _ := dirSize(r.dir); sz > maxBytes {
					exceeded.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	var args []string
	if !auth {
		args = append(args, "-c", "credential.helper=")
	}
	args = append(args, "-c", fmt.Sprintf("pack.packSizeLimit=%d", maxBytes), "fetch", fmt.Sprintf("--deepen=%d", depth), "--no-tags", rawURL, ref)
	r.lastFetchArgs = args
	_, err := r.gitContext(fetchCtx, args...)
	close(done)
	<-monitorDone

	if exceeded.Load() {
		return fmt.Errorf("fetch size exceeded limit (%d bytes)", maxBytes)
	}
	if err != nil {
		if sz, _ := dirSize(r.dir); sz > maxBytes {
			return fmt.Errorf("fetch size exceeded limit (%d bytes)", sz)
		}
		return err
	}
	if sz, _ := dirSize(r.dir); sz > maxBytes {
		return fmt.Errorf("fetch size exceeded limit (%d bytes)", sz)
	}
	return nil
}

func (r *repo) worktree(ctx context.Context, name, sha string) (string, error) {
	p := filepath.Join(r.dir, name)
	_, err := r.gitContext(ctx, "worktree", "add", "--detach", p, sha)
	return p, err
}

func scanAll(ctx context.Context, root string, extra []engines.Engine) ([]engines.Finding, []string, []string, error) {
	var all []engines.Finding
	var ran []string
	var skipped []string
	occ := map[string]int{}

	for _, e := range engines.Bundled(extra...) {
		fs, err := e.Scan(ctx, root)
		if err != nil {
			return nil, nil, nil, err // a broken engine must not look like a clean scan
		}
		ran = append(ran, e.Name())
		if se, ok := e.(interface{ Skipped() []string }); ok {
			skipped = append(skipped, se.Skipped()...)
		}
		for _, f := range fs {
			if f.Occurrence == 0 {
				m := f.Match
				if m == "" {
					m = f.Evidence
				}
				h := sha256.Sum256([]byte(m))
				key := f.Rule + ":" + hex.EncodeToString(h[:])
				f.Occurrence = occ[key]
				occ[key]++
			}
			all = append(all, rel(f, root))
		}
	}
	return all, ran, skipped, nil
}

// Review scans ev.HeadSHA and ev.BaseRef.
func Review(ctx context.Context, ev *githost.Event, user, token string, extra ...engines.Engine) (*Result, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context required")
	}
	if ev == nil || ev.CloneURL == "" {
		return nil, fmt.Errorf("invalid event")
	}
	if err := isConfiguredHost(ev.CloneURL, ev.ConfiguredHost); err != nil {
		return nil, err
	}
	if err := isConfiguredHost(ev.BaseCloneURL, ev.ConfiguredHost); err != nil {
		return nil, err
	}
	r, err := newRepo(ctx, user, token)
	if err != nil {
		return nil, err
	}
	defer r.close()

	// Fork heads are public clones: no token is sent to the fork's host.
	headRef := ev.HeadSHA
	if len(headRef) < 40 {
		headRef = ev.HeadRef
	}
	if headRef == "" {
		return nil, fmt.Errorf("missing head ref")
	}
	headSHA, err := r.fetch(ctx, ev.CloneURL, headRef, !ev.Fork)
	if err != nil {
		return nil, err
	}

	baseRefToFetch := ev.BaseRef
	if ev.BaseSHA != "" {
		baseRefToFetch = ev.BaseSHA
	}
	if baseRefToFetch == "" {
		baseRefToFetch = "main"
	}
	baseSHA, err := r.fetch(ctx, ev.BaseCloneURL, baseRefToFetch, true)
	if err != nil {
		return nil, err
	}

	baseCommit := baseSHA
	if ev.BaseSHA != "" {
		baseCommit = ev.BaseSHA
	} else if mb, err := r.gitContext(ctx, "merge-base", headSHA, baseSHA); err == nil && mb != "" {
		baseCommit = mb
	} else {
		// Deepen history to find merge base for GitLab/Bitbucket shallow fetches
		r.fetchDeepen(ctx, ev.CloneURL, headRef, !ev.Fork, 50)
		r.fetchDeepen(ctx, ev.BaseCloneURL, baseRefToFetch, true, 50)
		if mb, err := r.gitContext(ctx, "merge-base", headSHA, baseSHA); err == nil && mb != "" {
			baseCommit = mb
		}
	}

	head, err := r.worktree(ctx, "wt-head", headSHA)
	if err != nil {
		return nil, err
	}
	if sz, _ := dirSize(head); sz > MaxFetchBytes {
		return nil, fmt.Errorf("working-tree size exceeded limit (%d bytes)", sz)
	}
	base, err := r.worktree(ctx, "wt-base", baseCommit)
	if err != nil {
		return nil, err
	}
	if sz, _ := dirSize(base); sz > MaxFetchBytes {
		return nil, fmt.Errorf("working-tree size exceeded limit (%d bytes)", sz)
	}
	hf, ran, skipped, err := scanAll(ctx, head, extra)
	if err != nil {
		return nil, err
	}
	bf, _, _, err := scanAll(ctx, base, extra)
	if err != nil {
		return nil, err
	}

	// detect renames with git diff -M so a pure rename is not new
	renames := map[string]string{} // newPath -> oldPath
	if diffStatus, err := r.gitContext(ctx, "diff", "-M", "--name-status", baseCommit, headSHA); err == nil {
		for _, l := range strings.Split(diffStatus, "\n") {
			fields := strings.Fields(l)
			if len(fields) >= 3 && strings.HasPrefix(fields[0], "R") {
				renames[fields[2]] = fields[1]
			}
		}
	}

	changedFiles := map[string]bool{}
	if diffFiles, err := r.gitContext(ctx, "diff", "--name-only", baseCommit, headSHA); err == nil {
		for _, f := range strings.Split(diffFiles, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				changedFiles[f] = true
			}
		}
	}
	var diffScopedSkipped []string
	seenSkipped := map[string]bool{}
	for cf := range changedFiles {
		for _, sk := range skipped {
			if cf == sk || strings.HasPrefix(cf, sk+"/") {
				if !seenSkipped[cf] {
					seenSkipped[cf] = true
					diffScopedSkipped = append(diffScopedSkipped, cf)
				}
				break
			}
		}
	}
	sort.Strings(diffScopedSkipped)
	skipped = diffScopedSkipped

	known := policy.Baseline{}
	for _, f := range bf {
		known[f.Fingerprint()] = true
		for newP, oldP := range renames {
			if f.File == oldP {
				rf := f
				rf.File = newP
				known[rf.Fingerprint()] = true
			}
		}
	}

	res := &Result{New: known.New(hf), Ran: ran, Commit: headSHA, Skipped: skipped}
	if d, err := r.gitContext(ctx, "diff", "-U0", baseCommit, headSHA); err == nil {
		res.Surface, _ = diffmap.Parse(strings.NewReader(d))
	}
	return res, nil
}

// Full scans a whole branch. Findings are everything the engines report; state tracking is the store's job.
func Full(ctx context.Context, cloneURL, branch, user, token, configuredHost string, extra ...engines.Engine) (*Result, time.Duration, error) {
	if ctx == nil {
		return nil, 0, fmt.Errorf("context required")
	}
	if cloneURL == "" || branch == "" {
		return nil, 0, fmt.Errorf("invalid arguments to Full")
	}
	if err := isConfiguredHost(cloneURL, configuredHost); err != nil {
		return nil, 0, err
	}
	start := time.Now()
	r, err := newRepo(ctx, user, token)
	if err != nil {
		return nil, 0, err
	}
	defer r.close()
	sha, err := r.fetch(ctx, cloneURL, branch, true)
	if err != nil {
		return nil, 0, err
	}
	wt, err := r.worktree(ctx, "wt", sha)
	if err != nil {
		return nil, 0, err
	}
	if sz, _ := dirSize(wt); sz > MaxFetchBytes {
		return nil, 0, fmt.Errorf("working-tree size exceeded limit (%d bytes)", sz)
	}
	fs, ran, skipped, err := scanAll(ctx, wt, extra)
	if err != nil {
		return nil, 0, err
	}
	return &Result{New: fs, Ran: ran, Commit: sha, Components: advisories.Components(wt), Skipped: skipped}, time.Since(start), nil
}

func rel(f engines.Finding, root string) engines.Finding {
	f.File = strings.TrimPrefix(strings.TrimPrefix(f.File, "file://"+root), root)
	f.File = strings.TrimPrefix(f.File, "/")
	return f
}

// Snippet returns the lines around file:line from the tip of branch, for AI prompts. Only that snippet leaves the scan.
func Snippet(ctx context.Context, cloneURL, branch, user, token, configuredHost, file string, line int) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("context required")
	}
	if cloneURL == "" || branch == "" || file == "" {
		return "", fmt.Errorf("invalid arguments to Snippet")
	}
	if err := isConfiguredHost(cloneURL, configuredHost); err != nil {
		return "", err
	}
	if line < 1 {
		return "", fmt.Errorf("line %d must be >= 1", line)
	}
	r, err := newRepo(ctx, user, token)
	if err != nil {
		return "", err
	}
	defer r.close()
	if _, err := r.fetch(ctx, cloneURL, branch, true); err != nil {
		return "", err
	}
	src, err := r.gitContext(ctx, "show", "FETCH_HEAD:"+file)
	if err != nil {
		return "", err
	}
	ls := strings.Split(src, "\n")
	lo, hi := max(line-16, 0), min(line+15, len(ls))
	if lo >= hi || lo >= len(ls) {
		return "", fmt.Errorf("line %d is outside %s", line, file)
	}
	return strings.Join(ls[lo:hi], "\n"), nil
}

// WithTree checks out the tip of branch and calls fn with the directory; it is deleted afterwards.
func WithTree(ctx context.Context, cloneURL, branch, user, token, configuredHost string, fn func(dir string) error) error {
	if ctx == nil {
		return fmt.Errorf("context required")
	}
	if cloneURL == "" || branch == "" || fn == nil {
		return fmt.Errorf("invalid arguments to WithTree")
	}
	if err := isConfiguredHost(cloneURL, configuredHost); err != nil {
		return err
	}
	r, err := newRepo(ctx, user, token)
	if err != nil {
		return err
	}
	defer r.close()
	sha, err := r.fetch(ctx, cloneURL, branch, true)
	if err != nil {
		return err
	}
	wt, err := r.worktree(ctx, "wt", sha)
	if err != nil {
		return err
	}
	return fn(wt)
}
