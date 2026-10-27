// Package policy applies baselines and the severity gate.
package policy

import (
	"encoding/json"
	"os"
	"strings"

	"sentinix/pkg/engines"
)

type Baseline map[string]bool

func LoadBaseline(path string) (Baseline, error) {
	b := Baseline{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	var fps []string
	if err := json.Unmarshal(data, &fps); err != nil {
		return nil, err
	}
	for _, fp := range fps {
		b[fp] = true
	}
	return b, nil
}

func SaveBaseline(path string, fs []engines.Finding) error {
	fps := []string{}
	for _, f := range fs {
		fps = append(fps, f.Fingerprint())
	}
	data, _ := json.MarshalIndent(fps, "", "  ")
	return os.WriteFile(path, data, 0o644)
}

// New drops findings already in the baseline, so a PR shows only what it introduced.
func (b Baseline) New(fs []engines.Finding) []engines.Finding {
	var out []engines.Finding
	for _, f := range fs {
		if !b[f.Fingerprint()] {
			out = append(out, f)
		}
	}
	return out
}

// Fails reports whether any finding is at or above the threshold.
// Unattested CI findings are excluded from the gate.
func Fails(fs []engines.Finding, min engines.Severity) bool {
	return FailsWithHuntGate(fs, min, "advisory")
}

// FailsWithHuntGate evaluates gating taking into account the hunt gate mode ("advisory" or "confirmed-only").
// Under "advisory" (default), llm findings never gate. Under "confirmed-only", ONLY confirmed PoC findings
// with severity >= min block.
func FailsWithHuntGate(fs []engines.Finding, min engines.Severity, gateMode string) bool {
	for _, f := range fs {
		if f.Source == "ci-unattested" {
			continue
		}
		if f.Source == "llm" {
			if gateMode == "confirmed-only" {
				isConfirmed := strings.Contains(f.Evidence, "[exploitability:confirmed]") || strings.Contains(f.Evidence, "PoC reproduced")
				if isConfirmed && f.Severity >= min {
					return true
				}
			}
			continue
		}
		if f.Severity >= min {
			return true
		}
	}
	return false
}

// CategoryPolicy defines gating mode and severity threshold for a finding category.
type CategoryPolicy struct {
	Mode      string           `json:"mode"`      // "block" | "comment" | "off"
	Threshold engines.Severity `json:"threshold"` // "info" | "low" | "medium" | "high" | "critical"
}

// CategoryFails reports whether any finding in this category causes the category gate to fail.
// Only returns true if pol.Mode == "block" and there is a finding with severity >= pol.Threshold.
// Default unsigned (ci-unattested) is comment-only for security, style, quality, and accessibility
// to prevent merge DoS from unverified CI uploads.
// Only compiled architecture tests (Konsist in architecture category) may block from unsigned uploads
// when the project explicitly configures mode: block.
func CategoryFails(fs []engines.Finding, cat string, pol CategoryPolicy) bool {
	if pol.Mode != "block" {
		return false
	}
	for _, f := range fs {
		if f.Source == "llm" {
			continue
		}
		if f.Source == "ci-unattested" {
			if cat != engines.CategoryArchitecture || (f.Engine != "" && strings.ToLower(f.Engine) != "konsist") {
				continue
			}
		}
		fCat := f.Category
		if fCat == "" {
			fCat = engines.CategorySecurity
		}
		if fCat == cat && f.Severity >= pol.Threshold {
			return true
		}
	}
	return false
}

// GroupByCategory groups findings by their normalized category.
func GroupByCategory(fs []engines.Finding) map[string][]engines.Finding {
	m := make(map[string][]engines.Finding)
	for _, f := range fs {
		cat := f.Category
		if cat == "" {
			cat = engines.CategorySecurity
		}
		m[cat] = append(m[cat], f)
	}
	return m
}

func ModeRank(mode string) int {
	switch strings.ToLower(mode) {
	case "block":
		return 3
	case "comment":
		return 2
	case "off":
		return 1
	default:
		return 0
	}
}

// IsLoosening reports whether changing from oldPol to newPol weakens safeguards.
// Loosening includes:
// 1. Turning a check off (new mode is "off" when old was not "off").
// 2. Loosening mode (e.g. block -> comment, block -> off, comment -> off).
// 3. Loosening threshold (e.g. low -> medium -> high -> critical: higher threshold requires more severe finding to block).
func IsLoosening(oldPol, newPol CategoryPolicy) bool {
	if newPol.Mode == "off" && oldPol.Mode != "off" {
		return true
	}
	if ModeRank(newPol.Mode) < ModeRank(oldPol.Mode) {
		return true
	}
	if newPol.Mode == "block" && oldPol.Mode == "block" {
		if newPol.Threshold > oldPol.Threshold {
			return true
		}
	}
	return false
}

// RuleEnforcementLevel defines the action taken when a specific rule matches.
type RuleEnforcementLevel string

const (
	LevelBlock  RuleEnforcementLevel = "block"
	LevelWarn   RuleEnforcementLevel = "warn"
	LevelNotice RuleEnforcementLevel = "notice"
	LevelIgnore RuleEnforcementLevel = "ignore"
)

func ParseEnforcementLevel(s string) (RuleEnforcementLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "block":
		return LevelBlock, true
	case "warn", "warning":
		return LevelWarn, true
	case "notice", "info":
		return LevelNotice, true
	case "ignore", "off", "disable", "disabled":
		return LevelIgnore, true
	default:
		return "", false
	}
}

// ProjectPolicy defines per-project or repository-level rule enforcement policy.
type ProjectPolicy struct {
	Version      string                          `json:"version,omitempty"`
	DefaultLevel RuleEnforcementLevel            `json:"default_level,omitempty"`
	Rules        map[string]RuleEnforcementLevel `json:"rules,omitempty"`
	HuntGate     string                          `json:"hunt_gate,omitempty"` // "advisory" (default) or "confirmed-only"
}

func LoadProjectPolicy(path string) (*ProjectPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p ProjectPolicy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// EffectiveLevel returns the resolved enforcement level for a given rule.
func (p *ProjectPolicy) EffectiveLevel(ruleID string, defaultSeverity engines.Severity) RuleEnforcementLevel {
	if p != nil && p.Rules != nil {
		if lvl, ok := p.Rules[ruleID]; ok {
			if norm, valid := ParseEnforcementLevel(string(lvl)); valid {
				return norm
			}
		}
	}
	if strings.HasPrefix(ruleID, "hunt/") {
		return LevelNotice
	}
	if p != nil && p.DefaultLevel != "" {
		if norm, valid := ParseEnforcementLevel(string(p.DefaultLevel)); valid {
			return norm
		}
	}
	// Institutional default: high & critical block, medium & low warn, info notices
	if defaultSeverity >= engines.High {
		return LevelBlock
	} else if defaultSeverity >= engines.Low {
		return LevelWarn
	}
	return LevelNotice
}

// EffectiveFindingLevel returns the enforcement level for a concrete finding.
// Considers finding Source and confirmed-only hunt gating.
func (p *ProjectPolicy) EffectiveFindingLevel(f engines.Finding) RuleEnforcementLevel {
	if f.Source == "llm" {
		if p != nil && p.HuntGate == "confirmed-only" {
			isConfirmed := strings.Contains(f.Evidence, "[exploitability:confirmed]") || strings.Contains(f.Evidence, "PoC reproduced")
			if isConfirmed {
				if f.Severity >= engines.High {
					return LevelBlock
				} else if f.Severity >= engines.Low {
					return LevelWarn
				}
			}
		}
		return LevelNotice
	}
	return p.EffectiveLevel(f.Rule, f.Severity)
}

// EvaluatedFindings partitions findings into blocked, warned, noticed, and ignored.
type EvaluatedFindings struct {
	Blocked []engines.Finding
	Warned  []engines.Finding
	Noticed []engines.Finding
	Ignored []engines.Finding
}

func EvaluateFindings(fs []engines.Finding, pol *ProjectPolicy) EvaluatedFindings {
	var ev EvaluatedFindings
	for _, f := range fs {
		level := pol.EffectiveLevel(f.Rule, f.Severity)
		f.Level = string(level)
		switch level {
		case LevelBlock:
			ev.Blocked = append(ev.Blocked, f)
		case LevelWarn:
			ev.Warned = append(ev.Warned, f)
		case LevelNotice:
			ev.Noticed = append(ev.Noticed, f)
		case LevelIgnore:
			ev.Ignored = append(ev.Ignored, f)
		default:
			ev.Blocked = append(ev.Blocked, f)
		}
	}
	return ev
}

// Active returns all non-ignored findings (Blocked, Warned, and Noticed).
func (ev EvaluatedFindings) Active() []engines.Finding {
	var out []engines.Finding
	out = append(out, ev.Blocked...)
	out = append(out, ev.Warned...)
	out = append(out, ev.Noticed...)
	return out
}

// PolicyFails returns true if there are any findings assigned the 'block' enforcement level.
func PolicyFails(ev EvaluatedFindings) bool {
	for _, f := range ev.Blocked {
		if f.Source != "ci-unattested" {
			return true
		}
	}
	return false
}


