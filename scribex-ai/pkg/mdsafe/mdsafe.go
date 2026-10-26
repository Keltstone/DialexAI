// Package mdsafe provides Markdown neutralisation and sanitisation for untrusted text.
package mdsafe

import (
	"strings"
	"unicode"
)

// cleanControlAndLimit cleans bidi/zero-width (Cf), replaces \r\n\t with spaces,
// drops other control chars, and truncates to 300 runes (appending "…").
func cleanControlAndLimit(s string) string {
	var runes []rune
	for _, r := range s {
		// 1. Remove Unicode Cf (format / bidi / zero-width)
		if unicode.Is(unicode.Cf, r) {
			continue
		}
		// 2. Replace \r, \n, \t with single spaces
		if r == '\r' || r == '\n' || r == '\t' {
			runes = append(runes, ' ')
			continue
		}
		// 3. Drop other control characters
		if unicode.IsControl(r) {
			continue
		}
		runes = append(runes, r)
	}

	// 4. Truncate to 300 runes
	if len(runes) > 300 {
		runes = append(runes[:300], '…')
	}
	return string(runes)
}

// Text neutralises arbitrary string derived from files, symbols, errors, refs, branch names
// so it cannot inject headings, links, html, mentions, or markdown structure.
func Text(s string) string {
	cleaned := cleanControlAndLimit(s)

	// Escape chars: \ ` * _ { } [ ] ( ) < > # + - . ! | ~ @ :
	// and encode & as &amp; and < as &lt;
	var b strings.Builder
	for _, r := range cleaned {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '>', '#', '+', '-', '.', '!', '|', '~', '@', ':':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Code renders s as an inline code span using a backtick fence longer than the longest
// backtick run inside s. Content is sanitized with control-character dropping and truncation,
// but markdown characters inside the code span are not escaped.
func Code(s string) string {
	cleaned := cleanControlAndLimit(s)

	maxRun := 0
	currRun := 0
	for _, r := range cleaned {
		if r == '`' {
			currRun++
			if currRun > maxRun {
				maxRun = currRun
			}
		} else {
			currRun = 0
		}
	}

	fenceLen := 1
	if maxRun >= 1 {
		fenceLen = maxRun + 1
	}
	fence := strings.Repeat("`", fenceLen)

	// CommonMark: if content begins or ends with a backtick, pad with space.
	if strings.HasPrefix(cleaned, "`") || strings.HasSuffix(cleaned, "`") {
		return fence + " " + cleaned + " " + fence
	}
	return fence + cleaned + fence
}
