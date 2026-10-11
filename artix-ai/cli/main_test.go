package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"artix/pkg/audit"
	"artix/pkg/knowledge"
	"artix/pkg/policy"
)

func TestRunCLI_VersionAndHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// 1. Version command
	code := RunCLI("", nil, []string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0 for version, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Artix AI version") {
		t.Errorf("expected version output, got: %s", stdout.String())
	}

	// 2. Help command
	stdout.Reset()
	code = RunCLI("", nil, []string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0 for help, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("expected usage output, got: %s", stdout.String())
	}

	// 3. Empty args prints usage
	stdout.Reset()
	code = RunCLI("", nil, []string{}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0 for empty args, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("expected usage output, got: %s", stdout.String())
	}
}

func TestRunCLI_PersonaAndUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// 1. Persona list
	code := RunCLI("", nil, []string{"persona"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0 for persona, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Available SWE Personas") {
		t.Errorf("expected persona listing, got: %s", stdout.String())
	}

	// 2. Unknown command -> exit 1
	stdout.Reset()
	stderr.Reset()
	code = RunCLI("", nil, []string{"unknown_foo_bar"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown command, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Unknown command") {
		t.Errorf("expected unknown command error on stderr, got: %s", stderr.String())
	}
}

func TestRunCLI_MergeCommandValidation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// 1. Missing flags on verify-approval -> error mentioning required flags
	code := RunCLI("", nil, []string{"verify-approval"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for verify-approval without arguments, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--pr") || !strings.Contains(stderr.String(), "--sha") || !strings.Contains(stderr.String(), "--spec") {
		t.Errorf("expected stderr mentioning required flags, got: %s", stderr.String())
	}

	// 2. Invoking deprecated merge command -> error noting deprecation
	stdout.Reset()
	stderr.Reset()
	code = RunCLI("", nil, []string{"merge"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for deprecated merge command, got %d", code)
	}
	if !strings.Contains(stderr.String(), "deprecated") || !strings.Contains(stderr.String(), "verify-approval") {
		t.Errorf("expected stderr mentioning deprecation and verify-approval, got: %s", stderr.String())
	}
}

func TestRunCLI_MergeCommand_AcceptsVerdictHashFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// 1. When --verdict-hash is supplied to verify-approval, verify it parses and does not fail with unknown flag
	code := RunCLI("", nil, []string{"verify-approval", "--pr", "1", "--sha", "abcdef123456", "--spec", "S-01", "--verdict-hash", "hash1234"}, &stdout, &stderr)
	if strings.Contains(stderr.String(), "flag provided but not defined: -verdict-hash") {
		t.Fatalf("runVerifyApproval failed to recognize --verdict-hash flag: %s", stderr.String())
	}
	_ = code

	// 2. Deprecated merge alias with verdict hash returns deprecation error
	stdout.Reset()
	stderr.Reset()
	code = RunCLI("", nil, []string{"merge", "--pr", "1", "--sha", "abcdef123456", "--verdict-hash", "hash1234"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "deprecated") {
		t.Fatalf("expected merge alias to exit 1 with deprecation error, got code=%d stderr=%s", code, stderr.String())
	}
}

func TestR13_5_VerifyApprovalCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// 1. Missing flags -> error mentioning required flags
	code := RunCLI("", nil, []string{"verify-approval"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for verify-approval without arguments, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--pr") || !strings.Contains(stderr.String(), "--sha") || !strings.Contains(stderr.String(), "--spec") {
		t.Errorf("expected stderr mentioning required flags, got: %s", stderr.String())
	}

	// 2. Flags accepted
	stdout.Reset()
	stderr.Reset()
	code = RunCLI("", nil, []string{"verify-approval", "--pr", "1", "--sha", "abcdef123456", "--spec", "S-01", "--verdict-hash", "hash1234"}, &stdout, &stderr)
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("runVerifyApproval failed to recognize flags: %s", stderr.String())
	}
}

func TestR13_5_MergeCommand_Deprecated(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// Plain CLI invocation of 'artix merge' must error as deprecated
	code := RunCLI("", nil, []string{"merge"}, &stdout, &stderr)
	if code == 0 {
		t.Errorf("expected non-zero exit code for deprecated 'artix merge'")
	}
	if !strings.Contains(stderr.String(), "deprecated") || !strings.Contains(stderr.String(), "verify-approval") {
		t.Errorf("expected stderr to state 'artix merge' is deprecated and suggest 'artix verify-approval', got: %s", stderr.String())
	}

	// JSON invocation of 'artix merge' must return JSON error noting deprecation
	stdout.Reset()
	stderr.Reset()
	code = RunCLI("", nil, []string{"merge", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Errorf("expected non-zero exit code for deprecated 'artix merge --json'")
	}
	if !strings.Contains(stdout.String(), "deprecated") && !strings.Contains(stderr.String(), "deprecated") {
		t.Errorf("expected JSON/stderr output to mention deprecation, got stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestRunCLI_KnowledgeRatify(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup CODEOWNERS file in tmpDir
	codeownersContent := "* @alice @bob\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "CODEOWNERS"), []byte(codeownersContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a proposed Knowledge Item in .artix/knowledge/
	kStore := knowledge.NewStore(tmpDir)
	ki := &knowledge.KnowledgeItem{
		ID:           "ki-cli-01",
		Title:        "Memory Safety Invariant",
		Category:     knowledge.CategoryArchitecture,
		Context:      "Handler pool allocation",
		Breakthrough: "Use sync.Pool",
		Status:       "proposed",
	}
	if err := kStore.Save(ki); err != nil {
		t.Fatalf("failed to save proposed KI: %v", err)
	}

	var stdout, stderr bytes.Buffer

	// 1. Unauthorized approver must fail
	code := RunCLI(tmpDir, nil, []string{"knowledge", "ratify", "ki-cli-01", "--approver", "mallory"}, &stdout, &stderr)
	if code == 0 {
		t.Errorf("expected failure when non-CODEOWNER ratifies KI, got exit code 0")
	}
	if !strings.Contains(stderr.String(), "CODEOWNERS") && !strings.Contains(stderr.String(), "not authorized") {
		t.Errorf("expected unauthorized error on stderr, got: %s", stderr.String())
	}

	// 2. Legitimate CODEOWNER ratifies KI -> success
	stdout.Reset()
	stderr.Reset()
	code = RunCLI(tmpDir, nil, []string{"knowledge", "ratify", "ki-cli-01", "--approver", "alice"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for legitimate ratification, got %d: %s", code, stderr.String())
	}

	// Verify KI is now active in store
	updated, err := kStore.Get("ki-cli-01")
	if err != nil {
		t.Fatalf("failed to retrieve updated KI: %v", err)
	}
	if !updated.IsActive() || updated.Status != "active" {
		t.Errorf("expected updated KI to be active, got status=%q isActive=%v", updated.Status, updated.IsActive())
	}
	if updated.ApprovedBy != "alice" {
		t.Errorf("expected ApprovedBy=alice, got %q", updated.ApprovedBy)
	}
}

func TestRunCLI_Code_DisjointModelFamilies_Rejection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create story spec in docs/specs/
	specsDir := filepath.Join(tmpDir, "docs", "specs")
	_ = os.MkdirAll(specsDir, 0755)
	specPath := filepath.Join(specsDir, "STORY-001.md")
	specContent := `# Spec STORY-001: Implement Feature

User Story: As a user I want a feature

## Acceptance Criteria
- [Scenario 1] Given state, When action, Then success

## Verification Commands
` + "```bash\necho ok\n```\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Policy enforcing disjoint model families
	policyFile := filepath.Join(tmpDir, "policy.json")
	pol := policy.Policy{
		Reviewer: policy.ReviewerPolicyConfig{
			EnforceDisjointModelFamilies: true,
		},
		AllowedTestCommands: []string{"echo ok"},
	}
	data, _ := json.Marshal(pol)
	_ = os.WriteFile(policyFile, data, 0644)
	key := "policy-key"
	policy.SetTrustedKey("test-key", key)
	_ = policy.SignPolicyFile(policyFile, key)
	policy.SetDefaultPolicyPath(policyFile)
	defer func() {
		policy.SetDefaultPolicyPath("/etc/artix/policy.json")
		policy.ResetCache()
	}()

	t.Setenv("ANTHROPIC_API_KEY", "mock-anthropic-key")

	var stdout, stderr bytes.Buffer
	// Both coder and critic are set to same provider "anthropic"
	code := RunCLI(tmpDir, nil, []string{
		"code",
		"--provider", "anthropic",
		"--model", "claude-3-5-sonnet",
		"--review-provider", "anthropic",
		"--review-model", "claude-3-5-sonnet",
		"--confirm-tests",
		specPath,
	}, &stdout, &stderr)

	if code == 0 {
		t.Errorf("expected failure when coder and critic share same model family under EnforceDisjointModelFamilies")
	}
	if !strings.Contains(stderr.String(), "disjoint model families") && !strings.Contains(stderr.String(), "critic model family") {
		t.Errorf("expected error mentioning disjoint model families, got: %s", stderr.String())
	}
}

func TestDisjointModelFamilies_RejectsEmptyModelFamily(t *testing.T) {
	tmpDir := t.TempDir()
	policy.ResetCache()
	defer policy.ResetCache()

	specPath := filepath.Join(tmpDir, "spec.json")
	specData := `{
		"id": "SPEC-EMPTY-FAM-01",
		"title": "Empty Family Test",
		"testCommands": ["echo ok"]
	}`
	_ = os.WriteFile(specPath, []byte(specData), 0644)

	policyFile := filepath.Join(tmpDir, "policy.json")
	pol := policy.Policy{
		Reviewer: policy.ReviewerPolicyConfig{
			EnforceDisjointModelFamilies: true,
		},
		AllowedTestCommands: []string{"echo ok"},
	}
	data, _ := json.Marshal(pol)
	_ = os.WriteFile(policyFile, data, 0644)
	key := "policy-key"
	policy.SetTrustedKey("test-key", key)
	_ = policy.SignPolicyFile(policyFile, key)
	policy.SetDefaultPolicyPath(policyFile)
	defer func() {
		policy.SetDefaultPolicyPath("/etc/artix/policy.json")
		policy.ResetCache()
	}()

	var stdout, stderr bytes.Buffer
	code := RunCLI(tmpDir, nil, []string{
		"code",
		"--confirm-tests",
		specPath,
	}, &stdout, &stderr)

	if code == 0 {
		t.Errorf("expected failure when model families cannot be resolved under EnforceDisjointModelFamilies")
	}
}

func TestAutonomyGateErrorText_SignedPolicyRequirement(t *testing.T) {
	tmpDir := t.TempDir()
	policy.ResetCache()
	defer policy.ResetCache()

	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("CI", "true")

	var stdout, stderr bytes.Buffer
	code := RunCLI(tmpDir, nil, []string{
		"code",
		"--autonomy", "autonomous",
		"dummy.md",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("expected failure for autonomous mode without verified signed policy in enterprise mode")
	}

	errOutput := stderr.String()
	if strings.Contains(errOutput, "ARTIX_ALLOW_AUTONOMOUS") {
		t.Errorf("CLI error message must NOT mention obsolete ARTIX_ALLOW_AUTONOMOUS, got: %s", errOutput)
	}
	if !strings.Contains(errOutput, "signed policy") && !strings.Contains(errOutput, "allowAutonomous") {
		t.Errorf("CLI error message must describe signed policy / allowAutonomous requirement, got: %s", errOutput)
	}

	// Verify README and USER_GUIDE do not contain ARTIX_ALLOW_AUTONOMOUS
	readmeBytes, err := os.ReadFile("../README.md")
	if err == nil {
		if strings.Contains(string(readmeBytes), "ARTIX_ALLOW_AUTONOMOUS") {
			t.Errorf("README.md must NOT mention ARTIX_ALLOW_AUTONOMOUS")
		}
	}

	userGuideBytes, err := os.ReadFile("../docs/USER_GUIDE.md")
	if err == nil {
		if strings.Contains(string(userGuideBytes), "ARTIX_ALLOW_AUTONOMOUS") {
			t.Errorf("USER_GUIDE.md must NOT mention ARTIX_ALLOW_AUTONOMOUS")
		}
		if !strings.Contains(string(userGuideBytes), "verified, cryptographically signed policy with allowAutonomous=true") {
			t.Errorf("USER_GUIDE.md must describe signed policy requirement: 'verified, cryptographically signed policy with allowAutonomous=true'")
		}
	} else {
		t.Fatalf("failed to read USER_GUIDE.md: %v", err)
	}
}

func TestCLI_KnowledgeEval_Command(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	ki := &knowledge.KnowledgeItem{
		ID:           "ki-eval-01",
		Title:        "Eval CLI Test",
		Category:     knowledge.CategoryArchitecture,
		Breakthrough: "Test item for eval command",
		Status:       "active",
		ApprovedBy:   "alice-codeowner",
	}
	if err := store.Save(ki); err != nil {
		t.Fatalf("failed to save seed KI: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := RunCLI(tmpDir, nil, []string{
		"knowledge",
		"eval",
		"ki-eval-01",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("expected artix knowledge eval to fail when unmeasured, got exit 0")
	}
	outStr := stdout.String() + stderr.String()
	if !strings.Contains(outStr, "ki-eval-01") || !strings.Contains(outStr, "unmeasured") {
		t.Errorf("expected eval output to contain KI ID and unmeasured status, got: %s", outStr)
	}
}

func TestCLI_ModelFamilyResolution_BedrockClaudeVsAnthropicClaude_AndOllamaDifferentModels(t *testing.T) {
	tmpDir := t.TempDir()
	policy.ResetCache()
	defer policy.ResetCache()

	specDir := filepath.Join(tmpDir, "docs", "specs")
	_ = os.MkdirAll(specDir, 0755)
	specPath := filepath.Join(specDir, "STORY-R2-01.md")
	_ = os.WriteFile(specPath, []byte(`---
id: STORY-R2-01
title: R2 Model Family Resolution
---
# User Story
Test model family resolution from model IDs
# Test Commands
- echo ok
`), 0644)

	// Set up signed policy with EnforceDisjointModelFamilies
	policyFile := filepath.Join(tmpDir, "policy.json")
	pol := policy.Policy{
		Reviewer: policy.ReviewerPolicyConfig{
			EnforceDisjointModelFamilies: true,
		},
		AllowedTestCommands: []string{"echo ok"},
	}
	data, _ := json.Marshal(pol)
	_ = os.WriteFile(policyFile, data, 0644)
	key := "test-key-r2-12345"
	policy.SetTrustedKey("corp-root", key)
	_ = policy.SignPolicyFile(policyFile, key)
	policy.SetDefaultPolicyPath(policyFile)
	defer func() {
		policy.SetDefaultPolicyPath("/etc/artix/policy.json")
		policy.ResetCache()
	}()

	t.Setenv("ANTHROPIC_API_KEY", "mock-key")
	t.Setenv("AWS_ACCESS_KEY_ID", "mock-key")

	// Case 1: Bedrock Coder running Claude vs Anthropic Critic running Claude -> MUST BE REJECTED (same family)
	var stdout1, stderr1 bytes.Buffer
	code1 := RunCLI(tmpDir, nil, []string{
		"code",
		"--provider", "bedrock",
		"--model", "claude-3-5-sonnet-20241022",
		"--review-provider", "anthropic",
		"--review-model", "claude-3-5-sonnet-20241022",
		"--confirm-tests",
		specPath,
	}, &stdout1, &stderr1)

	if code1 == 0 {
		t.Fatalf("expected Bedrock Claude paired with Anthropic Claude to be REJECTED under disjoint model families")
	}
	if !strings.Contains(stderr1.String(), "disjoint model families") {
		t.Errorf("expected error mentioning disjoint model families, got: %s", stderr1.String())
	}

	// Case 2: Ollama Coder running llama3.3 vs Ollama Critic running qwen2.5 -> MUST PASS disjoint gate (different families)
	var stdout2, stderr2 bytes.Buffer
	code2 := RunCLI(tmpDir, nil, []string{
		"code",
		"--provider", "ollama",
		"--model", "llama3.3",
		"--review-provider", "ollama",
		"--review-model", "qwen2.5",
		"--no-model-review", // skip live network request
		"--confirm-tests",
		specPath,
	}, &stdout2, &stderr2)
	_ = code2

	// Since --no-model-review is passed or families are disjoint, it should not fail on disjoint model family check
	if strings.Contains(stderr2.String(), "disjoint model families") {
		t.Errorf("expected Ollama llama3.3 vs qwen2.5 to be accepted as disjoint families, got error: %s", stderr2.String())
	}
}

func TestAudit_ResolvedModelFamilyPairInConvergenceEvent(t *testing.T) {
	tmpDir := t.TempDir()
	policy.ResetCache()
	defer policy.ResetCache()

	specDir := filepath.Join(tmpDir, "docs", "specs")
	_ = os.MkdirAll(specDir, 0755)
	specPath := filepath.Join(specDir, "STORY-R2-02.md")
	_ = os.WriteFile(specPath, []byte(`---
id: STORY-R2-02
title: R2 Audit Model Family
---
# User Story
Audit logging test
# Test Commands
- echo ok
`), 0644)

	var stdout, stderr bytes.Buffer
	_ = RunCLI(tmpDir, nil, []string{
		"code",
		"--provider", "ollama",
		"--model", "llama3.3",
		"--review-provider", "ollama",
		"--review-model", "qwen2.5",
		"--no-model-review",
		"--confirm-tests",
		specPath,
	}, &stdout, &stderr)

	// Check audit log for coderModel, coderFamily, criticModel, criticFamily
	logPath := audit.Default(tmpDir).LogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected audit log file at %s: %v", logPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	found := false
	for _, line := range lines {
		var evt audit.AuditEvent
		if json.Unmarshal([]byte(line), &evt) == nil && evt.EventType == audit.EventCodeConvergence && evt.Details != nil {
			if evt.Details["coderModel"] == "llama3.3" && evt.Details["coderFamily"] == "meta" &&
				evt.Details["criticModel"] == "qwen2.5" && evt.Details["criticFamily"] == "qwen" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected CODE_CONVERGENCE audit event to contain resolved model families (coderFamily=meta, criticFamily=qwen), log data:\n%s", string(data))
	}
}

func TestCLI_KnowledgeEval_UnmeasuredWhenNoMeasurementCanRun(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	ki := &knowledge.KnowledgeItem{
		ID:           "ki-unmeasured-01",
		Title:        "Unmeasured Test Item",
		Category:     knowledge.CategoryArchitecture,
		Breakthrough: "Item with no measurement possible",
		Status:       "active",
		ApprovedBy:   "alice-codeowner",
	}
	if err := store.Save(ki); err != nil {
		t.Fatalf("failed to save seed KI: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := RunCLI(tmpDir, nil, []string{
		"knowledge",
		"eval",
		"ki-unmeasured-01",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("expected artix knowledge eval to fail/exit non-zero when no measurement can run, got exit 0")
	}

	outCombined := stdout.String() + stderr.String()
	if !strings.Contains(strings.ToLower(outCombined), "unmeasured") {
		t.Errorf("expected output to mention 'unmeasured', got: %s", outCombined)
	}

	reloaded, err := store.Get("ki-unmeasured-01")
	if err != nil {
		t.Fatalf("failed to reload KI: %v", err)
	}
	if reloaded.LastEvalResult == nil || reloaded.LastEvalResult.Status != "unmeasured" {
		t.Errorf("expected LastEvalResult status to be 'unmeasured', got: %+v", reloaded.LastEvalResult)
	}
}

func TestRunCLI_StreamFlag(t *testing.T) {
	tmpDir := t.TempDir()
	_ = exec.Command("git", "-C", tmpDir, "init").Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()
	var stdout, stderr bytes.Buffer

	// Test plan with --stream
	code := RunCLI(tmpDir, nil, []string{"plan", "--stream", "Add health check endpoint"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for plan --stream, got %d, stderr: %s", code, stderr.String())
	}

	// stdout should contain at least one valid JSON event or final output
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected stream output in stdout, got empty")
	}

	// Test review with --stream
	stdout.Reset()
	stderr.Reset()
	code = RunCLI(tmpDir, nil, []string{"review", "--stream"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for review --stream on clean dir, got %d, stderr: %s", code, stderr.String())
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &ev); err != nil {
		// Could be multiple lines
		firstLine := strings.Split(strings.TrimSpace(stdout.String()), "\n")[0]
		if err := json.Unmarshal([]byte(firstLine), &ev); err != nil {
			t.Fatalf("expected valid JSON stream event, got: %s (err: %v)", stdout.String(), err)
		}
	}
}

func TestRunCLI_ACPCommand(t *testing.T) {
	tmpDir := t.TempDir()
	in := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"shutdown\"}\n")
	var stdout, stderr bytes.Buffer

	code := RunCLIWithIO(tmpDir, nil, []string{"acp"}, in, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for acp shutdown, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"ok\":true") {
		t.Errorf("expected shutdown response in stdout, got: %s", stdout.String())
	}
}
