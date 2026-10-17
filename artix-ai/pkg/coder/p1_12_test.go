package coder

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"artix/pkg/persona"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
)

func TestP1_12_TokenAccounting_InputOutputSplit_ReconcilesToInvoice(t *testing.T) {
	// Pre-fix: Cost is TotalTokens * (estimated cost / estimated tokens) - one flat rate.
	// Fix: Support per-model input and output pricing (ModelPricing) and reconcile
	// exact invoice cost on skewed input/output counts.

	b := &TokenBudget{
		MaxStoryCost: 10.0,
		ModelPricingTable: map[string]ModelPricing{
			"claude-3-5-sonnet": {
				InputCostPer1k:  0.003, // $3 / million input
				OutputCostPer1k: 0.015, // $15 / million output
			},
		},
	}

	// Skewed counts: 10,000 prompt tokens ($0.030), 1,000 completion tokens ($0.015), 500 thinking tokens ($0.0075)
	// Expected invoice cost: 10.0 * 0.003 + (1.0 + 0.5) * 0.015 = 0.030 + 0.0225 = 0.0525 USD
	cost, err := b.CalculateModelCost("claude-3-5-sonnet", 10000, 1500)
	if err != nil {
		t.Fatalf("CalculateModelCost failed: %v", err)
	}

	expectedInvoiceUSD := 0.0525
	if math.Abs(cost-expectedInvoiceUSD) > 1e-6 {
		t.Fatalf("expected reconciled invoice cost $%.4f, got $%.4f", expectedInvoiceUSD, cost)
	}
}

func TestP1_12_TokenAccounting_RequireProviderUsageWhenCapSet(t *testing.T) {
	tempDir, d := setupTestRepo(t)
	defer os.RemoveAll(tempDir)

	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	_, _ = d.CommitAll("add main.go")

	reg := persona.NewRegistry(tempDir)
	dc, err := NewDomainCoder("backend_engineer", reg)
	if err != nil {
		t.Fatalf("NewDomainCoder failed: %v", err)
	}

	rev := reviewer.NewAdversarialReviewer(reg)
	box := sandbox.NewSandbox(tempDir)
	coord := NewCoordinator(dc, rev, d, box)

	story := &spec.StorySpec{
		ID:           "BUDGET-USAGE-REQ",
		Title:        "Cap requires provider usage",
		TestCommands: []string{"true"},
	}

	rc, _ := repo.DetectContext(tempDir)

	// Set a story financial cap and pricing
	t.Setenv("ARTIX_BUDGET_PRICE_PER_1K", "0.01")

	opts := &LoopOptions{
		MaxRounds:             1,
		MaxUSD:                1.00,
		Autonomy:              AutonomySupervised,
		TestCommandsConfirmed: true,
		RequireProviderUsage:  true,
		CoderUsageTracker:     nil, // Nil usage tracker when cap is set!
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -2,1 +2,2 @@\n func main() {}\n+// done\n"
		},
	}

	res := coord.Run(context.Background(), story, rc, nil, nil, opts)

	if res.Success {
		t.Fatalf("expected loop to fail when provider usage is missing under active budget cap")
	}
	if res.Error == "" || !containsAny(res.Error, "provider usage", "usage tracking required") {
		t.Fatalf("expected error regarding required provider usage, got: %s", res.Error)
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if stringsContains(s, sub) {
			return true
		}
	}
	return false
}

func stringsContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
