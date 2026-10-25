// Package gitx is the only place scribex shells out to git. Every call neutralises repo-controlled
// execution hooks (fsmonitor, hooks, ext transports) and turns any failure into an error: never "no change".
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

var safeCfg = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.hooksPath=" + os.DevNull,
	"-c", "protocol.ext.allow=never",
	"-c", "core.pager=cat",
}

// Run executes `git -C root <args>` and returns stdout. A non-zero exit returns an error carrying stderr.
func Run(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append(append([]string{"-C", root}, safeCfg...), args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_EXTERNAL_DIFF=", "GIT_PAGER=cat")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 512 {
			msg = msg[:512]
		}
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, msg)
	}
	return out.Bytes(), nil
}

// ValidRef rejects refs that could be parsed as options or smuggle control characters.
func ValidRef(ref string) error {
	if ref == "" || len(ref) > 256 {
		return errors.New("gitx: empty or oversized ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("gitx: ref %q looks like an option", ref)
	}
	for _, r := range ref {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("gitx: ref %q has whitespace or control characters", ref)
		}
	}
	return nil
}

// ResolveCommit returns the full commit SHA for ref; callers must use the SHA, never the raw ref.
func ResolveCommit(root, ref string) (string, error) {
	if err := ValidRef(ref); err != nil {
		return "", err
	}
	out, err := Run(root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsShallow reports whether the clone is shallow (history-based diffs are then unreliable).
func IsShallow(root string) (bool, error) {
	out, err := Run(root, "rev-parse", "--is-shallow-repository")
	return strings.TrimSpace(string(out)) == "true", err
}
