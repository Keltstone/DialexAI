package plugins

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewRegistry_ContainsSwiftAndXcode(t *testing.T) {
	reg := NewRegistry()
	var names []string
	for _, d := range reg.drivers {
		names = append(names, d.Name())
	}
	all := strings.Join(names, " ")
	if !strings.Contains(all, "swift") && !strings.Contains(all, "swiftpm") {
		t.Errorf("expected SwiftPMDriver in NewRegistry, got: %v", names)
	}
	if !strings.Contains(all, "xcode") {
		t.Errorf("expected XcodeDriver in NewRegistry, got: %v", names)
	}
}

func TestSwiftPMDriver_DetectionAndDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	pkgSwift := filepath.Join(tmpDir, "Package.swift")
	_ = os.WriteFile(pkgSwift, []byte("// swift-tools-version: 5.9\nimport PackageDescription\nlet package = Package(name: \"MyPkg\")\n"), 0644)

	d := &SwiftPMDriver{}
	if !d.Detect(tmpDir) {
		t.Fatalf("expected SwiftPMDriver.Detect to match Package.swift")
	}

	compileCmd := d.CompileCmd(tmpDir)
	if !strings.Contains(compileCmd, "swift build") {
		t.Errorf("expected CompileCmd to contain 'swift build', got: %s", compileCmd)
	}
	testCmd := d.TestCmd(tmpDir)
	if !strings.Contains(testCmd, "swift test") {
		t.Errorf("expected TestCmd to contain 'swift test', got: %s", testCmd)
	}

	// Test diagnostic parsing
	output := `/Users/dev/MyPkg/Sources/MyPkg.swift:15:10: error: cannot find 'InvalidSymbol' in scope
/Users/dev/MyPkg/Tests/MyPkgTests.swift:22: warning: variable 'unused' was never mutated
`
	diags := d.ParseErrorTrace(output)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].Line != 15 || diags[0].Column != 10 || diags[0].Severity != SeverityError || !strings.Contains(diags[0].Message, "cannot find 'InvalidSymbol'") {
		t.Errorf("unexpected first diagnostic: %+v", diags[0])
	}
	if diags[1].Line != 22 || diags[1].Severity != SeverityWarning {
		t.Errorf("unexpected second diagnostic: %+v", diags[1])
	}

	// If on macOS and swift exists, test actual run or simulated run
	if runtime.GOOS == "darwin" {
		res := d.Test(context.Background(), tmpDir)
		if res == nil {
			t.Errorf("expected non-nil ExecResult from Test")
		}
	}
}

func TestXcodeDriver_DetectionAndCommandBuilding(t *testing.T) {
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "SampleApp.xcodeproj")
	_ = os.MkdirAll(projDir, 0755)

	d := &XcodeDriver{}
	if !d.Detect(tmpDir) {
		t.Fatalf("expected XcodeDriver.Detect to match *.xcodeproj")
	}

	scheme := d.DetectScheme(tmpDir)
	if scheme != "SampleApp" {
		t.Errorf("expected detected scheme 'SampleApp', got: %q", scheme)
	}

	testCmd := d.TestCmd(tmpDir)
	if !strings.Contains(testCmd, "xcodebuild test") {
		t.Errorf("expected 'xcodebuild test' in testCmd, got: %s", testCmd)
	}
	if !strings.Contains(testCmd, "-scheme SampleApp") {
		t.Errorf("expected '-scheme SampleApp' in testCmd, got: %s", testCmd)
	}
	if !strings.Contains(testCmd, "-destination") || !strings.Contains(testCmd, "platform=iOS Simulator") {
		t.Errorf("expected platform=iOS Simulator destination in testCmd, got: %s", testCmd)
	}
	if !strings.Contains(testCmd, "-derivedDataPath") {
		t.Errorf("expected -derivedDataPath in testCmd, got: %s", testCmd)
	}
}

func TestSwiftPMAndXcode_SkippedOnNonDarwin(t *testing.T) {
	swiftDrv := &SwiftPMDriver{}
	xcodeDrv := &XcodeDriver{}
	tmpDir := t.TempDir()

	resSwift := swiftDrv.ExecuteOnHost("linux", context.Background(), tmpDir, "test")
	if !strings.Contains(resSwift.Stdout, "SKIPPED_HOST_UNSUPPORTED") && !strings.Contains(resSwift.Error, "SKIPPED_HOST_UNSUPPORTED") {
		t.Errorf("expected SKIPPED_HOST_UNSUPPORTED for swift on linux, got stdout=%s err=%s", resSwift.Stdout, resSwift.Error)
	}

	resXcode := xcodeDrv.ExecuteOnHost("linux", context.Background(), tmpDir, "test")
	if !strings.Contains(resXcode.Stdout, "SKIPPED_HOST_UNSUPPORTED") && !strings.Contains(resXcode.Error, "SKIPPED_HOST_UNSUPPORTED") {
		t.Errorf("expected SKIPPED_HOST_UNSUPPORTED for xcode on linux, got stdout=%s err=%s", resXcode.Stdout, resXcode.Error)
	}
}
