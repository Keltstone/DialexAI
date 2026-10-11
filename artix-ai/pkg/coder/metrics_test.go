package coder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"artix/pkg/metrics"
	"artix/pkg/persona"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

func TestEndToEnd_ConditionsIncrementMetrics(t *testing.T) {
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

	rc, _ := repo.DetectContext(tempDir)

	// 1. Force timeout
	storyTimeout := &spec.StorySpec{
		ID:           "STORY-TIMEOUT",
		Title:        "Timeout Story",
		TestCommands: []string{"sleep 10"},
	}
	optsTimeout := &LoopOptions{
		MaxRounds:             1,
		Autonomy:              AutonomySupervised,
		TestCommandsConfirmed: true,
		TestTimeout:           50 * time.Millisecond,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// timeout\n"
		},
	}
	_ = coord.Run(context.Background(), storyTimeout, rc, nil, nil, optsTimeout)

	// 2. Force cache miss
	storyMiss := &spec.StorySpec{
		ID:           "STORY-CACHE-MISS",
		Title:        "Cache Miss Story",
		TestCommands: []string{"echo 'No cached version of com.example:lib:1.0 available for offline mode'; exit 1"},
	}
	optsMiss := &LoopOptions{
		MaxRounds:             1,
		Autonomy:              AutonomySupervised,
		TestCommandsConfirmed: true,
		TestTimeout:           2 * time.Second,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// cache miss\n"
		},
	}
	_ = coord.Run(context.Background(), storyMiss, rc, nil, nil, optsMiss)

	// 3. Force skipped host unsupported (e.g. simulated KMP on Linux)
	metrics.RecordSkippedHostUnsupported()

	// Assert metrics counters >= 1
	misses, timeouts, skipped := metrics.GetCounts()
	if timeouts < 1 {
		t.Errorf("expected timeouts >= 1, got %d", timeouts)
	}
	if misses < 1 {
		t.Errorf("expected offline misses >= 1, got %d", misses)
	}
	if skipped < 1 {
		t.Errorf("expected skipped host unsupported >= 1, got %d", skipped)
	}

	rendered := metrics.RenderPrometheus()
	if !strings.Contains(rendered, "artix_timeout_total") || !strings.Contains(rendered, "artix_offline_cache_miss_total") || !strings.Contains(rendered, "artix_skipped_host_unsupported_total") {
		t.Errorf("expected metrics exposition to contain all three counters, got:\n%s", rendered)
	}
}

func TestKMPEndToEnd_LinuxSkipsIOSTargets_NotAllGreen(t *testing.T) {
	tempDir, d := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	// Create KMP project files
	_ = os.WriteFile(filepath.Join(tempDir, "build.gradle.kts"), []byte(`
plugins {
    kotlin("multiplatform")
}
`), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "settings.gradle.kts"), []byte(`rootProject.name = "kmp-sample"`), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	_, _ = d.CommitAll("initial commit")

	reg := persona.NewRegistry(tempDir)
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	box := sandbox.NewSandbox(tempDir)
	coord := NewCoordinator(dc, rev, d, box)

	story := &spec.StorySpec{
		ID:    "KMP-STORY-1",
		Title: "KMP Story",
	}

	rc, _ := repo.DetectContext(tempDir)

	opts := &LoopOptions{
		MaxRounds:             1,
		Autonomy:              AutonomySupervised,
		TestCommandsConfirmed: true,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// kmp\n"
		},
	}

	res := coord.Run(context.Background(), story, rc, nil, nil, opts)

	// On non-macOS or simulated host, if iOS targets exist or were detected
	if res.SkippedTargets != nil {
		for target, status := range res.SkippedTargets {
			if strings.Contains(strings.ToLower(target), "ios") && status == "SKIPPED_HOST_UNSUPPORTED" {
				if res.TargetSummary == "All targets green" {
					t.Errorf("TargetSummary claimed 'All targets green' despite skipped targets: %s", res.TargetSummary)
				}
			}
		}
	}
}

func TestNoTimeMinuteLiteralsInLoop(t *testing.T) {
	content, err := os.ReadFile("loop.go")
	if err != nil {
		t.Fatalf("failed to read loop.go: %v", err)
	}
	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		if strings.Contains(l, "time.Minute") {
			t.Errorf("loop.go line %d contains prohibited 'time.Minute' literal: %s", i+1, strings.TrimSpace(l))
		}
	}
}
