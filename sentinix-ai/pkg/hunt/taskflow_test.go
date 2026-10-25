package hunt

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	"sentinix/pkg/ai"
)

func TestBuiltinTaskflowsLoad(t *testing.T) {
	builtins := ListBuiltinTaskflows()
	expected := []string{"android-audit", "backend-audit", "ios-audit", "variant-analysis", "triage-codeql"}

	for _, exp := range expected {
		tf, err := LoadTaskflow(exp)
		if err != nil {
			t.Fatalf("failed to load built-in taskflow %s: %v", exp, err)
		}
		if tf.Name != exp || len(tf.Tasks) == 0 {
			t.Fatalf("malformed taskflow %s: %+v", exp, tf)
		}
	}
	if len(builtins) < len(expected) {
		t.Fatalf("expected at least %d built-ins, got %d: %v", len(expected), len(builtins), builtins)
	}
}

func TestSafeTemplateRendering(t *testing.T) {
	ctx := TemplateContext{
		Globals: map[string]string{"system_rules": "defensive audit rules"},
		Entry: &Entry{
			Name:  "TestActivity",
			File:  "app/src/main/TestActivity.kt",
			Kinds: []string{"component", "deeplink"},
		},
		Code:     `class TestActivity : Activity() { val x = 1 }`,
		ClassID:  "confused-deputy",
		ClassCWE: "CWE-441",
		Hint:     "check intent extras",
		PassName: "strict",
		PassAsk:  "Report obvious bugs.",
	}

	tmpl := "System: {{ globals.system_rules }}\n" +
		"Entry: {{ entry.name }} ({{ entry.kinds }}) at {{ entry.file }}\n" +
		"Class: {{ class.id }} ({{ class.cwe }})\n" +
		"{{ class.hint }}{{ pass.ask }}\n" +
		"Code:\n{{ entry.code }}"

	out, err := RenderTemplate(tmpl, ctx)
	if err != nil {
		t.Fatalf("rendering failed: %v", err)
	}

	if !strings.Contains(out, "TestActivity (component+deeplink)") {
		t.Errorf("missing entry metadata in output: %s", out)
	}
	if !strings.Contains(out, "UNTRUSTED CODE") || !strings.Contains(out, "val x = 1") {
		t.Errorf("untrusted code wrapper missing: %s", out)
	}
	if !strings.Contains(out, "Check specifically: check intent extras") {
		t.Errorf("missing hint: %s", out)
	}

	// Injection attempt rejection
	evilTmpl := "Template with <script>alert(1)</script>"
	if _, err := RenderTemplate(evilTmpl, ctx); err == nil {
		t.Fatal("expected script injection rejection")
	}

	evalTmpl := "Template with eval(rm -rf /)"
	if _, err := RenderTemplate(evalTmpl, ctx); err == nil {
		t.Fatal("expected eval injection rejection")
	}
}

func TestSignedTaskflow(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	raw := `{
		"version": 1,
		"name": "custom-security-flow",
		"tasks": [
			{"name": "check", "model": "classify", "prompt": "Audit: {{ entry.code }}"}
		]
	}`

	signed, err := SignTaskflow([]byte(raw), priv)
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	// Loading with trusted key succeeds
	tf, err := LoadSignedTaskflow(signed, []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if tf.Name != "custom-security-flow" {
		t.Errorf("unexpected name: %s", tf.Name)
	}

	// Loading without trusted key fails
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := LoadSignedTaskflow(signed, []ed25519.PublicKey{otherPub}); err == nil {
		t.Fatal("expected untrusted signature rejection")
	}

	// Tampered payload fails
	tampered := append([]byte("tampered"), signed...)
	if _, err := LoadSignedTaskflow(tampered, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("expected tampered signature rejection")
	}
}

func TestGoldenAndroidAuditEquivalence(t *testing.T) {
	// Baseline v1 run
	e1 := &Engine{Models: []ai.Model{fake{local: true}}, Policy: "local-only", MaxCalls: 100}
	fs1, err := e1.Scan(context.Background(), benchDir)
	if err != nil {
		t.Fatalf("v1 scan failed: %v", err)
	}

	// Run with explicit android-audit taskflow
	tf, err := LoadTaskflow("android-audit")
	if err != nil {
		t.Fatalf("load android-audit taskflow failed: %v", err)
	}
	e2 := &Engine{
		Models:   []ai.Model{fake{local: true}},
		Policy:   "local-only",
		MaxCalls: 100,
		Config: &HuntConfig{
			Taskflow: tf.Name,
		},
	}
	fs2, err := e2.Scan(context.Background(), benchDir)
	if err != nil {
		t.Fatalf("taskflow scan failed: %v", err)
	}

	if len(fs1) != len(fs2) {
		t.Fatalf("finding count mismatch between v1 and android-audit taskflow: %d vs %d", len(fs1), len(fs2))
	}
	for i := range fs1 {
		if fs1[i].Rule != fs2[i].Rule || fs1[i].File != fs2[i].File || fs1[i].Severity != fs2[i].Severity {
			t.Errorf("finding %d mismatch:\n  v1: %+v\n  tf: %+v", i, fs1[i], fs2[i])
		}
	}
}
