package hunt

import (
	"fmt"
	"strings"

	"sentinix/pkg/ai"
	"sentinix/pkg/engines"
)

// Vote records an individual model's verdict on a candidate finding.
type Vote struct {
	ModelName  string
	Vulnerable bool
	Severity   engines.Severity
	Confidence float64
	Reason     string
}

// ConsensusResult aggregates individual votes into an accountable collective verdict.
type ConsensusResult struct {
	Vulnerable    bool
	Severity      engines.Severity
	Confidence    float64
	Reason        string
	Votes         []Vote
	Evidence      string
	AgreedCount   int
	DissentCount  int
}

// EvaluateConsensus tallies votes from multiple models or self-consistency passes.
// If affirmative votes >= minVotes, or in case of a tie, it decides in favor of defensive warning.
func EvaluateConsensus(votes []Vote, minVotes int) ConsensusResult {
	if len(votes) == 0 {
		return ConsensusResult{Vulnerable: false}
	}
	if minVotes <= 0 {
		minVotes = (len(votes) / 2) + 1 // Simple majority
	}

	var affVotes, negVotes []Vote
	var totalConfidence float64
	highestSev := engines.Info
	reasons := []string{}

	for _, v := range votes {
		totalConfidence += v.Confidence
		if v.Vulnerable {
			affVotes = append(affVotes, v)
			if v.Severity > highestSev {
				highestSev = v.Severity
			}
			if v.Reason != "" {
				reasons = append(reasons, fmt.Sprintf("[%s]: %s", v.ModelName, v.Reason))
			}
		} else {
			negVotes = append(negVotes, v)
		}
	}

	avgConfidence := totalConfidence / float64(len(votes))

	// Defensive rule: ties or meeting minVotes decide as vulnerable
	isVulnerable := len(affVotes) >= minVotes || (len(affVotes) > 0 && len(affVotes) == len(negVotes))

	var agreeingModels, dissentingModels []string
	if isVulnerable {
		for _, v := range affVotes {
			agreeingModels = append(agreeingModels, v.ModelName)
		}
		for _, v := range negVotes {
			dissentingModels = append(dissentingModels, v.ModelName)
		}
	} else {
		for _, v := range negVotes {
			agreeingModels = append(agreeingModels, v.ModelName)
		}
		for _, v := range affVotes {
			dissentingModels = append(dissentingModels, v.ModelName)
		}
	}

	evidence := fmt.Sprintf("council [%d/%d agreed: %s",
		len(agreeingModels), len(votes), strings.Join(agreeingModels, ", "))
	if len(dissentingModels) > 0 {
		evidence += fmt.Sprintf("; dissent: %s", strings.Join(dissentingModels, ", "))
	}
	evidence += "]"

	finalReason := "No vulnerability found"
	if isVulnerable && len(reasons) > 0 {
		finalReason = strings.Join(reasons, " | ")
	}

	return ConsensusResult{
		Vulnerable:   isVulnerable,
		Severity:     highestSev,
		Confidence:   avgConfidence,
		Reason:       finalReason,
		Votes:        votes,
		Evidence:     evidence,
		AgreedCount:  len(agreeingModels),
		DissentCount: len(dissentingModels),
	}
}

// SelectModelForRole routes a specific task role ("discover", "classify", "validate", "poc", "report")
// to the preferred model configured in HuntConfig.Roles, with fallback to any allowed model.
func SelectModelForRole(models []ai.Model, policy string, roles map[string]string, role string) (ai.Model, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("%w: no models configured", engines.ErrEngineUnavailable)
	}

	// 1. Check if a model is explicitly assigned for this role
	if roles != nil {
		if preferredName, ok := roles[role]; ok && preferredName != "" {
			for _, m := range models {
				if m.Name() == preferredName && ai.Allowed(policy, m) {
					return m, nil
				}
			}
		}
	}

	// 2. Fallback to first policy-allowed model
	for _, m := range models {
		if ai.Allowed(policy, m) {
			return m, nil
		}
	}

	return nil, fmt.Errorf("%w: no model allowed by policy %q for role %q", engines.ErrEngineUnavailable, policy, role)
}
