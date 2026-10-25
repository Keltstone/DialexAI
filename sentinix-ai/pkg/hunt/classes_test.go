package hunt

import (
	"crypto/ed25519"
	"testing"

	"sentinix/pkg/engines"
	"sentinix/pkg/packs"
)

func TestBuiltinClassesLibrary(t *testing.T) {
	lib, err := NewClassLibrary(nil, nil)
	if err != nil {
		t.Fatalf("failed to create class library: %v", err)
	}

	// Must have at least 15 built-in classes
	if len(BuiltinClasses) < 15 {
		t.Errorf("expected at least 15 built-in classes, got %d", len(BuiltinClasses))
	}

	expectedClasses := []string{
		"confused-deputy", "intent-redirect", "path-traversal", "deeplink-host",
		"webview-bridge", "tapjacking", "pending-intent-hijack", "sql-injection",
		"injection", "authz", "command-injection", "authz-bypass", "ssrf",
		"insecure-deserialization", "xss-reflected", "idor", "hardcoded-secret",
		"xpc-authz", "graphql-depth",
	}

	for _, id := range expectedClasses {
		c, ok := lib.GetClass(id)
		if !ok {
			t.Errorf("expected class %q in library", id)
			continue
		}
		if c.CWE == "" {
			t.Errorf("class %q missing CWE", id)
		}
	}
}

func TestCustomClassOverridesAndPacks(t *testing.T) {
	custom := []ClassConfig{
		{
			ID:     "custom-auth",
			CWE:    "CWE-287",
			MaxSev: "critical",
			Hint:   "Verify JWT expiry and audience claim",
			Kinds:  []string{"route", "grpc"},
		},
		{
			ID:       "ssrf",
			Disabled: true,
		},
	}

	pub, priv, _ := ed25519.GenerateKey(nil)
	body := `{"name":"pack1","version":"1","classes":{"component":[{"id":"pack-class","cwe":"CWE-200","hint":"check leak"}]}}`
	signed, err := packs.Sign([]byte(body), priv)
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadPack(signed, []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatal(err)
	}

	lib, err := NewClassLibrary(custom, []*Pack{p})
	if err != nil {
		t.Fatalf("failed to create class library with custom: %v", err)
	}

	// Verify custom class
	cc, ok := lib.GetClass("custom-auth")
	if !ok || cc.CWE != "CWE-287" || cc.Severity != engines.Critical {
		t.Errorf("custom-auth not properly registered: %+v", cc)
	}

	// Verify disabled class
	ssrf, ok := lib.GetClass("ssrf")
	if !ok || !ssrf.Disabled {
		t.Errorf("expected ssrf to be disabled: %+v", ssrf)
	}

	// Verify pack class
	pc, ok := lib.GetClass("pack-class")
	if !ok || pc.CWE != "CWE-200" || pc.Hint != "check leak" {
		t.Errorf("pack class not properly registered: %+v", pc)
	}

	// Verify ClassesFor excludes disabled and includes custom & pack
	epRoute := Entry{Kinds: []string{"route"}}
	classesRoute := lib.ClassesFor(epRoute, "", []*Pack{p})
	for _, c := range classesRoute {
		if c == "ssrf" {
			t.Errorf("disabled class ssrf returned in ClassesFor: %v", classesRoute)
		}
	}
	foundCustom := false
	for _, c := range classesRoute {
		if c == "custom-auth" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Errorf("custom-auth not included in ClassesFor route: %v", classesRoute)
	}

	epComp := Entry{Kinds: []string{"component"}}
	classesComp := lib.ClassesFor(epComp, "mobile", []*Pack{p})
	if len(classesComp) == 0 || classesComp[len(classesComp)-1] != "pack-class" {
		t.Errorf("expected pack-class at the end of ClassesFor component, got: %v", classesComp)
	}
}

func TestCustomClassInvalidID(t *testing.T) {
	badCustom := []ClassConfig{
		{ID: "INVALID_ID!"},
	}
	if _, err := NewClassLibrary(badCustom, nil); err == nil {
		t.Error("expected error for invalid class ID")
	}
}
