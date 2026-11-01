package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSymlinkedOutside(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Symlink root/.scribex -> outside
	if err := os.Symlink(outside, filepath.Join(root, ".scribex")); err != nil {
		t.Skip("symlinks not supported")
	}

	c := Default()
	c.AI.Model = "test-model"

	err := Save(root, c)
	if err == nil {
		t.Fatalf("Save should fail when .scribex is symlinked outside")
	}

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside dir should remain empty, got %d entries", len(entries))
	}

	// Load should also fail when .scribex is symlinked
	_, err = Load(root)
	if err == nil {
		t.Fatalf("Load should fail when .scribex is symlinked outside")
	}
}

func TestConfigNormalFlow(t *testing.T) {
	root := t.TempDir()

	// Initial load returns default when missing
	c, err := Load(root)
	if err != nil {
		t.Fatalf("Load missing config failed: %v", err)
	}
	if c.AI.Provider != "cli" || c.AI.CLICommand != "claude" {
		t.Fatalf("expected default provider cli/claude, got %s/%s", c.AI.Provider, c.AI.CLICommand)
	}

	// Save and reload
	c.AI.Model = "custom-model"
	if err := Save(root, c); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("Load saved config failed: %v", err)
	}
	if loaded.AI.Model != "custom-model" {
		t.Fatalf("expected custom-model, got %s", loaded.AI.Model)
	}

	// Set helper
	if err := Set(root, "ai.max_tokens", "2048"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	loaded2, _ := Load(root)
	if loaded2.AI.MaxTokens != 2048 {
		t.Fatalf("expected 2048, got %d", loaded2.AI.MaxTokens)
	}
}

func TestPartH_APIKeyEnvValidation(t *testing.T) {
	accepted := []string{"ANTHROPIC_API_KEY", "MY_LLM_KEY_2", "OPENAI_API_KEY"}
	for _, a := range accepted {
		if err := ValidateAPIKeyEnv(a); err != nil {
			t.Fatalf("expected %q to be accepted, got err: %v", a, err)
		}
	}

	rejected := []string{
		"AKIA" + strings.Repeat("A", 16),
		"sk-" + strings.Repeat("a", 30),
		"ghp_" + strings.Repeat("a", 36),
		"toolong" + strings.Repeat("A", 60),
		"TWENTYFOURCHARSNOUNDERSCORE",
		"KEY_WITH_123456789012_DIGITS",
		"lower_case_env",
		"123_STARTS_WITH_NUMBER",
	}
	for _, r := range rejected {
		err := ValidateAPIKeyEnv(r)
		if err == nil {
			t.Fatalf("expected %q to be rejected, but got nil", r)
		}
		expectedErr := "ai.api_key_env must be the NAME of an environment variable (e.g. ANTHROPIC_API_KEY), not a key"
		if err.Error() != expectedErr {
			t.Fatalf("expected exact error %q, got %q", expectedErr, err.Error())
		}
		if strings.Contains(err.Error(), r) {
			t.Fatalf("error must NOT echo the rejected value: %v", err)
		}
	}

	root := t.TempDir()
	c := Default()
	if err := Save(root, c); err != nil {
		t.Fatal(err)
	}

	// config set ai.api_key_env <rejected> fails without writing file
	badKey := "sk-" + strings.Repeat("a", 30)
	if err := Set(root, "ai.api_key_env", badKey); err == nil {
		t.Fatalf("Set should fail for key-shaped value")
	}
	reloaded, _ := Load(root)
	if reloaded.AI.APIKeyEnv == badKey {
		t.Fatalf("Set should NOT have written file on failure")
	}
}

func TestValidateProviders(t *testing.T) {
	base := func() Config { return Default() }
	cases := []struct {
		name string
		mut  func(*Config)
		ok   bool
	}{
		{"default cli needs no key", func(c *Config) { c.AI.APIKeyEnv = "" }, true},
		{"cli codex", func(c *Config) { c.AI.CLICommand = "codex"; c.AI.Model = "" }, true},
		{"cli empty model ok", func(c *Config) { c.AI.Model = "" }, true},
		{"cli unknown command", func(c *Config) { c.AI.CLICommand = "bash" }, false},
		{"cli path", func(c *Config) { c.AI.CLICommand = "/bin/sh" }, false},
		{"cli args", func(c *Config) { c.AI.CLICommand = "claude --x" }, false},
		{"cli traversal", func(c *Config) { c.AI.CLICommand = "../claude" }, false},
		{"cli empty", func(c *Config) { c.AI.CLICommand = "" }, false},
		{"flag-like model", func(c *Config) { c.AI.Model = "--dangerously-skip-permissions" }, false},
		{"model with space", func(c *Config) { c.AI.Model = "a b" }, false},
		{"model newline", func(c *Config) { c.AI.Model = "a\nb" }, false},
		{"model too long", func(c *Config) { c.AI.Model = strings.Repeat("a", 101) }, false},
		{"ollama needs model", func(c *Config) { c.AI.Provider = "ollama"; c.AI.Model = "" }, false},
		{"ollama ok", func(c *Config) { c.AI.Provider = "ollama"; c.AI.Model = "llama3.2:3b" }, true},
		{"anthropic ok", func(c *Config) { c.AI.Provider = "anthropic"; c.AI.Model = "claude-sonnet-5-5" }, true},
		{"anthropic bad key env", func(c *Config) { c.AI.Provider = "anthropic"; c.AI.Model = "m"; c.AI.APIKeyEnv = "bad key" }, false},
		{"unknown provider", func(c *Config) { c.AI.Provider = "gemini-api" }, false},
		{"bad review", func(c *Config) { c.AI.Review = "yes" }, false},
		{"bad review mode", func(c *Config) { c.AI.ReviewMode = "x" }, false},
		{"bad branch prefix", func(c *Config) { c.AI.BranchPrefix = "../x" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mut(&c)
			if got := len(c.Validate()) == 0; got != tc.ok {
				t.Errorf("ok=%v want %v: %v", got, tc.ok, c.Validate())
			}
		})
	}
}

func TestExistingAnthropicConfigUnchanged(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".scribex"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := "ai:\n  provider: anthropic\n  api_key_env: ANTHROPIC_API_KEY\n  max_tokens: 1024\n  max_calls: 20\n"
	if err := os.WriteFile(filepath.Join(root, ".scribex", "config.yaml"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.AI.Provider != "anthropic" || c.AI.Model != "claude-sonnet-5-5" || len(c.Validate()) != 0 {
		t.Fatalf("legacy config changed behaviour: %+v %v", c.AI, c.Validate())
	}
}

func TestSetNewKeys(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, Default()); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"ai.cli_command": "agy", "ai.review": "socratix", "ai.review_mode": "tenthman", "ai.branch_prefix": "docs/bot-"} {
		if err := Set(root, k, v); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
	c, _ := Load(root)
	if c.AI.CLICommand != "agy" || c.AI.Review != "socratix" || c.AI.ReviewMode != "tenthman" || c.AI.BranchPrefix != "docs/bot-" {
		t.Fatalf("not persisted: %+v", c.AI)
	}
	if err := Set(root, "ai.cli_command", "/bin/sh"); err == nil {
		t.Fatal("arbitrary command accepted")
	}
}

func TestDefaultModelIsCLIOwnDefault(t *testing.T) {
	if Default().AI.Model != "" {
		t.Fatal("default model must be empty so any CLI uses its own default")
	}
}
