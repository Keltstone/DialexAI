// Package docs is the repo-level engine: anchor checks, link scavenging, contract deltas.
package docs

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ReportHeader = "## Scribex contract check"

const (
	MarkerV1   = "<!-- scribex-report:v1 -->"
	MaxBodyLen = 60000
)

// SanitizeReport validates an untrusted report markdown body.
// It accepts ONLY what Report.Markdown() can produce, rejecting hostile constructs.
func SanitizeReport(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", errors.New("report contains invalid UTF-8")
	}
	if strings.ContainsRune(raw, '\x00') {
		return "", errors.New("report contains forbidden NUL byte")
	}
	for _, r := range raw {
		if unicode.Is(unicode.Cf, r) {
			return "", errors.New("report contains forbidden Unicode Cf character")
		}
	}

	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || lines[0] != ReportHeader {
		return "", fmt.Errorf("line 1: header mismatch: expected %q", ReportHeader)
	}

	for i, l := range lines {
		lineNum := i + 1
		trimmed := strings.TrimRight(l, " \t\r")

		// Validate line shape
		if !isAllowedLineShape(trimmed, lineNum) {
			return "", fmt.Errorf("line %d: invalid report line shape", lineNum)
		}

		// Validate constructs outside code spans
		if err := validateLineCodeSpans(trimmed, lineNum); err != nil {
			return "", err
		}
	}

	// Truncate if over MaxBodyLen (with marker prefix)
	if len(MarkerV1)+1+len(raw) > MaxBodyLen {
		return truncateReport(raw), nil
	}

	return raw, nil
}

func isAllowedLineShape(line string, lineNum int) bool {
	if lineNum == 1 {
		return line == ReportHeader
	}
	if line == "" {
		return true
	}
	if line == "Unanalysed files:" || line == "Contract deltas:" {
		return true
	}
	if line == "Run `scribex sync` to refresh stale anchors." {
		return true
	}
	if strings.HasPrefix(line, "Scribex found ") && strings.HasSuffix(line, "):") {
		return true
	}
	if strings.HasPrefix(line, "Scribex: docs in sync (") && strings.HasSuffix(line, ").") {
		return true
	}
	if strings.HasPrefix(line, "- ") {
		return true
	}
	if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") {
		return true
	}
	if strings.HasPrefix(line, "… ") {
		return true
	}
	if strings.HasPrefix(line, "### ") {
		return true
	}
	if strings.HasPrefix(line, "```") {
		return true
	}
	return false
}

func validateLineCodeSpans(line string, lineNum int) error {
	i := 0
	for i < len(line) {
		if line[i] == '`' {
			// Backtick run
			start := i
			for i < len(line) && line[i] == '`' {
				i++
			}
			runLen := i - start
			delim := strings.Repeat("`", runLen)
			idx := strings.Index(line[i:], delim)
			if idx == -1 {
				return fmt.Errorf("line %d: unclosed code span", lineNum)
			}
			i += idx + runLen
			continue
		}

		// Outside code span
		if line[i] == '<' {
			return fmt.Errorf("line %d: raw HTML forbidden", lineNum)
		}
		if line[i] == '@' {
			backslashes := 0
			for k := i - 1; k >= 0 && line[k] == '\\'; k-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return fmt.Errorf("line %d: mention forbidden", lineNum)
			}
		}
		if strings.HasPrefix(line[i:], "![") {
			backslashes := 0
			for k := i - 1; k >= 0 && line[k] == '\\'; k-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return fmt.Errorf("line %d: markdown image forbidden", lineNum)
			}
		}
		if strings.HasPrefix(line[i:], "](") {
			backslashes := 0
			for k := i - 1; k >= 0 && line[k] == '\\'; k-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return fmt.Errorf("line %d: markdown link forbidden", lineNum)
			}
		}
		if strings.HasPrefix(line[i:], "http://") || strings.HasPrefix(line[i:], "https://") {
			return fmt.Errorf("line %d: bare URL forbidden", lineNum)
		}
		if strings.HasPrefix(line[i:], "<!--") {
			return fmt.Errorf("line %d: HTML comment forbidden", lineNum)
		}

		i++
	}
	return nil
}

func truncateReport(raw string) string {
	maxPayload := MaxBodyLen - len(MarkerV1) - 1 // 59973
	if len(raw) <= maxPayload {
		return raw
	}

	lines := strings.Split(raw, "\n")
	totalFindings := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "- ") {
			totalFindings++
		}
	}

	bestCut := 0
	bestDropped := totalFindings
	bestFenceClose := ""

	for cutIdx := 1; cutIdx <= len(lines); cutIdx++ {
		candidateLines := lines[:cutIdx]
		keptFindings := 0
		fenceCount := 0
		for _, cl := range candidateLines {
			if strings.HasPrefix(cl, "- ") {
				keptFindings++
			}
			if strings.HasPrefix(cl, "```") {
				fenceCount++
			}
		}

		fenceClose := ""
		if fenceCount%2 != 0 {
			fenceClose = "```\n"
		}

		dropped := totalFindings - keptFindings
		footer := fmt.Sprintf("… %d more findings not shown. Run `scribex check` locally for the full report.", dropped)
		joined := strings.Join(candidateLines, "\n") + "\n" + fenceClose + footer + "\n"

		if len(joined) <= maxPayload {
			bestCut = cutIdx
			bestDropped = dropped
			bestFenceClose = fenceClose
		} else {
			break
		}
	}

	if bestCut == 0 {
		bestCut = 1
	}

	footer := fmt.Sprintf("… %d more findings not shown. Run `scribex check` locally for the full report.", bestDropped)
	return strings.Join(lines[:bestCut], "\n") + "\n" + bestFenceClose + footer + "\n"
}
