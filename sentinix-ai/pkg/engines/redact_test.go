package engines

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tokenEntry struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

func loadCorpus(t *testing.T) []tokenEntry {
	t.Helper()
	data, err := os.ReadFile("testdata/redact/tokens.json")
	if err != nil {
		t.Fatalf("failed to read testdata/redact/tokens.json: %v", err)
	}
	var entries []tokenEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to unmarshal tokens.json: %v", err)
	}
	if len(entries) < 50 {
		t.Fatalf("expected at least 50 tokens in corpus, got %d", len(entries))
	}
	return entries
}

func TestRedact_CorpusMasked(t *testing.T) {
	entries := loadCorpus(t)
	var unredacted []string
	for _, entry := range entries {
		masked := Redact(entry.Token)
		if strings.Contains(masked, entry.Token) {
			unredacted = append(unredacted, entry.Provider+": "+entry.Token)
		}
	}
	if len(unredacted) > 0 {
		t.Fatalf("%d tokens were NOT redacted by engines.Redact:\n%s", len(unredacted), strings.Join(unredacted, "\n"))
	}
}

func TestSecrets_CorpusDetected(t *testing.T) {
	entries := loadCorpus(t)
	d := t.TempDir()
	for i, entry := range entries {
		fn := filepath.Join(d, "file"+strings.ReplaceAll(entry.Provider, " ", "_")+".txt")
		content := "token = \"" + entry.Token + "\"\n"
		if err := os.WriteFile(fn, []byte(content), 0o644); err != nil {
			t.Fatalf("failed writing test file %d: %v", i, err)
		}
	}

	sec := Secrets{}
	fs, err := sec.Scan(context.Background(), d)
	if err != nil {
		t.Fatalf("Secrets.Scan failed: %v", err)
	}
	t.Logf("Detected %d findings for %d tokens", len(fs), len(entries))
	if len(fs) < len(entries) {
		t.Fatalf("expected at least %d findings (one per token), got %d", len(entries), len(fs))
	}
}

func TestRedact_FailClosedWhenCorrupted(t *testing.T) {
	// When redaction engine/rules are corrupted, fail-closed must engage:
	// RedactFailClosed must report error and return empty string / refusal.
	oldRules := secretRules
	defer func() { secretRules = oldRules }()

	// Deliberately corrupt rules
	secretRules = nil

	res, err := RedactSafe("sensitive sk_live_123456789012345678901234")
	if err == nil {
		t.Fatal("expected error from RedactSafe when rules corrupted, got nil")
	}
	if res != "" {
		t.Fatalf("expected empty/refused output on redaction failure, got %q", res)
	}
}
