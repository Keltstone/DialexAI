package docs

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestSanitizeReport_RoundTrip(t *testing.T) {
	r := Report{
		Total: 10,
		Issues: []Issue{
			{File: "pkg/auth/auth.go", Line: 42, Ref: "Verify", Msg: "signature changed"},
		},
		Unknown: []Unknown{
			{File: "pkg/calc/calc.go", Reason: "unsupported extension"},
		},
	}

	rendered := r.Markdown()
	sanitized, err := SanitizeReport(rendered)
	if err != nil {
		t.Fatalf("SanitizeReport rejected genuine report: %v", err)
	}

	if sanitized != rendered {
		t.Errorf("round trip mismatch:\nGOT:\n%s\nWANT:\n%s", sanitized, rendered)
	}
}

func TestSanitizeReport_HostileRejection(t *testing.T) {
	cases := []struct {
		name    string
		hostile string
	}{
		{"mention_team", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: @org/team alert\n"},
		{"mention_user", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: @user please review\n"},
		{"html_img", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: <img src=x>\n"},
		{"html_script", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: <script>alert(1)</script>\n"},
		{"markdown_image", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: ![x](http://t/p.png)\n"},
		{"markdown_link", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: [click](http://evil)\n"},
		{"bare_url_http", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: http://evil.com/leak\n"},
		{"bare_url_https", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: https://evil.com/leak\n"},
		{"forged_marker", ReportHeader + "\n\n<!-- scribex-report:v1 -->\nScribex found 1 doc issue(s) (1 anchors):\n"},
		{"arbitrary_heading", ReportHeader + "\n\n# Approved by Security Team\n"},
		{"nul_byte", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: bad\x00byte\n"},
		{"bidi_override", ReportHeader + "\n\nScribex found 1 doc issue(s) (1 anchors):\n- `a.go` line 1: \u202Eoverride\n"},
		{"wrong_header", "# Different Header\n\nScribex: docs in sync (0 anchors checked).\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SanitizeReport(tc.hostile)
			if err == nil {
				t.Fatalf("expected rejection for hostile input %s, got nil error", tc.name)
			}
			// Verify error does not leak the content
			if strings.Contains(err.Error(), "http://evil") || strings.Contains(err.Error(), "@org/team") {
				t.Errorf("error message leaked content: %v", err)
			}
		})
	}
}

func TestSanitizeReport_Truncation(t *testing.T) {
	// Generate report with 2000 findings
	var issues []Issue
	for i := 0; i < 2000; i++ {
		issues = append(issues, Issue{
			File: "pkg/auth/auth.go",
			Line: i + 1,
			Ref:  "SomeSymbol",
			Msg:  "signature changed in contract",
		})
	}
	r := Report{Total: 2000, Issues: issues}
	md := r.Markdown()

	sanitized, err := SanitizeReport(md)
	if err != nil {
		t.Fatalf("SanitizeReport failed on large report: %v", err)
	}

	const markerLen = 26 // "<!-- scribex-report:v1 -->\n"
	if len(sanitized)+markerLen > 60000 {
		t.Errorf("sanitized length (%d + %d = %d) exceeds 60000", len(sanitized), markerLen, len(sanitized)+markerLen)
	}

	if !strings.Contains(sanitized, "not shown. Run `scribex check` locally for the full report.") {
		t.Errorf("expected truncation footer in sanitized output")
	}

	// Verify count in footer is exact
	lines := strings.Split(sanitized, "\n")
	shownCount := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "- ") {
			shownCount++
		}
	}
	expectedDropped := 2000 - shownCount
	expectedFooter := fmt.Sprintf("… %d more findings not shown. Run `scribex check` locally for the full report.", expectedDropped)
	if !strings.Contains(sanitized, expectedFooter) {
		t.Errorf("footer mismatch: expected %q in output", expectedFooter)
	}
}

func TestSanitizeReport_Truncation_Multibyte(t *testing.T) {
	// Ensure Malayalam and emoji multibyte runes are not cut mid-rune
	var issues []Issue
	for i := 0; i < 1500; i++ {
		issues = append(issues, Issue{
			File: "pkg/മലയാളം/പ്രമാണം.go",
			Line: i + 1,
			Ref:  "ചിഹ്നം🚀",
			Msg:  "കരാർ ലംഘനം കണ്ടത്തി 🔍 - подробности и детали договора",
		})
	}
	r := Report{Total: 1500, Issues: issues}
	md := r.Markdown()

	sanitized, err := SanitizeReport(md)
	if err != nil {
		t.Fatalf("SanitizeReport failed on multibyte report: %v", err)
	}

	const markerLen = 26
	if len(sanitized)+markerLen > 60000 {
		t.Errorf("multibyte sanitized length %d exceeds limit", len(sanitized)+markerLen)
	}
	if !utf8.ValidString(sanitized) {
		t.Errorf("sanitized report contains invalid UTF-8 after truncation (cut mid-rune)")
	}
}

func TestSanitizeReport_JustUnderLimit(t *testing.T) {
	// A report just under the limit should remain completely untouched
	var issues []Issue
	// Find issue count that produces ~50KB
	for i := 0; i < 400; i++ {
		issues = append(issues, Issue{
			File: "pkg/api/client.go",
			Line: i + 1,
			Ref:  "ClientFunc",
			Msg:  "contract parameter mismatch",
		})
	}
	r := Report{Total: 400, Issues: issues}
	md := r.Markdown()

	if len(md)+26 > 60000 {
		t.Fatalf("test setup issue: md len %d already over limit", len(md))
	}

	sanitized, err := SanitizeReport(md)
	if err != nil {
		t.Fatalf("SanitizeReport error: %v", err)
	}
	if sanitized != md {
		t.Errorf("report just under limit was modified by SanitizeReport")
	}
	if strings.Contains(sanitized, "not shown") {
		t.Errorf("unexpected truncation notice in untruncated report")
	}
}

func FuzzSanitizeReport(f *testing.F) {
	// Seed corpus
	r := Report{
		Total: 2,
		Issues: []Issue{
			{File: "pkg/auth.go", Line: 10, Ref: "Login", Msg: "signature mismatch"},
		},
	}
	f.Add(r.Markdown())
	f.Add(ReportHeader + "\n\nScribex: docs in sync (0 anchors checked).\n")
	f.Add("not a valid header")
	f.Add(ReportHeader + "\n\n- `a.go` line 1: @team alert")
	f.Add(ReportHeader + "\n\n- `a.go` line 1: <script>alert(1)</script>")
	f.Add(ReportHeader + "\n\n- `a.go` line 1: http://evil.com")
	f.Add(ReportHeader + "\n\n- `a.go` line 1: \x00nul")
	f.Add(ReportHeader + "\n\n- `a.go` line 1: \u202Ebidi")

	f.Fuzz(func(t *testing.T, input string) {
		out, err := SanitizeReport(input)
		if err != nil {
			// Validation rejected it, which is safe
			return
		}

		// Invariant checks on accepted output:
		if !utf8.ValidString(out) {
			t.Fatalf("accepted output has invalid UTF-8")
		}
		if strings.ContainsRune(out, '\x00') {
			t.Fatalf("accepted output contains NUL byte")
		}
		for _, r := range out {
			if unicode.Is(unicode.Cf, r) {
				t.Fatalf("accepted output contains Unicode Cf rune: %U", r)
			}
		}
		if !strings.HasPrefix(out, ReportHeader) {
			t.Fatalf("accepted output does not start with ReportHeader")
		}
		if len(MarkerV1)+1+len(out) > MaxBodyLen {
			t.Fatalf("accepted output length %d + marker exceeds MaxBodyLen", len(out))
		}

		// Verify no forbidden constructs outside code spans
		lines := strings.Split(out, "\n")
		for i, line := range lines {
			if err := validateLineCodeSpans(line, i+1); err != nil {
				t.Fatalf("accepted output violates line validation: %v", err)
			}
		}
	})
}
