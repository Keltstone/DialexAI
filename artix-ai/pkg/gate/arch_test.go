package gate

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// EntryPointSpec defines an enumerated production entry point that can start an LLM call or a job.
type EntryPointSpec struct {
	EntryPoint   EntryPoint
	SourceFile   string
	FunctionName string
}

// RequiredProductionEntryPoints is the canonical, non-negotiable list of entry points.
var RequiredProductionEntryPoints = []EntryPointSpec{
	{EntryPointCLIPlan, "cli/main.go", "runPlan"},
	{EntryPointCLICode, "cli/main.go", "runCode"},
	{EntryPointCLIReview, "cli/main.go", "runReview"},
	{EntryPointCLIRepl, "cli/main.go", "runREPL"},
	{EntryPointDaemonTasks, "pkg/forge/server.go", "handleTasks"},
	{EntryPointDaemonWebhook, "pkg/forge/server.go", "handleGitHubWebhook"},
	{EntryPointACPSession, "pkg/acp/server.go", "handlePlanPrompt"},
	{EntryPointLSPCommand, "pkg/lsp/server.go", "handleExecuteCommand"},
}

// TestArchitecture_UnifiedProductionGate_EnumeratesAllEntryPoints asserts that
// every production entry point that can start an LLM call or a job passes through
// the ONE shared gate: gate.Pass (auth -> policy -> budget -> audit).
func TestArchitecture_UnifiedProductionGate_EnumeratesAllEntryPoints(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to find repo root: %v", err)
	}

	for _, ep := range RequiredProductionEntryPoints {
		t.Run(string(ep.EntryPoint), func(t *testing.T) {
			filePath := filepath.Join(repoRoot, ep.SourceFile)
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse %s: %v", ep.SourceFile, err)
			}

			foundFn := false
			callsGate := false

			ast.Inspect(node, func(n ast.Node) bool {
				fnDecl, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}
				if fnDecl.Name.Name != ep.FunctionName {
					return true
				}
				foundFn = true

				// Inspect body of the function for gate.Pass call
				ast.Inspect(fnDecl.Body, func(inner ast.Node) bool {
					call, ok := inner.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					id, ok := sel.X.(*ast.Ident)
					if !ok {
						return true
					}
					if id.Name == "gate" && sel.Sel.Name == "Pass" {
						callsGate = true
						return false
					}
					return true
				})
				return false
			})

			if !foundFn {
				t.Fatalf("ARCHITECTURE VIOLATION: Entry point function %s not found in %s", ep.FunctionName, ep.SourceFile)
			}
			if !callsGate {
				t.Errorf("ARCHITECTURE VIOLATION: Entry point %s (%s in %s) does not pass through gate.Pass! It bypasses the unified production gate (auth -> policy -> budget -> audit).",
					ep.EntryPoint, ep.FunctionName, ep.SourceFile)
			}
		})
	}
}

func TestUnifiedGate_EnforcesFourStages(t *testing.T) {
	ctx := context.Background()

	// 1. Auth Stage: empty actor fails
	_, err := Pass(ctx, Request{
		EntryPoint: EntryPointCLIPlan,
		Actor:      "",
	})
	if err == nil {
		t.Fatalf("expected auth error for empty actor, got nil")
	}
	gateErr, ok := err.(*GateError)
	if !ok || gateErr.Stage != "auth" {
		t.Fatalf("expected GateError with stage 'auth', got %v", err)
	}

	// 2. Policy Stage: autonomous mode with unsigned policy fails
	_, err = Pass(ctx, Request{
		EntryPoint: EntryPointCLICode,
		Actor:      "developer",
		Autonomy:   "autonomous",
	})
	if err == nil {
		t.Fatalf("expected policy error for autonomous without verified policy, got nil")
	}
	gateErr, ok = err.(*GateError)
	if !ok || gateErr.Stage != "policy" {
		t.Fatalf("expected GateError with stage 'policy', got %v", err)
	}

	// 3. Budget Stage: exhausted budget fails
	_, err = Pass(ctx, Request{
		EntryPoint:   EntryPointCLIPlan,
		Actor:        "developer",
		BudgetTokens: 1000,
		TokensUsed:   1000,
	})
	if err == nil {
		t.Fatalf("expected budget error for exhausted tokens, got nil")
	}
	gateErr, ok = err.(*GateError)
	if !ok || gateErr.Stage != "budget" {
		t.Fatalf("expected GateError with stage 'budget', got %v", err)
	}

	// 4. Valid Request passes all stages
	res, err := Pass(ctx, Request{
		EntryPoint:   EntryPointCLIPlan,
		Actor:        "developer",
		BudgetTokens: 1000,
		TokensUsed:   500,
		Prompt:       "Plan story for auth module",
	})
	if err != nil {
		t.Fatalf("expected valid request to pass gate, got error: %v", err)
	}
	if !res.Passed || res.GateID == "" {
		t.Fatalf("unexpected gate result: %+v", res)
	}
}
