package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestP1_11_KMP_Prewarm_CoversNativeAndIOSTasks(t *testing.T) {
	// Pre-fix: GradleDriver.Warm only resolved testClasses or missed native task set that TestCmd will run.
	// Fix: warm with exact task set TestCmd runs, including downloadKotlinNativeDistribution and target klib resolutions.

	tmpDir := t.TempDir()
	buildKts := `
plugins {
    kotlin("multiplatform") version "2.2.20"
}
kotlin {
    jvm()
    iosSimulatorArm64()
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildKts), 0644); err != nil {
		t.Fatalf("failed to write build.gradle.kts: %v", err)
	}

	// Create fake gradlew that emits tasks including downloadKotlinNativeDistribution and iosSimulatorArm64Test
	gradlewScript := `#!/bin/sh
if [ "$1" = "tasks" ]; then
    echo "downloadKotlinNativeDistribution - Downloads Kotlin/Native prebuilt bundle"
    echo "jvmTest - Runs JVM tests"
    echo "iosSimulatorArm64Test - Runs iOS simulator tests"
    echo "compileTestKotlinIosSimulatorArm64 - Compiles iOS simulator test sources and resolves klibs"
    echo "linkDebugTestIosSimulatorArm64 - Links iOS simulator test binary"
    exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(tmpDir, "gradlew"), []byte(gradlewScript), 0755); err != nil {
		t.Fatalf("failed to write gradlew: %v", err)
	}

	gr := &GradleDriver{}
	ctx := context.Background()

	warmTasks := gr.ResolveTasksToWarm(ctx, tmpDir)

	hasJvmTest := false
	hasIosSimulatorArm64Test := false
	hasKlibResolution := false
	hasDownloadNative := false

	for _, task := range warmTasks {
		if task == "jvmTest" {
			hasJvmTest = true
		}
		if task == "iosSimulatorArm64Test" {
			hasIosSimulatorArm64Test = true
		}
		if task == "compileTestKotlinIosSimulatorArm64" || task == "linkDebugTestIosSimulatorArm64" {
			hasKlibResolution = true
		}
		if task == "downloadKotlinNativeDistribution" {
			hasDownloadNative = true
		}
	}

	if !hasJvmTest {
		t.Fatalf("ResolveTasksToWarm must include TestCmd task 'jvmTest'. Got tasks: %v", warmTasks)
	}
	if !hasIosSimulatorArm64Test {
		t.Fatalf("ResolveTasksToWarm must include TestCmd task 'iosSimulatorArm64Test'. Got tasks: %v", warmTasks)
	}
	if !hasDownloadNative {
		t.Fatalf("ResolveTasksToWarm missing 'downloadKotlinNativeDistribution'. Got tasks: %v", warmTasks)
	}
	if !hasKlibResolution {
		t.Fatalf("ResolveTasksToWarm missing target klib resolution task. Got tasks: %v", warmTasks)
	}
}
