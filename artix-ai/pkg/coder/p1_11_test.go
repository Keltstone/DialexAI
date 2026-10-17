package coder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

func TestP1_11_OfflineCacheMiss_BlocksAutonomy(t *testing.T) {
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
		ID:           "OFFLINE-NATIVE-1",
		Title:        "Cold cache native distribution test",
		TestCommands: []string{"./gradlew iosSimulatorArm64Test --offline"},
	}

	// Create fake gradlew emitting native offline cache miss
	gradlewScript := "#!/bin/sh\n" +
		"echo \"> No cached version of org.jetbrains.kotlin:kotlin-native-prebuilt:2.2.21 available for offline mode.\" >&2\n" +
		"exit 1\n"
	_ = os.WriteFile(filepath.Join(tempDir, "gradlew"), []byte(gradlewScript), 0755)

	rc, _ := repo.DetectContext(tempDir)

	policy.ResetAutonomyBlock()
	policy.SetActivePolicyForTest(&policy.Policy{
		IsVerified:       true,
		AutonomyVerified: true,
		AllowAutonomous:  true,
		Source:           "in_memory_test",
	})
	if err := policy.VerifyAutonomyGate(); err != nil {
		t.Fatalf("expected VerifyAutonomyGate to be permitted before run, got: %v", err)
	}

	opts := &LoopOptions{
		MaxRounds:             1,
		Autonomy:              AutonomyAutonomous,
		TestCommandsConfirmed: true,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// update\n"
		},
	}

	res := coord.Run(context.Background(), story, rc, nil, nil, opts)

	if res.Success {
		t.Fatalf("expected loop to fail on cold native distribution miss, but succeeded")
	}

	// Verify autonomy was blocked
	if err := policy.VerifyAutonomyGate(); err == nil {
		t.Fatalf("expected VerifyAutonomyGate to fail and block autonomy following OFFLINE_CACHE_MISS, got nil error")
	}
}
