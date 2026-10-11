package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDrivers_OfflineCommands(t *testing.T) {
	gr := &GradleDriver{}
	goDrv := &GoDriver{}
	cargo := &CargoDriver{}
	npm := &NpmDriver{}

	tmpDir := t.TempDir()

	// 1. Gradle driver test & compile commands must contain --offline
	gradleCompile := gr.CompileCmd(tmpDir)
	if !strings.Contains(gradleCompile, "--offline") {
		t.Errorf("expected Gradle compile command to contain '--offline', got: %s", gradleCompile)
	}
	gradleTest := gr.TestCmd(tmpDir)
	if !strings.Contains(gradleTest, "--offline") {
		t.Errorf("expected Gradle test command to contain '--offline', got: %s", gradleTest)
	}

	// 2. Go driver commands must contain GOPROXY=off and GOFLAGS=-mod=mod
	goCompile := goDrv.CompileCmd(tmpDir)
	if !strings.Contains(goCompile, "GOPROXY=off") || !strings.Contains(goCompile, "GOFLAGS=-mod=mod") {
		t.Errorf("expected Go compile command to contain 'GOPROXY=off' and 'GOFLAGS=-mod=mod', got: %s", goCompile)
	}
	goTest := goDrv.TestCmd(tmpDir)
	if !strings.Contains(goTest, "GOPROXY=off") || !strings.Contains(goTest, "GOFLAGS=-mod=mod") {
		t.Errorf("expected Go test command to contain 'GOPROXY=off' and 'GOFLAGS=-mod=mod', got: %s", goTest)
	}

	// 3. Cargo driver commands must contain --offline
	cargoCompile := cargo.CompileCmd(tmpDir)
	if !strings.Contains(cargoCompile, "--offline") {
		t.Errorf("expected Cargo compile command to contain '--offline', got: %s", cargoCompile)
	}
	cargoTest := cargo.TestCmd(tmpDir)
	if !strings.Contains(cargoTest, "--offline") {
		t.Errorf("expected Cargo test command to contain '--offline', got: %s", cargoTest)
	}

	// 4. Npm driver commands must contain --offline
	npmCompile := npm.CompileCmd(tmpDir)
	if !strings.Contains(npmCompile, "--offline") {
		t.Errorf("expected npm compile command to contain '--offline', got: %s", npmCompile)
	}
	npmTest := npm.TestCmd(tmpDir)
	if !strings.Contains(npmTest, "--offline") {
		t.Errorf("expected npm test command to contain '--offline', got: %s", npmTest)
	}
}

func TestOfflineCacheMiss_Extraction(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "Gradle could not resolve",
			output:   "> Could not resolve com.google.guava:guava:30.1-jre.\n  Required by:\n      project :app",
			expected: "com.google.guava:guava:30.1-jre",
		},
		{
			name:     "Gradle offline cached version missing",
			output:   "FAILURE: Build failed with an exception.\n* What went wrong:\nNo cached version of org.jetbrains.kotlin:kotlin-stdlib:2.0.0 available for offline mode.",
			expected: "org.jetbrains.kotlin:kotlin-stdlib:2.0.0",
		},
		{
			name:     "Go module goproxy off",
			output:   "go: github.com/stretchr/testify@v1.8.4: reading github.com/stretchr/testify/go.mod: GOPROXY=off",
			expected: "github.com/stretchr/testify@v1.8.4",
		},
		{
			name:     "Go cannot find module",
			output:   "main.go:4:2: cannot find module providing package rsc.io/quote: import lookup disabled by -mod=mod",
			expected: "rsc.io/quote",
		},
		{
			name:     "Cargo requirement offline",
			output:   "error: failed to select a version for the requirement `serde = \"^1.0\"`\n  candidate versions found which didn't match: 0.9.0\n  perhaps try running without --offline",
			expected: "serde",
		},
		{
			name:     "Npm enotcached",
			output:   "npm ERR! code ENOTCACHED\nnpm ERR! request to https://registry.npmjs.org/express failed: package express not in cache",
			expected: "express",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, ok := DetectOfflineCacheMiss(tc.output)
			if !ok {
				t.Fatalf("expected offline cache miss detected for %s", tc.name)
			}
			if !strings.Contains(artifact, tc.expected) {
				t.Errorf("expected artifact containing %q, got %q", tc.expected, artifact)
			}
		})
	}
}

func TestWarm_InterfaceAndExecution(t *testing.T) {
	tmpDir := t.TempDir()
	gradlew := filepath.Join(tmpDir, "gradlew")
	script := "#!/bin/sh\necho \"warming dependencies $1 $2 $3\" > warm.log\nexit 0\n"
	if err := os.WriteFile(gradlew, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create fake gradlew: %v", err)
	}

	gr := &GradleDriver{}
	ctx := context.Background()
	if err := gr.Warm(ctx, tmpDir); err != nil {
		t.Fatalf("gr.Warm failed: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(tmpDir, "warm.log"))
	if err != nil {
		t.Fatalf("failed to read warm.log: %v", err)
	}
	if !strings.Contains(string(logData), "--refresh-dependencies") {
		t.Errorf("expected warm execution with --refresh-dependencies, got: %s", string(logData))
	}
}
