package hunt

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"sentinix/pkg/ai"
	"sentinix/pkg/store"
)

func TestBudgetTrackerLimits(t *testing.T) {
	cfg := HuntConfig{
		MaxCalls:    3,
		MaxTokens:   1000,
		MaxCostUSD:  0.05,
		MaxWallTime: "100ms",
	}
	tr := NewBudgetTracker(cfg)

	// 1. Call budget enforcement
	if err := tr.RecordCall(); err != nil {
		t.Fatalf("call 1 should succeed: %v", err)
	}
	if err := tr.RecordCall(); err != nil {
		t.Fatalf("call 2 should succeed: %v", err)
	}
	if err := tr.RecordCall(); err != nil {
		t.Fatalf("call 3 should succeed: %v", err)
	}
	if err := tr.RecordCall(); err == nil || !strings.Contains(err.Error(), "call cap reached") {
		t.Fatalf("call 4 should fail with call cap: %v", err)
	}
	ex, reason := tr.Exhausted()
	if !ex || !strings.Contains(reason, "call cap") {
		t.Fatalf("expected exhausted reason, got %v: %s", ex, reason)
	}

	// 2. Cost limit enforcement
	cfg2 := HuntConfig{MaxCalls: 100, MaxCostUSD: 0.01}
	tr2 := NewBudgetTracker(cfg2)
	// gpt-4o input: $2.50 per M. 100,000 tokens = $0.25 -> exceeds $0.01
	err := tr2.RecordTokens("gpt-4o", 100000, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "cost limit reached") {
		t.Fatalf("expected cost limit breach, got: %v", err)
	}

	// 3. Wall time limit enforcement
	cfg3 := HuntConfig{MaxCalls: 100, MaxWallTime: "10ms"}
	tr3 := NewBudgetTracker(cfg3)
	time.Sleep(15 * time.Millisecond)
	err = tr3.RecordCall()
	if err == nil || !strings.Contains(err.Error(), "wall-time exceeded") {
		t.Fatalf("expected wall-time breach, got: %v", err)
	}
}

func TestBudgetTrackerConcurrency(t *testing.T) {
	cfg := HuntConfig{MaxCalls: 100, MaxTokens: 1000000}
	tr := NewBudgetTracker(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tr.RecordCall()
			_ = tr.RecordTokens("default", 50, 50, 10)
		}()
	}
	wg.Wait()

	calls, tokens, _, _ := tr.Stats()
	if calls != 20 {
		t.Errorf("expected 20 calls, got %d", calls)
	}
	if tokens != 2000 {
		t.Errorf("expected 2000 tokens, got %d", tokens)
	}
}

type flakyModel struct {
	attempts int
	mu       sync.Mutex
}

func (f *flakyModel) Name() string { return "flaky" }
func (f *flakyModel) Local() bool  { return true }
func (f *flakyModel) Complete(_ context.Context, _, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.attempts < 3 {
		return "", errors.New("rate limit 429: too many requests")
	}
	return `{"vulnerable":false}`, nil
}

func TestReliableModelRetriesOnRateLimit(t *testing.T) {
	flaky := &flakyModel{}
	tr := NewBudgetTracker(HuntConfig{MaxCalls: 10})
	rel := NewReliableModel(flaky, tr, 2, 2*time.Second)

	out, err := rel.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("expected retry success, got err: %v", err)
	}
	if out != `{"vulnerable":false}` {
		t.Fatalf("unexpected reply: %s", out)
	}
	if flaky.attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", flaky.attempts)
	}
}

func TestDryRunPreview(t *testing.T) {
	e := &Engine{
		Models:   []ai.Model{fake{local: true}},
		Policy:   "local-only",
		MaxCalls: 50,
		Config: &HuntConfig{
			DryRun:     true,
			MaxCalls:   50,
			MaxCostUSD: 1.0,
			Policy:     "local-only",
		},
	}
	fs, err := e.Scan(context.Background(), benchDir)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if len(fs) != 0 {
		t.Fatalf("dry run should not return findings, got %d", len(fs))
	}
}

func TestStorePersistenceAndCache(t *testing.T) {
	st, err := store.Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	calls := 0
	e := &Engine{
		Models:    []ai.Model{fake{local: true, calls: &calls}},
		Policy:    "local-only",
		MaxCalls:  100,
		Store:     st,
		ProjectID: "proj-1",
	}

	// First scan persists tasks to DB
	fs1, err := e.Scan(context.Background(), benchDir)
	if err != nil {
		t.Fatalf("scan 1 failed: %v", err)
	}
	firstCalls := calls
	if firstCalls == 0 {
		t.Fatal("expected model calls during first scan")
	}

	// Second scan should hit DB cache and make 0 new model calls
	e2 := &Engine{
		Models:    []ai.Model{fake{local: true, calls: &calls}},
		Policy:    "local-only",
		MaxCalls:  100,
		Store:     st,
		ProjectID: "proj-1",
	}
	fs2, err := e2.Scan(context.Background(), benchDir)
	if err != nil {
		t.Fatalf("scan 2 failed: %v", err)
	}
	if calls != firstCalls {
		t.Fatalf("expected cache hit with no new calls: %d -> %d", firstCalls, calls)
	}
	if len(fs1) != len(fs2) {
		t.Fatalf("cached findings count mismatch: %d vs %d", len(fs1), len(fs2))
	}
}
