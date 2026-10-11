package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_Warm_Command(t *testing.T) {
	tempDir := t.TempDir()

	// Fake gradlew in tempDir
	gradlew := filepath.Join(tempDir, "gradlew")
	script := "#!/bin/sh\necho \"warmed gradle\" > warm.log\nexit 0\n"
	if err := os.WriteFile(gradlew, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake gradlew: %v", err)
	}
	_ = os.WriteFile(filepath.Join(tempDir, "build.gradle.kts"), []byte("plugins { java }\n"), 0644)

	var stdout, stderr bytes.Buffer
	code := RunCLIWithIO(tempDir, nil, []string{"warm", tempDir}, os.Stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("artix warm failed with exit code %d, stderr: %s", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Pre-warmed offline dependency cache") {
		t.Errorf("unexpected stdout from artix warm: %s", stdout.String())
	}

	logData, err := os.ReadFile(filepath.Join(tempDir, "warm.log"))
	if err != nil {
		t.Fatalf("failed to read warm.log: %v", err)
	}
	if !strings.Contains(string(logData), "warmed gradle") {
		t.Errorf("expected warm.log to contain 'warmed gradle', got: %s", string(logData))
	}
}
