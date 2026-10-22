package engines

import (
	"errors"
	"strings"
	"testing"
)

func TestParseJUnit_Valid(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.ArchitectureTest" tests="2" failures="1" errors="0">
    <testcase name="domain layer has no android imports" classname="com.example.ArchitectureTest" time="0.045">
        <failure message="Domain layer violates rule" type="java.lang.AssertionError">
            at com.example.ArchitectureTest(DomainRules.kt:25)
            at org.junit.runners.ParentRunner.run(ParentRunner.java:363)
        </failure>
    </testcase>
    <testcase name="viewmodel depends on usecase" classname="com.example.ArchitectureTest" time="0.012">
    </testcase>
</testsuite>`

	fs, err := ParseJUnit("konsist", []byte(xmlData), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}

	f := fs[0]
	if f.Category != CategoryArchitecture {
		t.Errorf("expected category architecture, got %q", f.Category)
	}
	if f.Severity != High {
		t.Errorf("expected severity High, got %v", f.Severity)
	}
	if f.Rule != "com.example.ArchitectureTest.domain layer has no android imports" {
		t.Errorf("unexpected rule: %q", f.Rule)
	}
	if f.File != "DomainRules.kt" {
		t.Errorf("expected file DomainRules.kt, got %q", f.File)
	}
	if f.Line != 25 {
		t.Errorf("expected line 25, got %d", f.Line)
	}
}

func TestParseJUnit_Redaction(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="SecretLeakTest" tests="1" failures="1">
    <testcase name="testToken" classname="SecretLeakTest">
        <failure message="Found leaked AWS key AKIAIOSFODNN7EXAMPLE in config" type="AssertionError">
            Stack trace with secret: AKIAIOSFODNN7EXAMPLE
        </failure>
    </testcase>
</testsuite>`

	fs, err := ParseJUnit("konsist", []byte(xmlData), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	f := fs[0]
	if strings.Contains(f.Message, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("expected secret to be redacted in message, got %q", f.Message)
	}
	if strings.Contains(f.Evidence, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("expected secret to be redacted in evidence, got %q", f.Evidence)
	}
}

func TestParseJUnit_PathTraversal(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TraversalTest" tests="1" failures="1">
    <testcase name="testTraversal" classname="TraversalTest">
        <failure message="bad path">
            at com.example.Test(../../../../etc/passwd:1)
        </failure>
    </testcase>
</testsuite>`

	fs, err := ParseJUnit("konsist", []byte(xmlData), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	if fs[0].File != "" || fs[0].Line != 0 {
		t.Errorf("expected hostile path to be dropped (file='', line=0), got file=%q line=%d", fs[0].File, fs[0].Line)
	}
}

func TestParseJUnit_Oversized(t *testing.T) {
	largeData := make([]byte, MaxJUnitSize+100)
	_, err := ParseJUnit("konsist", largeData, "")
	if err == nil {
		t.Fatalf("expected oversized error, got nil")
	}
	if !errors.Is(err, ErrOversized) {
		t.Errorf("expected ErrOversized, got: %v", err)
	}
}

func TestParseJUnit_Malformed(t *testing.T) {
	malformed := []byte(`not valid xml at all <<>>`)
	_, err := ParseJUnit("konsist", malformed, "")
	if err == nil {
		t.Fatalf("expected error for malformed xml, got nil")
	}
}
