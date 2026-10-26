package anchors

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseGrammar(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []Anchor
	}{
		{
			name: "basic",
			md:   "<!-- @scribex:anchor(file#Sym) -->",
			want: []Anchor{{Line: 1, File: "file", Sym: "Sym", Sig: ""}},
		},
		{
			name: "with sig",
			md:   "<!-- @scribex:anchor(file#Sym) sig=1234abcd -->",
			want: []Anchor{{Line: 1, File: "file", Sym: "Sym", Sig: "1234abcd"}},
		},
		{
			name: "symbol with parens",
			md:   "<!-- @scribex:anchor(pkg/a.go#A.m(int,String)) sig=deadbeef -->",
			want: []Anchor{{Line: 1, File: "pkg/a.go", Sym: "A.m(int,String)", Sig: "deadbeef"}},
		},
		{
			name: "generic",
			md:   "<!-- @scribex:anchor(pkg/b.go#Box<T>.get) -->",
			want: []Anchor{{Line: 1, File: "pkg/b.go", Sym: "Box<T>.get", Sig: ""}},
		},
		{
			name: "extra spaces",
			md:   "  <!--   @scribex:anchor(file#Sym)   sig=1234abcd   -->  ",
			want: []Anchor{{Line: 1, File: "file", Sym: "Sym", Sig: "1234abcd"}},
		},
		{
			name: "trailing garbage",
			md:   "<!-- @scribex:anchor(file#Sym) --> extra",
			want: nil,
		},
		{
			name: "two anchors on one line",
			md:   "<!-- @scribex:anchor(a#A) --> <!-- @scribex:anchor(b#B) -->",
			want: nil,
		},
		{
			name: "anchor in backtick fence",
			md:   "```\n<!-- @scribex:anchor(file#Sym) -->\n```",
			want: nil,
		},
		{
			name: "anchor in tilde fence",
			md:   "~~~\n<!-- @scribex:anchor(file#Sym) -->\n~~~",
			want: nil,
		},
		{
			name: "4-backtick fence containing a 3-backtick line",
			md:   "````\n```\n<!-- @scribex:anchor(file#Sym) -->\n````",
			want: nil,
		},
		{
			name: "unclosed fence",
			md:   "```\n<!-- @scribex:anchor(file#Sym) -->",
			want: nil,
		},
		{
			name: "indented 4 spaces (indented code block, treated as NOT a fence opener; anchor still matches)",
			md:   "    ```\n<!-- @scribex:anchor(file#Sym) -->\n    ```",
			want: []Anchor{{Line: 2, File: "file", Sym: "Sym", Sig: ""}},
		},
		{
			name: "fence info string",
			md:   "```go\n<!-- @scribex:anchor(file#Sym) -->\n```",
			want: nil,
		},
		{
			name: "anchor immediately followed by Scribex signature fence then another anchor",
			md: "<!-- @scribex:anchor(a#A) sig=11111111 -->\n" +
				"```signature\n" +
				"func A()\n" +
				"```\n" +
				"<!-- @scribex:anchor(b#B) sig=22222222 -->\n" +
				"```signature\n" +
				"func B()\n" +
				"```",
			want: []Anchor{
				{Line: 1, File: "a", Sym: "A", Sig: "11111111"},
				{Line: 5, File: "b", Sym: "B", Sig: "22222222"},
			},
		},
		{
			name: "file 4097 bytes rejected",
			md:   "<!-- @scribex:anchor(" + strings.Repeat("a", 4097) + "#Sym) -->",
			want: nil,
		},
		{
			name: "NUL in path rejected",
			md:   "<!-- @scribex:anchor(a\x00b#Sym) -->",
			want: nil,
		},
		{
			name: "empty file part rejected",
			md:   "<!-- @scribex:anchor(#Sym) -->",
			want: nil,
		},
		{
			name: "sym exceeds 512 bytes rejected",
			md:   "<!-- @scribex:anchor(file#" + strings.Repeat("s", 513) + ") -->",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.md)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
