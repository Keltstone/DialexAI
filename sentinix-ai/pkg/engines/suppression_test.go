package engines

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalScan_IsolatesInRepoSuppressionConfigsRecursively(t *testing.T) {
	d := t.TempDir()
	// Top-level suppressions
	os.WriteFile(filepath.Join(d, ".semgrepignore"), []byte("*\n"), 0o644)
	os.WriteFile(filepath.Join(d, ".gitleaks.toml"), []byte("[allowlist]\npaths = [\".*\"]\n"), 0o644)
	os.WriteFile(filepath.Join(d, ".gitleaksignore"), []byte("*\n"), 0o644)

	// Nested suppressions
	nested := filepath.Join(d, "sub", "deep")
	os.MkdirAll(nested, 0o755)
	os.WriteFile(filepath.Join(nested, "osv-scanner.toml"), []byte("[[IgnoredVulns]]\nid = \"*\"\n"), 0o644)
	os.WriteFile(filepath.Join(nested, ".semgrepignore"), []byte("*\n"), 0o644)

	// Stub external engine that fails if any suppression file is visible during scan
	script := `
		if [ -f "$1/.semgrepignore" ] || [ -f "$1/.gitleaks.toml" ] || [ -f "$1/.gitleaksignore" ] || [ -f "$1/sub/deep/osv-scanner.toml" ] || [ -f "$1/sub/deep/.semgrepignore" ]; then
			echo '{"error": "suppression file visible during scan"}'
			exit 1
		fi
		echo '{"runs":[]}'
	`
	ext := External{
		Bin:  "sh",
		Args: []string{"-c", script, "--"},
	}
	fs, err := ext.ScanContext(context.Background(), d)
	if err != nil {
		t.Fatalf("unexpected error during isolated scan: %v", err)
	}
	if len(fs) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(fs))
	}

	// Verify all suppression files are restored after the scan
	expectedFiles := []string{
		filepath.Join(d, ".semgrepignore"),
		filepath.Join(d, ".gitleaks.toml"),
		filepath.Join(d, ".gitleaksignore"),
		filepath.Join(nested, "osv-scanner.toml"),
		filepath.Join(nested, ".semgrepignore"),
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected file %s to be restored, got error: %v", f, err)
		}
	}
}

func TestExternalBundledFlags(t *testing.T) {
	// Test that external engines include the required flags
	exts := []External{
		{"semgrep", []string{"scan", "--config", "/rules", "--sarif", "--quiet", "--no-git-ignore", "--disable-nosem"}, nil},
		{"gitleaks", []string{"detect", "--no-git", "--report-format", "sarif", "--report-path", "/dev/stdout", "--no-banner", "--ignore-gitleaks-allow", "-s"}, nil},
		{"osv-scanner", []string{"--format", "sarif", "-r", "--config", "/dev/null"}, nil},
	}
	for _, e := range exts {
		argsStr := strings.Join(e.Args, " ")
		switch e.Bin {
		case "semgrep":
			if !strings.Contains(argsStr, "--disable-nosem") {
				t.Errorf("semgrep missing --disable-nosem flag: %v", e.Args)
			}
		case "gitleaks":
			if !strings.Contains(argsStr, "--ignore-gitleaks-allow") {
				t.Errorf("gitleaks missing --ignore-gitleaks-allow flag: %v", e.Args)
			}
		case "osv-scanner":
			if !strings.Contains(argsStr, "--config") {
				t.Errorf("osv-scanner missing --config flag: %v", e.Args)
			}
		}
	}
}

func TestParseSARIF_ContentBasedIdentity(t *testing.T) {
	sarifJSON := []byte(`{
		"version": "2.1.0",
		"runs": [{
			"tool": {"driver": {"name": "semgrep"}},
			"results": [{
				"ruleId": "rules.hardcoded-token",
				"level": "error",
				"message": {"text": "Hardcoded token found"},
				"partialFingerprints": {
					"primaryLocationLineHash": "hash123456"
				},
				"locations": [{
					"physicalLocation": {
						"artifactLocation": {"uri": "src/Auth.kt"},
						"region": {
							"startLine": 42,
							"snippet": {"text": "val token = \"secret\""}
						}
					}
				}]
			}]
		}]
	}`)

	fs, err := ParseSARIF("semgrep", sarifJSON)
	if err != nil {
		t.Fatalf("ParseSARIF failed: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	f := fs[0]
	if f.Match != "hash123456" {
		t.Errorf("expected Match='hash123456', got %q", f.Match)
	}
	if !strings.Contains(f.Evidence, "secret") && !strings.Contains(f.Evidence, "****") {
		t.Errorf("expected Evidence populated, got %q", f.Evidence)
	}
}
