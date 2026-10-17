package gate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"artix/pkg/audit"
	"artix/pkg/policy"
)

// EntryPoint represents a production system entry point that can initiate an LLM call or a job.
type EntryPoint string

const (
	EntryPointCLIPlan        EntryPoint = "cli.plan"
	EntryPointCLICode        EntryPoint = "cli.code"
	EntryPointCLIReview      EntryPoint = "cli.review"
	EntryPointCLIRepl        EntryPoint = "cli.repl"
	EntryPointDaemonTasks    EntryPoint = "daemon.tasks"
	EntryPointDaemonWebhook  EntryPoint = "daemon.webhook"
	EntryPointACPSession     EntryPoint = "acp.session"
	EntryPointLSPCommand     EntryPoint = "lsp.executeCommand"
)

// GateError represents a failure at a specific stage of the unified gate.
type GateError struct {
	Stage   string // "auth", "policy", "budget", "audit"
	Message string
}

func (e *GateError) Error() string {
	return fmt.Sprintf("gate [%s]: %s", e.Stage, e.Message)
}

// Request encapsulates input parameters to be validated by the unified gate.
type Request struct {
	EntryPoint   EntryPoint
	Actor        string
	Autonomy     string
	ProjectDir   string
	Prompt       string
	BudgetTokens int64
	BudgetUSD    float64
	TokensUsed   int64
	CostUSD      float64
	Metadata     map[string]any
}

// Result records successful passage through the unified security gate.
type Result struct {
	Passed     bool
	GateID     string
	Actor      string
	EntryPoint EntryPoint
	Timestamp  time.Time
}

// Pass executes the mandatory 4-stage governance chain:
// 1. Auth: Validates caller/actor identity.
// 2. Policy: Verifies autonomy gate, enterprise signatures, and toolchain constraints.
// 3. Budget: Verifies token and financial limits.
// 4. Audit: Emits tamper-evident audit record, failing closed in enterprise mode.
func Pass(ctx context.Context, req Request) (*Result, error) {
	// Stage 1: Auth
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		return nil, &GateError{
			Stage:   "auth",
			Message: "unauthenticated request: caller identity is empty",
		}
	}

	// Stage 2: Policy
	if req.Autonomy == "autonomous" {
		if err := policy.VerifyAutonomyGate(); err != nil {
			return nil, &GateError{
				Stage:   "policy",
				Message: fmt.Sprintf("autonomous execution forbidden by policy: %v", err),
			}
		}
	}

	// Stage 3: Budget
	if req.BudgetTokens > 0 && req.TokensUsed >= req.BudgetTokens {
		return nil, &GateError{
			Stage:   "budget",
			Message: fmt.Sprintf("token budget exhausted: used %d of %d", req.TokensUsed, req.BudgetTokens),
		}
	}
	if req.BudgetUSD > 0 && req.CostUSD >= req.BudgetUSD {
		return nil, &GateError{
			Stage:   "budget",
			Message: fmt.Sprintf("financial budget exhausted: used $%.4f of $%.4f", req.CostUSD, req.BudgetUSD),
		}
	}

	// Stage 4: Audit
	gateID := fmt.Sprintf("gate-%d", time.Now().UnixNano())
	logger := audit.Default(req.ProjectDir)
	auditEvent := audit.AuditEvent{
		EventID:   gateID,
		EventType: audit.EventType("gate.evaluation"),
		Actor:     audit.RedactSecrets(actor),
		Status:    "SUCCESS",
		Timestamp: time.Now().UTC(),
		Details: map[string]any{
			"gateId":     gateID,
			"autonomy":   req.Autonomy,
			"entryPoint": string(req.EntryPoint),
		},
	}
	if req.Prompt != "" {
		auditEvent.Details["prompt"] = audit.RedactSecrets(req.Prompt)
	}

	if err := logger.Emit(auditEvent); err != nil {
		if policy.IsEnterprise() {
			return nil, &GateError{
				Stage:   "audit",
				Message: fmt.Sprintf("enterprise fail-closed: audit logging failed: %v", err),
			}
		}
	}

	return &Result{
		Passed:     true,
		GateID:     gateID,
		Actor:      actor,
		EntryPoint: req.EntryPoint,
		Timestamp:  time.Now().UTC(),
	}, nil
}
