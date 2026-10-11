package coder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"artix/pkg/audit"
	"artix/pkg/persona"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

func TestPhaseTimeout_RecordedAsTimeoutInAudit(t *testing.T) {
	tempDir, d := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	_, _ = d.CommitAll("add main.go")

	reg := persona.NewRegistry(tempDir)
	dc, err := NewDomainCoder("backend_engineer", reg)
	if err != nil {
		t.Fatalf("NewDomainCoder failed: %v", err)
	}

	rev := reviewer.NewAdversarialReviewer(reg)
	box := sandbox.NewSandbox(tempDir)
	coord := NewCoordinator(dc, rev, d, box)

	story := &spec.StorySpec{
		ID:           "TIMEOUT-STORY-1",
		Title:        "Command times out",
		TestCommands: []string{"sleep 10"},
	}

	rc, _ := repo.DetectContext(tempDir)

	opts := &LoopOptions{
		MaxRounds:             1,
		Autonomy:              AutonomySupervised,
		TestCommandsConfirmed: true,
		TestTimeout:           100 * time.Millisecond,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// updated\n"
		},
	}

	res := coord.Run(context.Background(), story, rc, nil, nil, opts)

	if res.Success {
		t.Fatalf("expected loop to fail on timeout, but succeeded")
	}

	if !strings.Contains(res.Error, "TIMEOUT") {
		t.Errorf("expected res.Error to contain 'TIMEOUT', got: %q", res.Error)
	}

	// Worktree must be clean (rolled back)
	statusOut, _ := d.Status()
	if statusOut != nil && !statusOut.IsClean {
		t.Errorf("expected clean rolled-back worktree, got status: %+v", statusOut)
	}

	// Verify audit log has TIMEOUT event with phase and elapsed time
	events, err := audit.Default(tempDir).ReadEvents()
	if err != nil {
		t.Fatalf("failed to read audit events: %v", err)
	}

	foundTimeout := false
	for _, e := range events {
		if e.Status == "TIMEOUT" {
			foundTimeout = true
			if e.Details["phase"] != "TEST" {
				t.Errorf("expected audit event phase to be TEST, got: %v", e.Details["phase"])
			}
			if e.Details["elapsed"] == nil && e.Details["elapsedMs"] == nil {
				t.Errorf("expected audit event to record elapsed time, got: %+v", e.Details)
			}
		}
	}

	if !foundTimeout {
		t.Errorf("expected TIMEOUT event in audit log, found: %+v", events)
	}
}
