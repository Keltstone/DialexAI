package scan

import (
	"math/rand"
	"strings"
	"testing"
)

func TestJoinTable30Cases(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []Token
		expected string
	}{
		{
			name: "Kotlin fun simple",
			tokens: []Token{
				{TokIdent, "fun"}, {TokIdent, "foo"}, {TokPunct, "("}, {TokIdent, "x"},
				{TokPunct, ":"}, {TokIdent, "Int"}, {TokPunct, ")"}, {TokPunct, ":"}, {TokIdent, "Unit"},
			},
			expected: "fun foo(x: Int): Unit",
		},
		{
			name: "Trailing comma dropped before paren",
			tokens: []Token{
				{TokIdent, "fun"}, {TokIdent, "bar"}, {TokPunct, "("}, {TokIdent, "a"},
				{TokPunct, ":"}, {TokIdent, "Int"}, {TokPunct, ","}, {TokPunct, ")"},
			},
			expected: "fun bar(a: Int)",
		},
		{
			name: "Trailing comma dropped before bracket",
			tokens: []Token{
				{TokPunct, "["}, {TokIdent, "1"}, {TokPunct, ","}, {TokPunct, "2"}, {TokPunct, ","}, {TokPunct, "]"},
			},
			expected: "[1, 2]",
		},
		{
			name: "Trailing comma dropped before angle bracket",
			tokens: []Token{
				{TokIdent, "Map"}, {TokPunct, "<"}, {TokIdent, "K"}, {TokPunct, ","}, {TokIdent, "V"}, {TokPunct, ","}, {TokPunct, ">"},
			},
			expected: "Map<K, V>",
		},
		{
			name: "Generic class",
			tokens: []Token{
				{TokIdent, "class"}, {TokIdent, "Box"}, {TokPunct, "<"}, {TokIdent, "T"},
				{TokPunct, ":"}, {TokIdent, "Any"}, {TokPunct, ">"}, {TokPunct, "("},
				{TokIdent, "val"}, {TokIdent, "item"}, {TokPunct, ":"}, {TokIdent, "T"}, {TokPunct, ")"},
			},
			expected: "class Box<T: Any>(val item: T)",
		},
		{
			name: "Nullable generic return type",
			tokens: []Token{
				{TokIdent, "fun"}, {TokIdent, "get"}, {TokPunct, "("}, {TokPunct, ")"},
				{TokPunct, ":"}, {TokIdent, "List"}, {TokPunct, "<"}, {TokIdent, "String"}, {TokPunct, ">"}, {TokPunct, "?"},
			},
			expected: "fun get(): List<String>?",
		},
		{
			name: "Annotation with string arg",
			tokens: []Token{
				{TokPunct, "@"}, {TokIdent, "Deprecated"}, {TokPunct, "("}, {TokStr, `"use newFoo"`}, {TokPunct, ")"},
				{TokIdent, "fun"}, {TokIdent, "oldFoo"}, {TokPunct, "("}, {TokPunct, ")"},
			},
			expected: `@Deprecated("use newFoo") fun oldFoo()`,
		},
		{
			name: "Function type with arrow",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "cb"}, {TokPunct, ":"}, {TokPunct, "("},
				{TokIdent, "Int"}, {TokPunct, ")"}, {TokPunct, "->"}, {TokIdent, "Unit"},
			},
			expected: "val cb: (Int) -> Unit",
		},
		{
			name: "TypeScript arrow function type",
			tokens: []Token{
				{TokIdent, "type"}, {TokIdent, "Handler"}, {TokPunct, "="}, {TokPunct, "("},
				{TokIdent, "x"}, {TokPunct, ":"}, {TokIdent, "number"}, {TokPunct, ")"}, {TokPunct, "=>"}, {TokIdent, "void"},
			},
			expected: "type Handler = (x: number) => void",
		},
		{
			name: "Kotlin double bang !!",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "x"}, {TokPunct, "="}, {TokIdent, "a"}, {TokPunct, "!!"},
			},
			expected: "val x = a!!",
		},
		{
			name: "Kotlin safe call ?.",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "len"}, {TokPunct, "="}, {TokIdent, "str"}, {TokPunct, "?."}, {TokIdent, "length"},
			},
			expected: "val len = str?.length",
		},
		{
			name: "Kotlin double colon ::",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "ref"}, {TokPunct, "="}, {TokIdent, "String"}, {TokPunct, "::"}, {TokIdent, "toInt"},
			},
			expected: "val ref = String::toInt",
		},
		{
			name: "Kotlin range ..",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "r"}, {TokPunct, "="}, {TokIdent, "1"}, {TokPunct, ".."}, {TokIdent, "10"},
			},
			expected: "val r = 1..10",
		},
		{
			name: "Java varargs ...",
			tokens: []Token{
				{TokIdent, "public"}, {TokIdent, "void"}, {TokIdent, "log"}, {TokPunct, "("},
				{TokIdent, "String"}, {TokPunct, "..."}, {TokIdent, "args"}, {TokPunct, ")"},
			},
			expected: "public void log(String... args)",
		},
		{
			name: "TS rest parameter ...",
			tokens: []Token{
				{TokIdent, "function"}, {TokIdent, "sum"}, {TokPunct, "("},
				{TokPunct, "..."}, {TokIdent, "nums"}, {TokPunct, ":"}, {TokIdent, "number"}, {TokPunct, "["}, {TokPunct, "]"}, {TokPunct, ")"},
			},
			expected: "function sum(... nums: number[])",
		},
		{
			name: "Equality comparison ==",
			tokens: []Token{
				{TokIdent, "const"}, {TokIdent, "eq"}, {TokPunct, "="}, {TokIdent, "a"}, {TokPunct, "=="}, {TokIdent, "b"},
			},
			expected: "const eq = a == b",
		},
		{
			name: "Inequality comparison !=",
			tokens: []Token{
				{TokIdent, "const"}, {TokIdent, "neq"}, {TokPunct, "="}, {TokIdent, "a"}, {TokPunct, "!="}, {TokIdent, "b"},
			},
			expected: "const neq = a != b",
		},
		{
			name: "Less than or equal <=",
			tokens: []Token{
				{TokIdent, "const"}, {TokIdent, "ok"}, {TokPunct, "="}, {TokIdent, "x"}, {TokPunct, "<="}, {TokIdent, "10"},
			},
			expected: "const ok = x <= 10",
		},
		{
			name: "Greater than or equal >=",
			tokens: []Token{
				{TokIdent, "const"}, {TokIdent, "ok"}, {TokPunct, "="}, {TokIdent, "x"}, {TokPunct, ">="}, {TokIdent, "10"},
			},
			expected: "const ok = x >= 10",
		},
		{
			name: "Logical and &&",
			tokens: []Token{
				{TokIdent, "const"}, {TokIdent, "both"}, {TokPunct, "="}, {TokIdent, "a"}, {TokPunct, "&&"}, {TokIdent, "b"},
			},
			expected: "const both = a && b",
		},
		{
			name: "Logical or ||",
			tokens: []Token{
				{TokIdent, "const"}, {TokIdent, "either"}, {TokPunct, "="}, {TokIdent, "a"}, {TokPunct, "||"}, {TokIdent, "b"},
			},
			expected: "const either = a || b",
		},
		{
			name: "Unary not ! and tilde ~",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "neg"}, {TokPunct, "="}, {TokPunct, "!"}, {TokIdent, "flag"},
				{TokPunct, ";"}, {TokIdent, "val"}, {TokIdent, "inv"}, {TokPunct, "="}, {TokPunct, "~"}, {TokIdent, "mask"},
			},
			expected: "val neg = !flag; val inv = ~mask",
		},
		{
			name: "Array type int[][]",
			tokens: []Token{
				{TokIdent, "int"}, {TokPunct, "["}, {TokPunct, "]"}, {TokPunct, "["}, {TokPunct, "]"}, {TokIdent, "matrix"},
			},
			expected: "int[][] matrix",
		},
		{
			name: "Nested parentheses",
			tokens: []Token{
				{TokPunct, "("}, {TokPunct, "("}, {TokIdent, "x"}, {TokPunct, "+"}, {TokIdent, "1"}, {TokPunct, ")"}, {TokPunct, ")"},
			},
			expected: "((x + 1))",
		},
		{
			name: "Default parameter with expression",
			tokens: []Token{
				{TokIdent, "fun"}, {TokIdent, "init"}, {TokPunct, "("}, {TokIdent, "timeout"},
				{TokPunct, ":"}, {TokIdent, "Long"}, {TokPunct, "="}, {TokIdent, "5000L"}, {TokPunct, ")"},
			},
			expected: "fun init(timeout: Long = 5000L)",
		},
		{
			name: "Multiple parameters",
			tokens: []Token{
				{TokIdent, "fun"}, {TokIdent, "add"}, {TokPunct, "("}, {TokIdent, "a"},
				{TokPunct, ":"}, {TokIdent, "Int"}, {TokPunct, ","}, {TokIdent, "b"},
				{TokPunct, ":"}, {TokIdent, "Int"}, {TokPunct, ")"}, {TokPunct, ":"}, {TokIdent, "Int"},
			},
			expected: "fun add(a: Int, b: Int): Int",
		},
		{
			name: "Constructor declaration",
			tokens: []Token{
				{TokIdent, "constructor"}, {TokPunct, "("}, {TokIdent, "name"}, {TokPunct, ":"}, {TokIdent, "String"}, {TokPunct, ")"},
			},
			expected: "constructor(name: String)",
		},
		{
			name: "Backticked Kotlin identifier",
			tokens: []Token{
				{TokIdent, "fun"}, {TokIdent, "`my special function`"}, {TokPunct, "("}, {TokPunct, ")"},
			},
			expected: "fun `my special function`()",
		},
		{
			name: "Unicode identifier",
			tokens: []Token{
				{TokIdent, "val"}, {TokIdent, "π"}, {TokPunct, "="}, {TokIdent, "3.14159"},
			},
			expected: "val π = 3.14159",
		},
		{
			name: "Throws clause in Java",
			tokens: []Token{
				{TokIdent, "public"}, {TokIdent, "void"}, {TokIdent, "read"}, {TokPunct, "("}, {TokPunct, ")"},
				{TokIdent, "throws"}, {TokIdent, "IOException"}, {TokPunct, ","}, {TokIdent, "TimeoutException"},
			},
			expected: "public void read() throws IOException, TimeoutException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Join(tt.tokens)
			if actual != tt.expected {
				t.Fatalf("\nexpected: %q\nactual:   %q", tt.expected, actual)
			}
		})
	}
}

func TestJoinPropertyArbitraryWhitespaceAndComments(t *testing.T) {
	snippets := []string{
		"fun <T: Comparable<T>> sort(list: List<T>): List<T>",
		"public static <K, V> Map<K, V> of(K k1, V v1, K k2, V v2) throws IllegalArgumentException",
		"export async function fetchUser<T>(id: string, options?: RequestInit): Promise<User<T>>",
		"def compute_metrics(data: list[int], threshold: float = 0.5) -> dict[str, float]:",
	}

	rng := rand.New(rand.NewSource(12345))

	for _, original := range snippets {
		sOrig, err := Lex(Kotlin, []byte(original))
		if err != nil {
			t.Fatalf("lex error for original: %v", err)
		}
		toksOrig := Tokenize(sOrig, 0, len(sOrig.B))
		canonical := Join(toksOrig)

		// Insert random whitespace, newlines, and comments between tokens
		for iter := 0; iter < 10; iter++ {
			var sb strings.Builder
			for i, tok := range toksOrig {
				sb.WriteString(tok.Text)
				if i < len(toksOrig)-1 {
					noiseType := rng.Intn(4)
					switch noiseType {
					case 0:
						sb.WriteString("   \t  ")
					case 1:
						sb.WriteString("\n\n  \t")
					case 2:
						sb.WriteString(" /* comment */ ")
					case 3:
						sb.WriteString(" // line comment\n   ")
					}
				}
			}
			mangled := sb.String()
			sMangled, err := Lex(Kotlin, []byte(mangled))
			if err != nil {
				t.Fatalf("lex error for mangled:\n%s\nerr: %v", mangled, err)
			}
			toksMangled := Tokenize(sMangled, 0, len(sMangled.B))
			mangledCanonical := Join(toksMangled)

			if mangledCanonical != canonical {
				t.Fatalf("Property violation!\nOriginal canonical: %q\nMangled canonical:  %q\nMangled input:\n%s",
					canonical, mangledCanonical, mangled)
			}
		}
	}
}

func BenchmarkLexer1MB(b *testing.B) {
	// 1 MB Kotlin file
	base := "fun foo(x: Int, y: String = \"hello\"): List<Int> { val z = 1 + 2; return listOf(z) }\n"
	repeats := (1024 * 1024) / len(base)
	data := []byte(strings.Repeat(base, repeats))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Lex(Kotlin, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
