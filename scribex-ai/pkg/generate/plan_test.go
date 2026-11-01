package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"scribex/pkg/config"
	"scribex/pkg/doctypes"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"}, {"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		c := exec.Command("git", append([]string{"-C", root}, a...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
	}
}

const goSrc = "package svc\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n\n// Name is a thing.\nfunc Name() string { return \"x\" }\n\nfunc private() {}\n"

func fixture(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.22\n")
	write(t, root, "svc/svc.go", goSrc)
	write(t, root, "svc/svc_test.go", "package svc\n")
	write(t, root, "vendor/v/v.go", "package v\nfunc V() {}\n")
	gitInit(t, root)
	return root
}

func opts(root string) Options {
	return Options{Root: root, AI: config.AI{Provider: "cli", CLICommand: "claude", MaxCalls: 20}}
}

func cat(t *testing.T, root string) doctypes.Catalog {
	c, err := doctypes.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPlanDeterministic(t *testing.T) {
	root := fixture(t)
	a, err := MakePlan(root, cat(t, root), opts(root))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := MakePlan(root, cat(t, root), opts(root))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("plans differ")
	}
}

func TestPlanSymbolCreateUpdateSkip(t *testing.T) {
	root := fixture(t)
	p, _ := MakePlan(root, cat(t, root), opts(root))
	if len(p.Items) != 1 || p.Items[0].Action != "create" || p.Items[0].Output != "docs/reference/svc/svc.go.md" {
		t.Fatalf("%+v", p.Items)
	}
	if got := p.Items[0].Syms; len(got) != 2 {
		t.Fatalf("syms %v (private and tests must be excluded)", got)
	}
	if p.Items[0].Calls != 1 || p.Items[0].Parser["ast"] != 1 {
		t.Fatalf("%+v", p.Items[0])
	}
	if strings.Join(p.WouldSend, ",") != "svc/svc.go" {
		t.Fatalf("would send %v (vendor and tests must not leave)", p.WouldSend)
	}
}

func TestPlanNeverTouchesHandWritten(t *testing.T) {
	root := fixture(t)
	// anchored by hand, no @scribex:ai marker -> never planned for rewrite
	write(t, root, "docs/reference/svc/svc.go.md", "# x\n<!-- @scribex:anchor(svc/svc.go#Add) -->\nhand prose\n<!-- @scribex:end -->\n")
	p, _ := MakePlan(root, cat(t, root), opts(root))
	for _, s := range p.Items[0].Syms {
		if s == "svc/svc.go#Add" {
			t.Fatal("hand-written anchored symbol planned")
		}
	}
	if p.Items[0].Action != "update" {
		t.Fatalf("%+v", p.Items[0])
	}
}

func TestPlanWritesOnlyRunsDir(t *testing.T) {
	root := fixture(t)
	p, _ := MakePlan(root, cat(t, root), opts(root))
	before, _ := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err := Save(root, p); err != nil {
		t.Fatal(err)
	}
	after, _ := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if string(before) != string(after) {
		t.Fatalf("tree dirtied: %q", after)
	}
	if _, err := LoadPlan(root, p.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPlan(root, "../x"); err == nil {
		t.Fatal("traversal id accepted")
	}
}

func TestPlanDerivedSourceHash(t *testing.T) {
	root := fixture(t)
	write(t, root, ".scribex/doctypes.json", `{"types":[{"id":"runbook-ops","enabled":true}]}`)
	o := opts(root)
	o.Types = []string{"runbook-ops"}
	p, err := MakePlan(root, cat(t, root), o)
	if err != nil || len(p.Items) != 1 || p.Items[0].Calls != 1 || len(p.Items[0].Sources) == 0 {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestPlanLocalFlag(t *testing.T) {
	root := fixture(t)
	o := opts(root)
	o.AI = config.AI{Provider: "ollama", Model: "llama3"}
	p, _ := MakePlan(root, cat(t, root), o)
	if !p.Local {
		t.Fatal("ollama must be local")
	}
	if q, _ := MakePlan(root, cat(t, root), opts(root)); q.Local {
		t.Fatal("cli must not be local")
	}
}

func TestPlanIncludeExclude(t *testing.T) {
	root := fixture(t)
	o := opts(root)
	o.Exclude = []string{"svc/**"}
	p, _ := MakePlan(root, cat(t, root), o)
	if len(p.Items) != 0 {
		t.Fatalf("%+v", p.Items)
	}
}

func TestPlanTestRequirementsReadsTests(t *testing.T) {
	root := fixture(t)
	write(t, root, ".scribex/doctypes.json", `{"discover":"auto","types":[{"id":"test-requirements","enabled":true}]}`)
	write(t, root, "svc/other_test.go", "package svc\n")
	gitCommit(t, root)
	o := opts(root)
	o.Types = []string{"test-requirements"}
	p, err := MakePlan(root, cat(t, root), o)
	if err != nil || len(p.Items) != 1 {
		t.Fatalf("%v %+v", err, p)
	}
	got := map[string]bool{}
	for _, s := range p.Items[0].Sources {
		got[s.Path] = true
	}
	if !got["svc/svc_test.go"] || !got["svc/svc.go"] {
		t.Fatalf("sources %v", p.Items[0].Sources)
	}
	// tests are never documented as symbols
	q, _ := MakePlan(root, cat(t, root), opts(root))
	for _, it := range q.Items {
		for _, f := range it.Files {
			if strings.HasSuffix(f, "_test.go") {
				t.Fatal("test file treated as source")
			}
		}
	}
}

func TestPlanDiscoverAutoAndExplicitFeatures(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module test\n")
	write(t, root, "src/features/auth/Auth.kt", "package auth\nclass LoginService {\n    fun login(): Boolean = true\n}\n")
	write(t, root, "src/presentation/profile/profile.ts", "export function getProfile(): string { return 'profile'; }\n")
	write(t, root, "pkg/billing/billing.py", "def process_payment(amount: float):\n    pass\n")
	gitInit(t, root)

	// Test 1: discover: auto
	write(t, root, ".scribex/doctypes.json", `{"discover":"auto","types":[{"id":"feature-design","enabled":true}]}`)
	gitCommit(t, root)
	o := opts(root)
	o.Types = []string{"feature-design"}
	p, err := MakePlan(root, cat(t, root), o)
	if err != nil {
		t.Fatal(err)
	}
	units := map[string]bool{}
	for _, it := range p.Items {
		units[it.Unit] = true
	}
	if !units["auth"] || !units["profile"] {
		t.Fatalf("expected auto features auth and profile, got %v", units)
	}

	// Test 2: explicit features map
	write(t, root, ".scribex/doctypes.json", `{"features":{"payments":["pkg/billing/**"],"accounts":["src/**"]},"types":[{"id":"feature-design","enabled":true}]}`)
	gitCommit(t, root)
	p2, err := MakePlan(root, cat(t, root), o)
	if err != nil {
		t.Fatal(err)
	}
	units2 := map[string]bool{}
	for _, it := range p2.Items {
		units2[it.Unit] = true
	}
	if !units2["payments"] || !units2["accounts"] {
		t.Fatalf("expected explicit features payments and accounts, got %v", units2)
	}
}

func TestPlanMishandledSyntaxDoesNotPanic(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module test\n")
	// Malformed files in Python, TS, Kotlin with unmatched delimiters and syntax errors
	write(t, root, "bad.py", "def (((( broken syntax\n   class 123 ::\n")
	write(t, root, "bad.ts", "export class { broken {{{ ]]]\n")
	write(t, root, "bad.kt", "class { fun (((( broken\n")
	gitInit(t, root)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MakePlan panicked on malformed syntax: %v", r)
		}
	}()

	p, err := MakePlan(root, cat(t, root), opts(root))
	if err != nil {
		t.Fatal(err)
	}
	// Plan succeeded without panic; mishandled files either yield 0 symbols or go to Skips
	if p == nil {
		t.Fatal("expected non-nil plan")
	}
}
