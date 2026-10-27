package scan

import (
	"strings"
	"testing"
)

func TestCleanDoc(t *testing.T) {
	// Unicode Cf should be removed
	cfStr := "Hello\u200bWorld\u200f!"
	cleaned := CleanDoc(cfStr)
	if cleaned != "HelloWorld!" {
		t.Errorf("CleanDoc(cfStr) = %q, want HelloWorld!", cleaned)
	}

	// Control chars except \n and \t should be removed
	ctrlStr := "Hello\x00\x01\x1f\n\tWorld\x7f"
	cleaned = CleanDoc(ctrlStr)
	if cleaned != "Hello\n\tWorld" {
		t.Errorf("CleanDoc(ctrlStr) = %q, want Hello\\n\\tWorld", cleaned)
	}

	// Max 2000 bytes capped on rune boundary with '…'
	longStr := strings.Repeat("A", 2500)
	cleaned = CleanDoc(longStr)
	if !strings.HasSuffix(cleaned, "…") {
		t.Errorf("expected ellipsis suffix")
	}
	// Prefix before '…' should be exactly 2000 bytes
	prefix := strings.TrimSuffix(cleaned, "…")
	if len([]byte(prefix)) != 2000 {
		t.Errorf("len(prefix) = %d, want 2000", len([]byte(prefix)))
	}

	// Multi-byte rune at boundary
	multiByte := strings.Repeat("€", 700) // each € is 3 bytes (2100 bytes)
	cleaned = CleanDoc(multiByte)
	if !strings.HasSuffix(cleaned, "…") {
		t.Errorf("expected ellipsis suffix")
	}
	prefix = strings.TrimSuffix(cleaned, "…")
	if len([]byte(prefix))%3 != 0 || len([]byte(prefix)) > 2000 {
		t.Errorf("invalid rune cut: len = %d", len([]byte(prefix)))
	}
}

func TestPrecedingDoc(t *testing.T) {
	cases := []struct {
		name         string
		lang         Lang
		src          string
		headerMarker string
		wantDoc      string
	}{
		{
			name: "line comment above func",
			lang: Kotlin,
			src: `
// This is a function
fun foo() {}
`,
			headerMarker: "fun foo",
			wantDoc:      "This is a function",
		},
		{
			name: "multi-line line comment",
			lang: Kotlin,
			src: `
// Line 1
// Line 2
fun foo() {}
`,
			headerMarker: "fun foo",
			wantDoc:      "Line 1\nLine 2",
		},
		{
			name: "doc above annotation",
			lang: Kotlin,
			src: `
// Doc above annotation
@Deprecated
fun foo() {}
`,
			headerMarker: "@Deprecated",
			wantDoc:      "Doc above annotation",
		},
		{
			name: "doc with blank line (not attached)",
			lang: Kotlin,
			src: `
// Unattached comment

fun foo() {}
`,
			headerMarker: "fun foo",
			wantDoc:      "",
		},
		{
			name: "block comment",
			lang: Java,
			src: `
/* Block comment */
public void run() {}
`,
			headerMarker: "public void run",
			wantDoc:      "Block comment",
		},
		{
			name: "javadoc style block comment",
			lang: Java,
			src: `
/**
 * Javadoc line 1
 * Javadoc line 2
 */
public void run() {}
`,
			headerMarker: "public void run",
			wantDoc:      "Javadoc line 1\nJavadoc line 2",
		},
		{
			name: "triple slash doc",
			lang: TS,
			src: `
/// Triple slash doc
export function test() {}
`,
			headerMarker: "export function",
			wantDoc:      "Triple slash doc",
		},
		{
			name: "hash comment",
			lang: Python,
			src: `
# Hash comment
def test():
    pass
`,
			headerMarker: "def test",
			wantDoc:      "Hash comment",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Lex(tc.lang, []byte(tc.src))
			if err != nil {
				t.Fatalf("Lex error: %v", err)
			}
			idx := strings.Index(string(s.B), tc.headerMarker)
			if idx < 0 {
				t.Fatalf("marker %q not found in source", tc.headerMarker)
			}
			got := PrecedingDoc(s, idx)
			if got != tc.wantDoc {
				t.Errorf("PrecedingDoc = %q, want %q", got, tc.wantDoc)
			}
		})
	}
}

func TestDocstring(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "single line triple double",
			raw:  `"""Hello World"""`,
			want: "Hello World",
		},
		{
			name: "single line triple single",
			raw:  `'''Hello World'''`,
			want: "Hello World",
		},
		{
			name: "raw prefix",
			raw:  `r"""Raw \n text"""`,
			want: `Raw \n text`,
		},
		{
			name: "multiline dedent pep257",
			raw: `"""
    Line 1
    Line 2
    """`,
			want: "Line 1\nLine 2",
		},
		{
			name: "multiline with summary on first line",
			raw: `"""Summary line.

    Detailed description line 1.
    Detailed description line 2.
    """`,
			want: "Summary line.\n\nDetailed description line 1.\nDetailed description line 2.",
		},
		{
			name: "empty docstring",
			raw:  `""""""`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Docstring(tc.raw)
			if got != tc.want {
				t.Errorf("Docstring(%q)\ngot:  %q\nwant: %q", tc.raw, got, tc.want)
			}
		})
	}
}
