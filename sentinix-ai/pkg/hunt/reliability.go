package hunt

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"sentinix/pkg/ai"
)

// ReliableModel wraps an ai.Model with concurrency throttling, timeouts, retries, and budget tracking.
type ReliableModel struct {
	model   ai.Model
	tracker *BudgetTracker
	sem     chan struct{} // bounds concurrent calls to this model
	timeout time.Duration
}

var (
	semaphoresMu sync.Mutex
	semaphores   = make(map[string]chan struct{})
)

func getProviderSemaphore(provider string, maxConcurrency int) chan struct{} {
	semaphoresMu.Lock()
	defer semaphoresMu.Unlock()
	if sem, ok := semaphores[provider]; ok {
		return sem
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	sem := make(chan struct{}, maxConcurrency)
	semaphores[provider] = sem
	return sem
}

// NewReliableModel wraps a base ai.Model with bounded concurrency and accounting.
func NewReliableModel(m ai.Model, tracker *BudgetTracker, maxConcurrency int, timeout time.Duration) *ReliableModel {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &ReliableModel{
		model:   m,
		tracker: tracker,
		sem:     getProviderSemaphore(m.Name(), maxConcurrency),
		timeout: timeout,
	}
}

func (r *ReliableModel) Name() string { return r.model.Name() }
func (r *ReliableModel) Local() bool  { return r.model.Local() }

// Complete executes a model call with exponential backoff + jitter on 429/5xx, bounded concurrency, and accounting.
func (r *ReliableModel) Complete(ctx context.Context, system, user string) (string, error) {
	if r.tracker != nil {
		if err := r.tracker.RecordCall(); err != nil {
			return "", err
		}
	}

	// Acquire concurrency slot
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	maxRetries := 3
	backoff := 300 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		out, err := r.model.Complete(callCtx, system, user)
		if err == nil {
			// Record estimated token usage if tracker is present
			if r.tracker != nil {
				// Estimate ~4 characters per token
				inTokens := (len(system) + len(user)) / 4
				outTokens := len(out) / 4
				cachedTokens := len(system) / 4 // system prompt is cached
				_ = r.tracker.RecordTokens(r.model.Name(), inTokens, outTokens, cachedTokens)
			}
			return out, nil
		}

		if !isRetryableError(err) || attempt == maxRetries {
			return "", err
		}

		// Jitter between 0 and 100ms
		jitterMs, _ := rand.Int(rand.Reader, big.NewInt(100))
		sleepDuration := backoff + time.Duration(jitterMs.Int64())*time.Millisecond

		select {
		case <-time.After(sleepDuration):
		case <-callCtx.Done():
			return "", callCtx.Err()
		}

		backoff *= 2
	}

	return "", errors.New("model call failed after retries")
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "too many requests") ||
		strings.Contains(s, "500") ||
		strings.Contains(s, "502") ||
		strings.Contains(s, "503") ||
		strings.Contains(s, "504") ||
		strings.Contains(s, "server error") ||
		strings.Contains(s, "timeout")
}
