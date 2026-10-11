package forge

import "artix/pkg/metrics"

// RecordJobMetric updates counters and statistics for a completed or failed job.
func RecordJobMetric(status string, rounds int, tokens int, costUSD float64) {
	metrics.RecordJobMetric(status, rounds, tokens, costUSD)
}

// RecordPhaseDuration records the duration of an execution phase in seconds.
func RecordPhaseDuration(phase string, durationSec float64) {
	metrics.RecordPhaseDuration(phase, durationSec)
}

// RecordOfflineCacheMiss increments the OFFLINE_CACHE_MISS counter.
func RecordOfflineCacheMiss() {
	metrics.RecordOfflineCacheMiss()
}

// RecordTimeout increments the TIMEOUT counter.
func RecordTimeout() {
	metrics.RecordTimeout()
}

// RecordSkippedHostUnsupported increments the SKIPPED_HOST_UNSUPPORTED counter.
func RecordSkippedHostUnsupported() {
	metrics.RecordSkippedHostUnsupported()
}

// SetQueueDepth sets the current queue depth gauge.
func SetQueueDepth(depth int) {
	metrics.SetQueueDepth(depth)
}

// RenderPrometheusMetrics formats all metrics into standard Prometheus text exposition format.
func RenderPrometheusMetrics() string {
	return metrics.RenderPrometheus()
}
