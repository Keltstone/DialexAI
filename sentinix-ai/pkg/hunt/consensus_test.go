package hunt

import (
	"context"
	"testing"

	"sentinix/pkg/ai"
	"sentinix/pkg/engines"
)

type namedFake struct {
	name  string
	local bool
}

func (f namedFake) Name() string { return f.name }
func (f namedFake) Local() bool  { return f.local }
func (f namedFake) Complete(_ context.Context, _, _ string) (string, error) {
	return `{"vulnerable":false}`, nil
}

func TestEvaluateConsensus(t *testing.T) {
	tests := []struct {
		name         string
		votes        []Vote
		minVotes     int
		wantVuln     bool
		wantSev      engines.Severity
		wantDissent  int
		wantHighConf bool
	}{
		{
			name: "unanimous positive",
			votes: []Vote{
				{ModelName: "claude-3-7-sonnet", Vulnerable: true, Severity: engines.High, Confidence: 0.9, Reason: "unvalidated intent"},
				{ModelName: "gpt-4o", Vulnerable: true, Severity: engines.High, Confidence: 0.85, Reason: "arbitrary activity launch"},
				{ModelName: "deepseek-r1", Vulnerable: true, Severity: engines.High, Confidence: 0.95, Reason: "confused deputy flow"},
			},
			minVotes:     2,
			wantVuln:     true,
			wantSev:      engines.High,
			wantDissent:  0,
			wantHighConf: true,
		},
		{
			name: "majority positive with dissenter",
			votes: []Vote{
				{ModelName: "model-a", Vulnerable: true, Severity: engines.Critical, Confidence: 0.9, Reason: "SQL injection in query"},
				{ModelName: "model-b", Vulnerable: true, Severity: engines.High, Confidence: 0.8, Reason: "raw string format"},
				{ModelName: "model-c", Vulnerable: false, Severity: engines.Info, Confidence: 0.7, Reason: "parameterized in wrapper"},
			},
			minVotes:     2,
			wantVuln:     true,
			wantSev:      engines.Critical, // highest severity among positive votes
			wantDissent:  1,
			wantHighConf: false,
		},
		{
			name: "tie handling is defensive (prefers vulnerable if tie)",
			votes: []Vote{
				{ModelName: "model-a", Vulnerable: true, Severity: engines.Medium, Confidence: 0.6, Reason: "reflected parameter"},
				{ModelName: "model-b", Vulnerable: false, Severity: engines.Info, Confidence: 0.6, Reason: "sanitized earlier"},
			},
			minVotes:     2,
			wantVuln:     true, // tie defaults to vulnerable defensively
			wantSev:      engines.Medium,
			wantDissent:  1,
			wantHighConf: false,
		},
		{
			name: "unanimous negative",
			votes: []Vote{
				{ModelName: "model-a", Vulnerable: false, Severity: engines.Info, Confidence: 0.9, Reason: "internal only"},
				{ModelName: "model-b", Vulnerable: false, Severity: engines.Info, Confidence: 0.85, Reason: "permission checked"},
			},
			minVotes:     2,
			wantVuln:     false,
			wantDissent:  0,
			wantHighConf: false,
		},
		{
			name: "minVotes not met defaults to false",
			votes: []Vote{
				{ModelName: "model-a", Vulnerable: true, Severity: engines.High, Confidence: 0.9, Reason: "might be vulnerable"},
				{ModelName: "model-b", Vulnerable: false, Severity: engines.Info, Confidence: 0.8, Reason: "not reachable"},
				{ModelName: "model-c", Vulnerable: false, Severity: engines.Info, Confidence: 0.85, Reason: "not reachable"},
			},
			minVotes:     2,
			wantVuln:     false,
			wantDissent:  1, // 1 affirmative is dissenting against the majority negative
			wantHighConf: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := EvaluateConsensus(tt.votes, tt.minVotes)
			if res.Vulnerable != tt.wantVuln {
				t.Errorf("got Vulnerable=%v, want %v", res.Vulnerable, tt.wantVuln)
			}
			if tt.wantVuln && res.Severity != tt.wantSev {
				t.Errorf("got Severity=%v, want %v", res.Severity, tt.wantSev)
			}
			if res.DissentCount != tt.wantDissent {
				t.Errorf("got %d dissenting votes, want %d", res.DissentCount, tt.wantDissent)
			}
			if tt.wantHighConf && res.Confidence < 0.8 {
				t.Errorf("expected high confidence, got %f", res.Confidence)
			}
		})
	}
}

func TestSelectModelForRole(t *testing.T) {
	primary := namedFake{name: "claude-3-7-sonnet", local: false}
	verifier := namedFake{name: "gpt-4o", local: false}
	localModel := namedFake{name: "qwen-local", local: true}

	roles := map[string]string{
		"discover": "claude-3-7-sonnet",
		"validate": "gpt-4o",
	}

	models := []ai.Model{primary, verifier, localModel}

	// Select discover role with cloud policy
	m1, err := SelectModelForRole(models, "cloud", roles, "discover")
	if err != nil || m1.Name() != "claude-3-7-sonnet" {
		t.Errorf("expected claude-3-7-sonnet for discover, got %v (err: %v)", m1, err)
	}

	// Select validate role with cloud policy
	m2, err := SelectModelForRole(models, "cloud", roles, "validate")
	if err != nil || m2.Name() != "gpt-4o" {
		t.Errorf("expected gpt-4o for validate, got %v (err: %v)", m2, err)
	}

	// Policy restricts cloud models: discover should fall back to local model if local-only
	m3, err := SelectModelForRole(models, "local-only", roles, "discover")
	if err != nil || m3.Name() != "qwen-local" {
		t.Errorf("expected fallback to qwen-local under local-only, got %v (err: %v)", m3, err)
	}
}
