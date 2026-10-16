package knowledge

import (
	"context"
	"fmt"
	"math"
)

// DefaultSamplesPerArm specifies the minimum sample count per arm in A/B evaluation (N >= 3).
const DefaultSamplesPerArm = 3

// DemotionThresholdRatio defines the maximum allowable ratio of mean rounds with KI vs baseline.
// When mean(roundsWithKI) > mean(roundsBaseline) * DemotionThresholdRatio (default 1.0),
// or when any sample fails convergence with KI while baseline succeeded, the KI is demoted.
const DemotionThresholdRatio = 1.0

// ABEvalOptions configures the A/B evaluation parameters.
type ABEvalOptions struct {
	SamplesPerArm int     `json:"samplesPerArm"` // N >= 3 samples per arm
	Threshold     float64 `json:"threshold"`     // Ratio above baseline triggering demotion
}

// ABEvalResult holds the outcome of an A/B evaluation measuring rounds-to-converge with vs without a knowledge item or rule.
type ABEvalResult struct {
	TargetID        string    `json:"targetId"`
	SamplesPerArm   int       `json:"samplesPerArm"`
	RoundsBaseline  int       `json:"roundsBaseline"`
	RoundsWithKI    int       `json:"roundsWithKi"`
	MeanBaseline    float64   `json:"meanBaseline"`
	MeanWithKI      float64   `json:"meanWithKi"`
	BaselineSamples []int     `json:"baselineSamples,omitempty"`
	KISamples       []int     `json:"kiSamples,omitempty"`
	Regression      bool      `json:"regression"`
	Status          string    `json:"status,omitempty"` // "pass", "regression", "unmeasured"
	Summary         string    `json:"summary"`
}

// ABEvalRunner is a function that executes the convergence loop under a specific knowledge configuration
// and returns the rounds taken to converge, whether it succeeded, and any error.
// When item is nil, it runs the baseline configuration without the candidate item.
type ABEvalRunner func(ctx context.Context, item any) (rounds int, success bool, err error)

// RunABEval executes an A/B evaluation harness comparing convergence rounds without vs with the target knowledge item/rule.
// It executes N >= 3 samples per arm (baseline arm with item = nil, and test arm with target item).
// Demotion threshold:
// If mean rounds with KI exceeds mean baseline rounds (or if KI samples fail convergence while baseline succeeded),
// the candidate item is automatically demoted.
func RunABEval(ctx context.Context, target any, runner ABEvalRunner, opts ...ABEvalOptions) (*ABEvalResult, error) {
	if target == nil {
		return nil, fmt.Errorf("target is nil")
	}
	if runner == nil {
		return nil, fmt.Errorf("loop runner harness is required")
	}

	samplesPerArm := DefaultSamplesPerArm
	threshold := DemotionThresholdRatio
	if len(opts) > 0 {
		if opts[0].SamplesPerArm >= 3 {
			samplesPerArm = opts[0].SamplesPerArm
		}
		if opts[0].Threshold > 0 {
			threshold = opts[0].Threshold
		}
	}

	// 1. Run baseline loop N times without the candidate item
	baselineSamples := make([]int, 0, samplesPerArm)
	totalBaselineRounds := 0
	baselineFailures := 0
	for i := 0; i < samplesPerArm; i++ {
		rounds, success, err := runner(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("baseline loop run (sample %d/%d) failed: %w", i+1, samplesPerArm, err)
		}
		if rounds <= 0 {
			rounds = 1
		}
		baselineSamples = append(baselineSamples, rounds)
		totalBaselineRounds += rounds
		if !success {
			baselineFailures++
		}
	}
	meanBaseline := float64(totalBaselineRounds) / float64(samplesPerArm)

	// 2. Run loop N times with the candidate item
	kiSamples := make([]int, 0, samplesPerArm)
	totalKIRounds := 0
	kiFailures := 0
	for i := 0; i < samplesPerArm; i++ {
		rounds, success, err := runner(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("loop run with target (sample %d/%d) failed: %w", i+1, samplesPerArm, err)
		}
		if rounds <= 0 {
			rounds = 1
		}
		kiSamples = append(kiSamples, rounds)
		totalKIRounds += rounds
		if !success {
			kiFailures++
		}
	}
	meanKI := float64(totalKIRounds) / float64(samplesPerArm)

	// Documented Demotion Rule:
	// A knowledge item regresses and is demoted if:
	// 1. kiFailures > baselineFailures, OR
	// 2. Convergence succeeded but meanKI > meanBaseline * threshold
	regression := (kiFailures > baselineFailures) || (meanKI > meanBaseline*threshold)

	var targetID string
	var summary string

	status := "pass"
	if regression {
		status = "regression"
		if kiFailures > baselineFailures {
			summary = fmt.Sprintf("A/B evaluation regression across %d samples: %d KI failures vs %d baseline failures", samplesPerArm, kiFailures, baselineFailures)
		} else {
			summary = fmt.Sprintf("A/B evaluation regression across %d samples: %.2f mean rounds with KI vs %.2f baseline rounds (threshold ratio %.2f)", samplesPerArm, meanKI, meanBaseline, threshold)
		}
	} else {
		summary = fmt.Sprintf("A/B evaluation pass across %d samples: %.2f mean rounds with KI vs %.2f baseline rounds", samplesPerArm, meanKI, meanBaseline)
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
		TargetID:        targetID,
		SamplesPerArm:   samplesPerArm,
		RoundsBaseline:  int(math.Round(meanBaseline)),
		RoundsWithKI:    int(math.Round(meanKI)),
		MeanBaseline:    meanBaseline,
		MeanWithKI:      meanKI,
		BaselineSamples: baselineSamples,
		KISamples:       kiSamples,
		Regression:      regression,
		Status:          status,
		Summary:         summary,
	}, nil
}
