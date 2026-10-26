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
	if c.AI.Provider != "anthropic" {
		t.Fatalf("expected default provider anthropic, got %s", c.AI.Provider)
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
