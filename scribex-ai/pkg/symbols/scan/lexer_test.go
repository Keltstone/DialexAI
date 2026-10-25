package scan

import (
	"math/rand"
	"strings"
	"testing"
)

func TestLexerTable(t *testing.T) {
	tests := []struct {
		name      string
		lang      Lang
		src       string
		wantErr   bool
		errSub    string
		checkCode []string // strings that must be in Code
		checkStr  []string // strings that must be in Str
		checkComm []string // strings that must be in Comment
	}{
		// Kotlin
		{
			name:      "Kotlin line comment",
			lang:      Kotlin,
			src:       "fun foo() // this is comment\nval x = 1",
			checkCode: []string{"fun foo() ", "val x = 1"},
			checkComm: []string{"// this is comment"},
		},
		{
			name:      "Kotlin nested block comments",
			lang:      Kotlin,
			src:       "/* outer /* inner */ still comment */ val x = 1",
			checkCode: []string{"val x = 1"},
			checkComm: []string{"/* outer /* inner */ still comment */"},
		},
		{
			name:    "Kotlin unclosed block comment",
			lang:    Kotlin,
			src:     "/* unclosed comment val x = 1",
			wantErr: true,
			errSub:  "unterminated block comment",
		},
		{
			name:     "Kotlin string with escapes and comment inside",
			lang:     Kotlin,
			src:      `val s = "hello // not comment \" still string"`,
			checkStr: []string{`"hello // not comment \" still string"`},
		},
		{
			name:     "Kotlin template with brace counting",
			lang:     Kotlin,
			src:      `val s = "result: ${ 1 + 2 } and ${ "nested: ${3}" } end"`,
			checkStr: []string{`"result: ${ 1 + 2 } and ${ "nested: ${3}" } end"`},
		},
		{
			name:     `Kotlin template with quotes like "${ "}" }"`,
			lang:     Kotlin,
			src:      `val s = "${ "}" }"`,
			checkStr: []string{`"${ "}" }"`},
		},
		{
			name:     "Kotlin raw string with newlines and quotes",
			lang:     Kotlin,
			src:      "val s = \"\"\"line 1\nline 2 \"quote\" // not comment\n\"\"\"",
			checkStr: []string{"\"\"\"line 1\nline 2 \"quote\" // not comment\n\"\"\""},
		},
		{
			name:    "Kotlin unclosed string",
			lang:    Kotlin,
			src:     "val s = \"unclosed\nval y = 2",
			wantErr: true,
		},
		{
			name:     "Kotlin char literals",
			lang:     Kotlin,
			src:      "val c1 = 'x'; val c2 = '\\n'; val c3 = '\\''",
			checkStr: []string{"'x'", "'\\n'", "'\\''"},
		},

		// Java
		{
			name:      "Java comments",
			lang:      Java,
			src:       "// line comment\n/* block comment */ int x = 1;",
			checkCode: []string{"int x = 1;"},
			checkComm: []string{"// line comment", "/* block comment */"},
		},
		{
			name:     "Java text block",
			lang:     Java,
			src:      "String s = \"\"\"\nhello \"world\" \\\n\"\"\" ;",
			checkStr: []string{"\"\"\"\nhello \"world\" \\\n\"\"\""},
		},
		{
			name:    "Java unclosed text block",
			lang:    Java,
			src:     "String s = \"\"\"\nhello world",
			wantErr: true,
			errSub:  "unterminated text block",
		},
		{
			name:    "Java unescaped newline in string",
			lang:    Java,
			src:     "String s = \"line 1\nline 2\";",
			wantErr: true,
		},

		// TS/JS
		{
			name:     "TS string and template literals",
			lang:     TS,
			src:      "const a = 'hello'; const b = \"world\"; const c = `temp ${a + `inner`} end`;",
			checkStr: []string{"'hello'", `"world"`, "`temp ${a + `inner`} end`"},
		},
		{
			name:     "TS regex after assignment",
			lang:     TS,
			src:      "const re = /ab+c/g; const y = 2;",
			checkStr: []string{"/ab+c/g"},
		},
		{
			name:      "TS division vs regex",
			lang:      TS,
			src:       "const x = a / b / c;",
			checkCode: []string{"a / b / c"},
		},
		{
			name:      "TS paren division",
			lang:      TS,
			src:       "const x = (a + b) / 2;",
			checkCode: []string{") / 2"},
		},
		{
			name:     "TS regex after brace",
			lang:     TS,
			src:      "if (x) {}\n/re/.test(x)",
			checkStr: []string{"/re/"},
		},
		{
			name:     "TS regex with block comment lookalike inside",
			lang:     TS,
			src:      "const re = /* regex */ /\\/*test/;",
			checkStr: []string{"/\\/*test/"},
		},
		{
			name:     "TS regex with bracket class",
			lang:     TS,
			src:      "const re = /[/]/;",
			checkStr: []string{"/[/]/"},
		},

		// Python
		{
			name:      "Python comments and strings",
			lang:      Python,
			src:       "# comment\nx = 'single' + \"double\"",
			checkComm: []string{"# comment"},
			checkStr:  []string{"'single'", `"double"`},
		},
		{
			name:     "Python triple quotes",
			lang:     Python,
			src:      "x = '''triple\none''' + \"\"\"triple\ntwo\"\"\"",
			checkStr: []string{"'''triple\none'''", "\"\"\"triple\ntwo\"\"\""},
		},
		{
			name:     "Python raw and f-string prefixes",
			lang:     Python,
			src:      `a = r"raw \" string" + f"f-string {x + 1}" + br'bytes'`,
			checkStr: []string{`r"raw \" string"`, `f"f-string {x + 1}"`, `br'bytes'`},
		},
		{
			name:     `Python r"\" edge case (escaped quote does not close)`,
			lang:     Python,
			src:      `x = r"\"" + 1`,
			checkStr: []string{`r"\""`},
		},
		{
			name:    "Python unescaped newline in single quote",
			lang:    Python,
			src:     "x = 'line 1\nline 2'",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Lex(tt.lang, []byte(tt.src))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("expected error containing %q, got %q", tt.errSub, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify substrings in checkCode
			for _, exp := range tt.checkCode {
				idx := strings.Index(string(s.B), exp)
				if idx < 0 {
					t.Fatalf("expected %q in source", exp)
				}
				for j := idx; j < idx+len(exp); j++ {
					if s.K[j] != Code {
						t.Fatalf("byte %d (%c) in %q is %v, expected Code", j, s.B[j], exp, s.K[j])
					}
				}
			}

			// Verify substrings in checkStr
			for _, exp := range tt.checkStr {
				idx := strings.Index(string(s.B), exp)
				if idx < 0 {
					t.Fatalf("expected %q in source", exp)
				}
				for j := idx; j < idx+len(exp); j++ {
					if s.K[j] != Str {
						t.Fatalf("byte %d (%c) in %q is %v, expected Str", j, s.B[j], exp, s.K[j])
					}
				}
			}

			// Verify substrings in checkComm
			for _, exp := range tt.checkComm {
				idx := strings.Index(string(s.B), exp)
				if idx < 0 {
					t.Fatalf("expected %q in source", exp)
				}
				for j := idx; j < idx+len(exp); j++ {
					if s.K[j] != Comment {
						t.Fatalf("byte %d (%c) in %q is %v, expected Comment", j, s.B[j], exp, s.K[j])
					}
				}
			}
		})
	}
}

func TestMatchClose(t *testing.T) {
	src := []byte("fun foo(a: Int, b: List<Int>): { val arr = [1, 2, (3 + 4)]; }")
	s, err := Lex(Kotlin, src)
	if err != nil {
		t.Fatalf("lex error: %v", err)
	}

	parenIdx := strings.Index(string(s.B), "(")
	closeParen, err := s.MatchClose(parenIdx)
	if err != nil {
		t.Fatalf("MatchClose paren error: %v", err)
	}
	if s.B[closeParen] != ')' {
		t.Fatalf("expected ')', got '%c'", s.B[closeParen])
	}

	braceIdx := strings.Index(string(s.B), "{")
	closeBrace, err := s.MatchClose(braceIdx)
	if err != nil {
		t.Fatalf("MatchClose brace error: %v", err)
	}
	if s.B[closeBrace] != '}' {
		t.Fatalf("expected '}', got '%c'", s.B[closeBrace])
	}

	bracketIdx := strings.Index(string(s.B), "[")
	closeBracket, err := s.MatchClose(bracketIdx)
	if err != nil {
		t.Fatalf("MatchClose bracket error: %v", err)
	}
	if s.B[closeBracket] != ']' {
		t.Fatalf("expected ']', got '%c'", s.B[closeBracket])
	}
}

func TestMatchCloseNestingLimit(t *testing.T) {
	// Construct 505 nested braces
	var sb strings.Builder
	for i := 0; i < 505; i++ {
		sb.WriteString("{\n")
	}
	for i := 0; i < 505; i++ {
		sb.WriteString("}\n")
	}
	// 1. Lex should reject nesting depth > 500
	_, err := Lex(Kotlin, []byte(sb.String()))
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected Lex error mentioning 500, got %v", err)
	}

	// 2. Direct MatchClose on Source should also reject depth > 500
	b := []byte(sb.String())
	k := make([]Kind, len(b))
	s := &Source{B: b, K: k}
	_, err = s.MatchClose(0)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected MatchClose error mentioning 500, got %v", err)
	}
}

func TestRandom1MBInputDoesNotPanic(t *testing.T) {
	// Generate 1 MB random bytes with typical code characters
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 \t\n{}()[]<>=+-*/\"'`#;,.:!?"
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = charset[rng.Intn(len(charset))]
	}

	langs := []Lang{Kotlin, Java, TS, Python, Go}
	for _, lang := range langs {
		// Should not panic, may return error or success
		_, _ = Lex(lang, data)
	}
}

func TestBOMAndCRLFNormalization(t *testing.T) {
	src := []byte("\xef\xbb\xbfLine 1\r\nLine 2\rLine 3\n")
	s, err := Lex(Kotlin, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Line 1\nLine 2\nLine 3\n"
	if string(s.B) != expected {
		t.Fatalf("expected %q, got %q", expected, string(s.B))
	}
	if len(s.LineStart) != 4 {
		t.Fatalf("expected 4 line starts, got %d: %v", len(s.LineStart), s.LineStart)
	}
}
