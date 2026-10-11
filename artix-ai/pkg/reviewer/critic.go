package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"artix/pkg/policy"
	"socratix/pkg/model"
	"socratix/pkg/runner"
)

// DefaultMaxCriticDiffBytes is the default maximum unified diff size submitted for model review (500 KB).
const DefaultMaxCriticDiffBytes = 500_000

// Critic asks a model one question and returns its raw reply. Any engine AgentRunner can back it.
type Critic func(ctx context.Context, prompt string) (string, error)

// RunnerCritic adapts an engine AgentRunner (API or CLI) into a Critic.
func RunnerCritic(r runner.AgentRunner, agent model.Agent) Critic {
	return RunnerCriticWithTracker(r, agent, nil)
}

// RunnerCriticWithTracker adapts an engine AgentRunner and reports provider usage via callback.
func RunnerCriticWithTracker(r runner.AgentRunner, agent model.Agent, onUsage func(promptTokens, completionTokens, totalTokens int)) Critic {
	return func(ctx context.Context, prompt string) (string, error) {
		reply, err := r.Respond(ctx, agent, "Adversarial code review", prompt, "You are an adversarial code reviewer. Reply with JSON only.", nil, "")
		if err != nil {
			return "", err
		}
		if onUsage != nil && (reply.TokensIn != nil || reply.TokensOut != nil) {
			tin := 0
			tout := 0
			if reply.TokensIn != nil {
				tin = *reply.TokensIn
			}
			if reply.TokensOut != nil {
				tout = *reply.TokensOut
			}
			onUsage(tin, tout, tin+tout)
		}
		return reply.Content, nil
	}
}

// SetCritic adds a model-backed review pass on top of the rule-based checks.
func (r *AdversarialReviewer) SetCritic(c Critic) { r.critic = c }

// ModelFamilyRule defines an exact prefix match rule to canonical family.
type ModelFamilyRule struct {
	Prefix string
	Family string
}

// CanonicalOrderedModelFamilyRules defines the documented ordered rules for model family resolution.
// When multiple family tokens match, the first rule in this ordered list wins.
var CanonicalOrderedModelFamilyRules = []ModelFamilyRule{
	{Prefix: "claude", Family: "anthropic"},
	{Prefix: "anthropic", Family: "anthropic"},
	{Prefix: "gpt", Family: "openai"},
	{Prefix: "o1", Family: "openai"},
	{Prefix: "o3", Family: "openai"},
	{Prefix: "o4", Family: "openai"},
	{Prefix: "chatgpt", Family: "openai"},
	{Prefix: "gemini", Family: "google"},
	{Prefix: "gemma", Family: "google"},
	{Prefix: "palm", Family: "google"},
	{Prefix: "codellama", Family: "meta"},
	{Prefix: "llama", Family: "meta"},
	{Prefix: "qwen", Family: "qwen"},
	{Prefix: "qwq", Family: "qwen"},
	{Prefix: "deepseek", Family: "deepseek"},
	{Prefix: "codestral", Family: "mistral"},
	{Prefix: "mixtral", Family: "mistral"},
	{Prefix: "pixtral", Family: "mistral"},
	{Prefix: "ministral", Family: "mistral"},
	{Prefix: "mistral", Family: "mistral"},
	{Prefix: "grok", Family: "xai"},
	{Prefix: "command", Family: "cohere"},
	{Prefix: "cohere", Family: "cohere"},
	{Prefix: "phi", Family: "microsoft"},
}

// ResolveModelFamilyWithDetails returns the resolved family and the method used to resolve it.
func ResolveModelFamilyWithDetails(provider, modelID string) (family string, resolution string) {
	rawModel := strings.ToLower(strings.TrimSpace(modelID))
	rawProvider := strings.ToLower(strings.TrimSpace(provider))

	pol := policy.Active()

	// 1. Check Policy ModelDerivations and ModelFamilies (exact and prefix match)
	if pol != nil && rawModel != "" {
		derivations := make(map[string]string)
		for k, v := range pol.ModelDerivations {
			derivations[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
		}
		for k, v := range pol.Reviewer.ModelDerivations {
			derivations[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
		}

		if base, ok := derivations[rawModel]; ok {
			baseFam, _ := resolveBaseFamilyOnly(base)
			if baseFam == "" {
				baseFam = base
			}
			return baseFam, fmt.Sprintf("policy-derivation:%s->%s", rawModel, baseFam)
		}

		families := make(map[string]string)
		for k, v := range pol.ModelFamilies {
			families[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
		}
		for k, v := range pol.Reviewer.ModelFamilies {
			families[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
		}

		if fam, ok := families[rawModel]; ok {
			baseFam, _ := resolveBaseFamilyOnly(fam)
			if baseFam == "" {
				baseFam = fam
			}
			return baseFam, fmt.Sprintf("policy-override:%s->%s", rawModel, baseFam)
		}

		// Check prefix match in policy families
		for pfx, fam := range families {
			if strings.HasPrefix(rawModel, pfx) {
				baseFam, _ := resolveBaseFamilyOnly(fam)
				if baseFam == "" {
					baseFam = fam
				}
				return baseFam, fmt.Sprintf("policy-override-prefix:%s->%s", pfx, baseFam)
			}
		}
	}

	// 2. Data-driven ordered prefix matching
	if rawModel != "" {
		candidates := []string{rawModel}
		// Also support stripped vendor prefixes (e.g., "anthropic.claude" -> "claude", "meta-llama/llama" -> "llama")
		if slashIdx := strings.LastIndex(rawModel, "/"); slashIdx != -1 && slashIdx < len(rawModel)-1 {
			candidates = append(candidates, rawModel[slashIdx+1:])
		}
		if dotIdx := strings.Index(rawModel, "."); dotIdx != -1 && dotIdx < len(rawModel)-1 {
			candidates = append(candidates, rawModel[dotIdx+1:])
		}

		// Exact prefix matching against ordered rules
		for _, rule := range CanonicalOrderedModelFamilyRules {
			for _, cand := range candidates {
				if strings.HasPrefix(cand, rule.Prefix) {
					return rule.Family, fmt.Sprintf("prefix:%s", rule.Prefix)
				}
			}
		}

		// Substring token match against ordered rules (first rule in list wins when multiple tokens exist)
		for _, rule := range CanonicalOrderedModelFamilyRules {
			for _, cand := range candidates {
				if strings.Contains(cand, rule.Prefix) {
					return rule.Family, fmt.Sprintf("ordered-rule:%s", rule.Prefix)
				}
			}
		}

		// Unknown model treated as its own family
		return rawModel, "unknown-model-fallback"
	}

	// 3. Fall back to provider if model ID is empty
	if rawProvider != "" {
		for _, rule := range CanonicalOrderedModelFamilyRules {
			if strings.HasPrefix(rawProvider, rule.Prefix) || strings.Contains(rawProvider, rule.Prefix) {
				return rule.Family, fmt.Sprintf("provider-fallback:%s", rule.Family)
			}
		}
		return rawProvider, "provider-fallback"
	}

	return "", "empty"
}

func resolveBaseFamilyOnly(name string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, rule := range CanonicalOrderedModelFamilyRules {
		if strings.HasPrefix(lower, rule.Prefix) || strings.Contains(lower, rule.Prefix) {
			return rule.Family, fmt.Sprintf("prefix:%s", rule.Prefix)
		}
	}
	return lower, "unknown"
}

// ResolveModelFamily derives the canonical model family from the resolved model ID
// via data-driven ordered rules and policy configuration.
func ResolveModelFamily(provider, modelID string) string {
	family, _ := ResolveModelFamilyWithDetails(provider, modelID)
	return family
}

// NormalizeModelFamily extracts the canonical model/provider family.
func NormalizeModelFamily(name string) string {
	return ResolveModelFamily("", name)
}

// SetCriticWithFamily registers a Critic along with its model family and verifies policy constraints.
func (r *AdversarialReviewer) SetCriticWithFamily(c Critic, family string) error {
	trimmed := strings.TrimSpace(family)
	coderNorm := NormalizeModelFamily(r.coderFamily)
	criticNorm := NormalizeModelFamily(trimmed)
	if coderNorm != "" && criticNorm != "" && coderNorm == criticNorm {
		return fmt.Errorf("reviewer policy violation: critic model family %q matches coder family %q (disjoint model families required)", trimmed, r.coderFamily)
	}
	pol := policy.Active()
	if len(pol.Reviewer.AllowedCriticFamilies) > 0 && trimmed != "" {
		allowed := false
		for _, f := range pol.Reviewer.AllowedCriticFamilies {
			if strings.EqualFold(NormalizeModelFamily(f), criticNorm) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("reviewer policy violation: critic model family %q not in allowedCriticFamilies", trimmed)
		}
	}
	r.critic = c
	r.criticFamily = trimmed
	return nil
}

// SetMaxCriticDiffBytes configures the maximum diff size in bytes accepted for model review.
func (r *AdversarialReviewer) SetMaxCriticDiffBytes(n int) {
	if n > 0 {
		r.maxCriticDiffBytes = n
	}
}

type criticReply struct {
	Approved bool     `json:"approved"`
	Blocking []string `json:"blocking"`
	Warnings []string `json:"warnings"`
}

// applyCritic runs the model pass. It fails closed:
// 1. If the diff exceeds maxCriticDiffBytes, it HARD REJECTS immediately with DIFF_TRUNCATED.
// 2. A critic error or unparsable reply marks the verdict as StatusUnreviewed and not approved.
func (r *AdversarialReviewer) applyCritic(ctx context.Context, rc *ReviewContext, v *ReviewVerdict) {
	if ctx == nil {
		ctx = context.Background()
	}

	maxLimit := r.maxCriticDiffBytes
	if maxLimit <= 0 {
		maxLimit = DefaultMaxCriticDiffBytes
	}

	// Hard rejection on diff truncation: model review cannot be authoritative on an incomplete diff
	if len(rc.Diff) > maxLimit {
		v.Approved = false
		v.Status = StatusRejected
		v.BlockingIssues = append(v.BlockingIssues, fmt.Sprintf("DIFF_TRUNCATED: model review cannot be authoritative on an incomplete diff (diff size %d bytes exceeds maximum review limit %d bytes)", len(rc.Diff), maxLimit))
		return
	}

	raw, err := r.critic(ctx, buildCriticPrompt(rc))
	if err != nil {
		v.Approved = false
		v.Status = StatusUnreviewed
		v.BlockingIssues = append(v.BlockingIssues, fmt.Sprintf("Model review failed or unreachable (%v); change marked as unreviewed.", err))
		return
	}
	parsed, err := parseCriticReply(raw)
	if err != nil {
		v.Approved = false
		v.Status = StatusUnreviewed
		v.BlockingIssues = append(v.BlockingIssues, fmt.Sprintf("Model review returned an unparsable reply (%v); change marked as unreviewed.", err))
		return
	}
	v.Warnings = append(v.Warnings, parsed.Warnings...)
	if !parsed.Approved || len(parsed.Blocking) > 0 {
		v.Approved = false
		v.Status = StatusRejected
		if len(parsed.Blocking) == 0 {
			parsed.Blocking = []string{"Model review rejected the change without giving a reason."}
		}
		for _, b := range parsed.Blocking {
			v.BlockingIssues = append(v.BlockingIssues, "Model review: "+b)
		}
	} else {
		v.Approved = true
		v.Status = StatusApproved
	}
}

func buildCriticPrompt(rc *ReviewContext) string {
	var sb strings.Builder
	sb.WriteString(`You are an Adversarial Code Reviewer and Security Auditor. Evaluate the provided diff against the following mandatory security and correctness rubric:

### 1. SECURITY RUBRIC (Zero Tolerance)
- Injection & Memory Safety: No SQL/command injection, path traversal, buffer overflows, or unchecked allocations.
- Concurrency & Race Conditions: No data races, deadlocks, goroutine leaks, or unconfined thread blocking.
- Auth & Secrets: No hardcoded credentials, plain-text tokens, or bypass of authentication/authorization gates.
- Sandbox & Resource Confinement: Respect process boundaries and resource limits.

### 2. CORRECTNESS & SPEC ALIGNMENT RUBRIC
- Acceptance Criteria: Satisfies all specified Given/When/Then scenarios.
- Regression Risk: Does not break existing interfaces, invariants, or edge cases.
- Invariants & Standards: Adheres strictly to project coding invariants and architectural constraints.
`)

	if len(rc.Criteria) > 0 {
		sb.WriteString("\nACCEPTANCE CRITERIA:\n")
		for i, c := range rc.Criteria {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, c)
		}
	} else {
		sb.WriteString("\nACCEPTANCE CRITERIA:\n(none provided; review for defects and security issues only)\n")
	}

	if len(rc.AnalyzerFindings) > 0 {
		sb.WriteString("\nSTATIC ANALYZER FINDINGS (Mandatory Evidence from Konsist/Detekt/Semgrep/Vet):\n")
		for _, f := range rc.AnalyzerFindings {
			fmt.Fprintf(&sb, "- [%s] (%s): %s\n", f.Tool, f.Severity, f.Message)
		}
	}

	if len(rc.TestResults) > 0 {
		sb.WriteString("\nTEST RESULTS:\n")
		for _, t := range rc.TestResults {
			fmt.Fprintf(&sb, "- %q exit=%d timedOut=%v\n", t.Command, t.ExitCode, t.TimedOut)
		}
	}

	sb.WriteString("\nUNIFIED PATCH DIFF:\n" + rc.Diff + "\n\n")
	sb.WriteString(`Reply with ONLY this JSON: {"approved": bool, "blocking": ["..."], "warnings": ["..."]}. Set approved=false and list blocking issues if any criterion is unmet or a security/correctness defect exists.`)
	return sb.String()
}

// parseCriticReply pulls the JSON object out of a reply that may wrap it in prose or fences.
func parseCriticReply(raw string) (criticReply, error) {
	var out criticReply
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start || !utf8.ValidString(raw) {
		return out, fmt.Errorf("no JSON object in reply")
	}
	dec := json.NewDecoder(strings.NewReader(raw[start : end+1]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}
