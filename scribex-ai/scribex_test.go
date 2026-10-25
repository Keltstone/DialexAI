package main

import (
	"strings"
	"testing"

	"scribex/pkg/anchors"
	"scribex/pkg/symbols"
)

func TestSymbolsAndAnchors(t *testing.T) {
	g, err := symbols.Extract("a.go", []byte("package a\nfunc Verify(t string) error { return nil }\nfunc hidden() {}\ntype S struct{ X int }\nfunc (s *S) Do() {}\n"))
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	for _, k := range []string{"Verify", "S", "S.Do"} {
		if g[k] == "" {
			t.Fatalf("missing %s in %v", k, g)
		}
	}
	if g["hidden"] != "" {
		t.Fatal("unexported leaked")
	}
	pySyms, err := symbols.Extract("a.py", []byte("def pub(x): pass\ndef _priv(): pass\n"))
	if err != nil {
		t.Fatal(err)
	}
	if pySyms["pub"] == "" {
		t.Fatal("py")
	}
	tsSyms, err := symbols.Extract("a.ts", []byte("export function f(a: number): void {\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if tsSyms["f"] == "" {
		t.Fatal("ts")
	}
	ktSyms, err := symbols.Extract("a.kt", []byte("private fun x() {}\nfun y(a: Int): Int = a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if ktSyms["x"] != "" {
		t.Fatal("kt private")
	}

	look := func(f, s string) (string, bool, error) { v, ok := g[s]; return v, ok, nil }
	md := "<!-- @scribex:anchor(a.go#Verify) -->\nprose\n<!-- @scribex:end -->\n<!-- @scribex:anchor(a.go#Gone) -->\n<!-- @scribex:end -->"
	issues, out := anchors.Process(md, look, true, nil)
	if len(issues) != 2 || !strings.Contains(out, "sig=") || !strings.Contains(out, "func Verify") {
		t.Fatalf("%v\n%s", issues, out)
	}
	issues, out2 := anchors.Process(out, look, true, nil) // idempotent; only dead anchor remains
	if len(issues) != 1 || out2 != out {
		t.Fatalf("not idempotent: %v\n%s", issues, out2)
	}
}
