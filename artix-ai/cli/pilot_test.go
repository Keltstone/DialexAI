package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"artix/pkg/audit"
)

func TestCLI_PilotReport_Command(t *testing.T) {
	tempDir := t.TempDir()

	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "SUCCESS",
		StorySpecID: "STORY-1",
		Round:       1,
		Cost:        0.10,
		DurationMs:  5000,
		Details:     map[string]any{"driver": "go", "humanEditDistance": 1},
		Timestamp:   time.Now().UTC(),
	})

	var stdout, stderr bytes.Buffer
	code := RunCLI(tempDir, nil, []string{"pilot", "report"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected artix pilot report to succeed, got %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Artix Pilot Success Report") {
		t.Errorf("expected report header in output, got: %s", out)
	}
	if !strings.Contains(out, "First-Pass Merge Rate:  100.0%") {
		t.Errorf("expected 100.0%% first pass merge rate, got: %s", out)
	}

	// JSON mode test
	var jsonStdout bytes.Buffer
	codeJSON := RunCLI(tempDir, nil, []string{"pilot", "report", "--json"}, &jsonStdout, &stderr)
	if codeJSON != 0 {
		t.Fatalf("expected artix pilot report --json to succeed, got %d", codeJSON)
	}
	if !strings.Contains(jsonStdout.String(), `"firstPassMergeRate":1`) {
		t.Errorf("expected firstPassMergeRate in JSON output, got: %s", jsonStdout.String())
	}
}
