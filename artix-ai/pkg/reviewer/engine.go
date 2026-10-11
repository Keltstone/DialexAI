package reviewer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/sandbox"
	"artix/pkg/steering"
	"socratix/pkg/model"
)

// ReviewStatus represents the outcome category of a review evaluation.
type ReviewStatus string

const (
	StatusApproved   ReviewStatus = "approved"
	StatusRejected   ReviewStatus = "rejected"
	StatusUnreviewed ReviewStatus = "unreviewed"
)

// ReviewVerdict represents the Adversarial Reviewer's evaluation result.
type ReviewVerdict struct {
	Status             ReviewStatus `json:"status"` // "approved", "rejected", "unreviewed"
	Approved           bool         `json:"approved"`
	ScanMode           string       `json:"scanMode,omitempty"`
	Summary            string       `json:"summary"`
	BlockingIssues     []string     `json:"blockingIssues"`
	Warnings           []string     `json:"warnings"`
	ActionableFeedback string       `json:"actionableFeedback"`
}

// AdversarialReviewer evaluates unified diffs and test results.
type AdversarialReviewer struct {
	persona            model.Persona
	registry           *persona.Registry
	critic             Critic
	coderFamily        string
	criticFamily       string
	maxCriticDiffBytes int
	globalTaboos       steering.GlobalTabooSpace
}

// NewAdversarialReviewer creates a Reviewer persona engine.
func NewAdversarialReviewer(registry *persona.Registry) *AdversarialReviewer {
	p, ok := registry.Get("adversarial_code_reviewer")
	if !ok {
		p = model.Persona{
			ID:   "adversarial_code_reviewer",
			Name: "Adversarial Code Reviewer",
			Role: "Diff Scrutinizer & Test Enforcer",
		}
	}

	return &AdversarialReviewer{
		persona:            p,
		registry:           registry,
		maxCriticDiffBytes: DefaultMaxCriticDiffBytes,
	}
}

// SetCoderFamily configures the model family of the coder agent (e.g. "anthropic", "openai").
func (r *AdversarialReviewer) SetCoderFamily(family string) {
	r.coderFamily = strings.TrimSpace(family)
}

// SetCriticFamily configures the model family of the critic agent (e.g. "openai", "google").
func (r *AdversarialReviewer) SetCriticFamily(family string) {
	r.criticFamily = strings.TrimSpace(family)
}

// SetGlobalTaboos sets workspace-wide taboo constraints.
func (r *AdversarialReviewer) SetGlobalTaboos(gt steering.GlobalTabooSpace) {
	r.globalTaboos = gt
}

// ReviewContext contains everything needed to review an iteration.
type ReviewContext struct {
	Diff             string
	TestResults      []*sandbox.ExecResult
	AnalyzerFindings []AnalyzerFinding
	SteeringContext  *steering.PersonaSteeringContext
	Ctx              context.Context
	Criteria         []string
	WorkspaceDir     string
	TestCountBefore          int
	TestCountAfter           int
	CoverageBefore           float64
	CoverageAfter            float64
	RequiresTestGates        bool
	SemanticRunnerConfigured bool
	SemanticRunnerExecuted   bool
}

func determineScanMode(diff string) string {
	touched := extractTouchedFiles(diff)
	for _, f := range touched {
		ext := strings.ToLower(filepath.Ext(f))
		if ext == ".kt" || ext == ".swift" || ext == ".py" || ext == ".ts" || ext == ".js" || ext == ".java" || ext == ".rb" || ext == ".sh" {
			return "heuristic"
		}
	}
	return "diff-literal | AST"
}

// Evaluate performs deterministic pre-filtering (tests, diff, taboos, static analyzers) and
// runs the model-backed Critic against security and correctness rubrics.
// If no model is configured or reachable, it returns StatusUnreviewed with Approved=false.
func (r *AdversarialReviewer) Evaluate(ctx *ReviewContext) *ReviewVerdict {
	scanMode := determineScanMode(ctx.Diff)
	verdict := &ReviewVerdict{
		Status:         StatusApproved,
		Approved:       true,
		ScanMode:       scanMode,
		BlockingIssues: make([]string, 0),
		Warnings:       make([]string, 0),
	}

	// 1. Deterministic Pre-Filter: Test Validation
	for _, tr := range ctx.TestResults {
		if !tr.Success() {
			verdict.Approved = false
			verdict.Status = StatusRejected
			failMsg := fmt.Sprintf("Command %q failed with exit code %d", tr.Command, tr.ExitCode)
			if tr.TimedOut {
				failMsg = fmt.Sprintf("Command %q timed out", tr.Command)
			}
			verdict.BlockingIssues = append(verdict.BlockingIssues, failMsg)
			if tr.Stderr != "" {
				verdict.BlockingIssues = append(verdict.BlockingIssues, "Error detail:\n"+tr.Stderr)
			}
		}
	}

	// 2. Deterministic Pre-Filter: Diff Validation (Empty diff check)
	if strings.TrimSpace(ctx.Diff) == "" {
		verdict.Approved = false
		verdict.Status = StatusRejected
		verdict.BlockingIssues = append(verdict.BlockingIssues, "Workspace diff is empty; no code changes were produced.")
	}

	if len(ctx.TestResults) == 0 {
		verdict.Warnings = append(verdict.Warnings, "No test commands were run; pre-filter verified diff only.")
	}

	// 2b. Deterministic Pre-Filter: Protected Paths Validation (including org-configured RestrictedPaths)
	pol := policy.Active()
	protectedViolations := CheckProtectedPaths(ctx.Diff, pol.Reviewer.RestrictedPaths...)
	if len(protectedViolations) > 0 {
		verdict.Approved = false
		verdict.Status = StatusRejected
		verdict.BlockingIssues = append(verdict.BlockingIssues, protectedViolations...)
	}

	// 2bb. Deterministic Pre-Filter: Disjoint Model Families Enforcement
	if pol.IsDisjointModelFamiliesEnforced() && r.critic != nil {
		if r.coderFamily == "" || r.criticFamily == "" {
			verdict.Approved = false
			verdict.Status = StatusRejected
			verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("reviewer policy violation: model families must be explicitly configured (coder=%q, critic=%q); disjoint model families required", r.coderFamily, r.criticFamily))
		} else if strings.EqualFold(r.coderFamily, r.criticFamily) {
			verdict.Approved = false
			verdict.Status = StatusRejected
			verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("reviewer policy violation: critic model family %q matches coder family %q (disjoint model families required)", r.criticFamily, r.coderFamily))
		}
	}

	// 2c. Deterministic Pre-Filter: Test Integrity Validation (no deleting/skipping tests)
	testIntegrityViolations := CheckTestIntegrity(ctx.Diff)
	if len(testIntegrityViolations) > 0 {
		verdict.Approved = false
		verdict.Status = StatusRejected
		verdict.BlockingIssues = append(verdict.BlockingIssues, testIntegrityViolations...)
	}

	// 2cc. Deterministic Pre-Filter: Test Count & Coverage Delta Gates
	if ctx.RequiresTestGates {
		if ctx.TestCountBefore == 0 && ctx.TestCountAfter == 0 {
			verdict.Approved = false
			verdict.Status = StatusRejected
			verdict.BlockingIssues = append(verdict.BlockingIssues, "test integrity gate violation: new implementation requires test coverage for acceptance criteria (found 0 tests); fail-closed")
		}
	}
	if ctx.TestCountBefore > 0 && ctx.TestCountAfter < ctx.TestCountBefore {
		verdict.Approved = false
		verdict.Status = StatusRejected
		verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("test integrity violation: test count decreased from %d to %d (test elimination is strictly forbidden)", ctx.TestCountBefore, ctx.TestCountAfter))
	}
	if ctx.CoverageBefore > 0 {
		if ctx.CoverageAfter < ctx.CoverageBefore-0.0001 {
			verdict.Approved = false
			verdict.Status = StatusRejected
			verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("test coverage gate violation: test coverage decreased from %.2f%% to %.2f%%", ctx.CoverageBefore*100, ctx.CoverageAfter*100))
		}
	}

	// 2d. Deterministic Pre-Filter: Script/Makefile Indirection Detection
	touchedFiles := extractTouchedFiles(ctx.Diff)
	for _, tr := range ctx.TestResults {
		for _, tf := range touchedFiles {
			if isScriptOrBuildFile(tf) {
				base := filepath.Base(tf)
				if strings.Contains(tr.Command, tf) || strings.Contains(tr.Command, "./"+tf) || (base != "" && strings.Contains(tr.Command, base)) {
					verdict.Approved = false
					verdict.Status = StatusRejected
					verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("test integrity violation: test command %q calls modified script/Makefile %q", tr.Command, tf))
				}
			}
		}
	}

	// 2e. Deterministic Pre-Filter: Whole-File Post-Patch Test Integrity
	if ctx.WorkspaceDir != "" {
		wholeFileViolations := CheckTestIntegrityWholeFile(ctx.WorkspaceDir, ctx.Diff)
		if len(wholeFileViolations) > 0 {
			verdict.Approved = false
			verdict.Status = StatusRejected
			verdict.BlockingIssues = append(verdict.BlockingIssues, wholeFileViolations...)
		}
	}

	// 3. Deterministic Pre-Filter: Static Analyzer Findings
	for _, f := range ctx.AnalyzerFindings {
		if f.Severity == "ERROR" {
			verdict.Approved = false
			verdict.Status = StatusRejected
			verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("Static analyzer %s violation: %s", f.Tool, f.Message))
		} else {
			verdict.Warnings = append(verdict.Warnings, fmt.Sprintf("Static analyzer %s warning: %s", f.Tool, f.Message))
		}
	}

	// 4. Deterministic Pre-Filter: Semantic AST & Taboo Space Violations (Persona Taboos + Global Taboos)
	tabooList := make([]string, 0)
	if ctx.SteeringContext != nil {
		tabooList = append(tabooList, ctx.SteeringContext.Taboos.ForbiddenArguments...)
		tabooList = append(tabooList, ctx.SteeringContext.GlobalTaboos.ForbiddenArguments...)
	}
	tabooList = append(tabooList, r.globalTaboos.ForbiddenArguments...)

	tabooViolations := CheckSemanticASTTaboos(ctx.Diff, tabooList, ctx.WorkspaceDir)
	if len(tabooViolations) > 0 {
		verdict.Approved = false
		verdict.Status = StatusRejected
		verdict.BlockingIssues = append(verdict.BlockingIssues, tabooViolations...)
	}

	// 4b. Review Escalation: Modifications to go.work, testdata/, or //go:generate directives must escalate to unreviewed (human review required)
	for _, tf := range touchedFiles {
		base := filepath.Base(tf)
		if base == "go.work" || base == "go.work.sum" || strings.Contains(tf, "testdata/") || strings.HasPrefix(tf, "testdata/") {
			if verdict.Status != StatusRejected {
				verdict.Approved = false
				verdict.Status = StatusUnreviewed
				verdict.BlockingIssues = append(verdict.BlockingIssues, fmt.Sprintf("UNREVIEWED: review escalation required for modification to baseline/governance file %q; fail-closed for human review", tf))
				return verdict
			}
		}
	}
	if strings.Contains(ctx.Diff, "//go:generate") {
		if verdict.Status != StatusRejected {
			verdict.Approved = false
			verdict.Status = StatusUnreviewed
			verdict.BlockingIssues = append(verdict.BlockingIssues, "UNREVIEWED: review escalation required for //go:generate directive; fail-closed for human review")
			return verdict
		}
	}

	// 5. Deterministic Pre-Filter: Fail Closed for Non-Go Languages without Semantic Runner
	hasNonGo := false
	for _, tf := range touchedFiles {
		ext := strings.ToLower(filepath.Ext(tf))
		if ext == ".go" {
			continue
		}
		if !isDocsOnlyAllowedFile(tf, ctx.Diff) {
			hasNonGo = true
			break
		}
	}
	if hasNonGo {
		hasRunner := ctx.SemanticRunnerConfigured && ctx.SemanticRunnerExecuted
		if !hasRunner {
			if verdict.Status != StatusRejected {
				verdict.Approved = false
				verdict.Status = StatusUnreviewed
				verdict.BlockingIssues = append(verdict.BlockingIssues, "UNREVIEWED: diff touches non-Go / non-allowlisted files and no semantic rule runner (Konsist/Detekt/Semgrep/SwiftLint) is configured and executed; fail-closed")
				return verdict
			}
		}
	}

	// If pre-filter failed, reject immediately without calling model
	if !verdict.Approved {
		var sb strings.Builder
		sb.WriteString("REVIEW REJECTED. The following pre-filter issues must be resolved:\n")
		for i, issue := range verdict.BlockingIssues {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, issue)
		}
		verdict.Summary = fmt.Sprintf("Pre-filter failed with %d blocking issue(s).", len(verdict.BlockingIssues))
		verdict.ActionableFeedback = sb.String()
		return verdict
	}

	// 5. Model Review: Critic is the default path
	if r.critic == nil {
		verdict.Approved = false
		verdict.Status = StatusUnreviewed
		verdict.Summary = "UNREVIEWED: No model-backed Critic is configured. Pre-filter passed, but change requires model or human review before approval."
		verdict.ActionableFeedback = "Configure a model-backed Critic or perform manual code review."
		return verdict
	}

	r.applyCritic(ctx.Ctx, ctx, verdict)

	if verdict.Status == StatusUnreviewed {
		verdict.Summary = "UNREVIEWED: Critic model unreachable or failed; autonomous commit blocked."
		verdict.ActionableFeedback = "Retry review when model endpoint is available, or review manually."
		return verdict
	}

	if verdict.Approved {
		verdict.Status = StatusApproved
		verdict.Summary = fmt.Sprintf("[scan mode: %s] Approved by rule-based pre-filter (%d test(s) passed, diff non-empty, no taboos violated) and model review against security/correctness rubric.", verdict.ScanMode, len(ctx.TestResults))
	} else {
		verdict.Status = StatusRejected
		var sb strings.Builder
		sb.WriteString("REVIEW REJECTED. The following issues must be resolved:\n")
		for i, issue := range verdict.BlockingIssues {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, issue)
		}
		verdict.Summary = fmt.Sprintf("[scan mode: %s] Review rejected with %d blocking issue(s).", verdict.ScanMode, len(verdict.BlockingIssues))
		verdict.ActionableFeedback = sb.String()
	}

	return verdict
}

func isDocsOnlyAllowedFile(path string, diff string) bool {
	tfLower := strings.ToLower(path)
	ext := strings.ToLower(filepath.Ext(tfLower))

	// Image files
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".svg" || ext == ".ico" || ext == ".webp" {
		return true
	}
	// Plain text files
	if ext == ".txt" {
		return true
	}
	// Markdown files (if no taboo attack payload or script commands present)
	if ext == ".md" {
		diffLower := strings.ToLower(diff)
		if strings.Contains(diffLower, "curl ") || strings.Contains(diffLower, "wget ") || strings.Contains(diffLower, "$github_token") || strings.Contains(diffLower, "aws_") || strings.Contains(diffLower, "/etc/") {
			return false
		}
		return true
	}
	return false
}

func isScriptOrBuildFile(path string) bool {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	if base == "Makefile" || base == "makefile" || base == "GNUmakefile" {
		return true
	}
	if ext == ".sh" || ext == ".bash" || ext == ".zsh" || ext == ".py" || ext == ".rb" || ext == ".pl" {
		return true
	}
	return false
}
