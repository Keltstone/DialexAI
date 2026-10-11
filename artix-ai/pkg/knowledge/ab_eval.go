package knowledge

import (
	"context"
	"fmt"
)

// ABEvalResult holds the outcome of an A/B evaluation measuring rounds-to-converge with vs without a knowledge item or rule.
type ABEvalResult struct {
	TargetID       string `json:"targetId"`
	RoundsBaseline int    `json:"roundsBaseline"`
	RoundsWithKI   int    `json:"roundsWithKi"`
	Regression     bool   `json:"regression"`
	Status         string `json:"status,omitempty"` // "pass", "regression", "unmeasured"
	Summary        string `json:"summary"`
}

// ABEvalRunner is a function that executes the convergence loop under a specific knowledge configuration
// and returns the rounds taken to converge, whether it succeeded, and any error.
// When item is nil, it runs the baseline configuration without the candidate item.
type ABEvalRunner func(ctx context.Context, item any) (rounds int, success bool, err error)

// RunABEval executes an A/B evaluation harness comparing convergence rounds without vs with the target knowledge item/rule.
// It runs the loop twice:
//  1. Baseline run without the target item (item = nil)
//  2. Test run with the target item
//
// If the knowledge item causes a regression (roundsWithKI > roundsBaseline or convergence failure), it is automatically demoted.
func RunABEval(ctx context.Context, target any, runner ABEvalRunner) (*ABEvalResult, error) {
	if target == nil {
		return nil, fmt.Errorf("target is nil")
	}
	if runner == nil {
		return nil, fmt.Errorf("loop runner harness is required")
	}

	// 1. Run baseline loop without the candidate item
	roundsBaseline, successBaseline, errBase := runner(ctx, nil)
	if errBase != nil {
		return nil, fmt.Errorf("baseline loop run failed: %w", errBase)
	}
	if roundsBaseline <= 0 {
		roundsBaseline = 1
	}

	// 2. Run loop with the candidate item
	roundsWithKI, successWithKI, errKI := runner(ctx, target)
	if errKI != nil {
		return nil, fmt.Errorf("loop run with target failed: %w", errKI)
	}
	if roundsWithKI <= 0 {
		roundsWithKI = 1
	}

	regression := !successWithKI || (successBaseline && roundsWithKI > roundsBaseline)
	var targetID string
	var summary string

	status := "pass"
	if regression {
		status = "regression"
		summary = fmt.Sprintf("A/B evaluation regression: %d rounds with KI vs %d baseline rounds", roundsWithKI, roundsBaseline)
	} else {
		summary = fmt.Sprintf("A/B evaluation pass: %d rounds with KI vs %d baseline rounds", roundsWithKI, roundsBaseline)
	}

	switch item := target.(type) {
	case *KnowledgeItem:
		if item == nil {
			return nil, fmt.Errorf("target knowledge item is nil")
		}
		targetID = item.ID
		if regression {
			item.AutoDemote(summary)
		}
	case *SynthesizedRule:
		if item == nil {
			return nil, fmt.Errorf("target synthesized rule is nil")
		}
		targetID = item.RuleID
		if regression {
			item.AutoDemote(summary)
		}
	default:
		return nil, fmt.Errorf("unsupported target type for A/B eval: %T", target)
	}

	return &ABEvalResult{
		TargetID:       targetID,
		RoundsBaseline: roundsBaseline,
		RoundsWithKI:   roundsWithKI,
		Regression:     regression,
		Status:         status,
		Summary:        summary,
	}, nil
}

