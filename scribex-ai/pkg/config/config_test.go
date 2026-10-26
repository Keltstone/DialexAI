package config

import (
	"os"
	"path/filepath"
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
