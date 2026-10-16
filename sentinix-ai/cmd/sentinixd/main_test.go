package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEnterpriseMode_RefusesStartupWithoutSandbox(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "-enterprise", "-sandbox-image=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected command to fail without -sandbox-image in enterprise mode, but succeeded: %s", out)
	}
	if !strings.Contains(string(out), "enterprise mode requires -sandbox-image") {
		t.Errorf("expected error message about -sandbox-image, got: %s", out)
	}
}
