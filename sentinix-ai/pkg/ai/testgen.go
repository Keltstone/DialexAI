package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sentinix/pkg/engines"
)

type Test struct {
	Path    string `json:"path"` // relative, inside the repo
	Content string `json:"content"`
}

// ProposeTest asks a model for a security regression test for the finding.
func ProposeTest(ctx context.Context, m Model, policy string, f engines.Finding, snippet, framework string) (*Test, error) {
	if !Allowed(policy, m) {
		return nil, fmt.Errorf("model %s is not allowed by the project's model policy", m.Name())
	}
	if policy == "cloud-redacted" {
		redacted, err := engines.RedactSafe(snippet)
		if err != nil {
			return nil, fmt.Errorf("REDACTION_UNAVAILABLE: %w", err)
		}
		snippet = redacted
	}

	untrustedData := fmt.Sprintf("Rule: %s (%s)\nFile: %s\nSnippet:\n%s", f.Rule, f.Message, f.File, snippet)
	user := fmt.Sprintf("Write one %s test proving the issue in the untrusted data below: it must FAIL while the issue exists and PASS once fixed.\n"+
		`Reply {"path":"relative/path/to/test","content":"full test file"}.`+"\n%s", framework, untrusted(untrustedData))
	out, err := m.Complete(ctx, sysRules, user)
	if err != nil {
		return nil, err
	}
	js, ok := extractJSON(out)
	var t Test
	if !ok || json.Unmarshal([]byte(js), &t) != nil {
		return nil, fmt.Errorf("model did not return a test")
	}
	if err := safePath(t.Path); err != nil || t.Content == "" || len(t.Content) > 50000 {
		return nil, fmt.Errorf("unusable test from model")
	}
	return &t, nil
}

func safePath(p string) error {
	c := filepath.Clean(p)
	if p == "" || filepath.IsAbs(p) || c == ".." || strings.HasPrefix(c, "../") || strings.HasPrefix(c, ".git") {
		return fmt.Errorf("unsafe path %q", p)
	}
	return nil
}

// Runner executes a command in dir and reports success. Production runners must sandbox (no network).
type Runner func(ctx context.Context, dir string, cmd []string) error

// HostRunner runs on the host with a minimal environment. Tests only; use the sandbox runner for model-written code.
func HostRunner(ctx context.Context, dir string, cmd []string) error {
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Dir, c.Env = dir, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}
	return c.Run()
}

// Verify is the acceptance gate for generated tests: the test must fail on the vulnerable tree and
// pass once the patch is applied. Neither input tree is modified; both are copied first.
func Verify(ctx context.Context, run Runner, tree string, t Test, patch string, cmd []string) error {
	if err := safePath(t.Path); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	work := func(patched bool) (string, error) {
		d, err := os.MkdirTemp("", "sentinix-verify-*")
		if err != nil {
			return "", err
		}
		if out, err := exec.Command("cp", "-R", tree+"/.", d).CombinedOutput(); err != nil {
			return d, fmt.Errorf("copy: %v: %s", err, out)
		}
		if patched {
			c := exec.Command("git", "apply", "--whitespace=nowarn", "-")
			c.Dir, c.Stdin = d, strings.NewReader(patch)
			if out, err := c.CombinedOutput(); err != nil {
				return d, fmt.Errorf("patch does not apply: %s", out)
			}
		}

		root, err := os.OpenRoot(d)
		if err != nil {
			return d, err
		}
		defer root.Close()

		// Refuse to write through symlinks
		cleanPath := filepath.Clean(filepath.ToSlash(t.Path))
		parts := strings.Split(cleanPath, "/")
		current := ""
		for i, part := range parts {
			if current == "" {
				current = part
			} else {
				current = current + "/" + part
			}
			if fi, err := root.Lstat(current); err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					return d, fmt.Errorf("refusing to write through symlink %q", current)
				}
			}
			if i < len(parts)-1 {
				if err := root.MkdirAll(current, 0o755); err != nil {
					return d, err
				}
			}
		}
		if err := root.WriteFile(cleanPath, []byte(t.Content), 0o644); err != nil {
			return d, err
		}
		return d, nil
	}
	bad, err := work(false)
	defer os.RemoveAll(bad)
	if err != nil {
		return err
	}
	if run(ctx, bad, cmd) == nil {
		return fmt.Errorf("rejected: test passes on the vulnerable code, so it proves nothing")
	}
	fixed, err := work(true)
	defer os.RemoveAll(fixed)
	if err != nil {
		return err
	}
	if err := run(ctx, fixed, cmd); err != nil {
		return fmt.Errorf("rejected: test still fails after the patch: %v", err)
	}
	return nil
}
