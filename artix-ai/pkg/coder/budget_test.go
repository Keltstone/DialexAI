package coder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"artix/pkg/audit"
	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

func TestBoard3_LoopBudgetLimits_Tokens(t *testing.T) {
	tempDir, driver := setupTestRepo(t)
	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(tempDir))
	s := &spec.StorySpec{ID: "B3-1", Title: "Budget test", TestCommands: []string{"grep '2' counter.txt"}}
	rc := &repo.RepositoryContext{RootDir: tempDir}

	// Token usage simulator tracking usage > MaxTokens
	opts := &LoopOptions{
		MaxRounds: 5,
		MaxTokens: 500, // hard cap 500 tokens
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
		CoderUsageTracker: func() *ProviderUsage {
			return &ProviderUsage{PromptTokens: 300, CompletionTokens: 300, TotalTokens: 600}
		},
	}

	res := coord.Run(context.Background(), s, rc, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to abort when MaxTokens exceeded, got success")
	}
	if !strings.Contains(res.Error, "budget") || !strings.Contains(res.Error, "token") {
		t.Fatalf("expected budget token error, got: %s", res.Error)
	}
}

func TestBoard3_LoopBudgetLimits_USD(t *testing.T) {
	tempDir, driver := setupTestRepo(t)
	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(tempDir))
	s := &spec.StorySpec{ID: "B3-2", Title: "USD budget test", TestCommands: []string{"grep '2' counter.txt"}}
	rc := &repo.RepositoryContext{RootDir: tempDir}

	opts := &LoopOptions{
		MaxRounds: 5,
		MaxUSD:    0.05, // hard cap $0.05
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
		CoderUsageTracker: func() *ProviderUsage {
			return &ProviderUsage{PromptTokens: 10000, CompletionTokens: 5000, TotalTokens: 15000} // $0.10+
		},
	}

	res := coord.Run(context.Background(), s, rc, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to abort when MaxUSD exceeded, got success")
	}
	if !strings.Contains(res.Error, "budget") || !strings.Contains(res.Error, "USD") {
		t.Fatalf("expected budget USD error, got: %s", res.Error)
	}
}

func TestBoard3_LoopBudgetLimits_WallClock(t *testing.T) {
	tempDir, driver := setupTestRepo(t)
	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(tempDir))
	s := &spec.StorySpec{ID: "B3-3", Title: "Wall clock test", TestCommands: []string{"grep '2' counter.txt"}}
	rc := &repo.RepositoryContext{RootDir: tempDir}

	opts := &LoopOptions{
		MaxRounds: 5,
		MaxWall:   10 * time.Millisecond, // very short timeout
		MockPatchGen: func(round int, feedback string) string {
			time.Sleep(25 * time.Millisecond)
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
	}

	res := coord.Run(context.Background(), s, rc, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to abort when MaxWall exceeded, got success")
	}
	if !strings.Contains(res.Error, "budget") || !strings.Contains(res.Error, "wall clock") {
		t.Fatalf("expected budget wall clock error, got: %s", res.Error)
	}
}

func TestBoard3_DisjointCoderAndCriticModelFamily(t *testing.T) {
	reg := persona.NewRegistry("")
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCoderFamily("openai")

	// 1. Same family -> must be rejected
	critic := func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	}
	errSame := rev.SetCriticWithFamily(critic, "openai")
	if errSame == nil {
		t.Errorf("expected SetCriticWithFamily to reject same model family (openai == openai)")
	}

	// 2. Disjoint family -> must be accepted
	errDiff := rev.SetCriticWithFamily(critic, "anthropic")
	if errDiff != nil {
		t.Errorf("expected SetCriticWithFamily to accept disjoint model family (openai != anthropic), got: %v", errDiff)
	}
}

func TestBoard3_Reviewer_TestDiff_Integrity(t *testing.T) {
	reg := persona.NewRegistry("")
	rev := reviewer.NewAdversarialReviewer(reg)

	// 1. Assertion removal -> must be rejected
	removalDiff := `diff --git a/app_test.go b/app_test.go
--- a/app_test.go
+++ b/app_test.go
@@ -10,2 +10,1 @@
-	if got != want { t.Fatalf("expected %d got %d", want, got) }
+	_ = got
`
	v1 := rev.Evaluate(&reviewer.ReviewContext{
		Diff: removalDiff,
	})
	if v1.Approved || len(v1.BlockingIssues) == 0 {
		t.Errorf("expected Reviewer to reject assertion removal in test diff, got verdict: %+v", v1)
	}

	// 2. Skip addition -> must be rejected
	skipDiff := `diff --git a/app_test.go b/app_test.go
--- a/app_test.go
+++ b/app_test.go
@@ -5,1 +5,2 @@
+	t.Skip("skipping flaky test")
 	assert.Equal(t, 1, 1)
`
	v2 := rev.Evaluate(&reviewer.ReviewContext{
		Diff: skipDiff,
	})
	if v2.Approved || len(v2.BlockingIssues) == 0 {
		t.Errorf("expected Reviewer to reject t.Skip addition in test diff, got verdict: %+v", v2)
	}
}

func TestMaxUSD_CustomModelPrice_AbortsAtCalculatedDollarAmount(t *testing.T) {
	tempDir, driver := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(tempDir))
	s := &spec.StorySpec{ID: "N6-USD", Title: "Custom model pricing test", TestCommands: []string{"grep '1' counter.txt"}}
	rc := &repo.RepositoryContext{RootDir: tempDir}

	// Non-default price: $0.075 per 1k tokens ($75/M tokens)
	customBudget := &TokenBudget{
		PriceTable: map[string]float64{
			"claude-3-opus": 0.075,
		},
	}

	opts := &LoopOptions{
		MaxRounds: 5,
		MaxUSD:    0.10, // Cap is $0.10
		Model:     "claude-3-opus",
		Budget:    customBudget,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
		CoderUsageTracker: func() *ProviderUsage {
			// 2,000 tokens @ $0.075/1k = $0.150 (> $0.10 cap)
			// If using default $0.015/1k, 2,000 tokens = $0.030 (< $0.10 cap and would NOT abort)
			return &ProviderUsage{PromptTokens: 1000, CompletionTokens: 1000, TotalTokens: 2000}
		},
	}

	res := coord.Run(context.Background(), s, rc, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to abort when MaxUSD ($0.10) exceeded by custom model price ($0.15), got success")
	}
	if (!strings.Contains(res.Error, "task budget limit exceeded") && !strings.Contains(res.Error, "story cost budget exceeded")) || (!strings.Contains(res.Error, "0.10") && !strings.Contains(res.Error, "0.15")) {
		t.Fatalf("expected task budget USD limit reached error with custom pricing ($0.15), got: %s", res.Error)
	}
}

func TestMaxUSD_UnpricedModel_AbortsAndAudits(t *testing.T) {
	tempDir, driver := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(tempDir))
	s := &spec.StorySpec{ID: "R5-UNPRICED", Title: "Unpriced model USD budget test", TestCommands: []string{"grep '1' counter.txt"}}
	rc := &repo.RepositoryContext{RootDir: tempDir}

	// Budget with a price table that does NOT include "unpriced-model-v1"
	customBudget := &TokenBudget{
		MaxStoryCost: 0.10,
		PriceTable: map[string]float64{
			"claude-3-opus": 0.075,
		},
	}

	opts := &LoopOptions{
		MaxRounds: 5,
		MaxUSD:    0.10,
		Model:     "unpriced-model-v1",
		Budget:    customBudget,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
	}

	res := coord.Run(context.Background(), s, rc, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to abort when MaxUSD is configured with an unpriced model, got success")
	}
	if !strings.Contains(res.Error, "unpriced-model-v1") && !strings.Contains(res.Error, "price") && !strings.Contains(res.Error, "budget") {
		t.Fatalf("expected error mentioning unpriced model / price table / budget, got: %s", res.Error)
	}

	// Verify audit log recorded the budget rejection with unpriced model details
	logPath := filepath.Join(tempDir, ".artix", "audit.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected audit log file at %s: %v", logPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	found := false
	for _, line := range lines {
		var evt audit.AuditEvent
		if json.Unmarshal([]byte(line), &evt) == nil && (evt.EventType == audit.EventBudgetExhausted || evt.EventType == "budget.exhausted") {
			if evt.Details != nil && (evt.Details["unpricedModel"] == "unpriced-model-v1" || evt.Details["model"] == "unpriced-model-v1" || strings.Contains(line, "unpriced-model-v1")) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected budget exhaustion audit event to record unpriced model details, got log:\n%s", string(data))
	}
}

func TestMaxUSD_NonDefaultModelPrice_AbortsWithoutCustomBudgetPassed(t *testing.T) {
	tempDir, driver := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(tempDir))
	s := &spec.StorySpec{ID: "N6-PRICING-ENV", Title: "Pricing derived from policy/model", TestCommands: []string{"grep '1' counter.txt"}}
	rc := &repo.RepositoryContext{RootDir: tempDir}

	// Active policy configured with model price: $0.080 per 1k tokens for "gpt-4-custom"
	policy.SetActivePolicyForTest(&policy.Policy{
		IsVerified: true,
		Budget: policy.BudgetConfig{
			PriceTable: map[string]float64{
				"gpt-4-custom": 0.080,
			},
		},
	})
	defer policy.ResetTestPolicy()

	opts := &LoopOptions{
		MaxRounds: 5,
		MaxUSD:    0.05, // Cap is $0.05
		Model:     "gpt-4-custom",
		Budget:    nil, // Budget is nil! Must be initialized and read pricing from active policy / TokenBudget.GetModelPrice
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
		CoderUsageTracker: func() *ProviderUsage {
			// 1,000 tokens @ $0.080/1k = $0.080 (> $0.05 cap -> must abort!)
			// If using hardcoded $0.015/1k, 1,000 tokens = $0.015 (< $0.05 cap and would NOT abort)
			return &ProviderUsage{PromptTokens: 500, CompletionTokens: 500, TotalTokens: 1000}
		},
	}

	res := coord.Run(context.Background(), s, rc, nil, nil, opts)
	if res.Success {
		t.Fatalf("expected loop to abort when MaxUSD ($0.05) exceeded by model price ($0.08), but succeeded (hardcoded 0.015 price bug)")
	}
	if (!strings.Contains(res.Error, "task budget limit exceeded") && !strings.Contains(res.Error, "story cost budget exceeded")) || (!strings.Contains(res.Error, "0.05") && !strings.Contains(res.Error, "0.08")) {
		t.Fatalf("expected task budget USD limit reached error, got: %s", res.Error)
	}
}


