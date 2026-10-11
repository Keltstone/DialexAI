package coder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

func TestPerJobRoundTrace_WrittenNextToAuditLog(t *testing.T) {
	tempDir, d := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	_, _ = d.CommitAll("initial commit")

	reg := persona.NewRegistry(tempDir)
	dc, err := NewDomainCoder("backend_engineer", reg)
	if err != nil {
		t.Fatalf("NewDomainCoder failed: %v", err)
	}

	rev := reviewer.NewAdversarialReviewer(reg)
	box := sandbox.NewSandbox(tempDir)
	coord := NewCoordinator(dc, rev, d, box)

	story := &spec.StorySpec{
		ID:           "TRACE-JOB-1",
		Title:        "Testing round trace recording",
		TestCommands: []string{"go test -v ./..."},
	}

	rc, _ := repo.DetectContext(tempDir)

	opts := &LoopOptions{
		MaxRounds:             1,
		Autonomy:              AutonomySupervised,
		TestCommandsConfirmed: true,
		TestTimeout:           5 * time.Second,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// trace tested\n"
		},
	}

	res := coord.Run(context.Background(), story, rc, nil, nil, opts)
	_ = res

	// Verify trace file written next to the audit log
	auditLogPath := policy.EffectiveAuditLogPath(tempDir)
	tracePath := filepath.Join(filepath.Dir(auditLogPath), "trace.jsonl")

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("expected trace file to exist at %s: %v", tracePath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("trace file is empty")
	}

	var trace RoundTrace
	if err := json.Unmarshal([]byte(lines[0]), &trace); err != nil {
		t.Fatalf("failed to parse trace entry: %v", err)
	}

	if trace.TaskID != "TRACE-JOB-1" {
		t.Errorf("expected TaskID TRACE-JOB-1, got %q", trace.TaskID)
	}
	if trace.Round != 1 {
		t.Errorf("expected round 1, got %d", trace.Round)
	}
	if trace.PromptHash == "" {
		t.Errorf("expected prompt hash to be non-empty")
	}
	if trace.PatchHash == "" {
		t.Errorf("expected patch hash to be non-empty")
	}
	if trace.SandboxCommand == "" {
		t.Errorf("expected sandbox command to be non-empty")
	}

	// Verify mode is 0600
	info, err := os.Stat(tracePath)
	if err != nil {
		t.Fatalf("failed to stat trace file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected trace file mode 0600, got %v", info.Mode().Perm())
	}
}

func TestRoundTrace_HashChainingAndTamperDetection(t *testing.T) {
	tempDir, _ := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	t1 := &RoundTrace{
		TaskID:          "TASK-1",
		Round:           1,
		PromptHash:      "p1",
		PatchHash:       "patch1",
		SandboxCommand:  "echo 1",
		ExitCode:        0,
		ReviewerVerdict: "APPROVED",
		Tokens:          100,
		Cost:            0.01,
		Phase:           "REVIEW",
	}
	if err := AppendRoundTrace(tempDir, t1); err != nil {
		t.Fatalf("AppendRoundTrace t1 failed: %v", err)
	}

	t2 := &RoundTrace{
		TaskID:          "TASK-1",
		Round:           2,
		PromptHash:      "p2",
		PatchHash:       "patch2",
		SandboxCommand:  "echo 2",
		ExitCode:        0,
		ReviewerVerdict: "APPROVED",
		Tokens:          200,
		Cost:            0.02,
		Phase:           "REVIEW",
	}
	if err := AppendRoundTrace(tempDir, t2); err != nil {
		t.Fatalf("AppendRoundTrace t2 failed: %v", err)
	}

	// Verification must pass
	traces, err := VerifyRoundTraces(tempDir)
	if err != nil {
		t.Fatalf("expected valid trace verification, got: %v", err)
	}
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(traces))
	}
	if traces[0].PrevHash != GenesisTraceHash {
		t.Errorf("expected first trace PrevHash to be GenesisTraceHash, got %s", traces[0].PrevHash)
	}
	if traces[1].PrevHash != traces[0].RecordHash {
		t.Errorf("expected second trace PrevHash to equal first RecordHash, got %s vs %s", traces[1].PrevHash, traces[0].RecordHash)
	}

	// Tamper with record
	auditLogPath := policy.EffectiveAuditLogPath(tempDir)
	tracePath := filepath.Join(filepath.Dir(auditLogPath), "trace.jsonl")
	data, _ := os.ReadFile(tracePath)
	tampered := strings.Replace(string(data), `"tokens":100`, `"tokens":9999`, 1)
	_ = os.WriteFile(tracePath, []byte(tampered), 0600)

	_, err = VerifyRoundTraces(tempDir)
	if err == nil {
		t.Fatalf("expected VerifyRoundTraces to fail on tampered trace, but succeeded")
	}
}
