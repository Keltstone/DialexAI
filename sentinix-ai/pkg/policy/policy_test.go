package policy

import (
	"sentinix/pkg/engines"
	"testing"
)

func TestCategoryFails(t *testing.T) {
	fsHigh := []engines.Finding{
		{Rule: "r1", Severity: engines.High, Category: engines.CategoryArchitecture},
	}
	fsLow := []engines.Finding{
		{Rule: "r2", Severity: engines.Low, Category: engines.CategoryArchitecture},
	}

	tests := []struct {
		name      string
		mode      string
		threshold engines.Severity
		findings  []engines.Finding
		wantFail  bool
	}{
		{"block mode with high findings at high threshold", "block", engines.High, fsHigh, true},
		{"block mode with low findings at high threshold", "block", engines.High, fsLow, false},
		{"comment mode with high findings at low threshold", "comment", engines.Low, fsHigh, false},
		{"off mode with high findings at low threshold", "off", engines.Low, fsHigh, false},
		{"block mode with no findings", "block", engines.Low, nil, false},
		{"block mode with unattested finding in architecture", "block", engines.High, []engines.Finding{{Rule: "arch", Severity: engines.High, Category: engines.CategoryArchitecture, Source: "ci-unattested"}}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CategoryFails(tc.findings, engines.CategoryArchitecture, CategoryPolicy{Mode: tc.mode, Threshold: tc.threshold})
			if got != tc.wantFail {
				t.Errorf("CategoryFails() = %v, want %v", got, tc.wantFail)
			}
		})
	}

	// Security category ignores unattested
	secUnattested := []engines.Finding{{Rule: "sec", Severity: engines.High, Category: engines.CategorySecurity, Source: "ci-unattested"}}
	if CategoryFails(secUnattested, engines.CategorySecurity, CategoryPolicy{Mode: "block", Threshold: engines.High}) {
		t.Errorf("CategoryFails() for security should ignore unattested findings to prevent merge DoS")
	}
}

func TestGroupByCategory(t *testing.T) {
	fs := []engines.Finding{
		{Rule: "sec1", Category: engines.CategorySecurity},
		{Rule: "arch1", Category: engines.CategoryArchitecture},
		{Rule: "style1", Category: engines.CategoryStyle},
		{Rule: "empty", Category: ""}, // should normalize to security
	}

	m := GroupByCategory(fs)
	if len(m[engines.CategorySecurity]) != 2 {
		t.Errorf("expected 2 security findings, got %d", len(m[engines.CategorySecurity]))
	}
	if len(m[engines.CategoryArchitecture]) != 1 {
		t.Errorf("expected 1 architecture finding, got %d", len(m[engines.CategoryArchitecture]))
	}
	if len(m[engines.CategoryStyle]) != 1 {
		t.Errorf("expected 1 style finding, got %d", len(m[engines.CategoryStyle]))
	}
}

func TestIsLoosening(t *testing.T) {
	tests := []struct {
		name     string
		oldPol   CategoryPolicy
		newPol   CategoryPolicy
		loosened bool
	}{
		{
			name:     "turning check off is loosening",
			oldPol:   CategoryPolicy{Mode: "comment", Threshold: engines.Medium},
			newPol:   CategoryPolicy{Mode: "off", Threshold: engines.Medium},
			loosened: true,
		},
		{
			name:     "lowering mode from block to comment is loosening",
			oldPol:   CategoryPolicy{Mode: "block", Threshold: engines.High},
			newPol:   CategoryPolicy{Mode: "comment", Threshold: engines.High},
			loosened: true,
		},
		{
			name:     "raising severity threshold in block mode is loosening",
			oldPol:   CategoryPolicy{Mode: "block", Threshold: engines.Medium},
			newPol:   CategoryPolicy{Mode: "block", Threshold: engines.High},
			loosened: true,
		},
		{
			name:     "tightening mode from comment to block is not loosening",
			oldPol:   CategoryPolicy{Mode: "comment", Threshold: engines.High},
			newPol:   CategoryPolicy{Mode: "block", Threshold: engines.High},
			loosened: false,
		},
		{
			name:     "lowering severity threshold in block mode is tightening (not loosening)",
			oldPol:   CategoryPolicy{Mode: "block", Threshold: engines.High},
			newPol:   CategoryPolicy{Mode: "block", Threshold: engines.Medium},
			loosened: false,
		},
		{
			name:     "same policy is not loosening",
			oldPol:   CategoryPolicy{Mode: "block", Threshold: engines.High},
			newPol:   CategoryPolicy{Mode: "block", Threshold: engines.High},
			loosened: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsLoosening(tc.oldPol, tc.newPol)
			if got != tc.loosened {
				t.Errorf("IsLoosening() = %v, want %v", got, tc.loosened)
			}
		})
	}
}

func TestRuleEnforcementLevelsAndPolicy(t *testing.T) {
	pol := &ProjectPolicy{
		DefaultLevel: LevelBlock,
		Rules: map[string]RuleEnforcementLevel{
			"android/allow-backup": LevelWarn,
			"rust/unsafe-block":    LevelNotice,
			"python/csrf-exempt":   LevelIgnore,
		},
	}

	fs := []engines.Finding{
		{Rule: "android/exported-component", Severity: engines.High},
		{Rule: "android/allow-backup", Severity: engines.Low},
		{Rule: "rust/unsafe-block", Severity: engines.Medium},
		{Rule: "python/csrf-exempt", Severity: engines.Critical},
	}

	ev := EvaluateFindings(fs, pol)
	if len(ev.Blocked) != 1 || ev.Blocked[0].Rule != "android/exported-component" {
		t.Errorf("expected 1 blocked finding, got %+v", ev.Blocked)
	}
	if len(ev.Warned) != 1 || ev.Warned[0].Rule != "android/allow-backup" {
		t.Errorf("expected 1 warned finding, got %+v", ev.Warned)
	}
	if len(ev.Noticed) != 1 || ev.Noticed[0].Rule != "rust/unsafe-block" {
		t.Errorf("expected 1 noticed finding, got %+v", ev.Noticed)
	}
	if len(ev.Ignored) != 1 || ev.Ignored[0].Rule != "python/csrf-exempt" {
		t.Errorf("expected 1 ignored finding, got %+v", ev.Ignored)
	}

	if !PolicyFails(ev) {
		t.Errorf("expected PolicyFails to be true when Blocked findings exist")
	}

	// When blocked is empty, policy must pass
	evClean := EvaluatedFindings{Warned: ev.Warned, Noticed: ev.Noticed}
	if PolicyFails(evClean) {
		t.Errorf("expected PolicyFails to be false when only Warned/Noticed findings exist")
	}
}


func TestLLMFindingsNeverGate(t *testing.T) {
	f := engines.Finding{Rule: "hunt/authz", Severity: engines.Critical, Source: "llm"}
	if Fails([]engines.Finding{f}, engines.Low) || CategoryFails([]engines.Finding{f}, engines.CategorySecurity, CategoryPolicy{Mode: "block", Threshold: engines.Low}) {
		t.Fatal("llm finding gated")
	}
	if (&ProjectPolicy{DefaultLevel: LevelBlock}).EffectiveLevel("hunt/authz", engines.Critical) != LevelNotice {
		t.Fatal("hunt rule not notice")
	}
}

func TestHuntConfirmedOnlyGating(t *testing.T) {
	unconfirmed := engines.Finding{
		Rule:     "hunt/sql-injection",
		Severity: engines.Critical,
		Source:   "llm",
		Evidence: "[exploitability:unconfirmed]",
	}
	likely := engines.Finding{
		Rule:     "hunt/sql-injection",
		Severity: engines.Critical,
		Source:   "llm",
		Evidence: "[exploitability:likely]",
	}
	confirmedHigh := engines.Finding{
		Rule:     "hunt/sql-injection",
		Severity: engines.High,
		Source:   "llm",
		Evidence: "[exploitability:confirmed] PoC reproduced: test/Exploit.java",
	}

	// 1. Under default advisory mode, even confirmed findings do not gate
	if FailsWithHuntGate([]engines.Finding{confirmedHigh}, engines.Medium, "advisory") {
		t.Error("advisory mode must never gate llm findings")
	}

	// 2. Under confirmed-only mode:
	// Unconfirmed & likely findings never gate
	if FailsWithHuntGate([]engines.Finding{unconfirmed}, engines.Low, "confirmed-only") {
		t.Error("unconfirmed finding gated in confirmed-only mode")
	}
	if FailsWithHuntGate([]engines.Finding{likely}, engines.Low, "confirmed-only") {
		t.Error("likely finding gated in confirmed-only mode")
	}

	// Confirmed finding gates if severity >= threshold
	if !FailsWithHuntGate([]engines.Finding{confirmedHigh}, engines.Medium, "confirmed-only") {
		t.Error("confirmed high finding failed to gate in confirmed-only mode")
	}
	// But not if threshold is Critical
	if FailsWithHuntGate([]engines.Finding{confirmedHigh}, engines.Critical, "confirmed-only") {
		t.Error("confirmed high finding gated when threshold was Critical")
	}

	// 3. EffectiveFindingLevel
	polAdvisory := &ProjectPolicy{HuntGate: "advisory"}
	if polAdvisory.EffectiveFindingLevel(confirmedHigh) != LevelNotice {
		t.Error("expected notice level under advisory policy")
	}

	polConfirmed := &ProjectPolicy{HuntGate: "confirmed-only"}
	if polConfirmed.EffectiveFindingLevel(confirmedHigh) != LevelBlock {
		t.Error("expected block level for confirmed finding under confirmed-only policy")
	}
	if polConfirmed.EffectiveFindingLevel(unconfirmed) != LevelNotice {
		t.Error("expected notice level for unconfirmed finding under confirmed-only policy")
	}
}

