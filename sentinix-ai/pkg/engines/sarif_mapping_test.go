package engines

import (
	"testing"
)

func TestMapSARIFSeverityAndCategory(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		ruleID          string
		level           string
		message         string
		defaultCategory string
		wantSev         Severity
		wantCat         string
	}{
		{
			name:    "detekt warning maps to low/style",
			tool:    "detekt",
			ruleID:  "detekt.formatting.Indentation",
			level:   "warning",
			message: "Line indented wrongly",
			wantSev: Low,
			wantCat: CategoryStyle,
		},
		{
			name:    "ktlint error maps to low/style",
			tool:    "ktlint",
			ruleID:  "standard:no-wildcard-imports",
			level:   "error",
			message: "Wildcard import not allowed",
			wantSev: Low,
			wantCat: CategoryStyle,
		},
		{
			name:    "android-lint accessibility error maps to high/accessibility",
			tool:    "android-lint",
			ruleID:  "ContentDescription",
			level:   "error",
			message: "Missing contentDescription on ImageView",
			wantSev: High,
			wantCat: CategoryAccessibility,
		},
		{
			name:    "android-lint accessibility warning maps to medium/accessibility",
			tool:    "android-lint",
			ruleID:  "ClickableViewAccessibility",
			level:   "warning",
			message: "Custom view should handle touch feedback",
			wantSev: Medium,
			wantCat: CategoryAccessibility,
		},
		{
			name:    "android-lint security error maps to high/security",
			tool:    "android-lint",
			ruleID:  "ExportedService",
			level:   "error",
			message: "Exported service with no permission",
			wantSev: High,
			wantCat: CategorySecurity,
		},
		{
			name:    "android-lint other error maps to medium/quality",
			tool:    "android-lint",
			ruleID:  "UnusedResources",
			level:   "error",
			message: "The resource R.string.unused appears to be unused",
			wantSev: Medium,
			wantCat: CategoryQuality,
		},
		{
			name:    "android-lint other warning maps to low/quality",
			tool:    "android-lint",
			ruleID:  "ObsoleteLayoutParam",
			level:   "warning",
			message: "Invalid layout param",
			wantSev: Low,
			wantCat: CategoryQuality,
		},
		{
			name:    "konsist architecture failure maps to high/architecture",
			tool:    "konsist",
			ruleID:  "DomainArchitectureRule",
			level:   "error",
			message: "Domain layer must have zero android imports",
			wantSev: High,
			wantCat: CategoryArchitecture,
		},
		{
			name:    "generic sarif error maps to high/security",
			tool:    "semgrep",
			ruleID:  "sqli",
			level:   "error",
			message: "SQL injection",
			wantSev: High,
			wantCat: CategorySecurity,
		},
		{
			name:    "generic sarif note maps to low/security",
			tool:    "semgrep",
			ruleID:  "info-rule",
			level:   "note",
			message: "Informational",
			wantSev: Low,
			wantCat: CategorySecurity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSev, gotCat := MapSARIFSeverityAndCategory(tc.tool, tc.ruleID, tc.level, tc.message, tc.defaultCategory)
			if gotSev != tc.wantSev {
				t.Errorf("severity mismatch: got %v, want %v", gotSev, tc.wantSev)
			}
			if gotCat != tc.wantCat {
				t.Errorf("category mismatch: got %q, want %q", gotCat, tc.wantCat)
			}
		})
	}
}

func TestParseSARIFWithCategory_ToolDriverDetection(t *testing.T) {
	sarifJSON := `{
		"version": "2.1.0",
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"runs": [
			{
				"tool": {
					"driver": {
						"name": "detekt"
					}
				},
				"results": [
					{
						"ruleId": "detekt.style.MagicNumber",
						"level": "warning",
						"message": { "text": "This expression contains a magic number." },
						"locations": [
							{
								"physicalLocation": {
									"artifactLocation": { "uri": "src/App.kt" },
									"region": { "startLine": 42 }
								}
							}
						]
					}
				]
			}
		]
	}`

	fs, err := ParseSARIFWithCategory("generic-sarif", []byte(sarifJSON), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	f := fs[0]
	if f.Category != CategoryStyle {
		t.Errorf("expected category %q, got %q", CategoryStyle, f.Category)
	}
	if f.Severity != Low {
		t.Errorf("expected severity %v, got %v", Low, f.Severity)
	}
	if f.File != "src/App.kt" {
		t.Errorf("expected file src/App.kt, got %q", f.File)
	}
	if f.Line != 42 {
		t.Errorf("expected line 42, got %d", f.Line)
	}
}
