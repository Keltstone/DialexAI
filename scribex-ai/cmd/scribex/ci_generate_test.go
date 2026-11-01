package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateWorkflowHardening(t *testing.T) {
	y := genWorkflows(t)["scribex-generate.yml"]
	if strings.Contains(y, "@@") {
		t.Fatal("unreplaced placeholder")
	}
	if !strings.Contains(y, "workflow_dispatch:") || strings.Contains(y, "pull_request") || strings.Contains(y, "\n  push:") || strings.Contains(y, "schedule:") {
		t.Fatal("generate must be manual-only")
	}
	if !strings.Contains(y, "environment: scribex-ai") || !strings.HasPrefix(y[strings.Index(y, "permissions:"):], "permissions: {}") {
		t.Fatal("needs protected environment and empty top-level permissions")
	}
	// no ${{ ... }} expression may be interpolated inside a run: script
	inRun := false
	for _, l := range strings.Split(y, "\n") {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "run: |") {
			inRun = true
			continue
		}
		if inRun && trim != "" && !strings.HasPrefix(l, "          ") {
			inRun = false
		}
		if inRun && strings.Contains(l, "${{") {
			t.Errorf("expression interpolated into script: %s", l)
		}
	}
	for _, w := range []string{"-confirm-send", "persist-credentials: true", "gh pr create", "invalid model"} {
		if !strings.Contains(y, w) {
			t.Errorf("missing %q", w)
		}
	}
	if regexp.MustCompile(`uses: [^@\s]+@[^0-9a-f\s]`).MatchString(y) {
		t.Error("action not pinned to a sha")
	}
}

func TestGenerateWorkflowActionlint(t *testing.T) {
	bin, err := exec.LookPath("actionlint")
	if err != nil {
		t.Skip("actionlint not installed")
	}
	tmp := t.TempDir()
	for n, y := range genWorkflows(t) {
		p := filepath.Join(tmp, n)
		os.WriteFile(p, []byte(y), 0o644)
		if out, err := exec.Command(bin, p).CombinedOutput(); err != nil {
			t.Errorf("%s: %s", n, out)
		}
	}
}

func TestGitLabGenerateJobManualOnly(t *testing.T) {
	if strings.Contains(glJob, "@@") {
		t.Fatal("unreplaced placeholder")
	}
	i := strings.Index(glJob, "scribex-generate:")
	if i < 0 {
		t.Fatal("no generate job")
	}
	job := glJob[i:]
	if !strings.Contains(job, `$CI_PIPELINE_SOURCE == "web"`) || !strings.Contains(job, "when: manual") || strings.Contains(job, "merge_request_event") {
		t.Fatal("generate job must be web+manual only")
	}
	if !strings.Contains(job, "sha256sum -c") || !strings.Contains(job, "SCRIBEX_PUSH_TOKEN") {
		t.Fatal("needs verified install and a token guard")
	}
	if strings.Count(glJob, "sha256sum -c") != 2 {
		t.Fatal("both jobs must verify the binary")
	}
}
