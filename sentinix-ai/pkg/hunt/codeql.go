package hunt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"sentinix/pkg/ai"
	"sentinix/pkg/engines"
)

// CodeFlowStep represents one step in a data flow trace (source -> step -> sink).
type CodeFlowStep struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// CodeFlow represents a complete taint propagation path.
type CodeFlow struct {
	ThreadID string         `json:"thread_id,omitempty"`
	Steps    []CodeFlowStep `json:"steps"`
}

// CodeQLFinding extends engines.Finding with data-flow paths extracted from SARIF codeFlows.
type CodeQLFinding struct {
	engines.Finding
	Flows []CodeFlow `json:"flows,omitempty"`
}

// ParseCodeQLSARIF parses a SARIF JSON byte slice, extracting both findings and detailed codeFlow traces.
func ParseCodeQLSARIF(data []byte) ([]CodeQLFinding, error) {
	var sarif struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Name string `json:"name"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				Level     string `json:"level"`
				Message   struct{ Text string } `json:"message"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct{ URI string } `json:"artifactLocation"`
						Region           struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
				CodeFlows []struct {
					ThreadFlows []struct {
						ID        string `json:"id"`
						Locations []struct {
							Location struct {
								Message          struct{ Text string } `json:"message"`
								PhysicalLocation struct {
									ArtifactLocation struct{ URI string } `json:"artifactLocation"`
									Region           struct {
										StartLine int                   `json:"startLine"`
										Snippet   struct{ Text string } `json:"snippet"`
									} `json:"region"`
								} `json:"physicalLocation"`
							} `json:"location"`
						} `json:"locations"`
					} `json:"threadFlows"`
				} `json:"codeFlows"`
			} `json:"results"`
		} `json:"runs"`
	}

	if err := json.Unmarshal(data, &sarif); err != nil {
		return nil, fmt.Errorf("bad SARIF: %w", err)
	}

	var findings []CodeQLFinding
	for _, run := range sarif.Runs {
		for _, res := range resSlice(run.Results) {
			file := ""
			line := 1
			if len(res.Locations) > 0 {
				file = res.Locations[0].PhysicalLocation.ArtifactLocation.URI
				line = res.Locations[0].PhysicalLocation.Region.StartLine
				if line < 1 {
					line = 1
				}
			}

			sev, cat := engines.MapSARIFSeverityAndCategory("CodeQL", res.RuleID, res.Level, res.Message.Text, engines.CategorySecurity)

			var flows []CodeFlow
			for _, cf := range res.CodeFlows {
				for _, tf := range cf.ThreadFlows {
					var steps []CodeFlowStep
					for _, loc := range tf.Locations {
						p := loc.Location.PhysicalLocation
						stepLine := p.Region.StartLine
						if stepLine < 1 {
							stepLine = 1
						}
						steps = append(steps, CodeFlowStep{
							File:    p.ArtifactLocation.URI,
							Line:    stepLine,
							Message: loc.Location.Message.Text,
							Snippet: p.Region.Snippet.Text,
						})
					}
					if len(steps) > 0 {
						flows = append(flows, CodeFlow{
							ThreadID: tf.ID,
							Steps:    steps,
						})
					}
				}
			}

			cqf := CodeQLFinding{
				Finding: engines.Finding{
					Rule:     res.RuleID,
					Engine:   "codeql",
					Category: cat,
					File:     file,
					Line:     line,
					Severity: sev,
					Message:  res.Message.Text,
					Source:   "sast",
				},
				Flows: flows,
			}

			if len(flows) > 0 {
				cqf.Finding.Evidence = FormatFlowTrace(flows[0])
			}

			findings = append(findings, cqf)
		}
	}

	return findings, nil
}

func resSlice[T any](s []T) []T { return s }

// FormatFlowTrace formats a data flow into a concise, readable trace for LLM prompts.
func FormatFlowTrace(flow CodeFlow) string {
	if len(flow.Steps) == 0 {
		return ""
	}
	var parts []string
	for i, step := range flow.Steps {
		role := "step"
		if i == 0 {
			role = "source"
		} else if i == len(flow.Steps)-1 {
			role = "sink"
		}
		desc := fmt.Sprintf("[%s] %s:%d", role, step.File, step.Line)
		if step.Message != "" {
			desc += fmt.Sprintf(" (%s)", step.Message)
		}
		parts = append(parts, desc)
	}
	return strings.Join(parts, " -> ")
}

// CodeQLCacheKey computes a deterministic cache key for a CodeQL database by commit and language.
func CodeQLCacheKey(commitSHA, lang string) string {
	h := sha256.Sum256([]byte(commitSHA + ":" + strings.ToLower(lang)))
	return fmt.Sprintf("codeql-%s-%s", strings.ToLower(lang), hex.EncodeToString(h[:])[:12])
}

// SemgrepRule represents a generated Semgrep taint or pattern rule.
type SemgrepRule struct {
	ID        string   `json:"id"`
	Pattern   string   `json:"pattern"`
	Message   string   `json:"message"`
	Severity  string   `json:"severity"`
	Languages []string `json:"languages"`
	YAML      string   `json:"yaml"`
}

// QuarantinedRule wraps a generated rule with quarantine metadata.
type QuarantinedRule struct {
	Rule      SemgrepRule `json:"rule"`
	Status    string      `json:"status"` // "quarantined", "approved", "rejected"
	Proposer  string      `json:"proposer"`
	CreatedAt int64       `json:"created_at"`
}

// GenerateSemgrepRule generates a Semgrep rule from a confirmed finding using the AI model.
func GenerateSemgrepRule(ctx context.Context, m ai.Model, f engines.Finding, code string) (*SemgrepRule, error) {
	ruleID := "sentinix-" + strings.TrimPrefix(f.Rule, "hunt/")
	prompt := fmt.Sprintf(
		"Generate a deterministic Semgrep YAML rule to detect this vulnerability pattern:\n"+
			"Rule ID: %s\n"+
			"File: %s\n"+
			"Finding message: %s\n"+
			"Code context:\n%s\n\n"+
			`Reply strictly with a JSON object: {"id":"%s","pattern":"regex_or_semgrep_pattern","message":"warning text","severity":"WARNING|ERROR","languages":["generic"]}`,
		ruleID, f.File, f.Message, ai.Untrusted(code), ruleID)

	out, err := m.Complete(ctx, ai.SysRules, prompt)
	if err != nil {
		return nil, fmt.Errorf("model failed to generate rule: %w", err)
	}

	js, found := ai.ExtractJSON(out)
	if !found {
		// Fallback: construct safe regex rule directly from finding match
		pattern := regexp.QuoteMeta(f.Match)
		if pattern == "" {
			pattern = regexp.QuoteMeta(f.File)
		}
		return &SemgrepRule{
			ID:        ruleID,
			Pattern:   pattern,
			Message:   f.Message,
			Severity:  "WARNING",
			Languages: []string{"generic"},
			YAML: fmt.Sprintf("rules:\n  - id: %s\n    pattern: %s\n    message: %s\n    severity: WARNING\n    languages: [generic]\n",
				ruleID, pattern, f.Message),
		}, nil
	}

	var rule SemgrepRule
	if err := json.Unmarshal([]byte(js), &rule); err != nil {
		return nil, fmt.Errorf("failed to parse rule JSON: %w", err)
	}

	if rule.ID == "" {
		rule.ID = ruleID
	}
	if rule.Severity == "" {
		rule.Severity = "WARNING"
	}
	if len(rule.Languages) == 0 {
		rule.Languages = []string{"generic"}
	}

	rule.YAML = fmt.Sprintf("rules:\n  - id: %s\n    pattern: %s\n    message: %s\n    severity: %s\n    languages: [%s]\n",
		rule.ID, rule.Pattern, rule.Message, rule.Severity, strings.Join(rule.Languages, ", "))

	return &rule, nil
}

// VerifyRuleFixture tests a generated rule against a synthetic code fixture.
func VerifyRuleFixture(rule *SemgrepRule, fixtureCode string, expectMatch bool) error {
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern regex %q: %w", rule.Pattern, err)
	}

	matched := re.MatchString(fixtureCode)
	if expectMatch && !matched {
		return fmt.Errorf("rule failed to match vulnerable fixture")
	}
	if !expectMatch && matched {
		return fmt.Errorf("rule falsely matched clean fixture")
	}
	return nil
}

// QuarantineRule wraps a proposed rule in quarantine state until approved.
func QuarantineRule(rule *SemgrepRule, proposer string) *QuarantinedRule {
	return &QuarantinedRule{
		Rule:     *rule,
		Status:   "quarantined",
		Proposer: proposer,
	}
}
