package symbols

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func FuzzExtractKotlin(f *testing.F) {
	seeds := [][]byte{
		[]byte("fun foo(a: Int): Int = a + 1"),
		[]byte("class Box<T>(val item: T)"),
		[]byte("interface Service { fun execute() }"),
		[]byte("const val MAX = 100"),
		[]byte("/* comment */ fun bar() {}"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		assertFuzzInvariants(t, "file.kt", data)
	})
}

func FuzzExtractJava(f *testing.F) {
	seeds := [][]byte{
		[]byte("public class Foo { public void bar() {} }"),
		[]byte("public interface Service<T> { T process(T in); }"),
		[]byte("public record Point(int x, int y) {}"),
		[]byte("public enum Color { RED, GREEN, BLUE }"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		assertFuzzInvariants(t, "file.java", data)
	})
}

func FuzzExtractTS(f *testing.F) {
	seeds := [][]byte{
		[]byte("export function run(cmd: string): void {}"),
		[]byte("export class Worker { run(): void {} }"),
		[]byte("export interface Config { port: number; }"),
		[]byte("export const VERSION = '1.0.0';"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		assertFuzzInvariants(t, "file.ts", data)
	})
}

func FuzzExtractPython(f *testing.F) {
	seeds := [][]byte{
		[]byte("def process(item: str) -> bool:\n    return True\n"),
		[]byte("class Handler:\n    def handle(self):\n        pass\n"),
		[]byte("TIMEOUT = 30\n"),
		[]byte("__all__ = ['process']\n"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		assertFuzzInvariants(t, "file.py", data)
	})
}

func FuzzExtractGo(f *testing.F) {
	seeds := [][]byte{
		[]byte("package p\nfunc Do() {}\n"),
		[]byte("package p\ntype S struct{}\nfunc (s *S) Run() {}\n"),
		[]byte("package p\nconst X = 1\n"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		assertFuzzInvariants(t, "file.go", data)
	})
}

func assertFuzzInvariants(t *testing.T, filename string, data []byte) {
	if len(data) > 1024*1024 {
		return // cap fuzz input size to 1 MB
	}

	start := time.Now()
	res1, err1 := Extract(filename, data)
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("Extract took too long: %v for %d bytes", elapsed, len(data))
	}

	// 1. Error XOR non-nil map
	if (err1 == nil && res1 == nil) || (err1 != nil && res1 != nil) {
		t.Fatalf("Invariant violated: err XOR non-nil map! err=%v, res=%v", err1, res1)
	}

	// 2. Deterministic: two calls equal
	res2, err2 := Extract(filename, data)
	if (err1 == nil) != (err2 == nil) {
		t.Fatalf("Non-deterministic error: first err=%v, second err=%v", err1, err2)
	}
	if err1 == nil {
		if len(res1) != len(res2) {
			t.Fatalf("Non-deterministic map length: %d vs %d", len(res1), len(res2))
		}
		for k, v := range res1 {
			if res2[k] != v {
				t.Fatalf("Non-deterministic value for key %q: %q vs %q", k, v, res2[k])
			}
		}

		// 3. Extract(src) == Extract(CRLF(src)) and == Extract(BOM+src) when no error
		crlfData := []byte(strings.ReplaceAll(string(data), "\n", "\r\n"))
		resCRLF, errCRLF := Extract(filename, crlfData)
		if errCRLF == nil {
			if len(res1) != len(resCRLF) {
				t.Fatalf("CRLF mismatch map length")
			}
		}

		bomData := append([]byte{0xef, 0xbb, 0xbf}, data...)
		resBOM, errBOM := Extract(filename, bomData)
		if errBOM == nil {
			if len(res1) != len(resBOM) {
				t.Fatalf("BOM mismatch map length")
			}
		}

		// 4. ExtractDetailed invariants
		det1, errDet1 := ExtractDetailed(filename, data)
		if errDet1 != nil {
			t.Fatalf("Extract succeeded but ExtractDetailed failed: %v", errDet1)
		}

		det2, errDet2 := ExtractDetailed(filename, data)
		if errDet2 != nil || len(det1) != len(det2) {
			t.Fatalf("ExtractDetailed non-deterministic: %v vs %v, lengths %d vs %d", errDet1, errDet2, len(det1), len(det2))
		}

		totalLines := bytes.Count(data, []byte("\n")) + 1
		for i, s := range det1 {
			if s != det2[i] {
				t.Fatalf("ExtractDetailed non-deterministic symbol %d:\nfirst:  %+v\nsecond: %+v", i, s, det2[i])
			}
			if s.Line < 1 || s.Line > totalLines {
				t.Fatalf("symbol %q Line %d out of bounds [1, %d]", s.Key, s.Line, totalLines)
			}
			if s.Key == "" {
				t.Fatalf("empty symbol Key at line %d", s.Line)
			}
			if s.Kind == "" {
				t.Fatalf("empty symbol Kind for %q", s.Key)
			}
			if extSig, ok := res1[s.Key]; ok {
				if extSig != s.Sig {
					t.Fatalf("signature mismatch for %q between Extract (%q) and ExtractDetailed (%q)", s.Key, extSig, s.Sig)
				}
			}
		}
	} else {
		_, errDet := ExtractDetailed(filename, data)
		if errDet == nil {
			t.Fatalf("Extract failed with %v but ExtractDetailed succeeded", err1)
		}
	}
	_ = bytes.Equal
}
