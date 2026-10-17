package knowledge

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
)

func TestP1_5_ABEval_NullSimulations_At_0_10_20_Percent_NonConvergence(t *testing.T) {
	// The reviewer reproduced false demotion rates:
	// 2.1% (0% non-convergence), 21.3% (10% non-convergence), 29.4% (20% non-convergence)
	// when N=3.
	// Fix requires: Fisher exact on failure counts with alpha split, so false demotion <= alpha (0.05)
	// across all non-convergence rates: 0%, 10%, and 20%.

	nonConvergenceRates := []float64{0.0, 0.10, 0.20}
	const trials = 10000

	for _, failureRate := range nonConvergenceRates {
		r := rand.New(rand.NewSource(1337 + int64(failureRate*100)))
		falseDemotions := 0

		for i := 0; i < trials; i++ {
			ki := &KnowledgeItem{
				ID:     fmt.Sprintf("ki-sim-null-%d", i),
				Status: "active",
			}

			// Identical distribution for both arms:
			// Rounds: uniform 1..4
			// Non-convergence: failureRate
			runner := func(ctx context.Context, item any) (int, bool, error) {
				rounds := r.Intn(4) + 1
				success := r.Float64() >= failureRate
				return rounds, success, nil
			}

			res, err := RunABEval(t.Context(), ki, runner, ABEvalOptions{
				SamplesPerArm: 3,
				Threshold:     1.0,
				Alpha:         DefaultAlpha,
			})
			if err != nil {
				t.Fatalf("RunABEval trial %d failed: %v", i, err)
			}
			if res.Regression {
				falseDemotions++
			}
		}

		rate := float64(falseDemotions) / float64(trials)
		t.Logf("Null simulation (%.0f%% non-convergence): false demotions %d/%d (rate: %.4f, alpha: %.2f)",
			failureRate*100, falseDemotions, trials, rate, DefaultAlpha)

		if rate > DefaultAlpha {
			t.Fatalf("SECURITY/STATISTICAL FLAW: false-demotion rate %.4f at %.0f%% non-convergence exceeds alpha %.2f (reviewer reproduced flaw)",
				rate, failureRate*100, DefaultAlpha)
		}
	}
}

func TestP1_5_ABEval_ReportsMDE_And_CatchesLargeRegression(t *testing.T) {
	r := rand.New(rand.NewSource(999))
	const regressionTrials = 1000
	regressionDemotions := 0

	for i := 0; i < regressionTrials; i++ {
		ki := &KnowledgeItem{
			ID:     fmt.Sprintf("ki-sim-regress-%d", i),
			Status: "active",
		}

		// Clear regression: baseline rounds 1..2 (always succeeds), KI rounds 5..8 (always succeeds)
		runner := func(ctx context.Context, item any) (int, bool, error) {
			if item == nil {
				return r.Intn(2) + 1, true, nil
			}
			return r.Intn(4) + 5, true, nil
		}

		res, err := RunABEval(t.Context(), ki, runner, ABEvalOptions{
			SamplesPerArm: 3,
			Threshold:     1.0,
			Alpha:         DefaultAlpha,
		})
		if err != nil {
			t.Fatalf("trial %d failed: %v", i, err)
		}
		if res.MDE <= 0 {
			t.Fatalf("expected non-zero MDE reported in ABEvalResult, got %.4f", res.MDE)
		}
		if res.Regression {
			regressionDemotions++
		}
	}

	regressionRate := float64(regressionDemotions) / float64(regressionTrials)
	t.Logf("Large-regression simulation: %d/%d demoted (rate: %.4f)", regressionDemotions, regressionTrials, regressionRate)
	if regressionRate < 0.95 {
		t.Fatalf("large-regression demotion rate %.4f is less than required 0.95", regressionRate)
	}
}
