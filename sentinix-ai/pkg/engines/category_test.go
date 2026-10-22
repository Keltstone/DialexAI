package engines

import (
	"testing"
)

func TestFindingCategoryDefaultsAndFingerprintStability(t *testing.T) {
	f1 := Finding{
		Rule:     "no-eval",
		Engine:   "semgrep",
		File:     "src/app.py",
		Line:     10,
		Severity: High,
		Message:  "Avoid using eval",
		Evidence: "eval(code)",
		Category: CategorySecurity,
	}

	f2 := f1
	f2.Category = CategoryArchitecture

	f3 := f1
	f3.Category = CategoryStyle

	f4 := f1
	f4.Category = "" // empty category

	fp1 := f1.Fingerprint()
	fp2 := f2.Fingerprint()
	fp3 := f3.Fingerprint()
	fp4 := f4.Fingerprint()

	if fp1 != fp2 {
		t.Errorf("Fingerprint mismatch between security (%s) and architecture (%s)", fp1, fp2)
	}
	if fp1 != fp3 {
		t.Errorf("Fingerprint mismatch between security (%s) and style (%s)", fp1, fp3)
	}
	if fp1 != fp4 {
		t.Errorf("Fingerprint mismatch between security (%s) and empty category (%s)", fp1, fp4)
	}

	if f4.NormalizedCategory() != CategorySecurity {
		t.Errorf("Expected empty category to normalize to %s, got %s", CategorySecurity, f4.NormalizedCategory())
	}
	if NormalizeCategory("unknown-cat") != CategorySecurity {
		t.Errorf("Expected invalid category to normalize to %s, got %s", CategorySecurity, NormalizeCategory("unknown-cat"))
	}
	if NormalizeCategory(CategoryAccessibility) != CategoryAccessibility {
		t.Errorf("Expected valid category to remain %s, got %s", CategoryAccessibility, NormalizeCategory(CategoryAccessibility))
	}
}
