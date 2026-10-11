package spec

import (
	"context"
	"strings"
	"testing"

	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
)

func TestCouncilAssemblyAndPlanning(t *testing.T) {
	reg := persona.NewRegistry("")
	council := NewCouncil(reg)

	if len(council.members) != 4 {
		t.Fatalf("expected 4 council members, got %d", len(council.members))
	}

	pCtx := &PlanningContext{
		StoryPrompt: "Add biometric authentication in mobile app",
		Style:       StylePonytail,
		RepoContext: &repo.RepositoryContext{
			DetectedEcos: []repo.Ecosystem{repo.EcosystemGradleKMP},
		},
	}

	spec, err := council.Plan(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("council.Plan failed: %v", err)
	}

	if spec.ID == "" {
		t.Errorf("expected non-empty spec ID")
	}
	if spec.Style != StylePonytail {
		t.Errorf("expected style to be ponytail, got %s", spec.Style)
	}
	if !strings.Contains(spec.UserStory, "Executive Summary") {
		t.Errorf("expected ponytail executive summary, got: %s", spec.UserStory)
	}
	if len(spec.AcceptanceCriteria) < 2 {
		t.Errorf("expected at least 2 acceptance criteria scenarios, got %d", len(spec.AcceptanceCriteria))
	}
	if len(spec.TestCommands) == 0 {
		t.Errorf("expected test commands to be generated")
	}
	if !strings.Contains(spec.TestCommands[0], "gradlew") {
		t.Errorf("expected gradlew test command for Gradle KMP repo, got: %s", spec.TestCommands[0])
	}
}

func TestCavemanStyle(t *testing.T) {
	reg := persona.NewRegistry("")
	council := NewCouncil(reg)

	pCtx := &PlanningContext{
		StoryPrompt: "Fix nil pointer dereference in auth handler",
		Style:       StyleCaveman,
	}

	spec, err := council.Plan(context.Background(), pCtx)
	if err != nil {
		t.Fatalf("council.Plan failed: %v", err)
	}

	if !strings.Contains(spec.UserStory, "Build \"Fix nil pointer dereference in auth handler\". Write tests.") {
		t.Errorf("unexpected caveman user story: %s", spec.UserStory)
	}
}

func TestFormatAndParseRoundtrip(t *testing.T) {
	spec := &StorySpec{
		ID:         "STORY-101",
		Title:      "OAuth2 PKCE Flow",
		UserStory:  "As an engineer, implement PKCE to eliminate client secret leakage.",
		Style:      StyleStandard,
		InScope:    []string{"PKCE code challenge verification", "Unit tests"},
		OutOfScope: []string{"Legacy implicit flow"},
		AcceptanceCriteria: []Scenario{
			{
				Name:  "Successful Code Exchange",
				Given: "a valid authorization code and code verifier",
				When:  "the client calls token endpoint",
				Then:  "tokens are issued with 200 OK",
			},
		},
		Decisions: []ArchitectureDecision{
			{
				ID:           "ADR-01",
				Title:        "Use SHA-256 for code_challenge",
				Context:      "S256 is the RFC 7636 recommended challenge method.",
				Decision:     "Mandate S256; reject plain challenges.",
				Consequences: []string{"Increases crypto safety"},
			},
		},
		FileManifest: []FileMutation{
			{
				Action:    "create",
				Path:      "pkg/auth/pkce.go",
				Rationale: "Implements SHA-256 code verifier calculation",
			},
		},
		TestCommands: []string{"go test -v ./pkg/auth/..."},
	}

	md := FormatToMarkdown(spec)
	if !strings.Contains(md, "# Story Spec: OAuth2 PKCE Flow") {
		t.Fatalf("formatted markdown missing header")
	}

	parsed, err := ParseFromMarkdown(md)
	if err != nil {
		t.Fatalf("ParseFromMarkdown failed: %v", err)
	}

	if parsed.Title != spec.Title {
		t.Errorf("expected title %q, got %q", spec.Title, parsed.Title)
	}
	if parsed.ID != spec.ID {
		t.Errorf("expected ID %q, got %q", spec.ID, parsed.ID)
	}
	if len(parsed.AcceptanceCriteria) != 1 {
		t.Fatalf("expected 1 acceptance criteria scenario, got %d", len(parsed.AcceptanceCriteria))
	}
	if parsed.AcceptanceCriteria[0].Given != spec.AcceptanceCriteria[0].Given {
		t.Errorf("scenario given mismatch")
	}
	if len(parsed.FileManifest) != 1 {
		t.Fatalf("expected 1 file manifest entry, got %d", len(parsed.FileManifest))
	}
	if parsed.FileManifest[0].Path != "pkg/auth/pkce.go" {
		t.Errorf("manifest path mismatch: %s", parsed.FileManifest[0].Path)
	}
	if len(parsed.TestCommands) != 1 || parsed.TestCommands[0] != "go test -v ./pkg/auth/..." {
		t.Errorf("test commands mismatch: %v", parsed.TestCommands)
	}
}

func TestR13_6_StoryProvenance_EnterpriseMode_ViaPolicyIsEnterprise(t *testing.T) {
	policy.EnforceSignedPolicy()
	defer func() {
		policy.RequireSignedPolicyFlag = "false"
		policy.ResetCachedPolicy()
	}()
	policy.ResetCachedPolicy()

	t.Setenv("ARTIX_ENTERPRISE", "")
	t.Setenv("KRITIX_ENTERPRISE", "")

	if !policy.IsEnterprise() {
		t.Fatalf("policy.IsEnterprise() must be true when EnforceSignedPolicy() is active")
	}

	st := &StorySpec{
		ID:    "STORY-ENT-01",
		Title: "Enterprise Provenance Test",
	}
	prov := BuildStoryProvenance(st, nil, "", "", nil)
	if !prov.EnterpriseMode {
		t.Errorf("R13-6 VIOLATION: BuildStoryProvenance must set EnterpriseMode=true when policy.IsEnterprise() is true")
	}
}
