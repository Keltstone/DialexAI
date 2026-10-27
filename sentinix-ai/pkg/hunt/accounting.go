package hunt

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrBudgetExhausted is returned when any configured resource limit is exceeded.
var ErrBudgetExhausted = errors.New("hunt budget exhausted")

// BudgetTracker records and caps model calls, token consumption, financial cost, and elapsed execution time.
type BudgetTracker struct {
	mu           sync.Mutex
	calls        int
	inputTokens  int
	outputTokens int
	cachedTokens int
	costUSD      float64
	startTime    time.Time

	maxCalls    int
	maxTokens   int
	maxCostUSD  float64
	maxWallTime time.Duration
	prices      map[string]ModelPrice
	exhausted   bool
	reason      string
}

// DefaultPrices provides baseline token price estimates per million tokens.
var DefaultPrices = map[string]ModelPrice{
	"claude-3-7-sonnet": {InputPerMillion: 3.00, OutputPerMillion: 15.00, CachedPerMillion: 0.30},
	"claude-3-5-sonnet": {InputPerMillion: 3.00, OutputPerMillion: 15.00, CachedPerMillion: 0.30},
	"claude-3-5-haiku":  {InputPerMillion: 0.80, OutputPerMillion: 4.00, CachedPerMillion: 0.08},
	"gpt-4o":            {InputPerMillion: 2.50, OutputPerMillion: 10.00, CachedPerMillion: 1.25},
	"gpt-4o-mini":       {InputPerMillion: 0.15, OutputPerMillion: 0.60, CachedPerMillion: 0.075},
	"default":           {InputPerMillion: 2.00, OutputPerMillion: 8.00, CachedPerMillion: 0.50},
}

// NewBudgetTracker initializes a thread-safe tracker configured with HuntConfig limits.
func NewBudgetTracker(cfg HuntConfig) *BudgetTracker {
	var maxDuration time.Duration
	if cfg.MaxWallTime != "" {
		maxDuration, _ = time.ParseDuration(cfg.MaxWallTime)
	}

	mergedPrices := make(map[string]ModelPrice)
	for k, v := range DefaultPrices {
		mergedPrices[k] = v
	}
	for k, v := range cfg.Prices {
		mergedPrices[k] = v
	}

	return &BudgetTracker{
		startTime:   time.Now(),
		maxCalls:    cfg.MaxCalls,
		maxTokens:   cfg.MaxTokens,
		maxCostUSD:  cfg.MaxCostUSD,
		maxWallTime: maxDuration,
		prices:      mergedPrices,
	}
}

// RecordCall checks if the run can make another call, increments the counter, and validates limits.
func (b *BudgetTracker) RecordCall() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.exhausted {
		return fmt.Errorf("%w: %s", ErrBudgetExhausted, b.reason)
	}

	if b.maxWallTime > 0 && time.Since(b.startTime) > b.maxWallTime {
		b.exhausted = true
		b.reason = fmt.Sprintf("wall-time exceeded (%v > %v)", time.Since(b.startTime).Round(time.Second), b.maxWallTime)
		return fmt.Errorf("%w: %s", ErrBudgetExhausted, b.reason)
	}

	if b.maxCalls > 0 && b.calls >= b.maxCalls {
		b.exhausted = true
		b.reason = fmt.Sprintf("call cap reached (%d calls)", b.maxCalls)
		return fmt.Errorf("%w: %s", ErrBudgetExhausted, b.reason)
	}

	b.calls++
	return nil
}

// RecordTokens records token usage and computes financial cost using configured pricing.
func (b *BudgetTracker) RecordTokens(model string, inTokens, outTokens, cachedTokens int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.inputTokens += inTokens
	b.outputTokens += outTokens
	b.cachedTokens += cachedTokens

	// Lookup price
	price, ok := b.prices[model]
	if !ok {
		price = b.prices["default"]
	}

	cost := (float64(inTokens)*price.InputPerMillion +
		float64(outTokens)*price.OutputPerMillion +
		float64(cachedTokens)*price.CachedPerMillion) / 1_000_000.0

	b.costUSD += cost

	totalTokens := b.inputTokens + b.outputTokens
	if b.maxTokens > 0 && totalTokens > b.maxTokens {
		b.exhausted = true
		b.reason = fmt.Sprintf("token limit reached (%d > %d tokens)", totalTokens, b.maxTokens)
		return fmt.Errorf("%w: %s", ErrBudgetExhausted, b.reason)
	}

	if b.maxCostUSD > 0 && b.costUSD > b.maxCostUSD {
		b.exhausted = true
		b.reason = fmt.Sprintf("cost limit reached ($%.2f > $%.2f)", b.costUSD, b.maxCostUSD)
		return fmt.Errorf("%w: %s", ErrBudgetExhausted, b.reason)
	}

	return nil
}

// Exhausted reports whether any budget constraint was breached.
func (b *BudgetTracker) Exhausted() (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exhausted, b.reason
}

// Stats returns a snapshot of current resource utilization.
func (b *BudgetTracker) Stats() (calls, totalTokens int, cost float64, elapsed time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls, b.inputTokens + b.outputTokens, b.costUSD, time.Since(b.startTime)
}

// Summary returns a human-readable one-line summary of resources consumed.
func (b *BudgetTracker) Summary() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fmt.Sprintf("calls=%d tokens=%d (in=%d, out=%d, cached=%d) cost=$%.4f time=%v",
		b.calls, b.inputTokens+b.outputTokens, b.inputTokens, b.outputTokens, b.cachedTokens,
		b.costUSD, time.Since(b.startTime).Round(time.Millisecond))
}
