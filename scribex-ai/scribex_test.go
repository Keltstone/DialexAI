package main

import (
	"strings"
	"testing"

	"scribex/pkg/anchors"
	"scribex/pkg/symbols"
)

func TestSymbolsAndAnchors(t *testing.T) {
	g := symbols.Extract("a.go", []byte("package a\nfunc Verify(t string) error { return nil }\nfunc hidden() {}\ntype S struct{ X int }\nfunc (s *S) Do() {}\n"))
	for _, k := range []string{"Verify", "S", "S.Do"} {
		if g[k] == "" {
			t.Fatalf("missing %s in %v", k, g)
		}
	}
	if g["hidden"] != "" {
		t.Fatal("unexported leaked")
	}
	if symbols.Extract("a.py", []byte("def pub(x): pass\ndef _priv(): pass\n"))["pub"] == "" {
		t.Fatal("py")
	}
	if symbols.Extract("a.ts", []byte("export function f(a: number): void {\n"))["f"] == "" {
		t.Fatal("ts")
	}
	if symbols.Extract("a.kt", []byte("private fun x() {}\nfun y(a: Int): Int = a\n"))["x"] != "" {
		t.Fatal("kt private")
	}

	look := func(f, s string) (string, bool) { v, ok := g[s]; return v, ok }
	md := "<!-- @scribex:anchor(a.go#Verify) -->\nprose\n<!-- @scribex:end -->\n<!-- @scribex:anchor(a.go#Gone) -->\n<!-- @scribex:end -->"
	issues, out := anchors.Process(md, look, true)
	if len(issues) != 2 || !strings.Contains(out, "sig=") || !strings.Contains(out, "func Verify") {
		t.Fatalf("%v\n%s", issues, out)
	}
	issues, out2 := anchors.Process(out, look, true) // idempotent; only dead anchor remains
	if len(issues) != 1 || out2 != out {
		t.Fatalf("not idempotent: %v\n%s", issues, out2)
	}
}
