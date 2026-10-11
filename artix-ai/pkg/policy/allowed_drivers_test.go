package policy

import (
	"strings"
	"testing"
)

func TestPolicy_DefaultAllowedDrivers(t *testing.T) {
	pol := Active()
	allowed := pol.GetAllowedDrivers()

	if len(allowed) != 2 || allowed[0] != "go" || allowed[1] != "gradle" {
		t.Errorf("expected default allowedDrivers to be [go, gradle], got: %v", allowed)
	}

	if !pol.IsDriverAllowed("go") {
		t.Errorf("expected 'go' to be allowed by default")
	}
	if !pol.IsDriverAllowed("gradle") {
		t.Errorf("expected 'gradle' to be allowed by default")
	}
	if pol.IsDriverAllowed("cargo") {
		t.Errorf("expected 'cargo' to be disallowed by default")
	}
	if pol.IsDriverAllowed("npm") {
		t.Errorf("expected 'npm' to be disallowed by default")
	}
}

func TestPolicy_ValidateDriverAllowed_RefusesUnallowedDriver(t *testing.T) {
	pol := Active()
	err := pol.ValidateDriverAllowed("cargo")
	if err == nil {
		t.Fatalf("expected ValidateDriverAllowed('cargo') to fail")
	}

	if !strings.Contains(err.Error(), "cargo") || !strings.Contains(err.Error(), "allowedDrivers") {
		t.Errorf("expected error to name 'cargo' and 'allowedDrivers', got: %v", err)
	}
}
