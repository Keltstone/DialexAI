package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// installScript extracts the run: block of the "Install scribex" step from a generated workflow.
func installScript(t *testing.T, yml string) string {
	t.Helper()
	lines := strings.Split(yml, "\n")
	for i, l := range lines {
		if !strings.Contains(l, "- name: Install scribex") {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) != "run: |" {
				continue
			}
			base := len(lines[j]) - len(strings.TrimLeft(lines[j], " "))
			var out []string
			for k := j + 1; k < len(lines); k++ {
				ln := lines[k]
				if strings.TrimSpace(ln) != "" && len(ln)-len(strings.TrimLeft(ln, " ")) <= base {
					break
				}
				if len(ln) > base+2 {
					out = append(out, ln[base+2:])
				} else {
					out = append(out, "")
				}
			}
			return strings.Join(out, "\n")
		}
	}
	t.Fatal("Install scribex step not found")
	return ""
}

func genWorkflows(t *testing.T) map[string]string {
	t.Helper()
	tmp := t.TempDir()
	f := false
	force = &f
	if err := ciCmd(tmp, []string{"init", "github"}); err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, n := range []string{"scribex-pr.yml", "scribex-docs.yml", "scribex-scavenge.yml", "scribex-generate.yml"} {
		b, err := os.ReadFile(filepath.Join(tmp, ".github", "workflows", n))
		if err != nil {
			t.Fatal(err)
		}
		m[n] = string(b)
	}
	return m
}

func TestWorkflowsInstallPinnedBinary(t *testing.T) {
	for name, y := range genWorkflows(t) {
		if !strings.Contains(y, "sha256sum -c") {
			t.Errorf("%s: install must verify sha256", name)
		}
		if !strings.Contains(y, "SCRIBEX_RELEASE_URL: ${{ vars.SCRIBEX_RELEASE_URL }}") {
			t.Errorf("%s: URL must come from vars via env", name)
		}
		if strings.Contains(y, "@@INSTALL@@") {
			t.Errorf("%s: unreplaced placeholder", name)
		}
		// every go build of ./cmd/scribex must sit inside the source-checkout guard
		for _, blk := range strings.Split(y, "- name:")[1:] {
			if strings.Contains(blk, "./cmd/scribex") && !strings.Contains(blk, "grep -qx 'module scribex'") {
				t.Errorf("%s: unguarded go build of ./cmd/scribex", name)
			}
		}
	}
	if n := strings.Count(genWorkflows(t)["scribex-pr.yml"], "- name: Install scribex"); n != 2 {
		t.Errorf("pr workflow should install in check and comment jobs, got %d", n)
	}
}

func runInstall(t *testing.T, script, dir string, env []string) (string, int) {
	t.Helper()
	rt := t.TempDir()
	gp := filepath.Join(rt, "gh_path")
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "RUNNER_TEMP=" + rt, "GITHUB_PATH=" + gp}, env...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

func TestInstallStepFailsClosed(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	script := installScript(t, genWorkflows(t)["scribex-pr.yml"])
	good := "a" + strings.Repeat("b", 63)
	for _, c := range []struct {
		name string
		env  []string
	}{
		{"unset", nil},
		{"http url", []string{"SCRIBEX_RELEASE_URL=http://x/y", "SCRIBEX_SHA256=" + good}},
		{"short sha", []string{"SCRIBEX_RELEASE_URL=https://x/y", "SCRIBEX_SHA256=abc"}},
		{"upper sha", []string{"SCRIBEX_RELEASE_URL=https://x/y", "SCRIBEX_SHA256=" + strings.ToUpper(good)}},
		{"space url", []string{"SCRIBEX_RELEASE_URL=https://x/y z", "SCRIBEX_SHA256=" + good}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, code := runInstall(t, script, t.TempDir(), c.env)
			if code != 2 || !strings.Contains(out, "set repository variables SCRIBEX_RELEASE_URL and SCRIBEX_SHA256") {
				t.Errorf("want exit 2 + message, got %d: %s", code, out)
			}
		})
	}
}

func TestInstallStepSourceBranch(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	script := installScript(t, genWorkflows(t)["scribex-pr.yml"])
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "scribex"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "cmd", "scribex", "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module scribex\n\ngo 1.27\n"), 0o644)
	stub := t.TempDir()
	rec := filepath.Join(stub, "args")
	os.WriteFile(filepath.Join(stub, "go"), []byte("#!/bin/sh\necho \"$@\" > "+rec+"\n"), 0o755)
	rt := t.TempDir()
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = repo
	cmd.Env = []string{"PATH=" + stub + ":" + os.Getenv("PATH"), "RUNNER_TEMP=" + rt, "GITHUB_PATH=" + filepath.Join(rt, "gp")}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("source branch failed: %v\n%s", err, out)
	}
	b, _ := os.ReadFile(rec)
	if !strings.Contains(string(b), "build") || !strings.Contains(string(b), "./cmd/scribex") {
		t.Errorf("stub go not called with build: %q", b)
	}
}
