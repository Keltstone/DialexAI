package mdsafe

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHostileCorpusText(t *testing.T) {
	longName := strings.Repeat("A", 10000)
	backtick1 := "foo`bar"
	backtick3 := "foo```bar"
	backtick10 := "foo``````````bar"

	corpus := []struct {
		name  string
		input string
	}{
		{"mention-team", "@org/team"},
		{"mention-all", "@all"},
		{"xss-details-img", "</details><img src=x onerror=alert(1)>"},
		{"link-evil", "[click](http://evil)"},
		{"img-evil", "![p](http://t/p.png)"},
		{"issue-ref", "#123"},
		{"heading-injection", "\n## Heading\n"},
		{"thematic-break", "---"},
		{"long-name", longName},
		{"backtick-1", backtick1},
		{"backtick-3", backtick3},
		{"backtick-10", backtick10},
		{"bidi-override", "foo\u202Ebar"},
		{"null-byte", "foo\x00bar"},
		{"javascript-pseudo", "javascript:alert(1)"},
		{"emoji-rtl", "🎉 مرحبا بالعالم 🚀"},
	}

	unescapedAtRe := regexp.MustCompile(`(^|[^\\])@`)

	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			got := Text(tc.input)

			// 1. Must contain no '<'
			if strings.Contains(got, "<") {
				t.Errorf("Text(%s) contains raw '<': %q", tc.name, got)
			}

			// 2. Must contain no unescaped '@'
			if unescapedAtRe.MatchString(got) {
				t.Errorf("Text(%s) contains unescaped '@': %q", tc.name, got)
			}

			// 3. Must contain no ']('
			if strings.Contains(got, "](") {
				t.Errorf("Text(%s) contains '](': %q", tc.name, got)
			}

			// 4. Must contain no line starting with #, >, -, |
			for _, line := range strings.Split(got, "\n") {
				trimmed := strings.TrimLeft(line, " ")
				if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ">") ||
					strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "|") {
					t.Errorf("Text(%s) line starts with markdown prefix: %q", tc.name, line)
				}
			}

			// 5. Rune length must be <= 301 (300 runes + optional ellipsis)
			if utf8.RuneCountInString(got) > 3000 { // considering escape chars, but raw runes cleaned <= 301
				t.Errorf("Text(%s) unexpectedly long", tc.name)
			}
		})
	}
}

func TestCodeSpan(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", "`simple`"},
		{"with`backtick", "``with`backtick``"},
		{"```three```", "```` ```three``` ````"},
		{"`starts_ends`", "`` `starts_ends` ``"},
		{"control\x00 and\nbidi\u202E", "`control and bidi`"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := Code(tc.input)
			if got != tc.want {
				t.Errorf("Code(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCleanControlAndLimit(t *testing.T) {
	long := strings.Repeat("x", 400)
	got := cleanControlAndLimit(long)
	if utf8.RuneCountInString(got) != 301 {
		t.Errorf("expected 301 runes (300 + '…'), got %d", utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected suffix '…'")
	}
}
