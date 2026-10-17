package knowledge

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// DefaultSamplesPerArm specifies the minimum sample count per arm in A/B evaluation (N >= 3).
const DefaultSamplesPerArm = 3

// DefaultAlpha specifies the documented significance level (α = 0.05) for the one-sided Mann-Whitney test.
// Under identical baseline and KI round distributions, the false-demotion rate is guaranteed to be ≤ α (≤ 5%).
const DefaultAlpha = 0.05

// DemotionThresholdRatio defines the maximum allowable ratio of mean rounds with KI vs baseline.
// When mean(roundsWithKI) > mean(roundsBaseline) * DemotionThresholdRatio (default 1.0),
// and one-sided Mann-Whitney test shows statistically significant regression (p <= alpha),
// or when Fisher exact test on failure counts shows statistically significant excess failures, the KI is demoted.
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

func isBudgetExhausted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "budget exhausted") || strings.Contains(msg, "budget exceeded") || strings.Contains(msg, "parent budget")
}

// RunABEval executes an A/B evaluation harness comparing convergence rounds without vs with the target knowledge item/rule.
// It executes N >= 3 samples per arm (baseline arm with item = nil, and test arm with target item).
// Demotion threshold:
// Uses Fisher exact test on failure counts and Mann-Whitney test on rounds with alpha split to keep
// false demotion <= alpha across null conditions (0%, 10%, 20% non-convergence).
func RunABEval(ctx context.Context, target any, runner ABEvalRunner, opts ...ABEvalOptions) (*ABEvalResult, error) {
	if target == nil {
		return nil, fmt.Errorf("target is nil")
	}
	if runner == nil {
		return nil, fmt.Errorf("loop runner harness is required")
	}

	var targetID string
	switch item := target.(type) {
	case *KnowledgeItem:
		if item == nil {
			return nil, fmt.Errorf("target knowledge item is nil")
		}
		targetID = item.ID
	case *SynthesizedRule:
		if item == nil {
			return nil, fmt.Errorf("target synthesized rule is nil")
		}
		targetID = item.RuleID
	default:
		return nil, fmt.Errorf("unsupported target type for A/B eval: %T", target)
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
			if isBudgetExhausted(err) {
				return &ABEvalResult{
					TargetID:      targetID,
					SamplesPerArm: samplesPerArm,
					Status:        "unmeasured",
					Regression:    false,
					Summary:       fmt.Sprintf("A/B evaluation unmeasured: sub-run killed by parent budget exhaustion (%v)", err),
				}, nil
			}
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
			if isBudgetExhausted(err) {
				return &ABEvalResult{
					TargetID:      targetID,
					SamplesPerArm: samplesPerArm,
					Status:        "unmeasured",
					Regression:    false,
					Summary:       fmt.Sprintf("A/B evaluation unmeasured: sub-run killed by parent budget exhaustion (%v)", err),
				}, nil
			}
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

	pRound := computeOneSidedMannWhitneyPValue(baselineSamples, kiSamples)
	pFailure := computeFisherExactPValue(kiFailures, samplesPerArm, baselineFailures, samplesPerArm)
	mde := computeMDE(baselineSamples, kiSamples, alpha)

	// Alpha split:
	// For small N (N < 5) where exact test distributions are highly discrete (min p-value is 0.05),
	// setting threshold to alpha bounds the combined false demotion rate to <= alpha (e.g. ~0.03 <= 0.05)
	// while retaining non-zero power. For N >= 5, Bonferroni split (alpha/2 for failure and alpha/2 for rounds)
	// strictly bounds the family-wise error rate by alpha/2 + alpha/2 = alpha.
	alphaFailure := alpha
	alphaRound := alpha
	if samplesPerArm >= 5 {
		alphaFailure = alpha / 2.0
		alphaRound = alpha / 2.0
	}

	// Documented Demotion Rule:
	// A knowledge item regresses and is demoted only when:
	// 1. Fisher exact test on failure counts indicates statistically significant excess failures (pFailure <= alphaFailure), OR
	// 2. Mean KI rounds exceeds baseline * threshold, AND one-sided Mann-Whitney test rejects H0 (pRound <= alphaRound).
	regression := (pFailure <= alphaFailure) || (meanKI > meanBaseline*threshold && pRound <= alphaRound)

	var summary string
	status := "pass"
	if regression {
		status = "regression"
		if pFailure <= alphaFailure {
			summary = fmt.Sprintf("A/B evaluation regression across %d samples: KI failures (%d/%d) significantly higher than baseline (%d/%d) (pFailure=%.4f <= alpha=%.2f, MDE=%.2f)", samplesPerArm, kiFailures, samplesPerArm, baselineFailures, samplesPerArm, pFailure, alphaFailure, mde)
		} else {
			summary = fmt.Sprintf("A/B evaluation regression across %d samples: %.2f mean rounds with KI vs %.2f baseline rounds (pRound=%.4f <= alpha=%.2f, pFailure=%.4f, MDE=%.2f, threshold ratio %.2f)", samplesPerArm, meanKI, meanBaseline, pRound, alphaRound, pFailure, mde, threshold)
		}
	} else {
		summary = fmt.Sprintf("A/B evaluation pass across %d samples: %.2f mean rounds with KI vs %.2f baseline rounds (pRound=%.4f, pFailure=%.4f, alpha=%.2f, MDE=%.2f)", samplesPerArm, meanKI, meanBaseline, pRound, pFailure, alpha, mde)
	}

	switch item := target.(type) {
	case *KnowledgeItem:
		if regression {
			item.AutoDemote(summary)
		}
	case *SynthesizedRule:
		if regression {
			item.AutoDemote(summary)
		}
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
		PValue:          pRound,
		PRound:          pRound,
		PFailure:        pFailure,
		MDE:             mde,
		KIFailures:      kiFailures,
		BaseFailures:    baselineFailures,
		Alpha:           alpha,
		Regression:      regression,
		Status:          status,
		Summary:         summary,
	}, nil
}

func computeMDE(baseline, ki []int, alpha float64) float64 {
	n1 := len(baseline)
	n2 := len(ki)
	if n1 == 0 || n2 == 0 {
		return 0
	}
	var sumB, sumK float64
	for _, v := range baseline {
		sumB += float64(v)
	}
	for _, v := range ki {
		sumK += float64(v)
	}
	meanB := sumB / float64(n1)
	meanK := sumK / float64(n2)

	var ssq float64
	for _, v := range baseline {
		diff := float64(v) - meanB
		ssq += diff * diff
	}
	for _, v := range ki {
		diff := float64(v) - meanK
		ssq += diff * diff
	}

	df := n1 + n2 - 2
	var s float64
	if df > 0 {
		s = math.Sqrt(ssq / float64(df))
	}
	if s <= 0 {
		s = 1.0 // fallback floor if sample variance is 0
	}

	// Critical value for one-sided alpha and 80% power (beta = 0.20 -> z = 0.842)
	zAlpha := 1.645
	if alpha <= 0.01 {
		zAlpha = 2.326
	} else if alpha <= 0.025 {
		zAlpha = 1.960
	}
	zBeta := 0.842
	se := s * math.Sqrt(1.0/float64(n1)+1.0/float64(n2))
	return (zAlpha + zBeta) * se
}

func computeFisherExactPValue(kiFailures, kiTotal, baseFailures, baseTotal int) float64 {
	a := kiFailures
	b := kiTotal - kiFailures
	c := baseFailures
	d := baseTotal - baseFailures

	n1 := a + b
	n2 := c + d
	k := a + c
	totalN := n1 + n2

	if k == 0 || n1 == 0 || n2 == 0 {
		return 1.0
	}
	if a <= c {
		return 1.0
	}

	denom := logComb(totalN, k)
	var pValue float64
	maxA := n1
	if k < maxA {
		maxA = k
	}
	for x := a; x <= maxA; x++ {
		logNum := logComb(n1, x) + logComb(n2, k-x)
		prob := math.Exp(logNum - denom)
		pValue += prob
	}
	if pValue > 1.0 {
		pValue = 1.0
	}
	return pValue
}

func logComb(n, k int) float64 {
	if k < 0 || k > n {
		return math.Inf(-1)
	}
	if k == 0 || k == n {
		return 0
	}
	return logFactorial(n) - logFactorial(k) - logFactorial(n-k)
}

func logFactorial(n int) float64 {
	val, _ := math.Lgamma(float64(n + 1))
	return val
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

