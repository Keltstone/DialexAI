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
	for _, f := range fs {
		if f.Source == "ci-unattested" {
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

