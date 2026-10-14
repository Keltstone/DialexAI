package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"sentinix/pkg/engines"
)

type Verdict struct {
	Real       bool     `json:"real"`
	Severity   string   `json:"severity"`
	Confidence float64  `json:"confidence"`
	Reasoning  string   `json:"reasoning"`
	Models     []string `json:"models"`
	Dissent    []string `json:"dissent,omitempty"` // models whose answer differed from the majority
	Skipped    []string `json:"skipped,omitempty"` // models barred by policy or that failed
}

type answer struct {
	Real       bool    `json:"real"`
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// Triage asks every allowed model whether the finding is real. Verdicts are advisory: they inform
// the reviewer and never dismiss a finding on their own. Ties count as real (the cautious reading).
func Triage(ctx context.Context, models []Model, policy string, f engines.Finding, snippet string) (*Verdict, error) {
	if f.Severity < engines.Medium {
		return nil, fmt.Errorf("triage is for medium and above")
	}
	if policy != "local-only" {
		snippet = engines.Redact(snippet)
	}
	if len(snippet) > 6000 {
		snippet = snippet[:6000]
	}
	user := fmt.Sprintf("Rule: %s (%s)\nFile: %s line %d\nEngine message: %s\n%s\n\nIs this a real, exploitable issue or a false positive? "+
		`Reply {"real":bool,"severity":"info|low|medium|high|critical","confidence":0..1,"reasoning":"short"}.`,
		f.Rule, f.CWE, f.File, f.Line, f.Message, untrusted(snippet))

	v := &Verdict{}
	var ans []answer
	var names []string
	for _, m := range models {
		if !Allowed(policy, m) {
			v.Skipped = append(v.Skipped, m.Name()+" (policy)")
			continue
		}
		out, err := m.Complete(ctx, sysRules, user)
		var a answer
		js, ok := extractJSON(out)
		if err != nil || !ok || json.Unmarshal([]byte(js), &a) != nil {
			v.Skipped = append(v.Skipped, m.Name()+" (failed)")
			continue
		}
		if _, err := engines.ParseSeverity(a.Severity); err != nil || a.Confidence < 0 || a.Confidence > 1 {
			v.Skipped = append(v.Skipped, m.Name()+" (invalid answer)")
			continue
		}
		ans, names = append(ans, a), append(names, m.Name())
	}
	if len(ans) == 0 {
		return nil, fmt.Errorf("no model produced a usable answer (skipped: %s)", strings.Join(v.Skipped, ", "))
	}
	real := 0
	for _, a := range ans {
		if a.Real {
			real++
		}
	}
	v.Real = real*2 >= len(ans)
	v.Models = names
	var sevs []int
	for i, a := range ans {
		if a.Real != v.Real {
			v.Dissent = append(v.Dissent, names[i])
			continue
		}
		s, _ := engines.ParseSeverity(a.Severity)
		sevs = append(sevs, int(s))
		v.Confidence += a.Confidence
		if v.Reasoning == "" {
			v.Reasoning = a.Reasoning
		}
	}
	sort.Ints(sevs)
	v.Severity = engines.Severity(sevs[len(sevs)/2]).String()
	v.Confidence /= float64(len(sevs))
	return v, nil
}
