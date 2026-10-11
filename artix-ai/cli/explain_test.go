package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"artix/pkg/audit"
	"artix/pkg/coder"
	"artix/pkg/persona"
	"artix/pkg/policy"
)

func setupTestGitRepo(t *testing.T) string {
	tempDir := t.TempDir()
	runCmd := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}
	runCmd("git", "init", "-b", "main")
	runCmd("git", "config", "user.name", "Test User")
	runCmd("git", "config", "user.email", "test@example.com")
	_ = os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".artix/\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "sample.txt"), []byte("initial content\n"), 0644)
	runCmd("git", "add", ".")
	runCmd("git", "commit", "-m", "initial commit")
	return tempDir
}

func TestExplain_OutputsFailingPhaseRejectionReasonsAndReproduceCommand(t *testing.T) {
	tempDir := setupTestGitRepo(t)
	taskID := "TASK-FAIL-101"

	// Seed audit log
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "FAILED",
		StorySpecID: taskID,
		Details: map[string]any{
			"error": "failed after 2 rounds",
		},
	})

	// Seed round traces
	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          taskID,
		Round:           1,
		PromptHash:      "p1hash",
		PatchHash:       "patch1hash",
		Patch:           "diff --git a/sample.txt b/sample.txt\n--- a/sample.txt\n+++ b/sample.txt\n@@ -1 +1 @@\n-initial content\n+round 1 change\n",
		SandboxCommand:  "sh -c 'exit 1'",
		ExitCode:        1,
		ReviewerVerdict: "REJECTED",
		BlockingIssues:  []string{"Missing error handling in sample.txt"},
		Tokens:          1200,
		Cost:            0.01,
		Phase:           "REVIEW",
		Timestamp:       time.Now().UTC(),
	})

	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          taskID,
		Round:           2,
		PromptHash:      "p2hash",
		PatchHash:       "patch2hash",
		Patch:           "diff --git a/sample.txt b/sample.txt\n--- a/sample.txt\n+++ b/sample.txt\n@@ -1 +1 @@\n-initial content\n+round 2 failing syntax\n",
		SandboxCommand:  "sh -c 'echo compilation failed; exit 42'",
		ExitCode:        42,
		ReviewerVerdict: "FAILED",
		BlockingIssues:  []string{"Compilation failed at line 1"},
		Tokens:          1500,
		Cost:            0.02,
		Phase:           "TEST",
		Timestamp:       time.Now().UTC(),
	})

	reg := persona.NewRegistry(tempDir)

	// 1. Plain text format
	var stdoutPlain, stderrPlain bytes.Buffer
	codePlain := RunCLI(tempDir, reg, []string{"explain", taskID}, &stdoutPlain, &stderrPlain)
	if codePlain != 0 {
		t.Fatalf("expected artix explain to exit 0, got %d, stderr: %s", codePlain, stderrPlain.String())
	}

	outText := stdoutPlain.String()
	if !strings.Contains(outText, "TEST") {
		t.Errorf("expected plain text explain to name failing phase 'TEST', got:\n%s", outText)
	}
	if !strings.Contains(outText, "Missing error handling in sample.txt") {
		t.Errorf("expected plain text explain to name round 1 rejection reason, got:\n%s", outText)
	}
	if !strings.Contains(outText, "artix replay TASK-FAIL-101 --round 2") {
		t.Errorf("expected plain text explain to print reproduce command 'artix replay TASK-FAIL-101 --round 2', got:\n%s", outText)
	}

	// 2. JSON format
	var stdoutJSON, stderrJSON bytes.Buffer
	codeJSON := RunCLI(tempDir, reg, []string{"explain", taskID, "--json"}, &stdoutJSON, &stderrJSON)
	if codeJSON != 0 {
		t.Fatalf("expected artix explain --json to exit 0, got %d, stderr: %s", codeJSON, stderrJSON.String())
	}

	var jsonResult map[string]any
	if err := json.Unmarshal(stdoutJSON.Bytes(), &jsonResult); err != nil {
		t.Fatalf("failed to parse json output: %v\nOutput: %s", err, stdoutJSON.String())
	}

	if jsonResult["taskId"] != taskID {
		t.Errorf("expected taskId %s, got %v", taskID, jsonResult["taskId"])
	}
	if jsonResult["failingPhase"] != "TEST" {
		t.Errorf("expected failingPhase 'TEST', got %v", jsonResult["failingPhase"])
	}
	if !strings.Contains(stdoutJSON.String(), "artix replay TASK-FAIL-101 --round 2") {
		t.Errorf("expected json output to include reproduce command, got: %s", stdoutJSON.String())
	}
}

func TestReplay_ReproducesSameExitCodeInFreshWorktree(t *testing.T) {
	tempDir := setupTestGitRepo(t)
	taskID := "TASK-REPLAY-42"

	patchContent := "diff --git a/sample.txt b/sample.txt\n--- a/sample.txt\n+++ b/sample.txt\n@@ -1 +1 @@\n-initial content\n+TRIGGER_FAIL\n"

	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          taskID,
		Round:           1,
		PromptHash:      "p1hash",
		PatchHash:       "patch1hash",
		Patch:           patchContent,
		SandboxCommand:  "sh -c 'if grep -q TRIGGER_FAIL sample.txt; then exit 42; else exit 0; fi'",
		ExitCode:        42,
		ReviewerVerdict: "FAILED",
		BlockingIssues:  []string{"deterministic failure"},
		Tokens:          1000,
		Cost:            0.01,
		Phase:           "TEST",
		Timestamp:       time.Now().UTC(),
	})

	reg := persona.NewRegistry(tempDir)
	var stdout, stderr bytes.Buffer
	exitCode := RunCLI(tempDir, reg, []string{"replay", taskID, "--round", "1"}, &stdout, &stderr)

	if exitCode != 42 {
		t.Fatalf("expected artix replay to reproduce exit code 42, got %d\nStdout: %s\nStderr: %s", exitCode, stdout.String(), stderr.String())
	}
}

func TestExplain_TamperedTrace_ExitsNonZero(t *testing.T) {
	tempDir := setupTestGitRepo(t)
	taskID := "TASK-TAMPER-1"

	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          taskID,
		Round:           1,
		PromptHash:      "p1",
		PatchHash:       "patch1",
		SandboxCommand:  "sh -c 'exit 0'",
		ExitCode:        0,
		ReviewerVerdict: "APPROVED",
		Tokens:          100,
		Cost:            0.001,
		Phase:           "REVIEW",
		Timestamp:       time.Now().UTC(),
	})

	// Tamper with trace.jsonl
	auditLogPath := policy.EffectiveAuditLogPath(tempDir)
	tracePath := filepath.Join(filepath.Dir(auditLogPath), "trace.jsonl")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("failed to read trace: %v", err)
	}
	tampered := strings.Replace(string(data), `"tokens":100`, `"tokens":9999`, 1)
	if err := os.WriteFile(tracePath, []byte(tampered), 0600); err != nil {
		t.Fatalf("failed to write tampered trace: %v", err)
	}

	reg := persona.NewRegistry(tempDir)
	var stdout, stderr bytes.Buffer
	exitCode := RunCLI(tempDir, reg, []string{"explain", taskID}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("expected artix explain to exit non-zero on tampered trace.jsonl, but got 0")
	}
}
