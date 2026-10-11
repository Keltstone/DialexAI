package pilot

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"artix/pkg/audit"
)

// PilotReportMetrics aggregates pilot performance indicators.
type PilotReportMetrics struct {
	TotalStories        int                `json:"totalStories"`
	TotalMerged         int                `json:"totalMerged"`
	FirstPassMergeRate  float64            `json:"firstPassMergeRate"`
	RoundsPerMerged     float64            `json:"roundsPerMerged"`
	CostPerMerged       float64            `json:"costPerMerged"`
	HumanEditDistance   float64            `json:"humanEditDistance"`
	AbortRateByCause    map[string]float64 `json:"abortRateByCause"`
	P50LatencyByDriver  map[string]float64 `json:"p50LatencyByDriver"`
	P95LatencyByDriver  map[string]float64 `json:"p95LatencyByDriver"`
}

// PilotStopRuleConfig defines pilot thresholds for automatic daemon backpressure.
type PilotStopRuleConfig struct {
	MaxAbortRate    float64 `json:"maxAbortRate,omitempty"`
	MaxCostPerStory float64 `json:"maxCostPerStory,omitempty"`
}

type storySummary struct {
	id                   string
	merged               bool
	firstPass            bool
	rounds               int
	cost                 float64
	durationSec          float64
	abortCause           string
	driver               string
	humanEditDistance   float64
	hasHumanEditDistance bool
}

// GeneratePilotReport reads the workspace audit log and computes the 7 key pilot metrics.
func GeneratePilotReport(workspaceDir string, since time.Time) (*PilotReportMetrics, error) {
	events, err := audit.Default(workspaceDir).ReadEvents()
	if err != nil {
		return nil, fmt.Errorf("failed to read audit events: %w", err)
	}

	stories := make(map[string]*storySummary)

	for _, e := range events {
		if !since.IsZero() && e.Timestamp.Before(since) {
			continue
		}
		if e.EventType == "STOP_RULE_TRIGGERED" {
			continue
		}

		storyID := e.StorySpecID
		if storyID == "" {
			if e.Details != nil {
				if tid, ok := e.Details["taskId"].(string); ok && tid != "" {
					storyID = tid
				}
			}
		}
		if storyID == "" {
			storyID = e.EventID
		}
		if storyID == "" {
			continue
		}

		st, exists := stories[storyID]
		if !exists {
			st = &storySummary{id: storyID, rounds: 1}
			stories[storyID] = st
		}

		if e.Cost > 0 {
			st.cost += e.Cost
		}
		if e.Round > st.rounds {
			st.rounds = e.Round
		}
		if e.DurationMs > 0 {
			st.durationSec = float64(e.DurationMs) / 1000.0
		}
		if e.Details != nil {
			if drv, ok := e.Details["driver"].(string); ok && drv != "" {
				st.driver = drv
			}
			if hed, ok := e.Details["humanEditDistance"]; ok {
				st.hasHumanEditDistance = true
				switch v := hed.(type) {
				case float64:
					st.humanEditDistance = v
				case int:
					st.humanEditDistance = float64(v)
				case int64:
					st.humanEditDistance = float64(v)
				}
			}
		}

		if e.Status == "SUCCESS" {
			st.merged = true
			if e.Round == 1 {
				st.firstPass = true
			}
		} else if e.Status != "" && e.Status != "RUNNING" && e.Status != "QUEUED" {
			st.abortCause = e.Status
		}
	}

	metrics := &PilotReportMetrics{
		TotalStories:       len(stories),
		AbortRateByCause:   make(map[string]float64),
		P50LatencyByDriver: make(map[string]float64),
		P95LatencyByDriver: make(map[string]float64),
	}

	var firstPassCount int
	var mergedRoundsSum int
	var mergedCostSum float64
	var humanEditSum float64
	var humanEditCount int
	abortCounts := make(map[string]int)
	driverDurations := make(map[string][]float64)

	for _, st := range stories {
		if st.merged {
			metrics.TotalMerged++
			if st.firstPass {
				firstPassCount++
			}
			mergedRoundsSum += st.rounds
			mergedCostSum += st.cost
			if st.hasHumanEditDistance {
				humanEditSum += st.humanEditDistance
				humanEditCount++
			}
		} else if st.abortCause != "" {
			abortCounts[st.abortCause]++
		}

		if st.driver != "" && st.durationSec > 0 {
			driverDurations[st.driver] = append(driverDurations[st.driver], st.durationSec)
		}
	}

	if metrics.TotalMerged > 0 {
		metrics.FirstPassMergeRate = float64(firstPassCount) / float64(metrics.TotalMerged)
		metrics.RoundsPerMerged = float64(mergedRoundsSum) / float64(metrics.TotalMerged)
		metrics.CostPerMerged = mergedCostSum / float64(metrics.TotalMerged)
	}
	if humanEditCount > 0 {
		metrics.HumanEditDistance = humanEditSum / float64(humanEditCount)
	}

	if metrics.TotalStories > 0 {
		for cause, count := range abortCounts {
			metrics.AbortRateByCause[cause] = float64(count) / float64(metrics.TotalStories)
		}
	}

	for drv, durs := range driverDurations {
		metrics.P50LatencyByDriver[drv] = percentile(durs, 0.50)
		metrics.P95LatencyByDriver[drv] = percentile(durs, 0.95)
	}

	return metrics, nil
}

func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cpy := make([]float64, len(vals))
	copy(cpy, vals)
	sort.Float64s(cpy)
	idx := int(math.Floor(p * float64(len(cpy)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cpy) {
		idx = len(cpy) - 1
	}
	return cpy[idx]
}

// CheckPilotStopRules evaluates configured stop thresholds and emits an audit event if exceeded.
func CheckPilotStopRules(workspaceDir string, cfg PilotStopRuleConfig) error {
	metrics, err := GeneratePilotReport(workspaceDir, time.Time{})
	if err != nil {
		return err
	}
	if metrics.TotalStories == 0 {
		return nil
	}

	totalAborted := metrics.TotalStories - metrics.TotalMerged
	abortRate := float64(totalAborted) / float64(metrics.TotalStories)

	if cfg.MaxAbortRate > 0 && abortRate > cfg.MaxAbortRate {
		_ = audit.Default(workspaceDir).Emit(audit.AuditEvent{
			EventType: "STOP_RULE_TRIGGERED",
			Status:    "REFUSED",
			Details: map[string]any{
				"rule":         "pilot.maxAbortRate",
				"abortRate":    abortRate,
				"maxAbortRate": cfg.MaxAbortRate,
			},
			Timestamp: time.Now().UTC(),
		})
		return fmt.Errorf("pilot stop rule triggered: abort rate %.2f exceeds maxAbortRate %.2f", abortRate, cfg.MaxAbortRate)
	}

	if cfg.MaxCostPerStory > 0 && metrics.CostPerMerged > cfg.MaxCostPerStory {
		_ = audit.Default(workspaceDir).Emit(audit.AuditEvent{
			EventType: "STOP_RULE_TRIGGERED",
			Status:    "REFUSED",
			Details: map[string]any{
				"rule":            "pilot.maxCostPerStory",
				"costPerMerged":   metrics.CostPerMerged,
				"maxCostPerStory": cfg.MaxCostPerStory,
			},
			Timestamp: time.Now().UTC(),
		})
		return fmt.Errorf("pilot stop rule triggered: cost per story $%.2f exceeds maxCostPerStory $%.2f", metrics.CostPerMerged, cfg.MaxCostPerStory)
	}

	return nil
}

// FormatPlainText formats the pilot report for human inspection.
func (m *PilotReportMetrics) FormatPlainText() string {
	var sb strings.Builder
	sb.WriteString("=== Artix Pilot Success Report ===\n")
	sb.WriteString(fmt.Sprintf("Total Stories:          %d\n", m.TotalStories))
	sb.WriteString(fmt.Sprintf("Total Merged:           %d\n", m.TotalMerged))
	sb.WriteString(fmt.Sprintf("First-Pass Merge Rate:  %.1f%%\n", m.FirstPassMergeRate*100))
	sb.WriteString(fmt.Sprintf("Rounds / Merged Story:  %.2f\n", m.RoundsPerMerged))
	sb.WriteString(fmt.Sprintf("Cost / Merged Story:    $%.2f\n", m.CostPerMerged))
	sb.WriteString(fmt.Sprintf("Human Edit Distance:    %.2f\n", m.HumanEditDistance))
	sb.WriteString("\nAbort Rate by Cause:\n")
	if len(m.AbortRateByCause) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for cause, rate := range m.AbortRateByCause {
			sb.WriteString(fmt.Sprintf("  %-20s: %.1f%%\n", cause, rate*100))
		}
	}
	sb.WriteString("\nJob Latency by Driver:\n")
	if len(m.P50LatencyByDriver) == 0 {
		sb.WriteString("  (no driver latency data)\n")
	} else {
		for drv, p50 := range m.P50LatencyByDriver {
			p95 := m.P95LatencyByDriver[drv]
			sb.WriteString(fmt.Sprintf("  %-10s p50: %6.1fs | p95: %6.1fs\n", drv, p50, p95))
		}
	}
	return sb.String()
}
