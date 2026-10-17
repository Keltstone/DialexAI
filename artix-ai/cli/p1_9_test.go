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
)

func setupP1_9Repo(t *testing.T) string {
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

func TestP1_9_PatchTampering_And_BlockingIssues_Detected(t *testing.T) {
	tempDir := setupP1_9Repo(t)
	taskID := "TASK-TAMPER-PATCH"

	origPatch := "diff --git a/sample.txt b/sample.txt\n--- a/sample.txt\n+++ b/sample.txt\n@@ -1 +1 @@\n-initial content\n+legit fix\n"
	tr := &coder.RoundTrace{
		TaskID:          taskID,
		Round:           1,
		PromptHash:      "p1",
		PatchHash:       coder.HashContent(origPatch),
		Patch:           origPatch,
		SandboxCommand:  "go test ./...",
		TestCommands:    []string{"go test ./..."},
		ExitCode:        0,
		ReviewerVerdict: "APPROVED",
		BlockingIssues:  []string{"issue-1"},
		Tokens:          100,
		Cost:            0.001,
		Phase:           "REVIEW",
		Timestamp:       time.Now().UTC(),
	}

	if err := coder.AppendRoundTrace(tempDir, tr); err != nil {
		t.Fatalf("AppendRoundTrace failed: %v", err)
	}

	// 1. Tamper Patch text while keeping PatchHash
	tracePath := filepath.Join(tempDir, ".artix", "trace.jsonl")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace failed: %v", err)
	}

	tamperedPatch := strings.Replace(string(data), "legit fix", "malicious injected code", 1)
	if err := os.WriteFile(tracePath, []byte(tamperedPatch), 0600); err != nil {
		t.Fatalf("write tampered patch failed: %v", err)
	}

	if _, err := coder.VerifyRoundTraces(tempDir); err == nil {
		t.Fatalf("SECURITY FLAW: VerifyRoundTraces verified trace after patch was replaced with arbitrary text!")
	}
}

func TestP1_9_Replay_RefusesUnsignedArbitraryCommand(t *testing.T) {
	tempDir := setupP1_9Repo(t)
	taskID := "TASK-REPLAY-EXPLOIT"
	markerFile := filepath.Join(tempDir, "artix_replay_marker")

	// Trace recorded with an arbitrary command not in signed test commands
	tr := &coder.RoundTrace{
		TaskID:          taskID,
		Round:           1,
		PromptHash:      "p1",
		PatchHash:       coder.HashContent(""),
		SandboxCommand:  "touch " + markerFile,
		TestCommands:    []string{"go test ./..."}, // Touch is NOT in test commands!
		ExitCode:        0,
		ReviewerVerdict: "FAILED",
		Tokens:          100,
		Cost:            0.001,
		Phase:           "TEST",
		Timestamp:       time.Now().UTC(),
	}

	_ = coder.AppendRoundTrace(tempDir, tr)

	reg := persona.NewRegistry(tempDir)
	var stdout, stderr bytes.Buffer
	exitCode := RunCLI(tempDir, reg, []string{"replay", taskID, "--round", "1"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("SECURITY FLAW: artix replay executed arbitrary command %q not in TestCommands!", tr.SandboxCommand)
	}
	if _, err := os.Stat(markerFile); err == nil {
		t.Fatalf("SECURITY FLAW: malicious marker file was created by artix replay!")
	}
}

func TestP1_9_Explain_ApprovedTask_DoesNotReportFailingPhase(t *testing.T) {
	tempDir := setupP1_9Repo(t)
	taskID := "TASK-APPROVED-01"

	// Audit log records SUCCESS / approved
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "APPROVED",
		StorySpecID: taskID,
		Details: map[string]any{
			"message": "task converged successfully",
		},
	})

	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          taskID,
		Round:           1,
		PromptHash:      "p1",
		PatchHash:       coder.HashContent(""),
		SandboxCommand:  "go test ./...",
		TestCommands:    []string{"go test ./..."},
		ExitCode:        0,
		ReviewerVerdict: "APPROVED",
		Tokens:          100,
		Cost:            0.001,
		Phase:           "CONVERGENCE",
		Timestamp:       time.Now().UTC(),
	})

	reg := persona.NewRegistry(tempDir)
	var stdoutPlain, stderrPlain bytes.Buffer
	code := RunCLI(tempDir, reg, []string{"explain", taskID}, &stdoutPlain, &stderrPlain)
	if code != 0 {
		t.Fatalf("expected explain to exit 0, got %d: %s", code, stderrPlain.String())
	}

	outText := stdoutPlain.String()
	if strings.Contains(outText, "Failing Phase: CONVERGENCE") {
		t.Fatalf("BUG: artix explain reported 'Failing Phase: CONVERGENCE' for an approved task:\n%s", outText)
	}

	var stdoutJSON, stderrJSON bytes.Buffer
	_ = RunCLI(tempDir, reg, []string{"explain", taskID, "--json"}, &stdoutJSON, &stderrJSON)
	var res ExplainResult
	_ = json.Unmarshal(stdoutJSON.Bytes(), &res)
	if res.FailingPhase != "" {
		t.Fatalf("BUG: ExplainResult.FailingPhase is %q for an approved task!", res.FailingPhase)
	}
}
