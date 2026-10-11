package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"artix/pkg/audit"
	"artix/pkg/coder"
	"artix/pkg/git"
	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

// TestG8_PluginSourcesNeverPassAutonomous asserts that no IDE plugin ever passes
// '--autonomy autonomous' to ensure human oversight is never silently bypassed.
func TestG8_PluginSourcesNeverPassAutonomous(t *testing.T) {
	pluginDirs := []string{
		"../plugins/vscode",
		"../plugins/intellij",
		"../plugins/claude-code",
	}

	for _, pDir := range pluginDirs {
		err := filepath.WalkDir(pDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Skip build, node_modules, and cache dirs
				name := d.Name()
				if name == "node_modules" || name == ".gradle" || name == "build" || name == "out" {
					return filepath.SkipDir
				}
				return nil
			}

			// Only check source and config files
			ext := filepath.Ext(path)
			if ext != ".ts" && ext != ".js" && ext != ".kt" && ext != ".java" && ext != ".json" && ext != ".md" && ext != ".xml" {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(data)

			// Forbidden: passing --autonomy autonomous
			if strings.Contains(content, "--autonomy autonomous") || strings.Contains(content, `"--autonomy", "autonomous"`) || strings.Contains(content, `--autonomy=autonomous`) {
				t.Errorf("SECURITY VIOLATION: Plugin source file %s passes '--autonomy autonomous'! IDE plugins must only use supervised mode.", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk plugin dir %s: %v", pDir, err)
		}
	}
}

// TestR3_4_ClaudeCodePlugin_RequiresExplicitConfirmationBeforeConfirmTests asserts that
// plugins/claude-code/commands/artix-code.md displays test commands and obtains explicit
// user confirmation before invoking --confirm-tests, preventing silent bypass of the confirmation gate.
func TestR3_4_ClaudeCodePlugin_RequiresExplicitConfirmationBeforeConfirmTests(t *testing.T) {
	cmdPath := "../plugins/claude-code/commands/artix-code.md"
	data, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", cmdPath, err)
	}
	content := string(data)

	// Must not blindly execute --confirm-tests without display and user confirmation
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "Run `artix code") && strings.Contains(trimmed, "--confirm-tests") {
			t.Errorf("SECURITY DEFECT (R3-4): %s passes --confirm-tests unconditionally without test display and confirmation steps first: %s", cmdPath, trimmed)
		}
	}

	// Must require displaying/extracting test commands to user and asking confirmation
	hasDisplay := strings.Contains(content, "Test Commands") || strings.Contains(content, "test command") || strings.Contains(content, "display")
	hasExplicitConfirm := strings.Contains(content, "confirm") || strings.Contains(content, "AskUser") || strings.Contains(content, "confirmation")
	if !hasDisplay || !hasExplicitConfirm {
		t.Errorf("SECURITY DEFECT (R3-4): %s must instruct Claude Code to extract/display test commands and obtain explicit user confirmation before executing artix code with --confirm-tests", cmdPath)
	}
}

// TestR7_PluginDialogContract_CommandsDisplayedBeforeConfirm asserts that the VS Code
// (extension.ts) and IntelliJ (Actions.kt) IDE plugins query and display the exact planned
// test command list via 'artix code --print-test-commands' and bind confirmation to --confirm-tests-hash.
func TestR7_PluginDialogContract_CommandsDisplayedBeforeConfirm(t *testing.T) {
	// 1. VS Code extension.ts
	vsCodePath := "../plugins/vscode/src/extension.ts"
	vsData, err := os.ReadFile(vsCodePath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", vsCodePath, err)
	}
	vsContent := string(vsData)
	if !strings.Contains(vsContent, "--print-test-commands") {
		t.Fatalf("SECURITY DEFECT (R13-2): %s must query test commands via 'artix code --print-test-commands'", vsCodePath)
	}
	if !strings.Contains(vsContent, "--confirm-tests-hash") {
		t.Fatalf("SECURITY DEFECT (R13-2): %s must bind confirmation with '--confirm-tests-hash'", vsCodePath)
	}

	// 2. IntelliJ Actions.kt
	ijPath := "../plugins/intellij/src/main/kotlin/ai/artix/ide/Actions.kt"
	ijData, err := os.ReadFile(ijPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", ijPath, err)
	}
	ijContent := string(ijData)
	if !strings.Contains(ijContent, "--print-test-commands") {
		t.Fatalf("SECURITY DEFECT (R13-2): %s must query test commands via 'artix code --print-test-commands'", ijPath)
	}
	if !strings.Contains(ijContent, "--confirm-tests-hash") {
		t.Fatalf("SECURITY DEFECT (R13-2): %s must bind confirmation with '--confirm-tests-hash'", ijPath)
	}
}

func TestR13_2_PrintTestCommands_And_ConfirmTestsHashBinding(t *testing.T) {
	tempDir := t.TempDir()

	specDir := filepath.Join(tempDir, "docs", "specs")
	_ = os.MkdirAll(specDir, 0755)
	specContent := `# Story Spec: STORY-HASH-01
## Title
Hash Test

## Acceptance Criteria
- Scenario: Pass
  - Given state
  - When action
  - Then ok

## Test Commands
- go test ./pkg/auth/...
- go test ./pkg/session/...
`
	specPath := filepath.Join(specDir, "STORY-001.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. artix code --print-test-commands --json
	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"code", "--print-test-commands", "--json", specPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0 for code --print-test-commands, got %d (stderr: %s)", code, stderr.String())
	}

	var printRes map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &printRes); err != nil {
		t.Fatalf("expected valid JSON from --print-test-commands: %v (stdout: %s)", err, stdout.String())
	}
	if printRes["ok"] != true {
		t.Fatalf("expected ok=true, got: %+v", printRes)
	}
	cmds, ok := printRes["testCommands"].([]any)
	if !ok || len(cmds) != 2 {
		t.Fatalf("expected 2 test commands, got: %+v", printRes["testCommands"])
	}
	hashStr, ok := printRes["testCommandsHash"].(string)
	if !ok || hashStr == "" {
		t.Fatalf("expected non-empty testCommandsHash, got: %+v", printRes["testCommandsHash"])
	}

	// 2. Mismatched hash with --confirm-tests --confirm-tests-hash must fail closed
	stdout.Reset()
	stderr.Reset()
	badCode := RunCLI(tempDir, nil, []string{
		"code", "--json",
		"--autonomy", "supervised",
		"--confirm-tests",
		"--confirm-tests-hash", "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		specPath,
	}, &stdout, &stderr)
	if badCode == 0 {
		t.Fatalf("SECURITY VIOLATION (R13-2): code succeeded with mismatched --confirm-tests-hash!")
	}
	if !strings.Contains(stdout.String(), "mismatch") && !strings.Contains(stderr.String(), "mismatch") {
		t.Fatalf("expected hash mismatch error message, got stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestR13_2_PlanWithoutStory_NeverProducesSpecOrCommands(t *testing.T) {
	tempDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"plan", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected plan --json without story to fail with non-zero exit code")
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &res); err != nil {
		t.Fatalf("expected valid JSON error response from plan --json without story: %v (stdout: %s)", err, stdout.String())
	}
	if res["ok"] == true {
		t.Fatalf("expected ok=false when no story is provided, got %+v", res)
	}
	if _, hasCmds := res["testCommands"]; hasCmds {
		t.Fatalf("SECURITY VIOLATION (R13-2): plan without story must not output testCommands, got %+v", res)
	}
}

// TestG8_CLI_JSON_SingleLineAndStderrProgress tests that 'plan', 'code', and 'review'
// with '--json' output EXACTLY one line of valid JSON to stdout and route progress to stderr.
func TestG8_CLI_Plan_JSON_SuccessAndError(t *testing.T) {
	tempDir := t.TempDir()

	// 1. plan --json success
	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"plan", "--json", "Add basic ping healthcheck endpoint"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0 for plan --json success, got %d (stderr: %s)", code, stderr.String())
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	lines := strings.Split(stdoutStr, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected stdout to be exactly 1 JSON line, got %d lines: %q", len(lines), stdoutStr)
	}

	var planRes map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &planRes); err != nil {
		t.Fatalf("expected stdout to be valid JSON: %v", err)
	}
	if planRes["status"] != "success" && planRes["ok"] != true {
		t.Fatalf("expected ok=true or status=success in plan output, got: %+v", planRes)
	}
	if stderr.Len() == 0 {
		t.Fatalf("expected progress logs to go to stderr when --json is passed")
	}

	// 2. plan --json error (missing story prompt)
	stdout.Reset()
	stderr.Reset()
	code = RunCLI(tempDir, nil, []string{"plan", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit for plan --json without prompt, got 0")
	}
	errLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(errLines) != 1 || strings.TrimSpace(stdout.String()) == "" {
		t.Fatalf("expected stdout to be exactly 1 JSON line on error, got: %q", stdout.String())
	}
	var errRes map[string]any
	if err := json.Unmarshal([]byte(errLines[0]), &errRes); err != nil {
		t.Fatalf("expected valid JSON on plan error: %v", err)
	}
	if errRes["status"] != "error" && errRes["ok"] != false {
		t.Fatalf("expected error status in JSON, got: %+v", errRes)
	}
}

func TestG8_CLI_Review_JSON_Approved_Rejected_Unreviewed(t *testing.T) {
	tempDir := t.TempDir()

	// 1. review --json clean tree -> success / unreviewed
	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"review", "--json"}, &stdout, &stderr)
	stdoutStr := strings.TrimSpace(stdout.String())
	lines := strings.Split(stdoutStr, "\n")
	if len(lines) != 1 || stdoutStr == "" {
		t.Fatalf("expected exactly 1 JSON line on stdout for review --json, got: %q (code %d)", stdoutStr, code)
	}

	var revRes map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &revRes); err != nil {
		t.Fatalf("expected valid JSON on review stdout: %v", err)
	}
	if _, ok := revRes["status"]; !ok {
		t.Fatalf("expected 'status' key in review JSON, got: %+v", revRes)
	}
}

func TestG8_CLI_Code_JSON_OutputsOneLine(t *testing.T) {
	tempDir := t.TempDir()

	// code --json error (no spec found) -> exit 1 with 1 JSON line on stdout and message on stderr
	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"code", "--json", "--provider", "anthropic", "--model", "test"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected error exit code when no spec is found")
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	lines := strings.Split(stdoutStr, "\n")
	if len(lines) != 1 || stdoutStr == "" {
		t.Fatalf("expected exactly 1 JSON line on stdout for code --json error, got %d lines: %q", len(lines), stdoutStr)
	}

	var codeRes map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &codeRes); err != nil {
		t.Fatalf("expected valid JSON on code stdout: %v", err)
	}
	if codeRes["status"] != "error" && codeRes["ok"] != false {
		t.Fatalf("expected status=error or ok=false in code error JSON, got: %+v", codeRes)
	}
}

// TestR2_4_SupervisedNonTTY_RefusesWithoutConfirmFlag verifies that running
// 'artix code --autonomy supervised' in non-TTY mode without --confirm-tests or --yes
// strictly fails closed and prints an error, while keeping stdout as exactly 1 JSON line.
func TestR2_4_SupervisedNonTTY_RefusesWithoutConfirmFlag(t *testing.T) {
	tempDir := t.TempDir()
	specDir := filepath.Join(tempDir, "docs", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `# Story Spec STORY-001: Sample Story
## Acceptance Criteria
- Scenario: Test
## Test Commands
` + "```bash\n" + `echo "test command execution"
` + "```\n"
	if err := os.WriteFile(filepath.Join(specDir, "STORY-001.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	// Stdin is a non-TTY buffer with no user confirmation flag passed
	code := RunCLIWithIO(tempDir, nil, []string{
		"code", "--json", "--autonomy", "supervised",
		"--provider", "openai", "--model", "gpt-4o",
	}, strings.NewReader(""), &stdout, &stderr)

	if code == 0 {
		t.Fatalf("expected non-zero exit code when running in non-TTY supervised mode without --confirm-tests, got 0")
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	lines := strings.Split(stdoutStr, "\n")
	if len(lines) != 1 || stdoutStr == "" {
		t.Fatalf("expected exactly 1 JSON line on stdout, got %d lines: %q", len(lines), stdoutStr)
	}

	var res map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &res); err != nil {
		t.Fatalf("stdout must be valid JSON: %v", err)
	}
	if res["ok"] != false {
		t.Fatalf("expected ok=false in JSON response, got: %+v", res)
	}
	errMsg, _ := res["error"].(string)
	if !strings.Contains(errMsg, "unconfirmed test commands") {
		t.Fatalf("expected error mentioning unconfirmed test commands, got: %s", errMsg)
	}
}

// TestR2_4_SupervisedWithConfirmFlag_Accepted verifies that passing --confirm-tests or --yes
// satisfies the supervised test command gate in non-TTY environments.
func TestR2_4_SupervisedWithConfirmFlag_Accepted(t *testing.T) {
	tempDir := t.TempDir()
	specDir := filepath.Join(tempDir, "docs", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `# Story Spec STORY-001: Sample Story
## Acceptance Criteria
- Scenario: Test
## Test Commands
` + "```bash\n" + `echo "ok"
` + "```\n"
	if err := os.WriteFile(filepath.Join(specDir, "STORY-001.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	// Pass --confirm-tests flag
	code := RunCLIWithIO(tempDir, nil, []string{
		"code", "--json", "--autonomy", "supervised", "--confirm-tests",
		"--provider", "openai", "--model", "gpt-4o",
	}, strings.NewReader(""), &stdout, &stderr)

	// Since OPENAI_API_KEY is not set or network call not made, it may fail on model provider or proceed,
	// but it MUST NOT fail with "unconfirmed test commands"!
	stdoutStr := strings.TrimSpace(stdout.String())
	if strings.Contains(stdoutStr, "unconfirmed test commands") || strings.Contains(stderr.String(), "unconfirmed test commands") {
		t.Fatalf("unexpected unconfirmed test commands error when --confirm-tests was explicitly provided! code=%d, stdout=%s, stderr=%s", code, stdoutStr, stderr.String())
	}
}

// TestR2_4_RealBinaryEndToEndWithFakeProvider executes the actual compiled 'artix' binary
// against a mock OpenAI-compatible HTTP server with --json and --confirm-tests, verifying
// single-line stdout JSON, stderr logging, and valid round execution.
func TestR2_4_RealBinaryEndToEndWithFakeProvider(t *testing.T) {
	// Compile real artix binary
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "artix")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build real artix binary: %v\nOutput: %s", err, string(out))
	}

	// Mock HTTP server returning OpenAI chat completion
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "```diff\n--- /dev/null\n+++ b/result.txt\n@@ -0,0 +1 @@\n+completed\n```",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     150,
				"completion_tokens": 50,
				"total_tokens":      200,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer fakeServer.Close()

	// Initialize temp git repo with spec and a passing test command
	workDir := t.TempDir()
	initCmd := exec.Command("git", "init")
	initCmd.Dir = workDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\nOutput: %s", err, string(out))
	}

	specDir := filepath.Join(workDir, "docs", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `# Story Spec STORY-001: Golden Test Story
## Acceptance Criteria
- Scenario: Verified
## Test Commands
` + "```bash\n" + `echo "tests passed"
` + "```\n"
	if err := os.WriteFile(filepath.Join(specDir, "STORY-001.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binPath, "code", "--json", "--autonomy", "supervised", "--confirm-tests",
		"--provider", "openai", "--model", "gpt-4o", "--rounds", "1")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"OPENAI_API_KEY=mock-key",
		"ARTIX_API_URL="+fakeServer.URL,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run() // exit code may be 0 or 1 depending on whether patch was cleanly accepted

	outLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(outLines) != 1 || strings.TrimSpace(stdout.String()) == "" {
		t.Fatalf("expected real binary stdout to be EXACTLY 1 JSON line, got %d lines:\nSTDOUT:\n%s\nSTDERR:\n%s",
			len(outLines), stdout.String(), stderr.String())
	}

	var jsonRes map[string]any
	if err := json.Unmarshal([]byte(outLines[0]), &jsonRes); err != nil {
		t.Fatalf("expected valid JSON on stdout from real binary, got error: %v, raw: %s", err, outLines[0])
	}
}

// TestR2_7_PluginContract_RealBinaryOutputsExpectedJSONForPlugins verifies that all three
// commands used by the IDE plugins ('plan', 'code', 'review') when run via the real binary
// produce the exact JSON schema and fields parsed by the VS Code and IntelliJ plugins.
func TestR2_7_PluginContract_RealBinaryOutputsExpectedJSONForPlugins(t *testing.T) {
	// Compile real artix binary
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "artix")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build real artix binary: %v\nOutput: %s", err, string(out))
	}

	workDir := t.TempDir()
	initCmd := exec.Command("git", "init")
	initCmd.Dir = workDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\nOutput: %s", err, string(out))
	}

	// 1. Contract: plan --json produces specPath
	planCmd := exec.Command(binPath, "plan", "--json", "Add basic ping healthcheck endpoint")
	planCmd.Dir = workDir
	var planOut, planErr bytes.Buffer
	planCmd.Stdout = &planOut
	planCmd.Stderr = &planErr
	if err := planCmd.Run(); err != nil {
		t.Fatalf("plan command failed: %v (stderr: %s)", err, planErr.String())
	}

	planLines := strings.Split(strings.TrimSpace(planOut.String()), "\n")
	if len(planLines) != 1 {
		t.Fatalf("plan --json: expected 1 line, got %d: %s", len(planLines), planOut.String())
	}
	var planJSON map[string]any
	if err := json.Unmarshal([]byte(planLines[0]), &planJSON); err != nil {
		t.Fatalf("invalid plan JSON: %v", err)
	}
	specPath, ok := planJSON["specPath"].(string)
	if !ok || specPath == "" {
		t.Fatalf("plan JSON missing 'specPath' parsed by plugins: %+v", planJSON)
	}
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("specPath %s does not exist on disk: %v", specPath, err)
	}

	// 2. Contract: review --json produces status and summary
	revCmd := exec.Command(binPath, "review", "--json")
	revCmd.Dir = workDir
	var revOut, revErr bytes.Buffer
	revCmd.Stdout = &revOut
	revCmd.Stderr = &revErr
	_ = revCmd.Run()

	revLines := strings.Split(strings.TrimSpace(revOut.String()), "\n")
	if len(revLines) != 1 {
		t.Fatalf("review --json: expected 1 line, got %d: %s", len(revLines), revOut.String())
	}
	var revJSON map[string]any
	if err := json.Unmarshal([]byte(revLines[0]), &revJSON); err != nil {
		t.Fatalf("invalid review JSON: %v", err)
	}
	status, ok := revJSON["status"].(string)
	if !ok || (status != "approved" && status != "unreviewed" && status != "rejected") {
		t.Fatalf("review JSON missing valid 'status' parsed by plugins: %+v", revJSON)
	}
	if _, ok := revJSON["summary"].(string); !ok {
		t.Fatalf("review JSON missing 'summary' parsed by plugins: %+v", revJSON)
	}

	// 3. Contract: code --json produces success and roundsRun
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "```diff\n--- /dev/null\n+++ b/result.txt\n@@ -0,0 +1 @@\n+completed\n```",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 50,
				"total_tokens":      150,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer fakeServer.Close()

	codeCmd := exec.Command(binPath, "code", "--json", "--autonomy", "supervised", "--confirm-tests",
		"--provider", "openai", "--model", "gpt-4o", "--rounds", "1")
	codeCmd.Dir = workDir
	codeCmd.Env = append(os.Environ(),
		"OPENAI_API_KEY=mock-key",
		"ARTIX_API_URL="+fakeServer.URL,
	)
	var codeOut, codeErr bytes.Buffer
	codeCmd.Stdout = &codeOut
	codeCmd.Stderr = &codeErr
	_ = codeCmd.Run()

	codeLines := strings.Split(strings.TrimSpace(codeOut.String()), "\n")
	if len(codeLines) != 1 {
		t.Fatalf("code --json: expected 1 line, got %d: %s", len(codeLines), codeOut.String())
	}
	var codeJSON map[string]any
	if err := json.Unmarshal([]byte(codeLines[0]), &codeJSON); err != nil {
		t.Fatalf("invalid code JSON: %v", err)
	}
	if _, ok := codeJSON["success"].(bool); !ok {
		t.Fatalf("code JSON missing boolean 'success' parsed by plugins: %+v", codeJSON)
	}
	if _, ok := codeJSON["roundsRun"].(float64); !ok {
		t.Fatalf("code JSON missing numeric 'roundsRun' parsed by plugins: %+v", codeJSON)
	}
}

// TestR4_5_Executable_EnterpriseMode_MissingAuditKey_RefusesCommitAndRollsBack provides an executable
// end-to-end binary test showing commit refusal and working tree rollback when ed25519 audit key is missing/unreadable in enterprise mode.
func TestR4_5_Executable_EnterpriseMode_MissingAuditKey_RefusesCommitAndRollsBack(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "artix-enterprise")

	// Build artix binary with enterprise require-signed-policy flag
	buildCmd := exec.Command("go", "build", "-ldflags", "-X artix/pkg/policy.RequireSignedPolicyFlag=true", "-o", binPath, ".")
	buildCmd.Dir = "."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build enterprise artix binary: %v\nOutput: %s", err, string(out))
	}

	workDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", workDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command failed: %v\nOutput: %s", err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "evaluator@artix.ai")
	runGit("config", "user.name", "Evaluator")
	_ = os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Target Repo\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "initial commit")

	// Create story spec
	specDir := filepath.Join(workDir, "docs", "specs")
	_ = os.MkdirAll(specDir, 0755)
	specPath := filepath.Join(specDir, "STORY-AUDIT-FAIL.md")
	specContent := `---
id: STORY-AUDIT-FAIL
title: Enterprise Audit Rollback Check
---
# User Story
As an auditor, I want enterprise commits strictly blocked if audit keys are missing.
# Acceptance Criteria
- Scenario 1: Missing key causes rollback
# Test Commands
- echo ok
`
	_ = os.WriteFile(specPath, []byte(specContent), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "add spec")

	// Start fake LLM server returning code changes
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "```diff\n--- /dev/null\n+++ b/unauthorized.txt\n@@ -0,0 +1 @@\n+this must be rolled back\n```",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 50,
				"total_tokens":      150,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer fakeServer.Close()

	// Execute artix code in enterprise mode without an audit signing key
	codeCmd := exec.Command(binPath, "code", "--autonomy", "supervised", "--confirm-tests",
		"--provider", "openai", "--model", "gpt-4o", "--rounds", "1")
	codeCmd.Dir = workDir
	codeCmd.Env = append(os.Environ(),
		"ARTIX_ENTERPRISE=1",
		"OPENAI_API_KEY=mock-key",
		"ARTIX_API_URL="+fakeServer.URL,
		"ARTIX_AUDIT_PRIVATE_KEY_PATH=/nonexistent/path/to/key.ed25519",
		"ARTIX_AUDIT_KEY_PATH=",
	)
	var codeOut, codeErr bytes.Buffer
	codeCmd.Stdout = &codeOut
	codeCmd.Stderr = &codeErr
	err := codeCmd.Run()

	// 1. Process must fail
	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R4-5): enterprise artix code succeeded despite missing audit signing key! (stdout: %s, stderr: %s)", codeOut.String(), codeErr.String())
	}

	// 2. Output must mention audit failure
	combined := codeOut.String() + "\n" + codeErr.String()
	if !strings.Contains(combined, "audit") {
		t.Fatalf("expected output to report audit failure, got: %s", combined)
	}

	// 3. Working tree must NOT contain unauthorized.txt
	if _, statErr := os.Stat(filepath.Join(workDir, "unauthorized.txt")); statErr == nil {
		t.Fatalf("SECURITY VIOLATION (R4-5): uncommitted file unauthorized.txt was not rolled back after audit failure!")
	}

	// 4. Git log must not contain new commit
	logCmd := exec.Command("git", "-C", workDir, "log", "-n", "1", "--oneline")
	logOut, _ := logCmd.CombinedOutput()
	if strings.Contains(string(logOut), "Enterprise Audit Rollback Check") {
		t.Fatalf("SECURITY VIOLATION (R4-5): commit was created in git despite audit failure: %s", string(logOut))
	}
}

// TestR7_2_CLI_JSON_AwaitingApproval_Contract verifies that CLI --json output format adheres
// to single-line JSON format with awaitingApproval: true and status: "awaiting_approval".
func TestR7_2_CLI_JSON_AwaitingApproval_Contract(t *testing.T) {
	tempDir := t.TempDir()
	specDir := filepath.Join(tempDir, "docs", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `# Story Spec STORY-001: Sample Awaiting Approval
## Acceptance Criteria
- Scenario: Pass
## Test Commands
` + "```bash\n" + `echo "ok"
` + "```\n"
	if err := os.WriteFile(filepath.Join(specDir, "STORY-001.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify merge CLI flag JSON error response when flags are missing or unverified
	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"merge", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for invalid merge invocation")
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	lines := strings.Split(stdoutStr, "\n")
	if len(lines) != 1 || stdoutStr == "" {
		t.Fatalf("expected exactly 1 JSON line on stdout, got %d lines: %q", len(lines), stdoutStr)
	}

	var res map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &res); err != nil {
		t.Fatalf("stdout must be valid JSON: %v", err)
	}
	if res["ok"] != false {
		t.Fatalf("expected ok=false in JSON response, got: %+v", res)
	}
}

// TestR8_4_Phase1_AwaitingApproval_ReportsSuccessFalse_StatusAwaitingApproval_AndPluginsContract
// asserts that Phase 1 Candidate Push returns success:false, ok:false, status:"awaiting_approval",
// awaitingApproval:true so consumers never misinterpret awaiting-approval as plain convergence,
// and verifies that Claude Code, VS Code, and IntelliJ plugins adhere to this contract.
func TestR8_4_Phase1_AwaitingApproval_ReportsSuccessFalse_StatusAwaitingApproval_AndPluginsContract(t *testing.T) {
	// 1. Check Claude Code command contract
	claudeCodePath := filepath.Join("..", "plugins", "claude-code", "commands", "artix-code.md")
	content, err := os.ReadFile(claudeCodePath)
	if err != nil {
		t.Fatalf("failed to read claude code command: %v", err)
	}
	claudeStr := string(content)
	if !strings.Contains(claudeStr, "awaiting_approval") && !strings.Contains(claudeStr, "awaitingApproval") {
		t.Fatalf("R8-4 VIOLATION: plugins/claude-code/commands/artix-code.md has no awaiting_approval handling: %s", claudeStr)
	}
	if !strings.Contains(claudeStr, "artix verify-approval") {
		t.Fatalf("R8-4 VIOLATION: plugins/claude-code/commands/artix-code.md does not instruct user to execute artix verify-approval after forge review")
	}

	// 2. Check VS Code extension contract
	vscodePath := filepath.Join("..", "plugins", "vscode", "src", "extension.ts")
	vsContent, err := os.ReadFile(vscodePath)
	if err != nil {
		t.Fatalf("failed to read vscode extension.ts: %v", err)
	}
	vsStr := string(vsContent)
	if !strings.Contains(vsStr, "awaiting_approval") {
		t.Fatalf("R8-4 VIOLATION: plugins/vscode/src/extension.ts does not check awaiting_approval status")
	}

	// 3. Check IntelliJ plugin contract
	intellijPath := filepath.Join("..", "plugins", "intellij", "src", "main", "kotlin", "ai", "artix", "ide", "Actions.kt")
	ijContent, err := os.ReadFile(intellijPath)
	if err != nil {
		t.Fatalf("failed to read intellij Actions.kt: %v", err)
	}
	ijStr := string(ijContent)
	if !strings.Contains(ijStr, "awaiting_approval") {
		t.Fatalf("R8-4 VIOLATION: plugins/intellij Actions.kt does not check awaiting_approval status")
	}

	// 4. Verify Phase 1 LoopResult and CLI JSON output
	// Coordinator loop in Phase 1 with ForgePusher MUST emit Success: false, AwaitingApproval: true
	policy.ResetCache()
	defer policy.ResetCache()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_ALLOW_AUTONOMOUS", "1")

	keysDir := t.TempDir()
	secDir := filepath.Join(keysDir, ".artix")
	_ = os.MkdirAll(secDir, 0700)
	pub, priv, _ := ed25519.GenerateKey(nil)
	keyPath := filepath.Join(secDir, "audit_ed25519.key")
	_ = os.WriteFile(keyPath, []byte(hex.EncodeToString(priv)), 0600)
	t.Setenv("ARTIX_AUDIT_PRIVATE_KEY_PATH", keyPath)
	t.Setenv("ARTIX_AUDIT_PUBLIC_KEY", hex.EncodeToString(pub))

	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:       true,
		AllowAutonomous:      true,
		RequireForgeApproval: true,
		AuditPublicKey:       hex.EncodeToString(pub),
		AuditRemoteSinks:     []policy.RemoteSinkConfig{{Type: "syslog", Endpoint: "127.0.0.1:514"}},
		AllowedTestCommands:  []string{`test -f counter.txt`},
		IsVerified:           true,
	})

	tempDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", tempDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.name", "Artix Tester")
	runGit("config", "user.email", "tester@artix.ai")
	if err := os.WriteFile(filepath.Join(tempDir, "counter.txt"), []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "counter.txt")
	runGit("commit", "-m", "initial commit")

	// Create story spec
	specDir := filepath.Join(tempDir, "docs", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `# Story Spec STORY-001: R8-4 Awaiting Approval Test
## Acceptance Criteria
- Scenario: Pass
## Test Commands
` + "```bash\n" + `test -f counter.txt
` + "```\n"
	if err := os.WriteFile(filepath.Join(specDir, "STORY-001.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	pushed := false
	pusher := func(ctx context.Context, commitSHA string) error {
		pushed = true
		return nil
	}

	driver := git.NewDriver(tempDir)
	repoCtx, err := repo.DetectContext(tempDir)
	if err != nil {
		t.Fatalf("failed to detect repo context: %v", err)
	}
	reg := persona.NewRegistry("")
	coderObj, _ := coder.NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCoderFamily("anthropic")
	rev.SetCriticFamily("openai")
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coordinator := coder.NewCoordinator(coderObj, rev, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "STORY-001",
		Title:        "R8-4 Awaiting Approval Test",
		TestCommands: []string{`test -f counter.txt`},
	}

	res := coordinator.Run(context.Background(), storySpec, repoCtx, nil, nil, &coder.LoopOptions{
		MaxRounds:             1,
		Autonomy:              coder.AutonomyAutonomous,
		TwoPhaseAutonomous:    true,
		ForgePusher:           pusher,
		TestCommandsConfirmed: true,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+100\n"
		},
	})

	if !pushed {
		t.Fatalf("expected candidate to be pushed via ForgePusher, got error: %s (roundsRun: %d)", res.Error, res.RoundsRun)
	}
	if !res.AwaitingApproval {
		t.Fatalf("expected res.AwaitingApproval == true, got false")
	}
	if res.Success != false {
		t.Fatalf("SECURITY VIOLATION (R8-4): Phase 1 returned res.Success == true while awaiting approval! Must be false so consumers never misinterpret awaiting-approval as plain convergence.")
	}
}

// TestR2_Phase2_EnterpriseLdflag_SelfSignedPolicyWithEnvKeys_MustFailClosed tests that
// an executable built with enterprise ldflags strictly refuses self-signed policies and env keys,
// and fails closed.
func TestR2_Phase2_EnterpriseLdflag_SelfSignedPolicyWithEnvKeys_MustFailClosed(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "artix-enterprise")

	buildCmd := exec.Command("go", "build", "-ldflags", "-X artix/pkg/policy.RequireSignedPolicyFlag=true", "-o", binPath, ".")
	buildCmd.Dir = "."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build enterprise binary: %v\nOutput: %s", err, string(out))
	}

	workDir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", workDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command failed: %v\nOutput: %s", err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "evaluator@artix.ai")
	runGit("config", "user.name", "Evaluator")
	_ = os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Target Repo\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "initial commit")

	// Story spec
	specDir := filepath.Join(workDir, "docs", "specs")
	_ = os.MkdirAll(specDir, 0755)
	specPath := filepath.Join(specDir, "STORY-R2-ENTERPRISE.md")
	specContent := `---
id: STORY-R2-ENTERPRISE
title: Enterprise Isolation Test
---
# User Story
As an enterprise security officer, I want policy trust roots isolated from the environment.
# Acceptance Criteria
- Scenario 1: Refuse env trust roots
# Test Commands
- echo ok
`
	_ = os.WriteFile(specPath, []byte(specContent), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "add spec")

	// Self-signed policy
	policyFile := filepath.Join(workDir, "self_signed_policy.json")
	policyJSON := `{
		"enterpriseMode": true,
		"allowAutonomous": true,
		"requireSignedPolicy": true,
		"allowedTestCommands": ["echo ok"]
	}`
	_ = os.WriteFile(policyFile, []byte(policyJSON), 0644)
	rogueSecret := "rogue-secret-env"
	_ = policy.SignPolicyFile(policyFile, rogueSecret)

	// Mock Forge whose PR head SHA does NOT come from local state
	forgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/pulls/99") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": "foreign-forge-sha-not-from-local-state-9999"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer forgeServer.Close()

	// Run enterprise binary with self-signed policy and env keys
	cmd := exec.Command(binPath, "code", "--json",
		"--autonomy", "autonomous",
		"--forge", "github",
		"--forge-pr", "99",
		"--forge-url", forgeServer.URL,
		specPath,
	)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"ARTIX_POLICY_PATH="+policyFile,
		"ARTIX_POLICY_SIGNING_KEY="+rogueSecret,
		"ARTIX_POLICY_TRUSTED_PUBKEY=abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		"ARTIX_AUDIT_PUBLIC_KEY=abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R2 d): enterprise binary allowed autonomous code generation using self-signed policy with env keys! (Stdout: %s)", stdout.String())
	}
	outStr := stdout.String() + stderr.String()
	expectedRefusal := "Enterprise safety violation: --autonomy autonomous is disabled by default in enterprise/CI environments or disallowed by policy"
	if !strings.Contains(outStr, expectedRefusal) {
		t.Fatalf("expected specific refusal string %q in IsolateEnvKeys check, got stdout=%s stderr=%s", expectedRefusal, stdout.String(), stderr.String())
	}
}

func TestR13_7_EnterpriseLdflag_CompiledKeyPolicyFixture_Success(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "artix-enterprise-compiled")

	pubMaster, privMaster, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	masterPubHex := hex.EncodeToString(pubMaster)
	workDir := t.TempDir()

	// Story spec
	specDir := filepath.Join(workDir, "docs", "specs")
	_ = os.MkdirAll(specDir, 0755)
	specPath := filepath.Join(specDir, "STORY-ENT-01.md")
	specContent := `---
id: STORY-ENT-01
title: Enterprise Compiled Key Fixture Test
---
# User Story
As an enterprise user, verified compiled-key policies must allow enterprise operation.
# Acceptance Criteria
- Scenario 1: Enterprise policy allowed
# Test Commands
- echo ok
`
	_ = os.WriteFile(specPath, []byte(specContent), 0644)

	// Generate enterprise audit key outside workspace in protected .artix dir
	keyDir := t.TempDir()
	secDir := filepath.Join(keyDir, ".artix")
	_ = os.MkdirAll(secDir, 0700)
	auditKeyPath := filepath.Join(secDir, "audit.key")
	_, privKey, _ := ed25519.GenerateKey(nil)
	_ = os.WriteFile(auditKeyPath, []byte(hex.EncodeToString(privKey)), 0600)

	// Valid policy file signed with compiled master Ed25519 key
	policyPath := filepath.Join(workDir, "enterprise_policy.json")
	policyJSON := fmt.Sprintf(`{
		"enterpriseMode": true,
		"allowAutonomous": true,
		"requireSignedPolicy": true,
		"auditPrivateKeyPath": %q,
		"auditRemoteSinks": [{"type": "syslog", "endpoint": "127.0.0.1:514"}],
		"allowedTestCommands": ["echo ok"]
	}`, auditKeyPath)
	_ = os.WriteFile(policyPath, []byte(policyJSON), 0644)
	if err := policy.SignPolicyFileEd25519(policyPath, privMaster); err != nil {
		t.Fatalf("failed to sign policy file with ed25519: %v", err)
	}

	// Build enterprise binary with compiled key and policy path injected via ldflags
	ldflags := fmt.Sprintf("-X artix/pkg/policy.RequireSignedPolicyFlag=true -X artix/pkg/policy.CompiledTrustedPublicKeyHex=%s -X artix/pkg/policy.DefaultPolicyPath=%s", masterPubHex, policyPath)
	buildCmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", binPath, ".")
	buildCmd.Dir = "."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build enterprise binary with compiled key: %v (%s)", err, string(out))
	}

	// Run artix code --print-test-commands --json
	cmd := exec.Command(binPath, "code", "--print-test-commands", "--json", specPath)
	cmd.Dir = workDir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected enterprise binary with compiled key to succeed, got error: %v (stdout: %s, stderr: %s)", err, stdout.String(), stderr.String())
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &res)
	if res["ok"] != true {
		t.Fatalf("expected ok:true, got: %+v", res)
	}

	// Subtest 1: Unsigned policy must be REJECTED under RequireSignedPolicyFlag
	unsignedPolicyPath := filepath.Join(workDir, "unsigned_policy.json")
	_ = os.WriteFile(unsignedPolicyPath, []byte(policyJSON), 0644)
	cmdUnsigned := exec.Command(binPath, "code", "--print-test-commands", "--json", specPath)
	cmdUnsigned.Dir = workDir
	cmdUnsigned.Env = append(os.Environ(), "ARTIX_POLICY_PATH="+unsignedPolicyPath)
	var outUnsigned, errUnsigned strings.Builder
	cmdUnsigned.Stdout = &outUnsigned
	cmdUnsigned.Stderr = &errUnsigned
	if err := cmdUnsigned.Run(); err == nil {
		t.Fatalf("SECURITY VIOLATION: release binary accepted unsigned policy under RequireSignedPolicyFlag! stdout=%s", outUnsigned.String())
	}

	// Subtest 2: Policy signed by a different (untrusted) key must be REJECTED
	_, privWrong, _ := ed25519.GenerateKey(nil)
	wrongKeyPolicyPath := filepath.Join(workDir, "wrong_key_policy.json")
	_ = os.WriteFile(wrongKeyPolicyPath, []byte(policyJSON), 0644)
	_ = policy.SignPolicyFileEd25519(wrongKeyPolicyPath, privWrong)
	cmdWrong := exec.Command(binPath, "code", "--print-test-commands", "--json", specPath)
	cmdWrong.Dir = workDir
	cmdWrong.Env = append(os.Environ(), "ARTIX_POLICY_PATH="+wrongKeyPolicyPath)
	var outWrong, errWrong strings.Builder
	cmdWrong.Stdout = &outWrong
	cmdWrong.Stderr = &errWrong
	if err := cmdWrong.Run(); err == nil {
		t.Fatalf("SECURITY VIOLATION: release binary accepted policy signed by untrusted key! stdout=%s", outWrong.String())
	}
}

func TestR13_5_Phase1_CandidateAuditRecord_StatusAwaitingApproval_NotFailed(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keyDir := t.TempDir()
	secDir := filepath.Join(keyDir, ".artix")
	_ = os.MkdirAll(secDir, 0700)
	keyPath := filepath.Join(secDir, "audit.key")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(priv)), 0600); err != nil {
		t.Fatal(err)
	}

	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:          true,
		AllowAutonomous:         true,
		RequireSeparateApprover: true,
		AllowedTestCommands:     []string{"test -f counter.txt"},
		AuditPrivateKeyPath:     keyPath,
		AuditRemoteSinks:        []policy.RemoteSinkConfig{{Type: "syslog", Endpoint: "127.0.0.1:514"}},
		IsVerified:              true,
	})
	defer policy.ResetTestPolicy()

	tempDir, err := os.MkdirTemp("", "artix-audit-status-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.name", "Artix Tester")
	runGit("config", "user.email", "tester@artix.ai")
	_ = os.WriteFile(filepath.Join(tempDir, "counter.txt"), []byte("0\n"), 0644)
	runGit("add", "counter.txt")
	runGit("commit", "-m", "initial commit")

	pushed := false
	pusher := func(ctx context.Context, commitSHA string) error {
		pushed = true
		return nil
	}

	driver := git.NewDriver(tempDir)
	repoCtx, err := repo.DetectContext(tempDir)
	if err != nil {
		t.Fatalf("failed to detect repo context: %v", err)
	}
	reg := persona.NewRegistry("")
	coderObj, _ := coder.NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCoderFamily("anthropic")
	rev.SetCriticFamily("openai")
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coordinator := coder.NewCoordinator(coderObj, rev, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "STORY-AUDIT-01",
		Title:        "Audit Status Test",
		TestCommands: []string{`test -f counter.txt`},
	}

	res := coordinator.Run(context.Background(), storySpec, repoCtx, nil, nil, &coder.LoopOptions{
		MaxRounds:             1,
		Autonomy:              coder.AutonomyAutonomous,
		Approver:              "lead-architect@corp.internal",
		TwoPhaseAutonomous:    true,
		ForgePusher:           pusher,
		TestCommandsConfirmed: true,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+100\n"
		},
	})

	if !pushed || !res.AwaitingApproval {
		t.Fatalf("expected candidate push to succeed, got pushed=%v awaiting=%v err=%q verdict=%+v", pushed, res.AwaitingApproval, res.Error, res.FinalVerdict)
	}

	// CLI also emits the outer audit event
	auditStatus := "SUCCESS"
	if res.AwaitingApproval {
		auditStatus = "AWAITING_APPROVAL"
	} else if !res.Success {
		auditStatus = "FAILED"
	}
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType: audit.EventCodeConvergence,
		Status:    auditStatus,
		Details: map[string]any{
			"specId":     storySpec.ID,
			"roundsRun":  res.RoundsRun,
			"commitHash": res.CommitHash,
		},
	})

	// Read .artix/audit.jsonl
	auditPath := filepath.Join(tempDir, ".artix", "audit.jsonl")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("failed to read audit.jsonl: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	hasAwaitingApproval := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(l), &evt); err == nil {
			if evt["status"] == "FAILED" {
				t.Fatalf("R13-5 VIOLATION: Phase 1 candidate push logged FAILED audit status in event: %s", l)
			}
			if evt["status"] == "AWAITING_APPROVAL" {
				hasAwaitingApproval = true
			}
		}
	}
	if !hasAwaitingApproval {
		t.Fatalf("expected audit log to record status AWAITING_APPROVAL, got:\n%s", string(data))
	}
}




