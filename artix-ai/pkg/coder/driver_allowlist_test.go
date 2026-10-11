package coder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/spec"
)

func TestLoop_UnallowedDriver_RefusesBeforeLLM(t *testing.T) {
	if len(policy.Active().GetAllowedDrivers()) == 0 {
		t.Fatalf("expected non-empty allowedDrivers")
	}
	tempDir := t.TempDir()
	// Create a Cargo.toml so Cargo driver is detected
	if err := os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte("[package]\nname = \"foo\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("failed to write Cargo.toml: %v", err)
	}

	coord := &ConvergenceCoordinator{}
	s := &spec.StorySpec{ID: "STORY-UNALLOWED"}
	repoCtx := &repo.RepositoryContext{RootDir: tempDir}

	llmCalled := false
	opts := &LoopOptions{
		MockPatchGen: func(round int, feedback string) string {
			llmCalled = true
			return ""
		},
	}

	res := coord.Run(context.Background(), s, repoCtx, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to fail for unallowed cargo driver")
	}
	if llmCalled {
		t.Fatalf("expected no LLM/patch generation call for unallowed driver")
	}
	if !strings.Contains(res.Error, "cargo") || !strings.Contains(res.Error, "allowedDrivers") {
		t.Errorf("expected error to name 'cargo' and 'allowedDrivers', got: %s", res.Error)
	}
}

func TestLoop_AutonomousWithSkippedHostUnsupported_Refuses(t *testing.T) {
	tempDir := t.TempDir()
	// Create mock gradle project
	_ = os.WriteFile(filepath.Join(tempDir, "settings.gradle.kts"), []byte("rootProject.name = \"kmp-app\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "build.gradle.kts"), []byte("plugins { kotlin(\"multiplatform\") }\n"), 0644)

	coord := &ConvergenceCoordinator{}
	s := &spec.StorySpec{ID: "STORY-KMP-AUTONOMOUS"}
	repoCtx := &repo.RepositoryContext{RootDir: tempDir}

	opts := &LoopOptions{
		Autonomy: AutonomyAutonomous,
		MockPatchGen: func(round int, feedback string) string {
			return ""
		},
	}

	res := coord.Run(context.Background(), s, repoCtx, nil, nil, opts)
	// If run on Linux (or simulated with skipped targets), autonomous execution must be refused.
	// Check if skipped targets exist, or verify the error condition if skipped targets are present.
	if len(res.SkippedTargets) > 0 {
		if res.Success {
			t.Fatalf("expected autonomous execution to be refused when skipped targets exist")
		}
		if !strings.Contains(res.Error, "SKIPPED_HOST_UNSUPPORTED") {
			t.Errorf("expected error to mention SKIPPED_HOST_UNSUPPORTED, got: %s", res.Error)
		}
	}
}
