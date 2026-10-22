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
