package trust

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuiltinHosts(t *testing.T) {
	hosts, err := AIHosts("anthropic", nil, "")
	if err != nil {
		t.Fatalf("AIHosts anthropic failed: %v", err)
	}
	if len(hosts) != 1 || hosts[0] != "api.anthropic.com" {
		t.Errorf("expected [api.anthropic.com], got %v", hosts)
	}

	hosts, err = AIHosts("openai", nil, "")
	if err != nil {
		t.Fatalf("AIHosts openai failed: %v", err)
	}
	if len(hosts) != 1 || hosts[0] != "api.openai.com" {
		t.Errorf("expected [api.openai.com], got %v", hosts)
	}

	hosts, err = AIHosts("unknown-provider", nil, "")
	if err != nil {
		t.Fatalf("AIHosts unknown failed: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("expected empty hosts for unknown provider, got %v", hosts)
	}
}

func TestEnvAllowedHosts(t *testing.T) {
	envMap := map[string]string{
		"SCRIBEX_AI_ALLOWED_HOSTS": " a.example , B.example:8443,127.0.0.1:9090 ",
	}
	envFn := func(k string) string { return envMap[k] }

	hosts, err := AIHosts("anthropic", envFn, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"api.anthropic.com", "a.example", "b.example:8443", "127.0.0.1:9090"}
	if len(hosts) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, hosts)
	}
	for i, exp := range expected {
		if hosts[i] != exp {
			t.Errorf("at %d: expected %s, got %s", i, exp, hosts[i])
		}
	}
}

func TestEnvAllowedHostsInvalid(t *testing.T) {
	invalidCases := []string{
		"https://a.example",
		"a.example/path",
		"*.example.com",
		"a.example?foo=bar",
		"user@a.example",
		"a.example:invalidport",
	}
	for _, tc := range invalidCases {
		envFn := func(k string) string { return tc }
		_, err := AIHosts("anthropic", envFn, "")
		if err == nil {
			t.Errorf("expected error for invalid host %q, got nil", tc)
		}
	}
}

func TestUserTrustFile(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, ".config/scribex")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	trustFile := filepath.Join(configDir, "trust.yaml")
	content := "ai_hosts: custom.example, proxy.local:8080\n"
	if err := os.WriteFile(trustFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	hosts, err := AIHosts("anthropic", nil, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 3 || hosts[1] != "custom.example" || hosts[2] != "proxy.local:8080" {
		t.Errorf("unexpected hosts from file: %v", hosts)
	}
}

func TestUserTrustFileGroupWritableRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions test not applicable on windows")
	}
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, ".config/scribex")
	_ = os.MkdirAll(configDir, 0755)
	trustFile := filepath.Join(configDir, "trust.yaml")
	_ = os.WriteFile(trustFile, []byte("ai_hosts: bad.example\n"), 0666)
	if err := os.Chmod(trustFile, 0666); err != nil {
		t.Fatal(err)
	}

	_, err := AIHosts("anthropic", nil, tmp)
	if err == nil {
		t.Fatal("expected error for group/world-writable trust file, got nil")
	}
	if !strings.Contains(err.Error(), "group or world writable") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUserTrustFileSymlinkRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks test not applicable on windows")
	}
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, ".config/scribex")
	_ = os.MkdirAll(configDir, 0700)
	target := filepath.Join(tmp, "real_trust.yaml")
	_ = os.WriteFile(target, []byte("ai_hosts: symlink.example\n"), 0600)
	sym := filepath.Join(configDir, "trust.yaml")
	if err := os.Symlink(target, sym); err != nil {
		t.Fatal(err)
	}

	_, err := AIHosts("anthropic", nil, tmp)
	if err == nil {
		t.Fatal("expected error for symlink trust file, got nil")
	}
	if !strings.Contains(err.Error(), "is a symlink") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRepoTrustFilesIgnored(t *testing.T) {
	repoDir := t.TempDir()
	scribexDir := filepath.Join(repoDir, ".scribex")
	if err := os.MkdirAll(scribexDir, 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(scribexDir, "trust.yaml"), []byte("ai_hosts: evil.example\n"), 0600)
	_ = os.WriteFile(filepath.Join(scribexDir, "config.yaml"), []byte("ai:\n  base_url: https://evil.example\n"), 0600)

	// AIHosts only looks at env and home; repoDir is never passed or inspected.
	hosts, err := AIHosts("anthropic", nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hosts {
		if strings.Contains(h, "evil.example") {
			t.Fatalf("evil.example from repo was trusted: %v", hosts)
		}
	}
}

func TestCheckAIEndpoint(t *testing.T) {
	hosts := []string{"api.anthropic.com", "trusted.example", "127.0.0.1:8080"}

	// Valid https
	clean, err := CheckAIEndpoint("anthropic", "https://api.anthropic.com/v1/", hosts, false)
	if err != nil {
		t.Fatalf("valid endpoint rejected: %v", err)
	}
	if clean != "https://api.anthropic.com/v1" {
		t.Errorf("expected clean URL https://api.anthropic.com/v1, got %q", clean)
	}

	// Unknown host
	_, err = CheckAIEndpoint("anthropic", "https://evil.example", hosts, false)
	if err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Errorf("expected untrusted error, got %v", err)
	}

	// Userinfo rejected
	_, err = CheckAIEndpoint("anthropic", "https://user:pass@api.anthropic.com", hosts, false)
	if err == nil || !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("expected userinfo error, got %v", err)
	}

	// Query rejected
	_, err = CheckAIEndpoint("anthropic", "https://api.anthropic.com?key=val", hosts, false)
	if err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("expected query error, got %v", err)
	}

	// Fragment rejected
	_, err = CheckAIEndpoint("anthropic", "https://api.anthropic.com#frag", hosts, false)
	if err == nil || !strings.Contains(err.Error(), "fragment") {
		t.Errorf("expected fragment error, got %v", err)
	}

	// http on non-loopback rejected
	_, err = CheckAIEndpoint("anthropic", "http://trusted.example", hosts, true)
	if err == nil || !strings.Contains(err.Error(), "http only allowed for loopback") {
		t.Errorf("expected http non-loopback error, got %v", err)
	}

	// http on loopback without insecureLocal rejected
	_, err = CheckAIEndpoint("anthropic", "http://127.0.0.1:8080", hosts, false)
	if err == nil || !strings.Contains(err.Error(), "http only allowed for loopback with SCRIBEX_ALLOW_INSECURE_LOCAL=1") {
		t.Errorf("expected insecure local error, got %v", err)
	}

	// http on loopback WITH insecureLocal, but NOT in hosts rejected
	_, err = CheckAIEndpoint("anthropic", "http://127.0.0.1:9999", hosts, true)
	if err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Errorf("expected not trusted error for unlisted loopback, got %v", err)
	}

	// http on loopback WITH insecureLocal AND in hosts allowed
	clean, err = CheckAIEndpoint("anthropic", "http://127.0.0.1:8080/", hosts, true)
	if err != nil {
		t.Fatalf("expected allowed loopback with flags, got %v", err)
	}
	if clean != "http://127.0.0.1:8080" {
		t.Errorf("expected clean URL http://127.0.0.1:8080, got %q", clean)
	}
}
