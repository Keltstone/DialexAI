package scheduler

import (
	"strings"
	"testing"

	"sentinix/pkg/githost"
)

// TestSelfHostedHostCheck tests that PR review and full scan on a configured GHE host
// proceed past the host check, and a forged host is still rejected.
func TestSelfHostedHostCheck(t *testing.T) {
	// 1. Configured GHE host: Review proceeds past the host check (fails at fetch, not host check)
	ev := &githost.Event{
		HeadSHA:        strings.Repeat("a", 40),
		HeadRef:        "x",
		BaseRef:        "main",
		CloneURL:       "https://ghe.corp.example/o/r.git",
		BaseCloneURL:   "https://ghe.corp.example/o/r.git",
		ConfiguredHost: "https://ghe.corp.example",
	}
	_, err := Review(t.Context(), ev, "x-access-token", "tok")
	t.Logf("PR review on GHE host: %v", err)
	if err != nil && strings.Contains(err.Error(), "differs from configured host") {
		t.Fatalf("PR review failed at host check instead of proceeding: %v", err)
	}

	// 2. Configured GHE host: Full scan proceeds past host check
	_, _, err = Full(t.Context(), "https://ghe.corp.example/o/r.git", "main", "x-access-token", "tok", "https://ghe.corp.example")
	t.Logf("Full scan on GHE host: %v", err)
	if err != nil && strings.Contains(err.Error(), "differs from configured host") {
		t.Fatalf("Full scan failed at host check instead of proceeding: %v", err)
	}

	// 3. Forged host: rejected by host check
	evForged := &githost.Event{
		HeadSHA:        strings.Repeat("a", 40),
		HeadRef:        "x",
		BaseRef:        "main",
		CloneURL:       "https://evil.corp.example/o/r.git",
		BaseCloneURL:   "https://evil.corp.example/o/r.git",
		ConfiguredHost: "https://ghe.corp.example",
	}
	_, errForged := Review(t.Context(), evForged, "x-access-token", "tok")
	t.Logf("PR review on forged host: %v", errForged)
	if errForged == nil || !strings.Contains(errForged.Error(), "differs from configured host") {
		t.Fatalf("expected forged host to be rejected by host check, got: %v", errForged)
	}

	// 4. Forged host in Full scan: rejected by host check
	_, _, errForgedFull := Full(t.Context(), "https://evil.corp.example/o/r.git", "main", "x-access-token", "tok", "https://ghe.corp.example")
	t.Logf("Full scan on forged host: %v", errForgedFull)
	if errForgedFull == nil || !strings.Contains(errForgedFull.Error(), "differs from configured host") {
		t.Fatalf("expected forged host to be rejected by host check in Full, got: %v", errForgedFull)
	}
}
