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
		snippet = engines.Redact(snippet)
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
	for _, l := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(l, "+++ "); ok {
			if strings.TrimPrefix(strings.TrimSpace(p), "b/") != file {
				return "", fmt.Errorf("patch touches %q, not %q", p, file)
			}
		}
		if p, ok := strings.CutPrefix(l, "--- "); ok && !strings.HasSuffix(strings.TrimSpace(p), file) && strings.TrimSpace(p) != "/dev/null" {
			return "", fmt.Errorf("patch touches %q, not %q", p, file)
		}
	}
	return out + "\n", nil
}
