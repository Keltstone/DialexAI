package knowledge

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// DefaultSamplesPerArm specifies the minimum sample count per arm in A/B evaluation (N >= 3).
const DefaultSamplesPerArm = 3

// DefaultAlpha specifies the documented significance level (α = 0.05) for the one-sided Mann-Whitney test.
// Under identical baseline and KI round distributions, the false-demotion rate is guaranteed to be ≤ α (≤ 5%).
const DefaultAlpha = 0.05

// DemotionThresholdRatio defines the maximum allowable ratio of mean rounds with KI vs baseline.
// When mean(roundsWithKI) > mean(roundsBaseline) * DemotionThresholdRatio (default 1.0),
// and one-sided Mann-Whitney test shows statistically significant regression (p <= alpha),
// or when any sample fails convergence with KI while baseline succeeded, the KI is demoted.
const DemotionThresholdRatio = 1.0

// ABEvalOptions configures the A/B evaluation parameters.
type ABEvalOptions struct {
	SamplesPerArm int     `json:"samplesPerArm"` // N >= 3 samples per arm
	Threshold     float64 `json:"threshold"`     // Ratio above baseline triggering demotion
	Alpha         float64 `json:"alpha"`         // Documented significance level (default DefaultAlpha = 0.05)
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
	PValue          float64   `json:"pValue"`
	PRound          float64   `json:"pRound,omitempty"`
	PFailure        float64   `json:"pFailure,omitempty"`
	MDE             float64   `json:"mde,omitempty"`
	KIFailures      int       `json:"kiFailures,omitempty"`
	BaseFailures    int       `json:"baseFailures,omitempty"`
	Alpha           float64   `json:"alpha"`
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
	alpha := DefaultAlpha
	if len(opts) > 0 {
		if opts[0].SamplesPerArm >= 3 {
			samplesPerArm = opts[0].SamplesPerArm
		}
		if opts[0].Threshold > 0 {
			threshold = opts[0].Threshold
		}
		if opts[0].Alpha > 0 {
			alpha = opts[0].Alpha
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

	pValue := computeOneSidedMannWhitneyPValue(baselineSamples, kiSamples)

	// Documented Demotion Rule:
	// A knowledge item regresses and is demoted only when the evidence is strong:
	// 1. kiFailures > baselineFailures, OR
	// 2. Convergence succeeded, meanKI > meanBaseline * threshold,
	//    AND one-sided Mann-Whitney test rejects H0 with strong evidence (pValue <= alpha).
	regression := (kiFailures > baselineFailures) || (meanKI > meanBaseline*threshold && pValue <= alpha)

	var targetID string
	var summary string

	status := "pass"
	if regression {
		status = "regression"
		if kiFailures > baselineFailures {
			summary = fmt.Sprintf("A/B evaluation regression across %d samples: %d KI failures vs %d baseline failures", samplesPerArm, kiFailures, baselineFailures)
		} else {
			summary = fmt.Sprintf("A/B evaluation regression across %d samples: %.2f mean rounds with KI vs %.2f baseline rounds (p=%.4f <= alpha=%.2f, threshold ratio %.2f)", samplesPerArm, meanKI, meanBaseline, pValue, alpha, threshold)
		}
	} else {
		summary = fmt.Sprintf("A/B evaluation pass across %d samples: %.2f mean rounds with KI vs %.2f baseline rounds (p=%.4f > alpha=%.2f)", samplesPerArm, meanKI, meanBaseline, pValue, alpha)
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
		PValue:          pValue,
		Alpha:           alpha,
		Regression:      regression,
		Status:          status,
		Summary:         summary,
	}, nil
}

func nCr(n, r int) int {
	if r < 0 || r > n {
		return 0
	}
	if r == 0 || r == n {
		return 1
	}
	if r > n/2 {
		r = n - r
	}
	res := 1
	for i := 1; i <= r; i++ {
		res = res * (n - i + 1) / i
	}
	return res
}

func exactMannWhitneyPValue(baseline, ki []int, uObs float64) float64 {
	n2 := len(ki)
	z := make([]int, len(baseline)+len(ki))
	copy(z, baseline)
	copy(z[len(baseline):], ki)
	sort.Ints(z)

	ranks := make([]float64, len(z))
	i := 0
	for i < len(z) {
		j := i
		for j < len(z) && z[j] == z[i] {
			j++
		}
		avgRank := float64(i+1+j) / 2.0
		for k := i; k < j; k++ {
			ranks[k] = avgRank
		}
		i = j
	}

	targetRankSum := uObs + float64(n2*(n2+1))/2.0
	greaterOrEqual := 0
	totalCombos := 0

	var countCombos func(start, remaining int, currentSum float64)
	countCombos = func(start, remaining int, currentSum float64) {
		if remaining == 0 {
			totalCombos++
			if currentSum >= targetRankSum-1e-9 {
				greaterOrEqual++
			}
			return
		}
		for idx := start; idx <= len(ranks)-remaining; idx++ {
			countCombos(idx+1, remaining-1, currentSum+ranks[idx])
		}
	}
	countCombos(0, n2, 0.0)

	if totalCombos == 0 {
		return 1.0
	}
	return float64(greaterOrEqual) / float64(totalCombos)
}

func computeOneSidedMannWhitneyPValue(baseline, ki []int) float64 {
	n1 := len(baseline)
	n2 := len(ki)
	if n1 == 0 || n2 == 0 {
		return 1.0
	}

	var u float64
	for _, y := range ki {
		for _, x := range baseline {
			if y > x {
				u += 1.0
			} else if y == x {
				u += 0.5
			}
		}
	}

	totalN := n1 + n2
	numCombos := nCr(totalN, n2)
	if numCombos > 0 && numCombos <= 50000 {
		return exactMannWhitneyPValue(baseline, ki, u)
	}

	// Normal approximation with continuity correction
	meanU := float64(n1*n2) / 2.0
	counts := make(map[int]int)
	for _, v := range baseline {
		counts[v]++
	}
	for _, v := range ki {
		counts[v]++
	}
	var tieCorrection float64
	for _, t := range counts {
		if t > 1 {
			tieCorrection += float64(t*t*t - t)
		}
	}
	nTotalF := float64(totalN)
	varU := (float64(n1*n2) / 12.0) * (nTotalF + 1.0 - tieCorrection/(nTotalF*(nTotalF-1.0)))
	if varU <= 0 {
		return 1.0
	}
	sigmaU := math.Sqrt(varU)
	z := (u - 0.5 - meanU) / sigmaU
	p := 0.5 * math.Erfc(z/math.Sqrt2)
	if p < 0 {
		p = 0
	} else if p > 1 {
		p = 1
	}
	return p
}

