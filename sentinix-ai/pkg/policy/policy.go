// Package policy applies baselines and the severity gate.
package policy

import (
	"encoding/json"
	"os"

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
func Fails(fs []engines.Finding, min engines.Severity) bool {
	for _, f := range fs {
		if f.Severity >= min {
			return true
		}
	}
	return false
}
