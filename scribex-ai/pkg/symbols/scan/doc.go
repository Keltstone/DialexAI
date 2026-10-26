package scan

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxDocBytes = 2000

// CleanDoc cleans raw doc text:
// - Removes Unicode Cf (format/invisible) characters
// - Removes control characters except '\n' and '\t'
// - Trims leading and trailing whitespace
// - Caps at 2 000 bytes on a rune boundary (appending "…")
func CleanDoc(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		if unicode.Is(unicode.Cf, r) {
			continue
		}
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}

	res := strings.TrimSpace(b.String())
	if len(res) <= maxDocBytes {
		return res
	}

	// Truncate at last rune boundary <= maxDocBytes
	cut := maxDocBytes
	for cut > 0 && !utf8.RuneStart(res[cut]) {
		cut--
	}
	return res[:cut] + "…"
}

// PrecedingDoc extracts the contiguous comment block immediately above headerOffset.
// If there is a blank line between the comment block and headerOffset, it is treated as
// unattached and returns "".
// Markers (//, /* */, /** */, leading ' * ', ///, #) are stripped, lines joined with '\n',
// and cleaned with CleanDoc.
func PrecedingDoc(s *Source, headerOffset int) string {
	if s == nil || headerOffset <= 0 || headerOffset > len(s.B) {
		return ""
	}

	// 1. Move backward from headerOffset - 1 skipping horizontal whitespace.
	// At most one newline is allowed. If a second newline occurs before any comment,
	// that's a blank line (unattached comment).
	pos := headerOffset - 1
	for pos >= 0 && (s.B[pos] == ' ' || s.B[pos] == '\t') {
		pos--
	}
	if pos < 0 {
		return ""
	}

	if s.B[pos] == '\n' {
		pos--
		// Check for another newline (blank line)
		for pos >= 0 && (s.B[pos] == ' ' || s.B[pos] == '\t') {
			pos--
		}
		if pos < 0 || s.B[pos] == '\n' {
			// Blank line between comment and header -> not attached
			return ""
		}
	}

	// We must now be at a comment byte
	if pos < 0 || s.K[pos] != Comment {
		return ""
	}

	commentEnd := pos + 1

	// 2. Scan backward to find the start of the contiguous comment block.
	// Comments can be multiple line comments or a block comment.
	// If a blank line (two newlines with only whitespace) or code is encountered, stop.
	commentStart := pos
	for pos >= 0 {
		if s.K[pos] == Comment {
			commentStart = pos
			pos--
			continue
		}
		if s.B[pos] == ' ' || s.B[pos] == '\t' {
			pos--
			continue
		}
		if s.B[pos] == '\n' {
			// Check if previous line has a comment
			prev := pos - 1
			for prev >= 0 && (s.B[prev] == ' ' || s.B[prev] == '\t') {
				prev--
			}
			if prev < 0 || s.B[prev] == '\n' || s.K[prev] != Comment {
				// Blank line or code above: stop comment block here
				break
			}
			pos = prev
			continue
		}
		// Any other character (code) means the comment block boundary
		break
	}

	raw := string(s.B[commentStart:commentEnd])
	return parseCommentText(raw)
}

func parseCommentText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Block comment: /* ... */ or /** ... */
	if strings.HasPrefix(trimmed, "/*") && strings.HasSuffix(trimmed, "*/") {
		inner := trimmed[2 : len(trimmed)-2]
		if strings.HasPrefix(inner, "*") && !strings.HasPrefix(inner, "**/") {
			inner = inner[1:] // strip extra '*' in /**
		}
		lines := strings.Split(inner, "\n")
		var cleanLines []string
		for _, line := range lines {
			l := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(l, "* ") {
				l = l[2:]
			} else if strings.HasPrefix(l, "*") {
				l = l[1:]
			}
			cleanLines = append(cleanLines, strings.TrimRight(l, " \t"))
		}
		return CleanDoc(strings.Join(cleanLines, "\n"))
	}

	// Line comments: //, ///, #
	lines := strings.Split(raw, "\n")
	var cleanLines []string
	for _, line := range lines {
		l := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(l, "///") {
			l = l[3:]
			if len(l) > 0 && l[0] == ' ' {
				l = l[1:]
			}
		} else if strings.HasPrefix(l, "//") {
			l = l[2:]
			if len(l) > 0 && l[0] == ' ' {
				l = l[1:]
			}
		} else if strings.HasPrefix(l, "#") {
			l = l[1:]
			if len(l) > 0 && l[0] == ' ' {
				l = l[1:]
			}
		}
		cleanLines = append(cleanLines, strings.TrimRight(l, " \t"))
	}
	return CleanDoc(strings.Join(cleanLines, "\n"))
}

// Docstring extracts and dedents a Python docstring literal (PEP 257).
// raw may include or omit quotes/prefixes.
func Docstring(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// Strip Python string prefix (e.g. r, u, f, b, rf, fr, rb, br)
	s = stripPyPrefix(s)

	// Strip enclosing quotes
	if strings.HasPrefix(s, `"""`) && strings.HasSuffix(s, `"""`) && len(s) >= 6 {
		s = s[3 : len(s)-3]
	} else if strings.HasPrefix(s, `'''`) && strings.HasSuffix(s, `'''`) && len(s) >= 6 {
		s = s[3 : len(s)-3]
	} else if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		s = s[1 : len(s)-1]
	} else if strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`) && len(s) >= 2 {
		s = s[1 : len(s)-1]
	}

	// Normalize CRLF to LF
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return CleanDoc(strings.TrimSpace(s))
	}

	// PEP 257 dedent:
	// First line is trimmed.
	// Determine minimum indentation across non-empty lines from lines[1:] onwards.
	minIndent := -1
	for _, l := range lines[1:] {
		trimmed := strings.TrimLeft(l, " \t")
		if len(trimmed) > 0 {
			indent := len(l) - len(trimmed)
			if minIndent < 0 || indent < minIndent {
				minIndent = indent
			}
		}
	}

	trimmedLines := make([]string, len(lines))
	trimmedLines[0] = strings.TrimSpace(lines[0])

	for i := 1; i < len(lines); i++ {
		l := lines[i]
		if minIndent > 0 {
			if len(l) >= minIndent {
				l = l[minIndent:]
			} else {
				l = strings.TrimLeft(l, " \t")
			}
		}
		trimmedLines[i] = strings.TrimRight(l, " \t")
	}

	// Strip leading and trailing empty lines
	for len(trimmedLines) > 0 && trimmedLines[0] == "" {
		trimmedLines = trimmedLines[1:]
	}
	for len(trimmedLines) > 0 && trimmedLines[len(trimmedLines)-1] == "" {
		trimmedLines = trimmedLines[:len(trimmedLines)-1]
	}

	return CleanDoc(strings.Join(trimmedLines, "\n"))
}

func stripPyPrefix(s string) string {
	prefixes := []string{"rf", "fr", "rb", "br", "r", "u", "f", "b"}
	lower := strings.ToLower(s)
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			rest := s[len(p):]
			if strings.HasPrefix(rest, `"""`) || strings.HasPrefix(rest, `'''`) ||
				strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, `'`) {
				return rest
			}
		}
	}
	return s
}
