package doctypes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func writeFile(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".scribex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".scribex", FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinsPresentAndOnlyTechReferenceEnabled(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Types) != 8 {
		t.Fatalf("want 8 built-ins, got %d", len(c.Types))
	}
	var on []string
	for _, ty := range c.Enabled() {
		on = append(on, ty.ID)
	}
	if len(on) != 1 || on[0] != "tech-reference" {
		t.Fatalf("enabled = %v", on)
	}
	if probs := Validate(t.TempDir(), c); len(probs) != 0 {
		t.Fatalf("built-ins must validate on an empty repo: %v", probs)
	}
}

func TestOverrideMergesFieldByField(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, `{"discover":"auto","types":[{"id":"feature-design","enabled":true,"max_calls":5,"language":"fr"}]}`)
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	ty, _ := c.Get("feature-design")
	if !ty.Enabled || ty.MaxCalls != 5 || ty.Language != "fr" {
		t.Fatalf("override not applied: %+v", ty)
	}
	if ty.Output != "docs/features/{feature}/design.md" || ty.Grounding != "derived" {
		t.Fatalf("untouched fields changed: %+v", ty)
	}
	if probs := Validate(root, c); len(probs) != 0 {
		t.Fatalf("%v", probs)
	}
}

func TestCannotDeleteBuiltin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, `{"types":[]}`)
	c, _ := Load(root)
	if len(c.Types) != 8 {
		t.Fatalf("built-ins removed: %d", len(c.Types))
	}
}

func TestCustomTypeRequiresFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, `{"types":[{"id":"my-doc"}]}`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "required for a custom type") {
		t.Fatalf("incomplete custom type accepted: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	custom := func(mut func(*Entry)) File {
		e := Entry{ID: "my-doc", Title: ptr("My doc"), Audience: ptr("dev"), Granularity: ptr("repo"),
			Output: ptr("docs/my.md"), Grounding: ptr("symbol"), Enabled: ptr(true)}
		mut(&e)
		return File{Types: []Entry{e}}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.md"), []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unsupported")
	}
	cases := []struct {
		name, want string
		f          File
	}{
		{"bad id", "id must match", custom(func(e *Entry) { e.ID = "Bad_ID" })},
		{"bad audience", "audience", custom(func(e *Entry) { e.Audience = ptr("everyone") })},
		{"bad grounding", "grounding", custom(func(e *Entry) { e.Grounding = ptr("magic") })},
		{"bad granularity", "granularity", custom(func(e *Entry) { e.Granularity = ptr("galaxy") })},
		{"bad policy", "id_policy", custom(func(e *Entry) { e.IDPolicy = ptr("mint") })},
		{"dotdot output", "escapes", custom(func(e *Entry) { e.Output = ptr("../x.md") })},
		{"abs output", "absolute", custom(func(e *Entry) { e.Output = ptr("/etc/x.md") })},
		{"unknown placeholder", "unknown placeholder", custom(func(e *Entry) { e.Output = ptr("docs/{bogus}.md") })},
		{"stray brace", "stray braces", custom(func(e *Entry) { e.Output = ptr("docs/{x.md") })},
		{"non md", ".md", custom(func(e *Entry) { e.Output = ptr("docs/x.txt") })},
		{"backslash", "forward slashes", custom(func(e *Entry) { e.Output = ptr(`docs\x.md`) })},
		{"granularity needs placeholder", "needs {feature}", custom(func(e *Entry) {
			e.Granularity = ptr("feature")
			e.Grounding = ptr("derived")
			e.Template = ptr("real.md")
		})},
		{"symbol uses feature", "cannot use {feature}", custom(func(e *Entry) { e.Output = ptr("docs/{feature}.md") })},
		{"template missing", "does not exist", custom(func(e *Entry) { e.Grounding = ptr("derived"); e.Template = ptr("nope.md") })},
		{"template escapes via symlink", "outside root", custom(func(e *Entry) { e.Grounding = ptr("derived"); e.Template = ptr("link/t.md") })},
		{"template traversal", "escapes", custom(func(e *Entry) { e.Grounding = ptr("derived"); e.Template = ptr("../t.md") })},
		{"derived no template", "template is required", custom(func(e *Entry) { e.Grounding = ptr("derived") })},
		{"input missing", "does not exist", custom(func(e *Entry) { e.Inputs = &[]string{"brief.md"} })},
		{"human with calls", "never calls a model", custom(func(e *Entry) { e.Grounding = ptr("human"); e.Template = ptr("real.md"); e.MaxCalls = ptr(3) })},
		{"human with review", "never calls a model", custom(func(e *Entry) { e.Grounding = ptr("human"); e.Template = ptr("real.md"); e.Review = ptr("socratix") })},
		{"bad review", "review", custom(func(e *Entry) { e.Review = ptr("magic") })},
		{"bad glob", "glob", custom(func(e *Entry) { e.Include = &[]string{"[bad"} })},
		{"glob traversal", "..", custom(func(e *Entry) { e.Exclude = &[]string{"../**"} })},
		{"bad id_pattern", "id_pattern", custom(func(e *Entry) { e.IDPattern = ptr("(") })},
		{"negative calls", "max_calls", custom(func(e *Entry) { e.MaxCalls = ptr(-1) })},
		{"feature without features", "features map", File{Types: []Entry{{ID: "feature-design", Enabled: ptr(true)}}}},
		{"bad discover", "discover", File{Discover: "yes"}},
		{"bad feature glob", "feature", File{Features: map[string][]string{"login": {"[bad"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, probs := Build(tc.f)
			probs = append(probs, Validate(root, c)...)
			if len(probs) == 0 || !strings.Contains(strings.Join(probs, "\n"), tc.want) {
				t.Errorf("want problem containing %q, got %v", tc.want, probs)
			}
		})
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	f := File{Types: []Entry{{ID: "a-doc", Title: ptr(""), Audience: ptr("x"), Granularity: ptr("y"), Output: ptr("../z"), Grounding: ptr("w")}}}
	c, probs := Build(f)
	probs = append(probs, Validate(t.TempDir(), c)...)
	if len(probs) < 5 {
		t.Fatalf("expected many problems, got %v", probs)
	}
}

func TestUnknownFieldsAndDuplicatesRejected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, `{"types":[{"id":"requirements","bogus":1}]}`)
	if _, err := Load(root); err == nil {
		t.Fatal("unknown field accepted")
	}
	writeFile(t, root, `{"types":[{"id":"requirements"},{"id":"requirements"}]}`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("dup accepted: %v", err)
	}
	writeFile(t, root, `{"types":[]} {"x":1}`)
	if _, err := Load(root); err == nil {
		t.Fatal("trailing data accepted")
	}
}

func TestResolveOutput(t *testing.T) {
	c, _ := Load(t.TempDir())
	td, _ := c.Get("tech-reference")
	fd, _ := c.Get("feature-design")
	for _, tc := range []struct {
		ty   Type
		vals map[string]string
		want string
	}{
		{td, map[string]string{"file": "pkg/auth/jwt.go"}, "docs/reference/pkg/auth/jwt.go.md"},
		{td, map[string]string{"file": `pkg\auth\jwt.go`}, "docs/reference/pkg/auth/jwt.go.md"},
		{td, map[string]string{"file": "../../etc/passwd"}, "docs/reference/etc/passwd.md"},
		{td, map[string]string{"file": "/abs/x.kt"}, "docs/reference/abs/x.kt.md"},
		{td, map[string]string{"file": ""}, "docs/reference/unnamed.md"},
		{fd, map[string]string{"feature": "Login & Sign-up"}, "docs/features/Login-Sign-up/design.md"},
		{fd, map[string]string{"feature": ".."}, "docs/features/unnamed/design.md"},
	} {
		got, err := Resolve(tc.ty, tc.vals)
		if err != nil || got != tc.want {
			t.Errorf("Resolve(%v) = %q, %v; want %q", tc.vals, got, err, tc.want)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	for _, tc := range []struct {
		pat, rel string
		want     bool
	}{
		{"**", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"**/*.go", "a/b/c.go", true},
		{"domain/**", "domain/usecase/X.kt", true},
		{"domain/**", "data/X.kt", false},
		{"*.go", "a/b.go", false},
		{"a/*/c", "a/b/c", true},
		{"a/*/c", "a/b/d/c", false},
		{"a/**/c", "a/c", true},
		{"a/**/c", "a/x/y/c", true},
		{"**/test/**", "a/test/b.kt", true},
	} {
		if got := Match(tc.pat, tc.rel); got != tc.want {
			t.Errorf("Match(%q,%q)=%v want %v", tc.pat, tc.rel, got, tc.want)
		}
	}
	if CheckGlob("a**b") == nil || CheckGlob("") == nil || CheckGlob("/abs") == nil {
		t.Error("bad globs accepted")
	}
}

func TestSelected(t *testing.T) {
	ty := Type{Include: []string{"src/**"}, Exclude: []string{"**/generated/**"}}
	if !ty.Selected("src/a/b.kt") || ty.Selected("src/generated/x.kt") || ty.Selected("other/x.kt") {
		t.Fatal("Selected wrong")
	}
	if !(Type{}).Selected("anything/x") {
		t.Fatal("empty include means everything")
	}
}

func TestSaveRoundTripAndOnlyOverrides(t *testing.T) {
	root := t.TempDir()
	f := File{Discover: "auto", Types: []Entry{{ID: "requirements", Enabled: ptr(true)}}}
	if err := Save(root, f); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(Path(root))
	if strings.Contains(string(b), "tech-reference") || strings.Contains(string(b), "docs/reference") {
		t.Fatalf("built-ins persisted: %s", b)
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if ty, _ := c.Get("requirements"); !ty.Enabled {
		t.Fatal("override lost")
	}
	if err := Save(root, f); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(Path(root))
	if string(b) != string(b2) {
		t.Fatal("save not stable")
	}
	bad := File{Types: []Entry{{ID: "requirements", Output: ptr("../x.md")}}}
	if err := Save(root, bad); err == nil {
		t.Fatal("invalid catalog saved")
	}
	if after, _ := os.ReadFile(Path(root)); string(after) != string(b) {
		t.Fatal("failed save modified the file")
	}
}

func TestLoadRefusesSymlinkedAndOversizedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".scribex"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(target, []byte(`{}`), 0o644)
	if err := os.Symlink(target, filepath.Join(root, ".scribex", FileName)); err != nil {
		t.Skip("symlinks unsupported")
	}
	if _, err := Load(root); err == nil {
		t.Fatal("symlinked doctypes.json accepted")
	}
	os.Remove(filepath.Join(root, ".scribex", FileName))
	writeFile(t, root, strings.Repeat(" ", maxFile+1))
	if _, err := Load(root); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestBuiltinSkeletons(t *testing.T) {
	for _, ty := range builtins() {
		if ty.Grounding == "derived" && len(BuiltinSkeleton(ty.ID)) == 0 {
			t.Errorf("%s: derived built-in without skeleton", ty.ID)
		}
	}
}
