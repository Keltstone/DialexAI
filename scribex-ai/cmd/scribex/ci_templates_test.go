package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var allowedRunContexts = []string{
	"runner.temp",
	"github.workspace",
	"github.action_path",
}

func isAllowedRunExpr(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	for _, allowed := range allowedRunContexts {
		if trimmed == allowed || strings.HasPrefix(trimmed, allowed+".") {
			return true
		}
	}
	return false
}

func checkNoIllegalRunInterpolation(t *testing.T, filename, content string) {
	lines := strings.Split(content, "\n")
	inRun := false
	runIndent := 0
	exprRe := regexp.MustCompile(`\$\{\{\s*([^}]+?)\s*\}\}`)

	for lineNo, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if inRun {
			if trimmed == "" {
				continue
			}
			if indent <= runIndent && !strings.HasPrefix(trimmed, "-") {
				inRun = false
			}
		}

		if strings.HasPrefix(trimmed, "run:") {
			inRun = true
			runIndent = indent
			// Check single-line run:
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "run:"))
			if val != "" && val != "|" && val != ">" {
				matches := exprRe.FindAllStringSubmatch(val, -1)
				for _, m := range matches {
					if !isAllowedRunExpr(m[1]) {
						t.Errorf("%s:%d: illegal ${{ %s }} interpolation inside run block", filename, lineNo+1, m[1])
					}
				}
			}
			continue
		}

		if inRun {
			matches := exprRe.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				if !isAllowedRunExpr(m[1]) {
					t.Errorf("%s:%d: illegal ${{ %s }} interpolation inside run block", filename, lineNo+1, m[1])
				}
			}
		}
	}
}

func checkPinnedUses(t *testing.T, filename, content string) {
	lines := strings.Split(content, "\n")
	usesRe := regexp.MustCompile(`uses:\s*['"]?([^\s'"]+)['"]?`)
	shaRe := regexp.MustCompile(`@[0-9a-f]{40}$`)

	for lineNo, line := range lines {
		trimmed := strings.TrimSpace(line)
		// strip trailing comments
		code := trimmed
		if idx := strings.Index(code, "#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if m := usesRe.FindStringSubmatch(code); len(m) > 1 {
			target := m[1]
			// Local action uses (./...) are allowed
			if strings.HasPrefix(target, "./") {
				continue
			}
			if !shaRe.MatchString(target) {
				t.Errorf("%s:%d: unpinned uses target: %q (must match @[0-9a-f]{40})", filename, lineNo+1, target)
			}
		}
	}
}

func TestActionYAMLHardening(t *testing.T) {
	path := filepath.Join("..", "..", "action", "action.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read action.yml: %v", err)
	}
	content := string(b)

	checkPinnedUses(t, "action.yml", content)
	checkNoIllegalRunInterpolation(t, "action.yml", content)

	if strings.Contains(content, "pull_request_target") {
		t.Errorf("action.yml must never use pull_request_target")
	}
	if strings.Contains(content, "@latest") {
		t.Errorf("action.yml must not use @latest")
	}
	if strings.Contains(content, "go-version: stable") {
		t.Errorf("action.yml must not use go-version: stable")
	}
}

func TestWorkflowTemplatesHardening(t *testing.T) {
	tmp := t.TempDir()
	fVal := false
	force = &fVal

	// Test init github
	err := ciCmd(tmp, []string{"init", "github"})
	if err != nil {
		t.Fatalf("ci init github failed: %v", err)
	}

	prPath := filepath.Join(tmp, ".github", "workflows", "scribex-pr.yml")
	docsPath := filepath.Join(tmp, ".github", "workflows", "scribex-docs.yml")
	scavengePath := filepath.Join(tmp, ".github", "workflows", "scribex-scavenge.yml")

	for _, p := range []string{prPath, docsPath, scavengePath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected generated workflow missing: %s", p)
		}
	}

	workflows := map[string]string{}
	for _, p := range []string{prPath, docsPath, scavengePath} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(tmp, p)
		workflows[rel] = string(b)
	}

	for rel, content := range workflows {
		checkPinnedUses(t, rel, content)
		checkNoIllegalRunInterpolation(t, rel, content)

		if strings.Contains(content, "pull_request_target") {
			t.Errorf("%s contains pull_request_target", rel)
		}
		if strings.Contains(content, "@latest") {
			t.Errorf("%s contains @latest", rel)
		}
		if strings.Contains(content, "golang:latest") {
			t.Errorf("%s contains golang:latest", rel)
		}
		if strings.Contains(content, "go-version: stable") {
			t.Errorf("%s contains go-version: stable", rel)
		}
	}

	// Assert scribex-pr.yml specific rules:
	// - permissions: {} at top-level
	// - no secrets. reference
	if prContent, ok := workflows[filepath.Join(".github", "workflows", "scribex-pr.yml")]; ok {
		if strings.Contains(prContent, "secrets.") {
			t.Errorf("scribex-pr.yml must contain no secrets. references")
		}
		if !strings.Contains(prContent, "permissions: {}") {
			t.Errorf("scribex-pr.yml must have top-level permissions: {}")
		}
		if strings.Contains(prContent, "contents: write") {
			t.Errorf("scribex-pr.yml must not contain contents: write")
		}
	}

	// Assert scribex-docs.yml specific rules:
	// - only workflow referencing ANTHROPIC_API_KEY/ATLASSIAN_TOKEN, under environment:
	// - contents: write only appears here
	// - git push targets refs/heads/scribex/
	if docsContent, ok := workflows[filepath.Join(".github", "workflows", "scribex-docs.yml")]; ok {
		if !strings.Contains(docsContent, "environment: scribex-ai") {
			t.Errorf("scribex-docs.yml must specify environment: scribex-ai")
		}
		if !strings.Contains(docsContent, "contents: write") {
			t.Errorf("scribex-docs.yml must have contents: write")
		}
		if !strings.Contains(docsContent, "refs/heads/scribex/") {
			t.Errorf("scribex-docs.yml git push must target refs/heads/scribex/")
		}
	}

	// Assert scavenge specific rules:
	if scavengeContent, ok := workflows[filepath.Join(".github", "workflows", "scribex-scavenge.yml")]; ok {
		if strings.Contains(scavengeContent, "contents: write") {
			t.Errorf("scribex-scavenge.yml must not have contents: write")
		}
		if strings.Contains(scavengeContent, "secrets.") {
			t.Errorf("scribex-scavenge.yml must contain no secrets. references")
		}
	}

	// Test init gitlab
	err = ciCmd(tmp, []string{"init", "gitlab"})
	if err != nil {
		t.Fatalf("ci init gitlab failed: %v", err)
	}
	glPath := filepath.Join(tmp, ".gitlab-ci.scribex.yml")
	glBytes, err := os.ReadFile(glPath)
	if err != nil {
		t.Fatalf("read gitlab ci file: %v", err)
	}
	glContent := string(glBytes)
	if strings.Contains(glContent, "golang:latest") {
		t.Errorf("gitlab ci contains golang:latest")
	}
	if strings.Contains(glContent, "@latest") {
		t.Errorf("gitlab ci contains @latest")
	}
	if !strings.Contains(glContent, "golang:1.27.0@sha256:") {
		t.Errorf("gitlab ci must use pinned golang:1.27.0@sha256:<digest>")
	}
}

func TestCIInitSymlinkAndForceGuards(t *testing.T) {
	tmp := t.TempDir()
	fVal := false
	force = &fVal

	// First init should succeed
	if err := ciCmd(tmp, []string{"init", "github"}); err != nil {
		t.Fatalf("first init github should succeed: %v", err)
	}

	// Second init without -force must fail
	if err := ciCmd(tmp, []string{"init", "github"}); err == nil {
		t.Errorf("second init without -force should fail")
	}

	// With -force it should succeed
	tVal := true
	force = &tVal
	if err := ciCmd(tmp, []string{"init", "github"}); err != nil {
		t.Fatalf("init with -force should succeed: %v", err)
	}

	// Refuse symlinked .github directory
	tmp2 := t.TempDir()
	outside := filepath.Join(tmp2, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(tmp2, ".github")); err != nil {
		t.Fatal(err)
	}
	if err := ciCmd(tmp2, []string{"init", "github"}); err == nil {
		t.Errorf("expected error when .github is a symlink")
	}
}

func TestActionlintExecution(t *testing.T) {
	actionlintPath, err := exec.LookPath("actionlint")
	if err != nil {
		t.Skip("actionlint not installed on PATH; skipping live actionlint run")
	}

	tmp := t.TempDir()
	tVal := true
	force = &tVal
	if err := ciCmd(tmp, []string{"init", "github"}); err != nil {
		t.Fatalf("ci init github failed: %v", err)
	}

	cmd := exec.Command(actionlintPath,
		filepath.Join(tmp, ".github", "workflows", "scribex-pr.yml"),
		filepath.Join(tmp, ".github", "workflows", "scribex-docs.yml"),
		filepath.Join(tmp, ".github", "workflows", "scribex-scavenge.yml"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("actionlint reported errors:\n%s", string(out))
	}
}

func TestCheckReportPlumbing(t *testing.T) {
	tmp := t.TempDir()
	docFile := filepath.Join(tmp, "doc.md")
	if err := os.WriteFile(docFile, []byte("# Test Doc\n<!-- @scribex:anchor file=main.go sym=Foo -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reportFile := filepath.Join(tmp, "report.md")

	_ = check(tmp, "", reportFile)
	b, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("expected report file to be written: %v", err)
	}
	if !strings.Contains(string(b), "Scribex") {
		t.Errorf("expected report content to contain Scribex, got: %s", string(b))
	}
}
