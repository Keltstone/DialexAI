package ai

import (
	"context"
	"fmt"
	"strings"

	"sentinix/pkg/engines"
)

// DraftPatch asks a model for a unified diff fixing the finding. The diff is only a draft:
// it must touch the finding's file alone and stay small, and is applied by Verify, never by this function.
func DraftPatch(ctx context.Context, m Model, policy string, f engines.Finding, snippet string) (string, error) {
	if !Allowed(policy, m) {
		return "", fmt.Errorf("model %s is not allowed by the project's model policy", m.Name())
	}
	if policy == "cloud-redacted" {
		redacted, err := engines.RedactSafe(snippet)
		if err != nil {
			return "", fmt.Errorf("REDACTION_UNAVAILABLE: %w", err)
		}
		snippet = redacted
	}

	untrustedData := fmt.Sprintf("Rule: %s (%s)\nFile: %s\nSnippet:\n%s", f.Rule, f.Message, f.File, snippet)
	user := fmt.Sprintf("Fix the issue described in the untrusted data below with the smallest change.\nReply with a unified diff only (--- a/%s, +++ b/%s), no prose.\n%s",
		f.File, f.File, untrusted(untrustedData))
	out, err := m.Complete(ctx, sysRules, user)
	if err != nil {
		return "", err
	}
	return CheckPatch(out, f.File)
}

// CheckPatch strips code fences and rejects diffs that touch other files or are oversized.
func CheckPatch(out, file string) (string, error) {
	out = strings.TrimSpace(out)
	if i := strings.Index(out, "```"); i >= 0 {
		out = strings.TrimSpace(strings.TrimPrefix(out[i+3:], "diff"))
		if j := strings.Index(out, "```"); j >= 0 {
			out = out[:j]
		}
	}
	if len(out) > 20000 || !strings.Contains(out, "@@") {
		return "", fmt.Errorf("model did not return a usable diff")
	}
	plus := 0
	for _, l := range strings.Split(out, "\n") {
		// Extended git headers can rename, copy, chmod, add symlinks or carry binary payloads: none are a plain edit.
		for _, bad := range []string{"rename ", "copy ", "similarity ", "dissimilarity ", "old mode", "new mode", "new file mode", "deleted file mode", "GIT binary patch", "Binary files"} {
			if strings.HasPrefix(l, bad) {
				return "", fmt.Errorf("patch uses %q, only a plain edit of %q is allowed", strings.TrimSpace(bad), file)
			}
		}
		if p, ok := strings.CutPrefix(l, "diff --git "); ok && p != "a/"+file+" b/"+file {
			return "", fmt.Errorf("patch touches %q, not %q", p, file)
		}
		if p, ok := strings.CutPrefix(l, "+++ "); ok {
			plus++
			if strings.TrimSpace(p) != "b/"+file {
				return "", fmt.Errorf("patch touches %q, not %q", p, file)
			}
		}
		if p, ok := strings.CutPrefix(l, "--- "); ok && strings.TrimSpace(p) != "a/"+file {
			return "", fmt.Errorf("patch touches %q, not %q", p, file)
		}
	}
	if plus != 1 {
		return "", fmt.Errorf("patch must edit exactly one file, found %d", plus)
	}
	return out + "\n", nil
}
