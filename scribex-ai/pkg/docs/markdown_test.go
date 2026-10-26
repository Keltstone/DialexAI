package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"scribex/pkg/anchors"
)

func TestReportMarkdownLengthCap(t *testing.T) {
	var issues []Issue
	for i := 0; i < 5000; i++ {
		issues = append(issues, Issue{
			File: fmt.Sprintf("pkg/sub%03d/file%03d.go", i%50, i),
			Issue: anchors.Issue{
				Line: i + 1,
				Ref:  fmt.Sprintf("pkg/sub%03d/file%03d.go#Symbol%d", i%50, i, i),
				Msg:  fmt.Sprintf("stale anchor: signature changed from Foo%d to Bar%d with extensive detailed description text", i, i),
			},
		})
	}

	r := Report{
		Issues: issues,
		Total:  10000,
	}

	md := r.Markdown()
	const marker = "<!-- scribex-report:v1 -->\n"
	totalLen := len(marker + md)

	if totalLen > 60000 {
		t.Fatalf("total body length %d exceeds 60,000 character limit", totalLen)
	}

	if !strings.Contains(md, "Scribex found 5000 doc issue(s) (10000 anchors):") {
		t.Errorf("header line should retain full count (5000 issues, 10000 anchors)")
	}

	if !strings.Contains(md, "not shown. Run `scribex check` locally for the full report.") {
		t.Errorf("expected cut notice in truncated report")
	}

	if !strings.Contains(md, "Run `scribex sync` to refresh stale anchors.") {
		t.Errorf("expected footer in report")
	}
}

func TestReportMarkdownDeterminismAndSafety(t *testing.T) {
	r := Report{
		Issues: []Issue{
			{
				File: "z.go",
				Issue: anchors.Issue{
					Line: 10,
					Ref:  "z.go#Z",
					Msg:  "plain error",
				},
			},
			{
				File: "a.go",
				Issue: anchors.Issue{
					Line: 5,
					Ref:  "a.go#A",
					Msg:  "@team/lead </details><img src=x onerror=alert(1)> [click](http://evil)",
				},
			},
			{
				File: "a.go",
				Issue: anchors.Issue{
					Line: 2,
					Ref:  "a.go#A0",
					Msg:  "#123 \n## Heading\n",
				},
			},
		},
		Total: 3,
	}

	md1 := r.Markdown()
	md2 := r.Markdown()

	if md1 != md2 {
		t.Errorf("Markdown rendering not deterministic")
	}

	// Verify order: `a.go` line 2 before `a.go` line 5 before `z.go` line 10
	posA0 := strings.Index(md1, "`a.go` line 2")
	posA := strings.Index(md1, "`a.go` line 5")
	posZ := strings.Index(md1, "`z.go` line 10")
	if posA0 < 0 || posA < 0 || posZ < 0 || !(posA0 < posA && posA < posZ) {
		t.Errorf("issues not sorted deterministically by (file, line): %s", md1)
	}

	// Hostile checks
	if strings.Contains(md1, "<img") || strings.Contains(md1, "</details>") {
		t.Errorf("HTML tags found unescaped in output: %s", md1)
	}
	if strings.Contains(md1, "](http://evil)") {
		t.Errorf("Markdown link found unescaped in output: %s", md1)
	}
	if regexp.MustCompile(`(^|[^\\])@`).MatchString(md1) {
		t.Errorf("Unescaped @mention found in output: %s", md1)
	}
}

func TestNoRawPrintfOfUnsanitisedFields(t *testing.T) {
	// Grep docs.go for any Fprintf/Sprintf with %s formatting raw i.File, i.Ref, i.Msg, u.File, u.Reason
	path := filepath.Join(".", "docs.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs.go: %v", err)
	}
	content := string(b)

	// Ensure mdsafe is called for fields in Markdown()
	if !strings.Contains(content, "mdsafe.Code(iss.File)") ||
		!strings.Contains(content, "mdsafe.Code(iss.Ref)") ||
		!strings.Contains(content, "mdsafe.Text(iss.Msg)") ||
		!strings.Contains(content, "mdsafe.Code(u.File)") ||
		!strings.Contains(content, "mdsafe.Text(u.Reason)") {
		t.Errorf("expected mdsafe sanitisation calls on all user-derived fields in docs.go")
	}

	// Forbid dangerous pattern: `fmt.Fprintf(&b, "- `%s` line %d `%s`: %s\n", i.File`
	if strings.Contains(content, "i.File, i.Line, i.Ref, i.Msg") || strings.Contains(content, "u.File, u.Reason") {
		t.Errorf("found raw unsanitised field arguments formatted directly")
	}
}
