package hunt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigValid(t *testing.T) {
	c := DefaultHuntConfig()
	if err := c.Validate(nil); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if c.Gate != "advisory" || c.Policy != "local-only" || c.MaxCalls != 200 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestUnknownKeyRejection(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, ".sentinix", "hunt.json")
	os.MkdirAll(filepath.Dir(confPath), 0o755)

	badJSON := `{
		"version": 1,
		"gate": "advisory",
		"unknown_field": "disallowed"
	}`
	os.WriteFile(confPath, []byte(badJSON), 0o644)

	_, err := LoadConfigFile(confPath)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, got: %v", err)
	}
}

func TestLiteralSecretRejection(t *testing.T) {
	c := DefaultHuntConfig()
	c.Tools.MCPServers = map[string]string{
		"auth_token": "ghp_1234567890abcdef", // literal secret!
	}
	err := c.Validate(nil)
	if err == nil || !strings.Contains(err.Error(), "literal secret") {
		t.Fatalf("expected literal secret rejection, got: %v", err)
	}

	// Valid secret reference
	c.Tools.MCPServers["auth_token"] = "${GITHUB_TOKEN}"
	if err := c.Validate(nil); err != nil {
		t.Fatalf("expected valid ${VAR} reference, got: %v", err)
	}
}

func TestOrgCeilingsEnforcement(t *testing.T) {
	c := DefaultHuntConfig()
	c.MaxCalls = 500
	c.MaxCostUSD = 25.0
	c.Policy = "cloud"

	ceilings := &OrgCeilings{
		MaxCalls:        250,
		MaxCostUSD:      10.0,
		AllowedPolicies: []string{"local-only", "cloud-redacted"},
	}

	err := c.Validate(ceilings)
	if err == nil {
		t.Fatal("expected ceiling validation failure")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "max_calls: 500 exceeds org ceiling of 250") {
		t.Errorf("missing calls ceiling error: %s", errStr)
	}
	if !strings.Contains(errStr, "max_cost_usd: 25.00 exceeds org ceiling of 10.00") {
		t.Errorf("missing cost ceiling error: %s", errStr)
	}
	if !strings.Contains(errStr, "policy: \"cloud\" not permitted") {
		t.Errorf("missing policy ceiling error: %s", errStr)
	}
}

func TestPrecedenceHierarchy(t *testing.T) {
	dir := t.TempDir()
	sentinixDir := filepath.Join(dir, ".sentinix")
	os.MkdirAll(sentinixDir, 0o755)

	// File config sets policy to cloud-redacted and max_calls to 150
	fileJSON := `{
		"version": 1,
		"policy": "cloud-redacted",
		"max_calls": 150
	}`
	os.WriteFile(filepath.Join(sentinixDir, "hunt.json"), []byte(fileJSON), 0o644)

	// Org default
	org := &HuntConfig{
		MaxCalls: 100, // Should be overridden by file (150)
	}

	// Env var override for policy
	os.Setenv("SENTINIX_HUNT_POLICY", "cloud")
	defer os.Unsetenv("SENTINIX_HUNT_POLICY")

	// CLI flag override for max_calls
	flags := CLIFlags{
		MaxCalls: 300,
	}

	eff, err := ResolveConfig(dir, org, flags)
	if err != nil {
		t.Fatalf("resolve config failed: %v", err)
	}

	// Effective max_calls should be 300 (from CLI flag)
	if eff.MaxCalls != 300 {
		t.Errorf("expected MaxCalls 300 from CLI, got %d", eff.MaxCalls)
	}
	if !strings.Contains(eff.Sources["max_calls"], SourceCLI) {
		t.Errorf("expected source CLI for max_calls, got %s", eff.Sources["max_calls"])
	}

	// Effective policy should be cloud (from Env)
	if eff.Policy != "cloud" {
		t.Errorf("expected Policy cloud from Env, got %s", eff.Policy)
	}
	if !strings.Contains(eff.Sources["policy"], SourceEnv) {
		t.Errorf("expected source Env for policy, got %s", eff.Sources["policy"])
	}

	// Version should be 1 (from file)
	if eff.Version != 1 || eff.Sources["version"] != SourceFile {
		t.Errorf("expected source File for version, got %s", eff.Sources["version"])
	}

	// Explain should list fields and sources
	expl := eff.Explain()
	if !strings.Contains(expl, "max_calls") || !strings.Contains(expl, SourceCLI) {
		t.Errorf("unexpected explain output:\n%s", expl)
	}
}

func TestLooseningSafeguardsDetection(t *testing.T) {
	oldCfg := DefaultHuntConfig()
	newCfg := DefaultHuntConfig()

	// Initially identical -> not loosening
	if loose, _ := IsLooseningSafeguards(oldCfg, newCfg, nil); loose {
		t.Fatal("identical configs reported as loosening")
	}

	// 1. Changing gate to confirmed-only
	newCfg.Gate = "confirmed-only"
	if loose, reason := IsLooseningSafeguards(oldCfg, newCfg, nil); !loose || !strings.Contains(reason, "confirmed-only") {
		t.Errorf("expected gate loosening detection, got %v: %s", loose, reason)
	}
	newCfg.Gate = "advisory"

	// 2. Widening model policy
	newCfg.Policy = "cloud"
	if loose, reason := IsLooseningSafeguards(oldCfg, newCfg, nil); !loose || !strings.Contains(reason, "widening model policy") {
		t.Errorf("expected policy loosening detection, got %v: %s", loose, reason)
	}
	newCfg.Policy = "local-only"

	// 3. Enabling tools
	newCfg.Tools.Enabled = true
	if loose, reason := IsLooseningSafeguards(oldCfg, newCfg, nil); !loose || !strings.Contains(reason, "tool execution") {
		t.Errorf("expected tool loosening detection, got %v: %s", loose, reason)
	}
	newCfg.Tools.Enabled = false

	// 4. Enabling PoC runner
	newCfg.PoC.Enabled = true
	if loose, reason := IsLooseningSafeguards(oldCfg, newCfg, nil); !loose || !strings.Contains(reason, "PoC") {
		t.Errorf("expected PoC loosening detection, got %v: %s", loose, reason)
	}
}

func TestInitConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".sentinix", "hunt.json")
	if err := InitConfigFile(p); err != nil {
		t.Fatalf("init config failed: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	// Calling again should fail
	if err := InitConfigFile(p); err == nil {
		t.Fatal("expected error on re-init over existing file")
	}
}
