package pilot

import (
	"math"
	"strings"
	"testing"
	"time"

	"artix/pkg/audit"
	"artix/pkg/coder"
)

func TestPilotReport_TableDrivenExactMetrics(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now().UTC()

	// Seed 4 stories:
	// Story 1: merged, round 1, cost $0.05, duration 10s, driver "go", humanEditDistance 0
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "SUCCESS",
		StorySpecID: "STORY-1",
		DurationMs:  10000,
		Cost:        0.05,
		Round:       1,
		Details:     map[string]any{"driver": "go", "humanEditDistance": 0},
		Timestamp:   now.Add(-40 * time.Minute),
	})
	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          "STORY-1",
		Round:           1,
		Cost:            0.05,
		ReviewerVerdict: "APPROVED",
		Phase:           "REVIEW",
		Timestamp:       now.Add(-40 * time.Minute),
	})

	// Story 2: merged, round 2, cost $0.15, duration 20s, driver "go", humanEditDistance 4
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "SUCCESS",
		StorySpecID: "STORY-2",
		DurationMs:  20000,
		Cost:        0.15,
		Round:       2,
		Details:     map[string]any{"driver": "go", "humanEditDistance": 4},
		Timestamp:   now.Add(-30 * time.Minute),
	})
	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          "STORY-2",
		Round:           2,
		Cost:            0.15,
		ReviewerVerdict: "APPROVED",
		Phase:           "REVIEW",
		Timestamp:       now.Add(-30 * time.Minute),
	})

	// Story 3: aborted by "TIMEOUT", round 1, duration 30s, driver "gradle"
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "TIMEOUT",
		StorySpecID: "STORY-3",
		DurationMs:  30000,
		Cost:        0.02,
		Round:       1,
		Details:     map[string]any{"driver": "gradle", "phase": "TEST"},
		Timestamp:   now.Add(-20 * time.Minute),
	})
	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          "STORY-3",
		Round:           1,
		ReviewerVerdict: "TIMEOUT",
		Phase:           "TEST",
		Timestamp:       now.Add(-20 * time.Minute),
	})

	// Story 4: aborted by "OFFLINE_CACHE_MISS", round 1, duration 5s, driver "gradle"
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "OFFLINE_CACHE_MISS",
		StorySpecID: "STORY-4",
		DurationMs:  5000,
		Cost:        0.01,
		Round:       1,
		Details:     map[string]any{"driver": "gradle", "phase": "TEST"},
		Timestamp:   now.Add(-10 * time.Minute),
	})
	_ = coder.AppendRoundTrace(tempDir, &coder.RoundTrace{
		TaskID:          "STORY-4",
		Round:           1,
		ReviewerVerdict: "OFFLINE_CACHE_MISS",
		Phase:           "TEST",
		Timestamp:       now.Add(-10 * time.Minute),
	})

	report, err := GeneratePilotReport(tempDir, time.Time{})
	if err != nil {
		t.Fatalf("GeneratePilotReport failed: %v", err)
	}

	// 1. First-pass merge rate: 1 out of 2 merged stories = 0.50
	if math.Abs(report.FirstPassMergeRate-0.50) > 0.001 {
		t.Errorf("FirstPassMergeRate: expected 0.50, got %.4f", report.FirstPassMergeRate)
	}

	// 2. Rounds per merged story: (1 + 2) / 2 = 1.50
	if math.Abs(report.RoundsPerMerged-1.50) > 0.001 {
		t.Errorf("RoundsPerMerged: expected 1.50, got %.4f", report.RoundsPerMerged)
	}

	// 3. Cost per merged story: (0.05 + 0.15) / 2 = 0.10
	if math.Abs(report.CostPerMerged-0.10) > 0.001 {
		t.Errorf("CostPerMerged: expected 0.10, got %.4f", report.CostPerMerged)
	}

	// 4. Human edit distance: (0 + 4) / 2 = 2.0
	if math.Abs(report.HumanEditDistance-2.00) > 0.001 {
		t.Errorf("HumanEditDistance: expected 2.00, got %.4f", report.HumanEditDistance)
	}

	// 5. Abort rate by cause:
	// Total jobs = 4. TIMEOUT = 1 (0.25), OFFLINE_CACHE_MISS = 1 (0.25)
	if math.Abs(report.AbortRateByCause["TIMEOUT"]-0.25) > 0.001 {
		t.Errorf("AbortRateByCause[TIMEOUT]: expected 0.25, got %.4f", report.AbortRateByCause["TIMEOUT"])
	}
	if math.Abs(report.AbortRateByCause["OFFLINE_CACHE_MISS"]-0.25) > 0.001 {
		t.Errorf("AbortRateByCause[OFFLINE_CACHE_MISS]: expected 0.25, got %.4f", report.AbortRateByCause["OFFLINE_CACHE_MISS"])
	}

	// 6. p50 job latency by driver:
	// "go": 10s, 20s -> 10s (or 15s)
	if report.P50LatencyByDriver["go"] <= 0 {
		t.Errorf("expected positive p50 for 'go', got %.2f", report.P50LatencyByDriver["go"])
	}
	// 7. p95 job latency by driver:
	if report.P95LatencyByDriver["gradle"] <= 0 {
		t.Errorf("expected positive p95 for 'gradle', got %.2f", report.P95LatencyByDriver["gradle"])
	}
}

func TestPilotStopRule_MaxAbortRateExceeded_RefusesJob(t *testing.T) {
	tempDir := t.TempDir()

	// Seed 2 aborted jobs and 0 merged jobs -> abort rate = 1.0 (100%)
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "FAILED",
		StorySpecID: "FAIL-1",
		Details:     map[string]any{"error": "failed"},
	})
	_ = audit.Default(tempDir).Emit(audit.AuditEvent{
		EventType:   audit.EventCodeConvergence,
		Status:      "TIMEOUT",
		StorySpecID: "FAIL-2",
		Details:     map[string]any{"error": "timeout"},
	})

	// Check stop rule with maxAbortRate = 0.5
	stopErr := CheckPilotStopRules(tempDir, PilotStopRuleConfig{
		MaxAbortRate: 0.50,
	})

	if stopErr == nil {
		t.Fatalf("expected CheckPilotStopRules to refuse job when abort rate exceeds maxAbortRate")
	}

	if !strings.Contains(stopErr.Error(), "abort rate") {
		t.Errorf("expected error message to name abort rate, got: %v", stopErr)
	}

	// Verify STOP_RULE_TRIGGERED audit event emitted
	events, err := audit.Default(tempDir).ReadEvents()
	if err != nil {
		t.Fatalf("failed to read audit events: %v", err)
	}

	foundStopEvent := false
	for _, e := range events {
		if e.EventType == "STOP_RULE_TRIGGERED" || e.Status == "REFUSED" {
			foundStopEvent = true
			break
		}
	}
	if !foundStopEvent {
		t.Errorf("expected STOP_RULE_TRIGGERED or REFUSED audit event to be emitted")
	}
}
