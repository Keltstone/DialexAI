package engines

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// InlineSuppression represents a sentinix:ignore directive in source code.
type InlineSuppression struct {
	Project     string `json:"project,omitempty"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Rule        string `json:"rule"`
	Until       int64  `json:"until"`
	Owner       string `json:"owner"`
	Reason      string `json:"reason,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// ParseInlineSuppressions parses any sentinix:ignore comments from a source file stream.
func ParseInlineSuppressions(r io.Reader, file string) ([]InlineSuppression, error) {
	scanner := bufio.NewScanner(r)
	var out []InlineSuppression
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		idx := strings.Index(text, "sentinix:ignore")
		if idx == -1 {
			continue
		}

		directive := strings.TrimSpace(text[idx+len("sentinix:ignore"):])
		directive = strings.TrimSuffix(directive, "*/")
		directive = strings.TrimSpace(directive)

		s := InlineSuppression{
			File: file,
			Line: lineNum,
			Rule: "*",
		}

		// Parse key=value or tokens
		parts := splitFieldsPreservingQuotes(directive)
		for _, part := range parts {
			k, v, found := strings.Cut(part, "=")
			if !found {
				if s.Rule == "*" {
					s.Rule = part
				}
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.Trim(strings.TrimSpace(v), `"'`)

			switch k {
			case "rule":
				s.Rule = v
			case "owner", "author", "by":
				s.Owner = v
			case "reason":
				s.Reason = v
			case "fingerprint", "fp":
				s.Fingerprint = v
			case "until", "expires", "exp":
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					s.Until = t.Unix()
				} else if t, err := time.Parse("2006-01-02", v); err == nil {
					s.Until = t.Unix()
				} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					s.Until = n
				}
			}
		}

		if s.Until == 0 {
			// default to 30 days time-box if not specified
			s.Until = time.Now().AddDate(0, 1, 0).Unix()
		}

		out = append(out, s)
	}

	return out, scanner.Err()
}

func splitFieldsPreservingQuotes(s string) []string {
	var fields []string
	var cur strings.Builder
	inQuote := false
	var quoteChar rune

	for _, r := range s {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// FindInlineSuppressions walks root (skipping .git) and extracts all inline suppressions.
func FindInlineSuppressions(root string) ([]InlineSuppression, error) {
	var out []InlineSuppression
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == ".git" {
			return nil
		}
		if info.Size() > 2<<20 { // skip files > 2 MiB
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sups, _ := ParseInlineSuppressions(f, rel)
		out = append(out, sups...)
		return nil
	})
	return out, err
}
