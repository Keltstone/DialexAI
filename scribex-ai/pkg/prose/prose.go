// Package prose validates LLM-suggested markdown rewrites.
package prose

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Violation reports a prose validation failure.
type Violation struct {
	Rule   string
	Detail string
}

func (v *Violation) Error() string {
	if v.Detail == "" {
		return "ai prose rejected: " + v.Rule
	}
	return "ai prose rejected: " + v.Rule + ": " + v.Detail
}

var (
	reHTMLTag  = regexp.MustCompile(`(?i)<\s*/?\s*(script|iframe|object|embed|style|link|meta|form|input|button|img|svg|math|base|frame|frameset|applet|video|audio|source|template)\b`)
	reHTMLAttr = regexp.MustCompile(`(?i)\bon[a-z]{3,}\s*=`)
	reScheme   = regexp.MustCompile(`(?i)(javascript|vbscript|data)\s*:`)
	reURL      = regexp.MustCompile(`https?://[^\s)>\]]+`)
	reMention  = regexp.MustCompile(`@[A-Za-z0-9_-]{2,}`)

	secretShapeRegexes = []*regexp.Regexp{
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
		regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
		regexp.MustCompile(`gh[os]_[A-Za-z0-9_]{20,}`),
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
		regexp.MustCompile(`AIza[0-9A-Za-z_-]{30,}`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
	}

	aiMarkLine = regexp.MustCompile(`^\s*<!--\s*@scribex:ai(?:\s+sig=[0-9a-fA-F]+)?\s*-->\s*$`)
)

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// Clean normalises line endings and trims leading and trailing newlines.
func Clean(new string) string {
	return strings.Trim(normalize(new), "\n")
}

// SecretShape reports whether s matches known API key/token patterns.
func SecretShape(s string) bool {
	for _, re := range secretShapeRegexes {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// StripAIMark removes lines matching the scribex ai marker.
func StripAIMark(s string) string {
	norm := normalize(s)
	lines := strings.Split(norm, "\n")
	var kept []string
	for _, line := range lines {
		if !aiMarkLine.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// Validate checks that new prose is safe plain markdown.
// Rules are checked in order; the first violation wins.
func Validate(old, new string, secrets ...string) error {
	// Rule 1: encoding (UTF-8, NUL, control chars other than \n and \t)
	if !utf8.ValidString(new) {
		return &Violation{Rule: "encoding", Detail: "invalid UTF-8"}
	}
	for _, r := range new {
		if r == 0 {
			return &Violation{Rule: "encoding", Detail: "contains NUL byte"}
		}
		if (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return &Violation{Rule: "encoding", Detail: "contains control character"}
		}
	}

	newNorm := normalize(new)
	oldNorm := normalize(old)

	// Rule 2: empty
	if strings.TrimSpace(newNorm) == "" {
		return &Violation{Rule: "empty", Detail: "empty or whitespace-only"}
	}

	// Rule 3: too-long
	maxLen := 4 * len(oldNorm)
	if maxLen < 2048 {
		maxLen = 2048
	}
	if len(newNorm) > maxLen {
		return &Violation{Rule: "too-long", Detail: "exceeds maximum allowed length"}
	}
	if len(oldNorm) >= 200 && len(newNorm) < len(oldNorm)/4 {
		return &Violation{Rule: "too-long", Detail: "collapsed reply"}
	}

	// Rule 4: marker (@scribex, case-insensitive)
	if strings.Contains(strings.ToLower(newNorm), "@scribex") {
		return &Violation{Rule: "marker", Detail: "contains @scribex"}
	}

	// Rule 5: html-comment
	if strings.Contains(newNorm, "<!") || strings.Contains(newNorm, "-->") {
		return &Violation{Rule: "html-comment", Detail: "contains HTML comment delimiter"}
	}

	// Rule 6: fence
	for _, line := range strings.Split(newNorm, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			return &Violation{Rule: "fence", Detail: "code fence at line start"}
		}
	}

	// Rule 7: json
	if json.Valid([]byte(strings.TrimSpace(newNorm))) {
		return &Violation{Rule: "json", Detail: "valid JSON"}
	}

	// Rule 8: html
	if reHTMLTag.MatchString(newNorm) || reHTMLAttr.MatchString(newNorm) {
		return &Violation{Rule: "html", Detail: "prohibited HTML tag or event handler"}
	}

	// Rule 9: scheme
	if reScheme.MatchString(newNorm) {
		return &Violation{Rule: "scheme", Detail: "prohibited URI scheme"}
	}

	// Rule 10: image
	if strings.Contains(newNorm, "![") {
		return &Violation{Rule: "image", Detail: "markdown image"}
	}

	// Rule 11: front-matter
	for _, line := range strings.Split(newNorm, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line == "---" || strings.TrimRight(line, " \t") == "---" {
			return &Violation{Rule: "front-matter", Detail: "front-matter delimiter"}
		}
		break
	}

	// Rule 12: heading
	countHeadings := func(s string) int {
		n := 0
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
				n++
			}
		}
		return n
	}
	if countHeadings(newNorm) > countHeadings(oldNorm) {
		return &Violation{Rule: "heading", Detail: "heading count increased"}
	}

	// Rule 13: url
	for _, u := range reURL.FindAllString(newNorm, -1) {
		if !strings.Contains(oldNorm, u) {
			return &Violation{Rule: "url", Detail: "new URL not present in existing prose"}
		}
	}

	// Rule 14: mention
	for _, m := range reMention.FindAllString(newNorm, -1) {
		if !strings.Contains(oldNorm, m) {
			return &Violation{Rule: "mention", Detail: "new @mention not present in existing prose"}
		}
	}

	// Rule 15: secret
	for _, sec := range secrets {
		secNorm := normalize(sec)
		if len(secNorm) >= 8 && strings.Contains(newNorm, secNorm) {
			return &Violation{Rule: "secret", Detail: "contains provided secret"}
		}
	}
	if SecretShape(newNorm) {
		return &Violation{Rule: "secret", Detail: "matches secret shape"}
	}

	return nil
}
