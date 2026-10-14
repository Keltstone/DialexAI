// Package report renders findings as text or SARIF.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sentinix/pkg/engines"
)

// Text ends with a coverage statement so a clean report is not read as "secure".
func Text(w io.Writer, fs []engines.Finding, ran []string) {
	for _, f := range fs {
		fmt.Fprintf(w, "[%s] %s %s:%d %s\n", f.Severity, f.Rule, f.File, f.Line, f.Message)
	}
	fmt.Fprintf(w, "%d finding(s). Engines run: %s. Not covered: engines not installed, AI triage.\n",
		len(fs), strings.Join(ran, ", "))
}

func SARIF(w io.Writer, fs []engines.Finding) error {
	level := func(s engines.Severity) string {
		switch {
		case s >= engines.High:
			return "error"
		case s == engines.Medium:
			return "warning"
		}
		return "note"
	}
	results := []map[string]any{}
	for _, f := range fs {
		results = append(results, map[string]any{
			"ruleId": f.Rule, "level": level(f.Severity),
			"message": map[string]string{"text": f.Message},
			"locations": []any{map[string]any{"physicalLocation": map[string]any{
				"artifactLocation": map[string]string{"uri": f.File},
				"region":           map[string]int{"startLine": f.Line},
			}}},
		})
	}
	return json.NewEncoder(w).Encode(map[string]any{
		"version": "2.1.0",
		"runs": []any{map[string]any{
			"tool":    map[string]any{"driver": map[string]string{"name": "sentinix"}},
			"results": results,
		}},
	})
}
