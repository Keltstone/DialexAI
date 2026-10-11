package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGradleDriver_MultiModuleDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Only settings.gradle.kts and nested app/build.gradle.kts, no root build.gradle
	_ = os.WriteFile(filepath.Join(tmpDir, "settings.gradle.kts"), []byte("include(\":app\")\n"), 0644)
	appDir := filepath.Join(tmpDir, "app")
	_ = os.MkdirAll(appDir, 0755)
	_ = os.WriteFile(filepath.Join(appDir, "build.gradle.kts"), []byte("plugins { kotlin(\"multiplatform\") }\n"), 0644)

	gr := &GradleDriver{}
	if !gr.Detect(tmpDir) {
		t.Fatalf("expected GradleDriver.Detect to return true for multi-module project with settings.gradle.kts and subproject")
	}

	if !gr.IsKMP(tmpDir) {
		t.Fatalf("expected GradleDriver.IsKMP to return true for kotlin(\"multiplatform\") project")
	}
}

func TestGradleDriver_NoNoDaemonFlag(t *testing.T) {
	gr := &GradleDriver{}
	tmpDir := t.TempDir()

	compileCmd := gr.CompileCmd(tmpDir)
	if strings.Contains(compileCmd, "--no-daemon") {
		t.Errorf("expected CompileCmd to drop '--no-daemon', got: %s", compileCmd)
	}

	testCmd := gr.TestCmd(tmpDir)
	if strings.Contains(testCmd, "--no-daemon") {
		t.Errorf("expected TestCmd to drop '--no-daemon', got: %s", testCmd)
	}

	lintCmd := gr.LintCmd(tmpDir)
	if strings.Contains(lintCmd, "--no-daemon") {
		t.Errorf("expected LintCmd to drop '--no-daemon', got: %s", lintCmd)
	}
}

func TestGradleDriver_KMPTaskSelectionAndLinuxSkipped(t *testing.T) {
	tmpDir := t.TempDir()

	// Create KMP project structure
	_ = os.WriteFile(filepath.Join(tmpDir, "settings.gradle.kts"), []byte("rootProject.name = \"kmp-sample\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(`
plugins {
    kotlin("multiplatform")
}
`), 0644)
	srcCommon := filepath.Join(tmpDir, "src", "commonMain")
	srcAndroid := filepath.Join(tmpDir, "src", "androidMain")
	srcIos := filepath.Join(tmpDir, "src", "iosMain")
	_ = os.MkdirAll(srcCommon, 0755)
	_ = os.MkdirAll(srcAndroid, 0755)
	_ = os.MkdirAll(srcIos, 0755)

	// Fake gradlew tasks --all output
	gradlew := filepath.Join(tmpDir, "gradlew")
	script := `#!/bin/sh
if [ "$1" = "tasks" ]; then
  cat << 'EOF'
jvmTest - Runs JVM tests
desktopTest - Runs Desktop tests
testDebugUnitTest - Runs Android unit tests
iosSimulatorArm64Test - Runs iOS simulator tests
EOF
  exit 0
fi
echo "running test tasks $@"
exit 0
`
	if err := os.WriteFile(gradlew, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write gradlew: %v", err)
	}

	gr := &GradleDriver{}
	ctx := context.Background()

	// Query selected tasks and skipped targets
	selectedTasks, skippedTargets, err := gr.ResolveKMPTasks(ctx, tmpDir)
	if err != nil {
		t.Fatalf("ResolveKMPTasks failed: %v", err)
	}
	_ = skippedTargets

	// jvmTest, desktopTest, testDebugUnitTest must be selected
	tasksStr := strings.Join(selectedTasks, " ")
	if !strings.Contains(tasksStr, "jvmTest") {
		t.Errorf("expected jvmTest in selected tasks: %v", selectedTasks)
	}
	if !strings.Contains(tasksStr, "desktopTest") {
		t.Errorf("expected desktopTest in selected tasks: %v", selectedTasks)
	}
	if !strings.Contains(tasksStr, "testDebugUnitTest") {
		t.Errorf("expected testDebugUnitTest in selected tasks: %v", selectedTasks)
	}

	// Test host behavior:
	// When simulating Linux or on Linux host, iosSimulatorArm64Test must be in skippedTargets with SKIPPED_HOST_UNSUPPORTED
	skippedMap := gr.EvaluateKMPTargetsForHost("linux", []string{"jvmTest", "desktopTest", "testDebugUnitTest", "iosSimulatorArm64Test"})
	if status, ok := skippedMap["iosSimulatorArm64Test"]; !ok || status != "SKIPPED_HOST_UNSUPPORTED" {
		t.Errorf("expected iosSimulatorArm64Test to be SKIPPED_HOST_UNSUPPORTED on linux, got: %s", status)
	}

	// Verify that the story is not reported 'all targets green' when targets are skipped
	verdict := gr.FormatTargetStatusSummary([]string{"jvmTest", "desktopTest", "testDebugUnitTest"}, skippedMap)
	if strings.Contains(strings.ToLower(verdict), "all targets green") {
		t.Errorf("story must not be reported 'all targets green' when targets are skipped: %s", verdict)
	}
	if !strings.Contains(verdict, "SKIPPED_HOST_UNSUPPORTED") {
		t.Errorf("expected summary to state SKIPPED_HOST_UNSUPPORTED, got: %s", verdict)
	}
}
