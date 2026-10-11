package metrics

import (
	"fmt"
	"strings"
	"sync"
)

type Store struct {
	mu                    sync.RWMutex
	jobsTotal             map[string]int
	roundsTotal           int
	roundsCount           int
	tokensTotal           int
	costUSDTotal          float64
	phaseDurations        map[string][]float64
	queueDepth            int
	offlineCacheMissTotal int
	timeoutTotal          int
	skippedHostUnsupTotal int
}

var globalStore = &Store{
	jobsTotal:      make(map[string]int),
	phaseDurations: make(map[string][]float64),
}

// RecordJobMetric records metrics for a completed or failed job.
func RecordJobMetric(status string, rounds int, tokens int, costUSD float64) {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	globalStore.jobsTotal[status]++
	globalStore.roundsTotal += rounds
	globalStore.roundsCount++
	globalStore.tokensTotal += tokens
	globalStore.costUSDTotal += costUSD
}

// RecordPhaseDuration records the duration of an execution phase in seconds.
func RecordPhaseDuration(phase string, durationSec float64) {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	globalStore.phaseDurations[phase] = append(globalStore.phaseDurations[phase], durationSec)
}

// RecordOfflineCacheMiss increments the OFFLINE_CACHE_MISS counter.
func RecordOfflineCacheMiss() {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	globalStore.offlineCacheMissTotal++
}

// RecordTimeout increments the TIMEOUT counter.
func RecordTimeout() {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	globalStore.timeoutTotal++
}

// RecordSkippedHostUnsupported increments the SKIPPED_HOST_UNSUPPORTED counter.
func RecordSkippedHostUnsupported() {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	globalStore.skippedHostUnsupTotal++
}

// SetQueueDepth sets the current queue depth gauge.
func SetQueueDepth(depth int) {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	globalStore.queueDepth = depth
}

// GetCounts returns the current counts for offline misses, timeouts, and skipped targets.
func GetCounts() (offlineMisses, timeouts, skippedHost int) {
	globalStore.mu.RLock()
	defer globalStore.mu.RUnlock()
	return globalStore.offlineCacheMissTotal, globalStore.timeoutTotal, globalStore.skippedHostUnsupTotal
}

// RenderPrometheus exports all recorded metrics in Prometheus exposition format.
func RenderPrometheus() string {
	globalStore.mu.RLock()
	defer globalStore.mu.RUnlock()

	var sb strings.Builder

	// Jobs total by status
	sb.WriteString("# HELP artix_jobs_total Total number of jobs by status\n")
	sb.WriteString("# TYPE artix_jobs_total counter\n")
	statuses := []string{"completed", "failed", "queued", "running"}
	for _, st := range statuses {
		count := globalStore.jobsTotal[st]
		fmt.Fprintf(&sb, "artix_jobs_total{status=%q} %d\n", st, count)
	}

	// Rounds per job
	sb.WriteString("\n# HELP artix_rounds_per_job Number of convergence rounds per job\n")
	sb.WriteString("# TYPE artix_rounds_per_job histogram\n")
	fmt.Fprintf(&sb, "artix_rounds_per_job_count %d\n", globalStore.roundsCount)
	fmt.Fprintf(&sb, "artix_rounds_per_job_sum %d\n", globalStore.roundsTotal)

	// Tokens total
	sb.WriteString("\n# HELP artix_tokens_total Total tokens consumed across jobs\n")
	sb.WriteString("# TYPE artix_tokens_total counter\n")
	fmt.Fprintf(&sb, "artix_tokens_total %d\n", globalStore.tokensTotal)

	// Cost USD total
	sb.WriteString("\n# HELP artix_cost_usd_total Total cost in USD across jobs\n")
	sb.WriteString("# TYPE artix_cost_usd_total counter\n")
	fmt.Fprintf(&sb, "artix_cost_usd_total %.6f\n", globalStore.costUSDTotal)

	// Phase duration
	sb.WriteString("\n# HELP artix_phase_duration_seconds Execution phase duration in seconds\n")
	sb.WriteString("# TYPE artix_phase_duration_seconds histogram\n")
	for phase, durations := range globalStore.phaseDurations {
		sum := 0.0
		for _, d := range durations {
			sum += d
		}
		fmt.Fprintf(&sb, "artix_phase_duration_seconds_count{phase=%q} %d\n", phase, len(durations))
		fmt.Fprintf(&sb, "artix_phase_duration_seconds_sum{phase=%q} %.4f\n", phase, sum)
	}

	// Queue depth
	sb.WriteString("\n# HELP artix_queue_depth Current number of queued jobs\n")
	sb.WriteString("# TYPE artix_queue_depth gauge\n")
	fmt.Fprintf(&sb, "artix_queue_depth %d\n", globalStore.queueDepth)

	// OFFLINE_CACHE_MISS
	sb.WriteString("\n# HELP artix_offline_cache_miss_total Total occurrences of OFFLINE_CACHE_MISS\n")
	sb.WriteString("# TYPE artix_offline_cache_miss_total counter\n")
	fmt.Fprintf(&sb, "artix_offline_cache_miss_total %d\n", globalStore.offlineCacheMissTotal)
	fmt.Fprintf(&sb, "artix_errors_total{cause=%q} %d\n", "OFFLINE_CACHE_MISS", globalStore.offlineCacheMissTotal)

	// TIMEOUT
	sb.WriteString("\n# HELP artix_timeout_total Total occurrences of TIMEOUT\n")
	sb.WriteString("# TYPE artix_timeout_total counter\n")
	fmt.Fprintf(&sb, "artix_timeout_total %d\n", globalStore.timeoutTotal)
	fmt.Fprintf(&sb, "artix_errors_total{cause=%q} %d\n", "TIMEOUT", globalStore.timeoutTotal)

	// SKIPPED_HOST_UNSUPPORTED
	sb.WriteString("\n# HELP artix_skipped_host_unsupported_total Total occurrences of SKIPPED_HOST_UNSUPPORTED\n")
	sb.WriteString("# TYPE artix_skipped_host_unsupported_total counter\n")
	fmt.Fprintf(&sb, "artix_skipped_host_unsupported_total %d\n", globalStore.skippedHostUnsupTotal)
	fmt.Fprintf(&sb, "artix_errors_total{cause=%q} %d\n", "SKIPPED_HOST_UNSUPPORTED", globalStore.skippedHostUnsupTotal)

	return sb.String()
}
