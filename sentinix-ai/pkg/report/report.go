// Package report renders findings as text or SARIF.
package report

import (
	"unicode"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sentinix/pkg/engines"
)

// SanitizeField escapes backticks, newlines, and markdown brackets to prevent code fence breakout and injection.
func SanitizeField(s string) string {
	// Drop control, bidi-override and line/paragraph separators (filename spoofing), cap length.
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, s)
	if rs := []rune(s); len(rs) > 300 {
		s = string(rs[:300]) + "..."
	}
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, "@", "&#64;")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "```", `\x60\x60\x60`)
	s = strings.ReplaceAll(s, "`", `\x60`)
	s = strings.ReplaceAll(s, "[", `\[`)
	s = strings.ReplaceAll(s, "]", `\]`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}

// Text ends with a coverage statement so a clean report is not read as "secure".
func Text(w io.Writer, fs []engines.Finding, ran []string, skipped ...[]string) {
	for _, f := range fs {
		rule := SanitizeField(f.Rule)
		file := SanitizeField(f.File)
		msg := SanitizeField(f.Message)
		tag := f.Severity.String()
		if f.Level != "" {
			tag = fmt.Sprintf("%s|%s", f.Severity.String(), f.Level)
		}
		fmt.Fprintf(w, "[%s] %s %s:%d %s\n", tag, rule, file, f.Line, msg)
	}
	fmt.Fprintf(w, "%d finding(s). Engines run: %s. Not covered: engines not installed, AI triage.\n",
		len(fs), strings.Join(ran, ", "))
	if len(skipped) > 0 && len(skipped[0]) > 0 {
		list := skipped[0]
		total := len(list)
		if len(list) > 20 {
			list = list[:20]
		}
		var escaped []string
		for _, s := range list {
			escaped = append(escaped, EscapeMarkdown(s))
		}
		if total > 20 {
			fmt.Fprintf(w, "Skipped files (%d): %s, and %d more\n", total, strings.Join(escaped, ", "), total-20)
		} else {
			fmt.Fprintf(w, "Skipped files (%d): %s\n", total, strings.Join(escaped, ", "))
		}
	}
}

// EscapeMarkdown escapes markdown special characters.
func EscapeMarkdown(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '#', '+', '-', '.', '!', '<', '>':
			sb.WriteRune('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
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
