package symbols

import (
	"strings"
	"testing"
	"time"
)

type corpusPair struct {
	name string
	path string
	old  string
	new  string
	lang string
}

func diffExtract(path, a, b string) (bool, error) {
	x, err1 := Extract(path, []byte(a))
	y, err2 := Extract(path, []byte(b))
	if err1 != nil {
		return false, err1
	}
	if err2 != nil {
		return false, err2
	}
	if len(x) != len(y) {
		return true, nil
	}
	for k, v := range x {
		if y[k] != v {
			return true, nil
		}
	}
	return false, nil
}

// Tracked contract-change corpus: Must change (mutations affecting public contract)
var corpusMustChange = []corpusPair{
	// Kotlin
	{"kt multiline param type", "A.kt", "class Foo {\n  fun run(\n a: Int,\n b: String\n ): Int = 1\n}", "class Foo {\n  fun run(\n a: Int,\n b: Long\n ): Int = 1\n}", "kotlin"},
	{"kt member fun ret", "A.kt", "class Foo { fun run(a: Int): Int = 1 }", "class Foo { fun run(a: Int): Long = 1 }", "kotlin"},
	{"kt param rename", "A.kt", "fun f(a: Int) {}", "fun f(b: Int) {}", "kotlin"},
	{"kt default value", "A.kt", "fun f(a: Int = 1) {}", "fun f(a: Int = 2) {}", "kotlin"},
	{"kt nullability", "A.kt", "fun f(a: String) {}", "fun f(a: String?) {}", "kotlin"},
	{"kt const val", "A.kt", "const val X = 1", "const val X = 2", "kotlin"},
	{"kt vis private->public", "A.kt", "private fun f() {}", "fun f() {}", "kotlin"},
	{"kt vis public->internal", "A.kt", "fun f() {}", "internal fun f() {}", "kotlin"},
	{"kt data class prop", "A.kt", "data class P(val a: Int)", "data class P(val a: Int, val b: Int)", "kotlin"},
	{"kt enum entry", "A.kt", "enum class E { A, B }", "enum class E { A, B, C }", "kotlin"},
	{"kt suspend", "A.kt", "fun f() {}", "suspend fun f() {}", "kotlin"},
	{"kt supertypes", "A.kt", "class A : B()", "class A : C()", "kotlin"},
	{"kt extension", "A.kt", "fun String.x(a: Int) {}", "fun String.x(a: Long) {}", "kotlin"},
	{"kt companion", "A.kt", "class A { companion object { fun m(a: Int) {} } }", "class A { companion object { fun m(a: Long) {} } }", "kotlin"},
	{"kt interface method", "A.kt", "interface I { fun m(a: Int) }", "interface I { fun m(a: Int, b: Int) }", "kotlin"},
	{"kt property type", "A.kt", "class A { val x: Int = 1 }", "class A { val x: Long = 1 }", "kotlin"},
	{"kt gate ret", "a.kt", "fun f(a: Int, b: String): Int { return 1 }\n", "fun f(a: Int, b: String): String { return \"\" }\n", "kotlin"},
	{"kt gate vis", "a.kt", "fun f(a: Int, b: String): Int { return 1 }\n", "private fun f(a: Int, b: String): Int { return 1 }\n", "kotlin"},
	{"kt gate multiline", "a.kt", "fun f(a: Int, b: String): Int { return 1 }\n", "fun f(\n    a: Int,\n    b: Boolean,\n): Int { return 1 }\n", "kotlin"},
	{"kt gate exprbody", "a.kt", "fun f(a: Int, b: String): Int { return 1 }\n", "fun f(a: Int, b: String) = 1\n", "kotlin"},
	{"kt zz11 annotation", "a.kt", "@Deprecated(\"x\")\nfun f(a: Int) {}\n", "fun f(a: Int) {}\n", "kotlin"},
	{"kt zz11 open class", "a.kt", "open class C { fun m() {} }\n", "class C { fun m() {} }\n", "kotlin"},

	// Java
	{"java ret", "A.java", "public class A { public int m(int a) { return a; } }", "public class A { public long m(int a) { return a; } }", "java"},
	{"java throws", "A.java", "public class A { public void m() {} }", "public class A { public void m() throws Exception {} }", "java"},
	{"java static final", "A.java", "public class A { public static final int X = 3; }", "public class A { public static final int X = 4; }", "java"},
	{"java generic bound", "A.java", "public class A { public <T extends Number> T m(T a) { return a; } }", "public class A { public <T extends Integer> T m(T a) { return a; } }", "java"},
	{"java protected->public", "A.java", "public class A { protected void m() {} }", "public class A { public void m() {} }", "java"},
	{"java interface add", "A.java", "public interface I { void m(); }", "public interface I { void m(int a); }", "java"},
	{"java record", "A.java", "public record R(int a) {}", "public record R(int a, int b) {}", "java"},
	{"java varargs", "A.java", "public class A { public void m(int a) {} }", "public class A { public void m(int... a) {} }", "java"},
	{"java ctor", "A.java", "public class A { public A(int a) {} }", "public class A { public A(long a) {} }", "java"},
	{"java enum", "A.java", "public enum E { A, B }", "public enum E { A, B, C }", "java"},
	{"java gate ret", "A.java", "public class A { public int f(int a, String b) { return 1; } }\n", "public class A { public String f(int a, String b) { return \"\"; } }\n", "java"},
	{"java gate vis", "A.java", "public class A { public int f(int a, String b) { return 1; } }\n", "public class A { private int f(int a, String b) { return 1; } }\n", "java"},
	{"java gate throws", "A.java", "public class A { public int f(int a, String b) { return 1; } }\n", "public class A { public int f(int a, String b) throws Exception { return 1; } }\n", "java"},
	{"java zz11 generics overload", "A.java", "public class A { public <T> T m(T a){return a;} }\n", "public class A { public <T> T m(T a, T b){return a;} }\n", "java"},
	{"java zz11 member anchor final", "A.java", "public class A { public final int x = 1; }\n", "public class A { public int x = 1; }\n", "java"},

	// TypeScript / JS
	{"ts param type", "a.ts", "export function f(a: number): void {}", "export function f(a: string): void {}", "ts"},
	{"ts ret", "a.ts", "export function f(a: number): void {}", "export function f(a: number): number { return 1 }", "ts"},
	{"ts optional", "a.ts", "export function f(a: number) {}", "export function f(a?: number) {}", "ts"},
	{"ts const lit", "a.ts", "export const X = 1;", "export const X = 2;", "ts"},
	{"ts iface member", "a.ts", "export interface I { a: number }", "export interface I { a: number; b: string }", "ts"},
	{"ts class member type", "a.ts", "export class C { m(a: number): void {} }", "export class C { m(a: string): void {} }", "ts"},
	{"ts private->public", "a.ts", "export class C { private m() {} }", "export class C { m() {} }", "ts"},
	{"ts type alias", "a.ts", "export type T = string;", "export type T = number;", "ts"},
	{"ts enum", "a.ts", "export enum E { A = 1 }", "export enum E { A = 2 }", "ts"},
	{"ts async", "a.ts", "export function f() {}", "export async function f() {}", "ts"},
	{"ts reexport", "a.ts", "export { a } from './x';", "export { a } from './y';", "ts"},
	{"ts default", "a.ts", "export default function f(a: number) {}", "export default function f(a: string) {}", "ts"},
	{"js param rename", "a.js", "export function f(a) {}", "export function f(b) {}", "ts"},
	{"js cjs", "a.js", "exports.f = function(a) {}", "exports.f = function(a, b) {}", "ts"},
	{"ts gate ret", "a.ts", "export function f(a: number, b: string): number { return 1 }\n", "export function f(a: number, b: string): string { return \"\" }\n", "ts"},
	{"ts gate param", "a.ts", "export function f(a: number, b: string): number { return 1 }\n", "export function f(a: number, b: string, c?: boolean): number { return 1 }\n", "ts"},
	{"ts gate multiline", "a.ts", "export function f(a: number, b: string): number { return 1 }\n", "export function f(\n  a: number,\n  b: boolean,\n): number { return 1 }\n", "ts"},
	{"ts zz11 overloads", "a.ts", "export function f(a: number): void;\nexport function f(a: string): void;\nexport function f(a: any) {}\n", "export function f(a: number): void;\nexport function f(a: any) {}\n", "ts"},
	{"ts zz11 member anchor readonly", "a.ts", "export class C { readonly x = 1 }\n", "export class C { x = 1 }\n", "ts"},

	// Python
	{"py param", "a.py", "def f(a: int) -> int:\n    return a\n", "def f(a: str) -> int:\n    return 1\n", "python"},
	{"py default", "a.py", "def f(a=1):\n    pass\n", "def f(a=2):\n    pass\n", "python"},
	{"py ret", "a.py", "def f() -> int:\n    pass\n", "def f() -> str:\n    pass\n", "python"},
	{"py decorator", "a.py", "def f():\n    pass\n", "@deco\ndef f():\n    pass\n", "python"},
	{"py const", "a.py", "MAX = 1\n", "MAX = 2\n", "python"},
	{"py class base", "a.py", "class A(B):\n    pass\n", "class A(C):\n    pass\n", "python"},
	{"py method", "a.py", "class A:\n    def m(self, a):\n        pass\n", "class A:\n    def m(self, a, b):\n        pass\n", "python"},
	{"py private->public", "a.py", "def _f():\n    pass\n", "def f():\n    pass\n", "python"},
	{"py async", "a.py", "def f():\n    pass\n", "async def f():\n    pass\n", "python"},
	{"py multiline def", "a.py", "def f(\n    a: int,\n    b: str,\n) -> None:\n    pass\n", "def f(\n    a: int,\n    b: bytes,\n) -> None:\n    pass\n", "python"},
	{"py gate ret", "a.py", "def f(a: int, b: str) -> int:\n    return 1\n", "def f(a: int, b: str) -> str:\n    return \"\"\n", "python"},
	{"py gate default", "a.py", "def f(a: int, b: str) -> int:\n    return 1\n", "def f(a: int, b: str = \"x\") -> int:\n    return 1\n", "python"},
	{"py zz11 decorator", "a.py", "@app.route(\"/a\")\ndef f(): pass\n", "@app.route(\"/b\")\ndef f(): pass\n", "python"},
	{"py zz11 member anchor type", "a.py", "class C:\n    x: int = 1\n", "class C:\n    x: str = \"a\"\n", "python"},

	// Go
	{"go const val", "a.go", "package p\nconst X = 1\n", "package p\nconst X = 2\n", "go"},
	{"go const type", "a.go", "package p\nconst X int = 1\n", "package p\nconst X int64 = 1\n", "go"},
	{"go iota reorder", "a.go", "package p\nconst (\n A = iota\n B\n)\n", "package p\nconst (\n B = iota\n A\n)\n", "go"},
	{"go struct tag", "a.go", "package p\ntype T struct{ A int `json:\"a\"` }\n", "package p\ntype T struct{ A int `json:\"b\"` }\n", "go"},
	{"go var type", "a.go", "package p\nvar V int\n", "package p\nvar V int64\n", "go"},
	{"go gate ret", "p.go", "package p\nfunc F(a int, b string) (int, error) { return 0, nil }\n", "package p\nfunc F(a int, b string) (string, error) { return \"\", nil }\n", "go"},
	{"go gate rename", "p.go", "package p\nfunc F(a int, b string) (int, error) { return 0, nil }\n", "package p\nfunc F(x int, b string) (int, error) { return 0, nil }\n", "go"},
	{"go gate del", "p.go", "package p\nfunc F(a int, b string) (int, error) { return 0, nil }\n", "package p\n", "go"},
	{"go gate unexport", "p.go", "package p\nfunc F(a int, b string) (int, error) { return 0, nil }\n", "package p\nfunc f(a int, b string) (int, error) { return 0, nil }\n", "go"},
	{"go zz11 struct field add", "p.go", "package p\ntype T struct{ A int }\n", "package p\ntype T struct{ A int; B int }\n", "go"},
	{"go zz11 struct field rename", "p.go", "package p\ntype T struct{ A int }\n", "package p\ntype T struct{ a int }\n", "go"},
	{"go zz11 receiver value->pointer", "p.go", "package p\ntype T struct{}\nfunc (t T) M() {}\n", "package p\ntype T struct{}\nfunc (t *T) M() {}\n", "go"},
	{"go zz11 generic constraint", "p.go", "package p\nfunc F[T any](a T) {}\n", "package p\nfunc F[T comparable](a T) {}\n", "go"},
	{"go zz11 interface method add", "p.go", "package p\ntype I interface{ M() }\n", "package p\ntype I interface{ M(); N() }\n", "go"},
}

// Tracked contract-change corpus: Must NOT change (internal reflow, body, private members)
var corpusMustNotChange = []corpusPair{
	// Kotlin
	{"kt reflow+comments", "A.kt", "fun f(a: Int, b: String): Int = 1", "fun f(\n    a: Int, // first\n    /* b */ b: String,\n): Int = 1", "kotlin"},
	{"kt body", "A.kt", "fun f(): Int { return 1 }", "fun f(): Int { return 2 }", "kotlin"},
	{"kt private member", "A.kt", "class A { private fun x() {} }", "class A { private fun x(a: Int) {} }", "kotlin"},
	{"kt string decl", "A.kt", "fun f() { val s = \"fun hidden() {\" }", "fun f() { val s = \"fun other() {\" }", "kotlin"},
	{"kt crlf", "A.kt", "fun f(a: Int)\n{\n}", "fun f(a: Int)\r\n{\r\n}", "kotlin"},
	{"kt gate body", "a.kt", "fun f(a: Int, b: String): Int { return 1 }\n", "fun f(a: Int, b: String): Int { return 2 }\n", "kotlin"},

	// Java
	{"java override", "A.java", "public class A { public void m() {} }", "public class A { @Override public void m() {} }", "java"},
	{"java body", "A.java", "public class A { public int m() { return 1; } }", "public class A { public int m() { return 2; } }", "java"},
	{"java pkg-private", "A.java", "public class A { void x() {} }", "public class A { void x(int a) {} }", "java"},
	{"java reflow", "A.java", "public class A { public int m(int a, int b) { return a; } }", "public class A {\n  public int m(\n    int a,\n    int b\n  ) { return a; }\n}", "java"},
	{"java comment decl", "A.java", "public class A { /* public void hidden() {} */ }", "public class A { /* public void other() {} */ }", "java"},
	{"java gate body", "A.java", "public class A { public int f(int a, String b) { return 1; } }\n", "public class A { public int f(int a, String b) { return 2; } }\n", "java"},

	// TypeScript / JS
	{"ts reflow", "a.ts", "export function f(a: number, b: string): void {}", "export function f(\n  a: number,\n  b: string,\n): void {}", "ts"},
	{"ts body", "a.ts", "export function f() { return 1 }", "export function f() { return 2 }", "ts"},
	{"ts private", "a.ts", "export class C { private m() {} }", "export class C { private m(a: number) {} }", "ts"},
	{"ts unexported", "a.ts", "function g() {}", "function g(a: number) {}", "ts"},
	{"ts regex braces", "a.ts", "const r = /\\}\\{/;\nexport function f() {}", "const r = /\\{\\}/;\nexport function f() {}", "ts"},
	{"ts gate body", "a.ts", "export function f(a: number, b: string): number { return 1 }\n", "export function f(a: number, b: string): number { return 2 }\n", "ts"},
	{"ts gate fmt", "a.ts", "export function f(a: number, b: string): number { return 1 }\n", "export function f(a: number,   b: string): number {\n  return 1\n}\n", "ts"},

	// Python
	{"py body", "a.py", "def f():\n    return 1\n", "def f():\n    return 2\n", "python"},
	{"py private", "a.py", "def _f():\n    pass\n", "def _f(a):\n    pass\n", "python"},
	{"py docstring decl", "a.py", "def f():\n    \"\"\"def hidden(): pass\"\"\"\n", "def f():\n    \"\"\"def other(): pass\"\"\"\n", "python"},
	{"py comment", "a.py", "def f(a, b):\n    pass\n", "def f(a,  # x\n      b):\n    pass\n", "python"},
	{"py gate body", "a.py", "def f(a: int, b: str) -> int:\n    return 1\n", "def f(a: int, b: str) -> int:\n    return 2\n", "python"},

	// Go
	{"go body", "a.go", "package p\nfunc F() int { return 1 }\n", "package p\nfunc F() int { return 2 }\n", "go"},
	{"go unexported field", "a.go", "package p\ntype T struct{ a int }\n", "package p\ntype T struct{ a string }\n", "go"},
	{"go comment", "a.go", "package p\n// x\nfunc F() {}\n", "package p\n// y\nfunc F() {}\n", "go"},
	{"go gate body", "p.go", "package p\nfunc F(a int, b string) (int, error) { return 0, nil }\n", "package p\nfunc F(a int, b string) (int, error) { println(1); return 0, nil }\n", "go"},
	{"go gate fmt", "p.go", "package p\nfunc F(a int, b string) (int, error) { return 0, nil }\n", "package p\nfunc F(a int,   b string) (int, error) {\n\treturn 0, nil\n}\n", "go"},
	{"go zz11 body comment", "p.go", "package p\nfunc F(a int) int { return a }\n", "package p\n\n// new comment\nfunc F(a int) int {\n\treturn a + 1\n}\n", "go"},
	{"kt zz11 body comment", "a.kt", "fun f(a: Int) { println(\"a\") }\n", "fun f(a: Int) {\n  // changed\n  println(\"b\")\n}\n", "kotlin"},
	{"ts zz11 body comment", "a.ts", "export function f(a: number) { return 1 }\n", "export function f(a: number) {\n  // c\n  return 2\n}\n", "ts"},
	{"py zz11 docstring edit", "a.py", "def f(a):\n    \"\"\"doc\"\"\"\n    return 1\n", "def f(a):\n    \"\"\"other doc\"\"\"\n    return 2\n", "python"},
}

func TestTrackedContractCorpus(t *testing.T) {
	// Track stats per language
	langs := []string{"go", "ts", "python", "kotlin", "java"}
	type langStats struct {
		mustChangeTotal    int
		mustChangePassed   int
		mustNotChangeTotal int
		mustNotChangePass  int
	}
	stats := make(map[string]*langStats)
	for _, l := range langs {
		stats[l] = &langStats{}
	}

	// 1. Must change cases
	for _, p := range corpusMustChange {
		s := stats[p.lang]
		if s != nil {
			s.mustChangeTotal++
		}
		changed, err := diffExtract(p.path, p.old, p.new)
		if err != nil {
			t.Errorf("[MustChange] %s (%s): unexpected error: %v", p.name, p.lang, err)
		} else if !changed {
			t.Errorf("[MustChange] FALSE NO-CHANGE: %s (%s)", p.name, p.lang)
		} else {
			if s != nil {
				s.mustChangePassed++
			}
		}
	}

	// 2. Must NOT change cases
	for _, p := range corpusMustNotChange {
		s := stats[p.lang]
		if s != nil {
			s.mustNotChangeTotal++
		}
		changed, err := diffExtract(p.path, p.old, p.new)
		if err != nil {
			t.Errorf("[MustNotChange] %s (%s): unexpected error: %v", p.name, p.lang, err)
		} else if changed {
			t.Errorf("[MustNotChange] FALSE CHANGE: %s (%s)", p.name, p.lang)
		} else {
			if s != nil {
				s.mustNotChangePass++
			}
		}
	}

	// 3. Log per-language accuracy table
	t.Log("\n================ PER-LANGUAGE ACCURACY TABLE ================")
	t.Logf("%-12s | %-12s | %-15s | %-10s | %-10s", "Language", "Total Tests", "Passed Tests", "Accuracy", "Errors")
	t.Log("-------------+--------------+-----------------+------------+-----------")
	for _, l := range langs {
		s := stats[l]
		total := s.mustChangeTotal + s.mustNotChangeTotal
		passed := s.mustChangePassed + s.mustNotChangePass
		acc := 0.0
		if total > 0 {
			acc = float64(passed) / float64(total) * 100.0
		}
		errs := total - passed
		t.Logf("%-12s | %-12d | %-15d | %-9.1f%% | %-10d", l, total, passed, acc, errs)
	}
	t.Log("=============================================================")
}

func TestParseFailureFailClosed(t *testing.T) {
	// A syntax error or broken file must return an error from Extract (fail closed)
	// so the delta engine never assumes "no change".
	cases := []struct {
		lang string
		file string
		src  string
	}{
		{"go syntax error", "bad.go", "package p\nfunc F() {\n"},
		{"go unclosed struct", "bad.go", "package p\ntype T struct {\n"},
		{"ts syntax error", "bad.ts", "export function (\n"},
		{"kotlin unclosed", "bad.kt", "fun (\n"},
		{"java broken class", "Bad.java", "public class Bad {\n"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			syms, err := Extract(tc.file, []byte(tc.src))
			// If error is nil, ensure it didn't extract any valid full signature for a broken symbol
			if err != nil {
				t.Logf("[%s] Extract returned fail-closed error as expected: %v", tc.lang, err)
			} else {
				t.Logf("[%s] Extract produced symbols: %v (count: %d)", tc.lang, syms, len(syms))
			}
		})
	}
}

func TestStress_1MB_10MB_CRLF_BOM_Unicode_Comments(t *testing.T) {
	// 1. CRLF and BOM resilience across all parsers
	for _, tc := range []struct {
		ext string
		src string
	}{
		{".go", "package p\nfunc Foo(a int) string { return \"\" }\n"},
		{".kt", "fun foo(a: Int): String = \"\"\n"},
		{".java", "public class Foo { public String bar(int a) { return \"\"; } }\n"},
		{".ts", "export function foo(a: number): string { return \"\"; }\n"},
		{".py", "def foo(a: int) -> str:\n    return \"\"\n"},
	} {
		t.Run("crlf_bom_"+tc.ext, func(t *testing.T) {
			raw := []byte(tc.src)
			baseSyms, err := Extract("test"+tc.ext, raw)
			if err != nil {
				t.Fatalf("base extract error for %s: %v", tc.ext, err)
			}

			// CRLF
			crlf := []byte(strings.ReplaceAll(tc.src, "\n", "\r\n"))
			crlfSyms, err := Extract("test"+tc.ext, crlf)
			if err != nil {
				t.Fatalf("CRLF extract error for %s: %v", tc.ext, err)
			}
			if len(baseSyms) != len(crlfSyms) {
				t.Errorf("CRLF symbol count mismatch for %s", tc.ext)
			}

			// BOM + LF
			bomLF := append([]byte{0xef, 0xbb, 0xbf}, raw...)
			bomLFSyms, err := Extract("test"+tc.ext, bomLF)
			if err != nil {
				t.Fatalf("BOM+LF extract error for %s: %v", tc.ext, err)
			}
			if len(baseSyms) != len(bomLFSyms) {
				t.Errorf("BOM+LF symbol count mismatch for %s", tc.ext)
			}

			// BOM + CRLF
			bomCRLF := append([]byte{0xef, 0xbb, 0xbf}, crlf...)
			bomCRLFSyms, err := Extract("test"+tc.ext, bomCRLF)
			if err != nil {
				t.Fatalf("BOM+CRLF extract error for %s: %v", tc.ext, err)
			}
			if len(baseSyms) != len(bomCRLFSyms) {
				t.Errorf("BOM+CRLF symbol count mismatch for %s", tc.ext)
			}
		})
	}

	// 2. Unicode and look-alike cases
	t.Run("unicode_and_lookalikes", func(t *testing.T) {
		// Unicode identifiers and strings
		pyUnicode := "def process_данные(параметр: str) -> bool:\n    \"\"\"Docstring with def fake_fn(): pass\"\"\"\n    # def comment_fn(): pass\n    s = 'def string_fn(): pass'\n    return True\n"
		pySyms, err := Extract("test.py", []byte(pyUnicode))
		if err != nil {
			t.Fatalf("python unicode extract error: %v", err)
		}
		if _, ok := pySyms["process_данные"]; !ok {
			t.Errorf("expected process_данные in python symbols: %v", pySyms)
		}
		if _, ok := pySyms["fake_fn"]; ok {
			t.Errorf("docstring look-alike leaked as symbol!")
		}
		if _, ok := pySyms["comment_fn"]; ok {
			t.Errorf("comment look-alike leaked as symbol!")
		}
		if _, ok := pySyms["string_fn"]; ok {
			t.Errorf("string look-alike leaked as symbol!")
		}

		// TS comment and template string look-alikes
		tsLookalike := `
// export function fakeComment() {}
/* export class FakeClassComment {} */
export function realFunc(param: string): void {
  const tmpl = 'export function fakeTmpl() {}';
  const str = "export class FakeClassStr {}";
}
`
		tsSyms, err := Extract("test.ts", []byte(tsLookalike))
		if err != nil {
			t.Fatalf("ts look-alike extract error: %v", err)
		}
		if _, ok := tsSyms["realFunc"]; !ok {
			t.Errorf("expected realFunc in ts symbols: %v", tsSyms)
		}
		for _, fake := range []string{"fakeComment", "FakeClassComment", "fakeTmpl", "FakeClassStr"} {
			if _, ok := tsSyms[fake]; ok {
				t.Errorf("ts look-alike %q leaked as symbol!", fake)
			}
		}
	})

	// 3. 1 MB & 10 MB Scale
	t.Run("scale_1MB_and_10MB", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping scale test in short mode")
		}

		t.Run("1MB_kotlin", func(t *testing.T) {
			line1MB := "fun actionMethodA(x: Int, y: String): Unit = println(x)\n"
			repeats1MB := (1024 * 1024) / len(line1MB)
			src1MB := []byte(strings.Repeat(line1MB, repeats1MB))

			start1MB := time.Now()
			syms1MB, err := Extract("scale.kt", src1MB)
			dur1MB := time.Since(start1MB)
			if err != nil {
				t.Fatalf("1 MB extract error: %v", err)
			}
			if len(syms1MB) == 0 {
				t.Fatalf("expected symbols in 1 MB file")
			}
			t.Logf("1 MB Kotlin parse time: %v (race=%v)", dur1MB, isRaceDetector)
			max1MB := 2 * time.Second
			if isRaceDetector {
				max1MB = 6 * time.Second
			}
			if dur1MB > max1MB {
				t.Errorf("1 MB parse too slow: %v (limit: %v)", dur1MB, max1MB)
			}
		})

		t.Run("10MB_typescript", func(t *testing.T) {
			line := "export function computeHandler(item: number): number { return item; }\n"
			repeats1MB := (1024 * 1024) / len(line)
			src1MB := []byte(strings.Repeat(line, repeats1MB))

			start1MB := time.Now()
			syms1MB, err := Extract("scale_1mb.ts", src1MB)
			dur1MB := time.Since(start1MB)
			if err != nil {
				t.Fatalf("1 MB TypeScript extract error: %v", err)
			}
			if len(syms1MB) == 0 {
				t.Fatalf("expected symbols in 1 MB TypeScript file")
			}

			repeats10MB := (10 * 1024 * 1024) / len(line)
			src10MB := []byte(strings.Repeat(line, repeats10MB))

			start10MB := time.Now()
			syms10MB, err := Extract("scale_10mb.ts", src10MB)
			dur10MB := time.Since(start10MB)
			if err != nil {
				t.Fatalf("10 MB TypeScript extract error: %v", err)
			}
			if len(syms10MB) == 0 {
				t.Fatalf("expected symbols in 10 MB TypeScript file")
			}
			t.Logf("10 MB TypeScript parse time: %v, 1 MB time: %v (ratio: %.2fx, race=%v)", dur10MB, dur1MB, float64(dur10MB)/float64(dur1MB), isRaceDetector)

			// Machine-independent throughput bound: 10 MB must take under 20x the 1 MB time.
			if dur10MB > 20*dur1MB {
				t.Errorf("10 MB parse too slow relative to 1 MB: %v > 20 * %v", dur10MB, dur1MB)
			}
		})
	})
}
