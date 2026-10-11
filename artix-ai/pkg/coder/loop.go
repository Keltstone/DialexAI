package coder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"artix/pkg/audit"
	"artix/pkg/git"
	"artix/pkg/knowledge"
	"artix/pkg/metrics"
	"artix/pkg/plugins"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
	"artix/pkg/steering"
)

// AutonomyLevel defines human-in-the-loop gates.
type AutonomyLevel string

const (
	AutonomySupervised  AutonomyLevel = "supervised"  // Confirm before apply & commit
	AutonomyInteractive AutonomyLevel = "interactive" // Callback per iteration
	AutonomyAutonomous  AutonomyLevel = "autonomous"  // Auto-commit on green tests
)

// LoopOptions specifies execution limits and autonomy behavior.
type LoopOptions struct {
	MaxRounds        int           `json:"maxRounds"`
	Autonomy         AutonomyLevel `json:"autonomy"`
	Domain           string        `json:"domain"`
	TestTimeout      time.Duration `json:"testTimeout"`
	Budget           *TokenBudget  `json:"budget,omitempty"`
	AnalyzerCommands []string      `json:"analyzerCommands,omitempty"`
	OnIteration      func(round int, diff string, v *reviewer.ReviewVerdict) bool
	MockPatchGen     func(round int, feedback string) string // for tests and offline runs
	// PatchGenerator produces each round's patch, typically NewRunnerPatchGenerator. Without
	// it (or MockPatchGen) the loop cannot generate code and stops immediately.
	PatchGenerator                     PatchGenerator
	Approver                           string              `json:"approver,omitempty"`
	ForgeApproval                      *policy.PRApproval  `json:"forgeApproval,omitempty"`
	ForgeVerifier                      func(ctx context.Context, commitSHA string) (*policy.PRApproval, error)
	ForgePusher                        func(ctx context.Context, commitSHA string) error
	TwoPhaseAutonomous                 bool                `json:"twoPhaseAutonomous,omitempty"`
	PRBranch                           string              `json:"prBranch,omitempty"`
	MaxConsecutiveIdenticalRejections int                 `json:"maxConsecutiveIdenticalRejections,omitempty"`
	TestCommandsConfirmed              bool                `json:"testCommandsConfirmed,omitempty"`
	ConfirmTestCommands                func(commands []string) bool
	Model                              string              `json:"model,omitempty"`
	CoderFamily                        string              `json:"coderFamily,omitempty"`
	CoderFamilyResolution              string              `json:"coderFamilyResolution,omitempty"`
	ReviewerModel                      string              `json:"reviewerModel,omitempty"`
	ReviewerFamily                     string              `json:"reviewerFamily,omitempty"`
	ReviewerFamilyResolution           string              `json:"reviewerFamilyResolution,omitempty"`
	MaxTokens                          int                 `json:"maxTokens,omitempty"`
	MaxUSD                             float64             `json:"maxUsd,omitempty"`
	MaxWall                            time.Duration       `json:"maxWall,omitempty"`
	CoderUsageTracker                  func() *ProviderUsage
	ReviewerUsageTracker               func() *ProviderUsage
	TriggerABEval                      bool                                  `json:"triggerAbEval,omitempty"`
	ABEvalInterval                     int                                   `json:"abEvalInterval,omitempty"`
	ABEvalRunner                       knowledge.ABEvalRunner                `json:"-"`
	KnowledgeFilter                    func(ki knowledge.KnowledgeItem) bool `json:"-"`
	StreamEmitter                      *StreamEmitter                        `json:"-"`
	TaskID                             string                                `json:"taskId,omitempty"`
}

// LoopResult represents the final convergence outcome.
type LoopResult struct {
	Success          bool                    `json:"success"`
	AwaitingApproval bool                    `json:"awaitingApproval,omitempty"`
	RoundsRun        int                     `json:"roundsRun"`
	FinalVerdict     *reviewer.ReviewVerdict `json:"finalVerdict"`
	VerdictHash      string                  `json:"verdictHash,omitempty"`
	AppliedPatch     string                  `json:"appliedPatch,omitempty"`
	CommitHash       string                  `json:"commitHash,omitempty"`
	CostReport       *CostReport             `json:"costReport,omitempty"`
	Error            string                  `json:"error,omitempty"`
	SkippedTargets   map[string]string       `json:"skippedTargets,omitempty"`
	TargetSummary    string                  `json:"targetSummary,omitempty"`
}

// ConvergenceCoordinator manages the iterative Coder <--> Reviewer <--> Sandbox loop.
type ConvergenceCoordinator struct {
	coder    *DomainCoder
	reviewer *reviewer.AdversarialReviewer
	driver   *git.Driver
	sandbox  *sandbox.Sandbox
}

// NewCoordinator creates a new convergence coordinator.
func NewCoordinator(
	coder *DomainCoder,
	rev *reviewer.AdversarialReviewer,
	driver *git.Driver,
	box *sandbox.Sandbox,
) *ConvergenceCoordinator {
	return &ConvergenceCoordinator{
		coder:    coder,
		reviewer: rev,
		driver:   driver,
		sandbox:  box,
	}
}

// Run executes the convergence loop until tests pass and reviewer signs off.
func (c *ConvergenceCoordinator) Run(
	ctx context.Context,
	s *spec.StorySpec,
	repoCtx *repo.RepositoryContext,
	coderSteering *steering.PersonaSteeringContext,
	revSteering *steering.PersonaSteeringContext,
	opts *LoopOptions,
) *LoopResult {
	startTime := time.Now()
	maxRounds := 3
	autonomy := AutonomySupervised

	if opts != nil {
		if opts.MaxRounds > 0 {
			maxRounds = opts.MaxRounds
		}
		if opts.Autonomy != "" {
			autonomy = opts.Autonomy
		}
	}

	res := &LoopResult{
		Success:   false,
		RoundsRun: 0,
	}

	defer func() {
		if opts != nil && opts.StreamEmitter != nil {
			if res.AwaitingApproval {
				opts.StreamEmitter.Emit(StreamEvent{
					Type:      EventAwaitingApproval,
					TaskID:    opts.TaskID,
					Phase:     "APPROVAL",
					Status:    "awaiting_approval",
					Message:   "Candidate commit staged/pushed; awaiting approval",
					Payload:   map[string]any{"commitHash": res.CommitHash},
				})
			} else if res.Success {
				opts.StreamEmitter.Emit(StreamEvent{
					Type:      EventConverged,
					TaskID:    opts.TaskID,
					Phase:     "CONVERGED",
					Status:    "success",
					Round:     res.RoundsRun,
					Message:   fmt.Sprintf("Converged in %d round(s)", res.RoundsRun),
					Payload:   map[string]any{"commitHash": res.CommitHash},
				})
			} else if res.Error != "" {
				opts.StreamEmitter.Emit(StreamEvent{
					Type:      EventError,
					TaskID:    opts.TaskID,
					Phase:     "ERROR",
					Status:    "failed",
					Message:   res.Error,
				})
			}
		}
	}()

	var budget *TokenBudget
	if opts != nil && opts.Budget != nil {
		budget = opts.Budget
	} else {
		budget = LoadBudgetFromEnv()
	}
	if budget == nil && opts != nil && (opts.MaxUSD > 0 || opts.MaxTokens > 0) {
		budget = &TokenBudget{
			MaxStoryCost:   opts.MaxUSD,
			MaxStoryTokens: opts.MaxTokens,
		}
	} else if budget != nil && opts != nil {
		if opts.MaxUSD > 0 && budget.MaxStoryCost <= 0 {
			budget.MaxStoryCost = opts.MaxUSD
		}
		if opts.MaxTokens > 0 && budget.MaxStoryTokens <= 0 {
			budget.MaxStoryTokens = opts.MaxTokens
		}
	}

	costReport := &CostReport{
		StoryID: s.ID,
		Rounds:  make([]RoundCost, 0),
	}
	if budget != nil {
		costReport.BudgetStoryCap = budget.MaxStoryTokens
		costReport.BudgetTeamCap = budget.MaxTeamTokens
		costReport.BudgetDayCap = budget.MaxDayTokens
		costReport.BudgetStoryCostCap = budget.MaxStoryCost
		costReport.BudgetTeamCostCap = budget.MaxTeamCost
		costReport.BudgetDayCostCap = budget.MaxDayCost
		costReport.TeamID = budget.TeamID

		if err := budget.ValidatePricing(); err != nil {
			res.Error = err.Error()
			res.CostReport = costReport
			return res
		}
	}

	// Enterprise Autonomous Gate (ARTIX-SEC-02):
	// In enterprise/CI environments, autonomous commits require an explicit verified signed policy with allowAutonomous=true.
	if autonomy == AutonomyAutonomous {
		if !policy.IsAutonomousAllowed() {
			res.Error = "autonomous commit blocked: in enterprise/CI environments, autonomous commits require a verified signed policy with allowAutonomous=true"
			res.CostReport = costReport
			return res
		}
	}

	reg := plugins.NewRegistry()
	buildDrv, hasDrv := reg.DetectDriver(repoCtx.RootDir)
	driverName := ""
	if hasDrv {
		driverName = buildDrv.Name()
		if err := policy.Active().ValidateDriverAllowed(driverName); err != nil {
			res.Error = fmt.Sprintf("execution blocked: %v", err)
			res.CostReport = costReport
			return res
		}
	}

	if gr, ok := buildDrv.(*plugins.GradleDriver); ok && gr.IsKMP(repoCtx.RootDir) {
		selected, skipped, _ := gr.ResolveKMPTasks(ctx, repoCtx.RootDir)
		if len(skipped) > 0 {
			res.SkippedTargets = skipped
			for range skipped {
				metrics.RecordSkippedHostUnsupported()
			}
			res.TargetSummary = gr.FormatTargetStatusSummary(selected, skipped)
		} else if len(selected) > 0 {
			res.TargetSummary = "All targets green"
		}
	}

	if autonomy == AutonomyAutonomous && len(res.SkippedTargets) > 0 {
		res.Error = fmt.Sprintf("autonomous execution refused: driver has targets in SKIPPED_HOST_UNSUPPORTED mode (%s)", res.TargetSummary)
		res.CostReport = costReport
		return res
	}

	testCommandsToValidate := s.TestCommands
	if len(testCommandsToValidate) == 0 && hasDrv {
		if gr, ok := buildDrv.(*plugins.GradleDriver); ok {
			testCommandsToValidate = []string{gr.TestCmd(repoCtx.RootDir)}
		}
	}

	phaseTimeout := policy.GetTimeout(driverName, "test")

	// Validate test commands against signed enterprise policy before any execution
	effectiveTestCommands, pErr := policy.ValidateTestCommands(testCommandsToValidate)
	if pErr != nil {
		res.Error = fmt.Sprintf("test execution blocked by policy: %v", pErr)
		res.CostReport = costReport
		return res
	}

	// Compute test commands hash for provenance and audit verification
	cmdBytes := []byte(strings.Join(effectiveTestCommands, "\n"))
	cmdHashArr := sha256.Sum256(cmdBytes)
	testCommandsHash := hex.EncodeToString(cmdHashArr[:])

	// G4: Test-command trust outside enterprise:
	// In supervised/interactive mode outside enterprise, require explicit user confirmation and print command list
	if !policy.IsEnterprise() && opts != nil && (opts.Autonomy == AutonomySupervised || opts.Autonomy == AutonomyInteractive) {
		fmt.Fprintf(os.Stderr, "Proposed test commands (%d):\n", len(effectiveTestCommands))
		for i, cmd := range effectiveTestCommands {
			fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, cmd)
		}

		confirmed := false
		if opts != nil {
			if opts.ConfirmTestCommands != nil {
				confirmed = opts.ConfirmTestCommands(effectiveTestCommands)
			} else if opts.TestCommandsConfirmed {
				confirmed = true
			}
		}
		if !confirmed {
			res.Error = "unconfirmed test commands: supervised/interactive mode outside enterprise requires explicit user confirmation of test commands"
			res.CostReport = costReport
			return res
		}
	}

	var lastFeedback string
	var lastVerdictFeedback string
	var consecutiveIdenticalCount int
	maxIdentical := 2
	if opts != nil && opts.MaxConsecutiveIdenticalRejections > 0 {
		maxIdentical = opts.MaxConsecutiveIdenticalRejections
	}
	var priorFailures []string
	var activeSession *git.PatchSession

	initialTestCount := reviewer.CountTestsInWorkspace(repoCtx.RootDir)
	initialCoverage := 0.0

	auditLogger := audit.Default(repoCtx.RootDir)
	if policy.IsEnterprise() {
		if initErr := auditLogger.InitError(); initErr != nil {
			res.Success = false
			res.Error = fmt.Sprintf("enterprise audit failure: %v (convergence refused)", initErr)
			return res
		}
	}

	if opts != nil && opts.StreamEmitter != nil {
		opts.StreamEmitter.Emit(StreamEvent{
			Type:      EventWorktreeProvisioned,
			TaskID:    opts.TaskID,
			Phase:     "PROVISIONING",
			Message:   fmt.Sprintf("Initialized execution context for spec %s", s.ID),
			Payload:   map[string]any{"specId": s.ID, "title": s.Title, "testCommands": effectiveTestCommands},
		})
	}

	for round := 1; round <= maxRounds; round++ {
		res.RoundsRun = round

		if opts != nil && opts.StreamEmitter != nil {
			opts.StreamEmitter.Emit(StreamEvent{
				Type:      EventPatchGenStart,
				TaskID:    opts.TaskID,
				Phase:     "PATCH_GENERATION",
				Round:     round,
				MaxRounds: maxRounds,
				Message:   fmt.Sprintf("Generating patch for round %d", round),
			})
		}

		// 1. Coder generates patch
		var patch string
		kStore := knowledge.NewStore(repoCtx.RootDir)
		activeKIs, _ := kStore.ListActive()
		if opts != nil && opts.KnowledgeFilter != nil {
			var filtered []knowledge.KnowledgeItem
			for _, ki := range activeKIs {
				if opts.KnowledgeFilter(ki) {
					filtered = append(filtered, ki)
				}
			}
			activeKIs = filtered
		}

		promptCtx := PromptContext{
			Spec:             s,
			RepoContext:      repoCtx,
			SteeringContext:  coderSteering,
			ReviewerFeedback: lastFeedback,
			PriorFailures:    priorFailures,
			KnowledgeItems:   activeKIs,
		}
		sysPrompt, userPrompt := c.coder.CompilePrompt(&promptCtx)
		coderPromptTokens := EstimateTokens(sysPrompt) + EstimateTokens(userPrompt)
		coderDNATokens := 0
		if c.coder.persona.DNA != nil {
			coderDNATokens = EstimateTokens(CompileDNALayers(c.coder.persona.DNA, TaskCodeGeneration))
		}

		if opts != nil && opts.MockPatchGen != nil {
			patch = opts.MockPatchGen(round, lastFeedback)
		} else if opts != nil && opts.PatchGenerator != nil {
			var genErr error
			patch, genErr = opts.PatchGenerator(ctx, PatchRequest{
				Round:         round,
				Feedback:      lastFeedback,
				PriorFailures: priorFailures,
				PromptContext: &promptCtx,
			})
			if genErr != nil {
				res.Error = fmt.Sprintf("patch generation failed in round %d: %v", round, genErr)
				break
			}
		} else {
			res.Error = "no patch generator configured: pass LoopOptions.PatchGenerator (a model-backed generator) to generate code"
			break
		}

		if patch == "" {
			res.Error = fmt.Sprintf("no patch generated in round %d", round)
			break
		}

		if opts != nil && opts.StreamEmitter != nil {
			opts.StreamEmitter.Emit(StreamEvent{
				Type:      EventPatchGenDone,
				TaskID:    opts.TaskID,
				Phase:     "PATCH_GENERATION",
				Round:     round,
				MaxRounds: maxRounds,
				Message:   fmt.Sprintf("Generated patch (%d bytes) for round %d", len(patch), round),
			})
		}

		coderPatchTokens := EstimateTokens(patch)
		totalCoderTokens := coderPromptTokens + coderPatchTokens

		// Mid-round budget check: abort immediately before sandbox/critic if coder tokens exhausted cap
		if budget != nil {
			modelKey := "default"
			if opts != nil && opts.Model != "" {
				modelKey = opts.Model
			}
			costPer1k, err := budget.GetModelPrice(modelKey)
			if err != nil {
				res.Error = err.Error()
				res.CostReport = costReport
				_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType:   audit.EventBudgetExhausted,
					Status:      "FAILED",
					StorySpecID: s.ID,
					Details: map[string]any{
						"storyId":       s.ID,
						"limitType":     "max_usd",
						"unpricedModel": modelKey,
						"model":         modelKey,
						"error":         err.Error(),
						"limit":         opts.MaxUSD,
					},
				})
				return res
			}
			coderCostUSD := (float64(totalCoderTokens) / 1000.0) * costPer1k
			var coderUsage *ProviderUsage
			if opts != nil && opts.CoderUsageTracker != nil {
				coderUsage = opts.CoderUsageTracker()
			}
			budget.RecordRoundUsage(totalCoderTokens, coderCostUSD, coderUsage)
			costReport.TotalTokens += totalCoderTokens
			costReport.TotalCost += coderCostUSD

			if exhausted, reason := budget.CheckExhaustion(); exhausted {
				roundCost := RoundCost{
					Round: round,
					RoleTokens: map[string]int{
						"coder": totalCoderTokens,
					},
					DNAOverhead: map[string]int{
						"coder_dna": coderDNATokens,
					},
					TotalRoundTokens: totalCoderTokens,
				}
				costReport.Rounds = append(costReport.Rounds, roundCost)
				costReport.DNAOverheadTokens += coderDNATokens

				costReport.Exhausted = true
				costReport.ExhaustionReason = reason
				res.CostReport = costReport
				res.Error = fmt.Sprintf("budget exhausted in round %d: %s", round, reason)
				emitErr := audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType: audit.EventType("budget.exhausted"),
					Status:    "FAILED",
					Details: map[string]any{
						"storyId":          s.ID,
						"roundsRun":        round,
						"totalTokens":      costReport.TotalTokens,
						"totalCost":        costReport.TotalCost,
						"exhaustionReason": costReport.ExhaustionReason,
						"testCommandsHash": testCommandsHash,
					},
				})
				if emitErr != nil && res.Error == "" {
					res.Error = fmt.Sprintf("budget exhausted and audit emission failed: %v", emitErr)
				}
				return res
			}
		}

		// Hard task budget limits (MaxTokens, MaxUSD, MaxWall per task)
		if opts != nil {
			var currentTotalTokens int
			var currentTotalUSD float64

			if opts.CoderUsageTracker != nil {
				if u := opts.CoderUsageTracker(); u != nil {
					currentTotalTokens += u.TotalTokens
					modelKey := "default"
					if opts.Model != "" {
						modelKey = opts.Model
					}
					var costPer1k float64
					if budget != nil {
						var err error
						costPer1k, err = budget.GetModelPrice(modelKey)
						if err != nil && opts.MaxUSD > 0 {
							res.CostReport = costReport
							res.Error = err.Error()
							_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
								EventType:   audit.EventBudgetExhausted,
								Status:      "FAILED",
								StorySpecID: s.ID,
								Details: map[string]any{
									"storyId":       s.ID,
									"limitType":     "max_usd",
									"unpricedModel": modelKey,
									"model":         modelKey,
									"error":         err.Error(),
									"limit":         opts.MaxUSD,
								},
							})
							return res
						}
					}
					if costPer1k <= 0 {
						if opts.MaxUSD > 0 {
							res.CostReport = costReport
							res.Error = fmt.Sprintf("budget error: model %q has no configured price under active MaxUSD limit $%.2f", modelKey, opts.MaxUSD)
							_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
								EventType:   audit.EventBudgetExhausted,
								Status:      "FAILED",
								StorySpecID: s.ID,
								Details: map[string]any{
									"storyId":       s.ID,
									"limitType":     "max_usd",
									"unpricedModel": modelKey,
									"model":         modelKey,
									"error":         res.Error,
									"limit":         opts.MaxUSD,
								},
							})
							return res
						}
					}
					currentTotalUSD += (float64(u.TotalTokens) / 1000.0) * costPer1k
				}
			} else {
				currentTotalTokens = costReport.TotalTokens
				currentTotalUSD = costReport.TotalCost
			}

			if opts.MaxTokens > 0 && currentTotalTokens > opts.MaxTokens {
				res.CostReport = costReport
				res.Error = fmt.Sprintf("task budget limit exceeded: tokens limit %d reached (used %d tokens)", opts.MaxTokens, currentTotalTokens)
				_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType:   audit.EventBudgetExhausted,
					Status:      "FAILED",
					StorySpecID: s.ID,
					Details: map[string]any{
						"limit":     opts.MaxTokens,
						"current":   currentTotalTokens,
						"limitType": "max_tokens",
					},
				})
				return res
			}

			if opts.MaxUSD > 0 && currentTotalUSD > opts.MaxUSD {
				res.CostReport = costReport
				res.Error = fmt.Sprintf("task budget limit exceeded: USD cost limit $%.2f reached (cost $%.2f)", opts.MaxUSD, currentTotalUSD)
				_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType:   audit.EventBudgetExhausted,
					Status:      "FAILED",
					StorySpecID: s.ID,
					Details: map[string]any{
						"limit":     opts.MaxUSD,
						"current":   currentTotalUSD,
						"limitType": "max_usd",
					},
				})
				return res
			}

			if opts.MaxWall > 0 && time.Since(startTime) > opts.MaxWall {
				res.CostReport = costReport
				res.Error = fmt.Sprintf("task budget limit exceeded: wall clock duration %s reached (elapsed %s)", opts.MaxWall, time.Since(startTime).Round(time.Millisecond))
				_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType:   audit.EventBudgetExhausted,
					Status:      "FAILED",
					StorySpecID: s.ID,
					Details: map[string]any{
						"limit":     opts.MaxWall.String(),
						"elapsed":   time.Since(startTime).String(),
						"limitType": "max_wall",
					},
				})
				return res
			}
		}

		// 2. Apply patch via PatchSession
		activeSession = git.NewPatchSession(repoCtx.RootDir, patch)
		if err := activeSession.Apply(); err != nil {
			lastFeedback = fmt.Sprintf("Patch application failed: %v", err)
			continue
		}

		// 3. Inspect git diff
		diff, err := c.driver.Diff(false)
		if err != nil {
			diff = patch
		}

		// 4. Sandbox executes project test commands
		var testResults []*sandbox.ExecResult
		priorFailures = nil // only the latest round's failures are relevant to the next attempt

		if opts != nil && opts.StreamEmitter != nil {
			opts.StreamEmitter.Emit(StreamEvent{
				Type:      EventSandboxRunStart,
				TaskID:    opts.TaskID,
				Phase:     "SANDBOX_EXECUTION",
				Round:     round,
				MaxRounds: maxRounds,
				Message:   fmt.Sprintf("Running %d test command(s) in sandbox", len(effectiveTestCommands)),
				Payload:   map[string]any{"commands": effectiveTestCommands},
			})
		}

		// Script/Makefile indirection check: refuse commands referencing or executing modified scripts/Makefiles/build configs
		touchedByPatch := extractTouchedFilesFromDiff(diff)
		var indirectionViolations []string
		for _, cmdStr := range effectiveTestCommands {
			if IsScriptOrBuildIndirection(cmdStr, touchedByPatch) {
				indirectionViolations = append(indirectionViolations, fmt.Sprintf("test execution blocked: command %q references or executes script/build configuration modified by patch (indirection forbidden)", cmdStr))
			}
		}

		if len(indirectionViolations) > 0 {
			for _, iv := range indirectionViolations {
				priorFailures = append(priorFailures, iv)
				testResults = append(testResults, &sandbox.ExecResult{
					Command:  effectiveTestCommands[0],
					ExitCode: 1,
					Stderr:   iv,
				})
			}
		} else {
			for _, cmdStr := range effectiveTestCommands {
				tOpts := &sandbox.ExecOptions{
					Cwd:     repoCtx.RootDir,
					Timeout: phaseTimeout,
				}
				if opts != nil && opts.TestTimeout > 0 {
					tOpts.Timeout = opts.TestTimeout
				}
				tRes := c.sandbox.Run(ctx, cmdStr, tOpts)
				testResults = append(testResults, tRes)
				if !tRes.Success() {
					if tRes.TimedOut {
						metrics.RecordTimeout()
						if activeSession != nil {
							_ = activeSession.Rollback()
						}
						res.Error = fmt.Sprintf("TIMEOUT: test phase timed out after %s", tOpts.Timeout)
						_ = AppendRoundTrace(repoCtx.RootDir, &RoundTrace{
							TaskID:          s.ID,
							Round:           round,
							PromptHash:      HashContent(sysPrompt + "\n" + userPrompt),
							PatchHash:       HashContent(patch),
							Patch:           patch,
							SandboxCommand:  cmdStr,
							ExitCode:        tRes.ExitCode,
							ReviewerVerdict: "TIMEOUT",
							BlockingIssues:  []string{res.Error},
							Tokens:          totalCoderTokens,
							Cost:            0,
							Phase:           "TEST",
							Timestamp:       time.Now().UTC(),
						})
						_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
							EventType:   audit.EventCodeConvergence,
							Status:      "TIMEOUT",
							StorySpecID: s.ID,
							Details: map[string]any{
								"phase":     "TEST",
								"elapsed":   (time.Duration(tRes.DurationMs) * time.Millisecond).String(),
								"elapsedMs": tRes.DurationMs,
								"timeout":   tOpts.Timeout.String(),
								"command":   cmdStr,
							},
						})
						return res
					}
					combinedErr := tRes.Stderr + "\n" + tRes.Stdout
					if artifact, ok := plugins.DetectOfflineCacheMiss(combinedErr); ok {
						metrics.RecordOfflineCacheMiss()
						res.Error = fmt.Sprintf("OFFLINE_CACHE_MISS: %s", artifact)
						_ = AppendRoundTrace(repoCtx.RootDir, &RoundTrace{
							TaskID:          s.ID,
							Round:           round,
							PromptHash:      HashContent(sysPrompt + "\n" + userPrompt),
							PatchHash:       HashContent(patch),
							Patch:           patch,
							SandboxCommand:  cmdStr,
							ExitCode:        tRes.ExitCode,
							ReviewerVerdict: "OFFLINE_CACHE_MISS",
							BlockingIssues:  []string{res.Error},
							Tokens:          totalCoderTokens,
							Cost:            0,
							Phase:           "TEST",
							Timestamp:       time.Now().UTC(),
						})
						_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
							EventType:   audit.EventCodeConvergence,
							Status:      "OFFLINE_CACHE_MISS",
							StorySpecID: s.ID,
							Details: map[string]any{
								"artifact": artifact,
								"error":    res.Error,
								"command":  cmdStr,
							},
						})
						return res
					}
					priorFailures = append(priorFailures, fmt.Sprintf("%s: exit %d\n%s", cmdStr, tRes.ExitCode, tRes.Stderr))
				}
			}
		}

		if opts != nil && opts.StreamEmitter != nil {
			allPassed := len(priorFailures) == 0
			opts.StreamEmitter.Emit(StreamEvent{
				Type:      EventSandboxRunDone,
				TaskID:    opts.TaskID,
				Phase:     "SANDBOX_EXECUTION",
				Round:     round,
				MaxRounds: maxRounds,
				Status:    func() string { if allPassed { return "passed" } else { return "failed" } }(),
				Message:   fmt.Sprintf("Sandbox test execution finished in round %d: passed=%v", round, allPassed),
				Payload:   map[string]any{"passed": allPassed, "failureCount": len(priorFailures)},
			})
		}

		// 4b. Run configured static analyzers (Konsist, Detekt, Semgrep, SwiftLint, go vet) in sandbox
		var analyzerFindings []reviewer.AnalyzerFinding
		hasAnalyzers := opts != nil && len(opts.AnalyzerCommands) > 0
		analyzersRanSuccessfully := false
		if hasAnalyzers {
			findings, execResults := reviewer.RunAnalyzers(ctx, repoCtx.RootDir, c.sandbox, opts.AnalyzerCommands)
			analyzerFindings = findings
			allSuccess := true
			hasRealTool := false
			for i, cmd := range opts.AnalyzerCommands {
				if policy.IsAllowedSemanticRunner(cmd) {
					hasRealTool = true
				}
				if i < len(execResults) && !execResults[i].Success() {
					allSuccess = false
				}
			}
			if allSuccess && hasRealTool {
				analyzersRanSuccessfully = true
			}
			for _, f := range findings {
				if f.Severity == "ERROR" {
					priorFailures = append(priorFailures, fmt.Sprintf("Analyzer %s violation: %s", f.Tool, f.Message))
				}
			}
		}

		currentTestCount := reviewer.CountTestsInWorkspace(repoCtx.RootDir)
		var currentCoverage float64
		for _, tr := range testResults {
			if cov := reviewer.ExtractCoverage(tr.Stdout + "\n" + tr.Stderr); cov > 0 {
				currentCoverage = cov
				break
			}
		}
		if initialCoverage == 0.0 && currentCoverage > 0.0 {
			initialCoverage = currentCoverage
		}

		if opts != nil && opts.StreamEmitter != nil {
			opts.StreamEmitter.Emit(StreamEvent{
				Type:      EventReviewStart,
				TaskID:    opts.TaskID,
				Phase:     "REVIEW",
				Round:     round,
				MaxRounds: maxRounds,
				Message:   fmt.Sprintf("Reviewing patch and test results for round %d", round),
			})
		}

		// 5. Reviewer evaluates diff + tests + analyzers + steering
		rCtx := &reviewer.ReviewContext{
			Diff:                     diff,
			TestResults:              testResults,
			AnalyzerFindings:         analyzerFindings,
			SteeringContext:          revSteering,
			Ctx:                      ctx,
			Criteria:                 criteriaOf(s),
			WorkspaceDir:             repoCtx.RootDir,
			TestCountBefore:          initialTestCount,
			TestCountAfter:           currentTestCount,
			CoverageBefore:           initialCoverage,
			CoverageAfter:            currentCoverage,
			RequiresTestGates:        initialTestCount > 0,
			SemanticRunnerConfigured: hasAnalyzers,
			SemanticRunnerExecuted:   analyzersRanSuccessfully,
		}
		verdict := c.reviewer.Evaluate(rCtx)
		res.FinalVerdict = verdict

		if opts != nil && opts.StreamEmitter != nil {
			opts.StreamEmitter.Emit(StreamEvent{
				Type:      EventReviewVerdict,
				TaskID:    opts.TaskID,
				Phase:     "REVIEW",
				Round:     round,
				MaxRounds: maxRounds,
				Status:    string(verdict.Status),
				Message:   fmt.Sprintf("Review verdict for round %d: %s", round, verdict.Status),
				Payload:   map[string]any{"status": verdict.Status, "approved": verdict.Approved, "feedback": verdict.ActionableFeedback},
			})
		}

		// Track reviewer tokens and round cost
		reviewerPromptTokens := EstimateTokens(diff)
		for _, crit := range criteriaOf(s) {
			reviewerPromptTokens += EstimateTokens(crit)
		}
		reviewerVerdictTokens := 0
		if verdict != nil {
			reviewerVerdictTokens = EstimateTokens(verdict.Summary) + EstimateTokens(verdict.ActionableFeedback)
			for _, bi := range verdict.BlockingIssues {
				reviewerVerdictTokens += EstimateTokens(bi)
			}
			for _, w := range verdict.Warnings {
				reviewerVerdictTokens += EstimateTokens(w)
			}
		}
		totalReviewerTokens := reviewerPromptTokens + reviewerVerdictTokens
		reviewerDNATokens := 0
		if revSteering != nil && len(revSteering.Taboos.ForbiddenArguments) > 0 {
			reviewerDNATokens = EstimateTokens(fmt.Sprintf("%v", revSteering.Taboos.ForbiddenArguments))
		}

		roundCost := RoundCost{
			Round: round,
			RoleTokens: map[string]int{
				"coder":    totalCoderTokens,
				"reviewer": totalReviewerTokens,
			},
			DNAOverhead: map[string]int{
				"coder_dna":    coderDNATokens,
				"reviewer_dna": reviewerDNATokens,
			},
			TotalRoundTokens: totalCoderTokens + totalReviewerTokens,
		}
		costReport.Rounds = append(costReport.Rounds, roundCost)
		costReport.DNAOverheadTokens += coderDNATokens + reviewerDNATokens

		// Record round trace next to audit log
		cmdUsed := ""
		exitCode := 0
		if len(testResults) > 0 {
			lastTR := testResults[len(testResults)-1]
			cmdUsed = lastTR.Command
			exitCode = lastTR.ExitCode
		} else if len(effectiveTestCommands) > 0 {
			cmdUsed = effectiveTestCommands[0]
		}
		var bIssues []string
		vVerdict := ""
		if verdict != nil {
			vVerdict = string(verdict.Status)
			bIssues = verdict.BlockingIssues
		}
		_ = AppendRoundTrace(repoCtx.RootDir, &RoundTrace{
			TaskID:          s.ID,
			Round:           round,
			PromptHash:      HashContent(sysPrompt + "\n" + userPrompt),
			PatchHash:       HashContent(patch),
			Patch:           patch,
			SandboxCommand:  cmdUsed,
			ExitCode:        exitCode,
			ReviewerVerdict: vVerdict,
			BlockingIssues:  bIssues,
			Tokens:          roundCost.TotalRoundTokens,
			Cost:            0,
			Phase:           "REVIEW",
			Timestamp:       time.Now().UTC(),
		})

		// Check budget exhaustion after reviewer pass
		if budget != nil {
			revModelKey := "default"
			if opts != nil && opts.ReviewerModel != "" {
				revModelKey = opts.ReviewerModel
			} else if opts != nil && opts.Model != "" {
				revModelKey = opts.Model
			}
			costPer1k, err := budget.GetModelPrice(revModelKey)
			if err != nil {
				res.Error = err.Error()
				res.CostReport = costReport
				if activeSession != nil {
					_ = activeSession.Rollback()
				}
				_ = audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType:   audit.EventBudgetExhausted,
					Status:      "FAILED",
					StorySpecID: s.ID,
					Details: map[string]any{
						"storyId":       s.ID,
						"limitType":     "max_usd",
						"unpricedModel": revModelKey,
						"model":         revModelKey,
						"error":         err.Error(),
						"limit":         opts.MaxUSD,
					},
				})
				return res
			}
			reviewerCostUSD := (float64(totalReviewerTokens) / 1000.0) * costPer1k
			var revUsage *ProviderUsage
			if opts != nil && opts.ReviewerUsageTracker != nil {
				revUsage = opts.ReviewerUsageTracker()
			}
			budget.RecordRoundUsage(totalReviewerTokens, reviewerCostUSD, revUsage)
			costReport.TotalTokens += totalReviewerTokens
			costReport.TotalCost += reviewerCostUSD

			if exhausted, reason := budget.CheckExhaustion(); exhausted {
				costReport.Exhausted = true
				costReport.ExhaustionReason = reason
				res.CostReport = costReport
				res.Error = fmt.Sprintf("budget exhausted in round %d: %s", round, reason)
				if activeSession != nil {
					_ = activeSession.Rollback()
				}
				emitErr := audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
					EventType: audit.EventType("budget.exhausted"),
					Status:    "FAILED",
					Details: map[string]any{
						"storyId":          s.ID,
						"roundsRun":        round,
						"totalTokens":      costReport.TotalTokens,
						"totalCost":        costReport.TotalCost,
						"exhaustionReason": costReport.ExhaustionReason,
						"testCommandsHash": testCommandsHash,
					},
				})
				if emitErr != nil && res.Error == "" {
					res.Error = fmt.Sprintf("budget exhausted and audit emission failed: %v", emitErr)
				}
				return res
			}
		} else {
			costReport.TotalTokens += totalReviewerTokens
		}

		// Handle unreviewed status: fail closed and block auto-commit
		if verdict.Status == reviewer.StatusUnreviewed {
			if activeSession != nil {
				_ = activeSession.Rollback()
			}
			res.Success = false
			res.Error = "autonomous commit blocked: review status is 'unreviewed' (model critic was unavailable)"
			return res
		}

		// Interactive callback hook
		if opts != nil && opts.OnIteration != nil {
			keepGoing := opts.OnIteration(round, diff, verdict)
			if !keepGoing {
				_ = activeSession.Rollback()
				res.Error = "stopped by user in interactive mode"
				res.CostReport = costReport
				return res
			}
		}

		// 6. Check convergence
		if verdict.Approved && verdict.Status == reviewer.StatusApproved {
			res.Success = true
			res.AppliedPatch = patch
			res.CostReport = costReport

			var forgeApproval *policy.PRApproval
			approverIdentity := ""
			if forgeApproval != nil {
				approverIdentity = forgeApproval.ApproverUsername
			} else if opts != nil {
				approverIdentity = opts.Approver
			}

			// Autonomous commit gate: only reached when autonomy has already been verified via signed policy
			// at loop entry (enterprise gate fires before the first round). The guard
			// below exists solely to require at least one verified test command before committing.
			if autonomy == AutonomyAutonomous {
				approver := ""
				if opts != nil {
					approver = opts.Approver
				}

				if len(testResults) == 0 {
					_ = activeSession.Rollback()
					res.Success = false
					res.Error = "autonomous commit refused: the spec defines no test commands, so nothing verified the change"
					return res
				}

				// Post-merge test re-verification: Ensure all test commands pass against the final
				// state before committing (double-checks race conditions vs. the mid-loop run).
				for _, cmdStr := range effectiveTestCommands {
					tOpts := &sandbox.ExecOptions{
						Cwd:     repoCtx.RootDir,
						Timeout: phaseTimeout,
					}
					if opts != nil && opts.TestTimeout > 0 {
						tOpts.Timeout = opts.TestTimeout
					}
					reverify := c.sandbox.Run(ctx, cmdStr, tOpts)
					if !reverify.Success() {
						_ = activeSession.Rollback()
						res.Success = false
						res.Error = fmt.Sprintf("post-merge test re-verification failed on %q: exit %d (commit aborted, changes rolled back)", cmdStr, reverify.ExitCode)
						return res
					}
				}

				// Post-test workspace scan: check for secret canary / env-value leaks in all workspace files before commit
				if scanErr := scanWorkspaceForSecretLeaks(repoCtx.RootDir); scanErr != nil {
					_ = activeSession.Rollback()
					res.Success = false
					res.Error = fmt.Sprintf("pre-commit security canary scan failed: %v (commit aborted, changes rolled back)", scanErr)
					return res
				}

				headBeforeCommit, headBeforeErr := c.driver.HeadHash()
				if headBeforeErr != nil {
					_ = activeSession.Rollback()
					res.Success = false
					res.Error = fmt.Sprintf("failed to resolve repository HEAD before commit: %v", headBeforeErr)
					return res
				}

				// Commit candidate changes to produce post-patch head SHA
				commitMsg := fmt.Sprintf("feat: %s (Spec: %s)", s.Title, s.ID)
				newCommitSHA, commitErr := c.driver.CommitAll(commitMsg)
				if commitErr != nil {
					_ = activeSession.Rollback()
					res.Success = false
					res.Error = fmt.Sprintf("changes approved but commit failed: %v", commitErr)
					return res
				}
				res.CommitHash = newCommitSHA

				// Helper for safe rollback of candidate commit: ensures we NEVER reset if no new commit was made
				safeRollbackCandidate := func() {
					if currentHead, err := c.driver.HeadHash(); err == nil && currentHead == newCommitSHA && newCommitSHA != headBeforeCommit {
						_ = c.driver.ResetHard("HEAD~1")
					}
					_ = activeSession.Rollback()
				}

				if policy.IsEnterprise() || policy.Active().RequireForgeApproval {
					if opts != nil && opts.ForgeApproval != nil {
						safeRollbackCandidate()
						res.Success = false
						res.Error = "autonomous commit blocked: separation of duties violation: caller-supplied ForgeApproval is strictly forbidden in enterprise mode; approvals must be verified server-side from forge API"
						return res
					}

					verdictBytes, _ := json.Marshal(verdict)
					verdictHash := fmt.Sprintf("%x", sha256.Sum256(verdictBytes))
					res.VerdictHash = verdictHash

					// If ForgePusher is configured, push the candidate commit to the PR branch on the remote
					if opts != nil && opts.ForgePusher != nil {
						branch := opts.PRBranch
						if branch == "" {
							branch = os.Getenv("ARTIX_PR_BRANCH")
						}
						details := map[string]any{
							"storyId":             s.ID,
							"title":               s.Title,
							"candidateSHA":        newCommitSHA,
							"commitHash":          newCommitSHA,
							"roundsRun":           round,
							"reviewerVerdictHash": verdictHash,
						}
						if branch != "" {
							details["prBranch"] = branch
						}

						// Emit Phase 1 CANDIDATE_PUSHED audit record BEFORE pushing, failing closed on emit error
						l := auditLogger
						if l == nil {
							l = audit.Default(repoCtx.RootDir)
						}
						emitErr := l.Emit(audit.AuditEvent{
							EventType:   "CANDIDATE_PUSHED",
							Status:      "AWAITING_APPROVAL",
							StorySpecID: s.ID,
							Details:     details,
						})
						if emitErr != nil {
							safeRollbackCandidate()
							res.Success = false
							res.Error = fmt.Sprintf("phase 1 candidate audit emission failed: %v (commit aborted, changes rolled back)", emitErr)
							return res
						}

						if pushErr := opts.ForgePusher(ctx, newCommitSHA); pushErr != nil {
							safeRollbackCandidate()
							res.Success = false
							res.Error = fmt.Sprintf("autonomous candidate push to PR branch failed: %v (commit aborted, changes rolled back)", pushErr)
							return res
						}

						// In two-phase autonomous mode or when verification is handled asynchronously by a later step
						if opts.TwoPhaseAutonomous || opts.ForgeVerifier == nil {
							res.Success = false
							res.AwaitingApproval = true
							res.CommitHash = newCommitSHA
							res.Error = ""
							return res
						}
					}

					if opts == nil || opts.ForgeVerifier == nil {
						safeRollbackCandidate()
						res.Success = false
						res.Error = "autonomous commit blocked: separation of duties violation: server-side forge verification is required in enterprise mode"
						return res
					}

					var err error
					// Verify approval against the exact resulting commit being merged
					forgeApproval, err = opts.ForgeVerifier(ctx, newCommitSHA)
					if err != nil {
						safeRollbackCandidate()
						res.Success = false
						res.Error = fmt.Sprintf("autonomous commit blocked: %v", err)
						return res
					}

					if forgeApproval == nil || !forgeApproval.VerifiedByForge {
						safeRollbackCandidate()
						res.Success = false
						res.Error = "autonomous commit blocked: separation of duties violation: caller-supplied or forged approvals are strictly forbidden in enterprise mode; approval must be verified server-side from forge API"
						return res
					}
					if err := policy.ValidateForgeApproval(forgeApproval, "artix-agent", "artix-agent"); err != nil {
						safeRollbackCandidate()
						res.Success = false
						res.Error = fmt.Sprintf("autonomous commit blocked: %v", err)
						return res
					}
					approverIdentity = forgeApproval.ApproverUsername
				} else {
					if opts != nil && opts.ForgeApproval != nil {
						forgeApproval = opts.ForgeApproval
						approverIdentity = forgeApproval.ApproverUsername
					}
					// Validate approver unconditionally under Separation of Duties
					if err := policy.ValidateApprover("artix-agent", approver); err != nil {
						safeRollbackCandidate()
						res.Success = false
						res.Error = fmt.Sprintf("autonomous commit blocked: %v", err)
						return res
					}
				}
				if policy.IsEnterprise() {
					coderRes := opts.CoderFamilyResolution
					if coderRes == "" && opts.Model != "" {
						_, coderRes = reviewer.ResolveModelFamilyWithDetails("", opts.Model)
					}
					criticRes := opts.ReviewerFamilyResolution
					if criticRes == "" && opts.ReviewerModel != "" {
						_, criticRes = reviewer.ResolveModelFamilyWithDetails("", opts.ReviewerModel)
					}
					preCommitAuditErr := auditLogger.Emit(audit.AuditEvent{
						EventType: audit.EventCodeConvergence,
						Status:    "PENDING_COMMIT",
						Approver:  approverIdentity,
						Details: map[string]any{
							"storyId":                s.ID,
							"roundsRun":              res.RoundsRun,
							"commitHash":             newCommitSHA,
							"testCommandsHash":       testCommandsHash,
							"coderModel":             opts.Model,
							"coderFamily":            opts.CoderFamily,
							"coderFamilyResolution":  coderRes,
							"criticModel":            opts.ReviewerModel,
							"criticFamily":           opts.ReviewerFamily,
							"criticFamilyResolution": criticRes,
						},
					})
					if preCommitAuditErr != nil {
						safeRollbackCandidate()
						res.Success = false
						res.Error = fmt.Sprintf("enterprise audit logging failed: %v (commit aborted, changes rolled back)", preCommitAuditErr)
						return res
					}
				}
			}

			convergenceDetails := map[string]any{
				"storyId":          s.ID,
				"roundsRun":        res.RoundsRun,
				"success":          true,
				"commitHash":       res.CommitHash,
				"totalTokens":      costReport.TotalTokens,
				"dnaOverhead":      costReport.DNAOverheadTokens,
				"testCommandsHash": testCommandsHash,
			}
			if opts != nil {
				coderRes := opts.CoderFamilyResolution
				if coderRes == "" && opts.Model != "" {
					_, coderRes = reviewer.ResolveModelFamilyWithDetails("", opts.Model)
				}
				criticRes := opts.ReviewerFamilyResolution
				if criticRes == "" && opts.ReviewerModel != "" {
					_, criticRes = reviewer.ResolveModelFamilyWithDetails("", opts.ReviewerModel)
				}
				convergenceDetails["coderModel"] = opts.Model
				convergenceDetails["coderFamily"] = opts.CoderFamily
				convergenceDetails["coderFamilyResolution"] = coderRes
				convergenceDetails["criticModel"] = opts.ReviewerModel
				convergenceDetails["criticFamily"] = opts.ReviewerFamily
				convergenceDetails["criticFamilyResolution"] = criticRes
			}
			if res.SkippedTargets != nil {
				convergenceDetails["skippedTargets"] = res.SkippedTargets
				convergenceDetails["targetSummary"] = res.TargetSummary
			}

			emitErr := auditLogger.Emit(audit.AuditEvent{
				EventType: audit.EventCodeConvergence,
				Status:    "SUCCESS",
				Approver:  approverIdentity,
				Details:   convergenceDetails,
			})
			if emitErr != nil && policy.IsEnterprise() {
				_ = activeSession.Rollback()
				res.Success = false
				res.Error = fmt.Sprintf("enterprise audit logging failed: %v (commit aborted, changes rolled back)", emitErr)
				return res
			}

			// Post-convergence Knowledge A/B evaluation: measure active KIs to auto-demote regressing items
			for i := range activeKIs {
				ki := &activeKIs[i]
				newCount, err := kStore.IncrementSessionCount(ki.ID)
				if err == nil && newCount > 0 {
					ki.SessionCount = newCount
				} else {
					ki.SessionCount++
				}
				shouldEval := opts != nil && (opts.TriggerABEval || (opts.ABEvalInterval > 0 && ki.SessionCount%opts.ABEvalInterval == 0))
				if shouldEval {
					var evalRunner knowledge.ABEvalRunner
					if opts != nil && opts.ABEvalRunner != nil {
						evalRunner = opts.ABEvalRunner
					} else {
						// Real harness: rerun the same story twice against a worktree snapshot,
						// once with the KI excluded from PromptContext.KnowledgeItems and once with it included.
						targetKI := *ki
						evalRunner = func(evalCtx context.Context, item any) (int, bool, error) {
							wtDir, wtErr := os.MkdirTemp("", "artix-ab-eval-*")
							if wtErr != nil {
								return 0, false, fmt.Errorf("failed to create temp worktree dir: %w", wtErr)
							}
							defer os.RemoveAll(wtDir)

							addCmd := exec.Command("git", "-C", repoCtx.RootDir, "worktree", "add", "--detach", wtDir, "HEAD")
							if out, err := addCmd.CombinedOutput(); err != nil {
								return 0, false, fmt.Errorf("git worktree add failed: %w (%s)", err, string(out))
							}
							defer func() {
								_ = exec.Command("git", "-C", repoCtx.RootDir, "worktree", "remove", "--force", wtDir).Run()
							}()

							srcKnowledge := filepath.Join(repoCtx.RootDir, ".artix", "knowledge")
							if _, statErr := os.Stat(srcKnowledge); statErr == nil {
								dstArtix := filepath.Join(wtDir, ".artix")
								_ = os.MkdirAll(dstArtix, 0755)
								_ = os.Symlink(srcKnowledge, filepath.Join(dstArtix, "knowledge"))
							}

							subDriver := git.NewDriver(wtDir)
							subBox := sandbox.NewSandbox(wtDir)
							subRepoCtx := &repo.RepositoryContext{RootDir: wtDir}
							subCoord := NewCoordinator(c.coder, c.reviewer, subDriver, subBox)

							subOpts := *opts
							subOpts.TriggerABEval = false
							subOpts.ABEvalInterval = 0
							subOpts.ABEvalRunner = nil

							if item == nil {
								// Baseline run: exclude target KI from KnowledgeItems
								subOpts.KnowledgeFilter = func(k knowledge.KnowledgeItem) bool {
									return k.ID != targetKI.ID
								}
							} else {
								// Test run: include target KI in KnowledgeItems
								subOpts.KnowledgeFilter = nil
							}

							subRes := subCoord.Run(evalCtx, s, subRepoCtx, coderSteering, revSteering, &subOpts)
							return subRes.RoundsRun, subRes.Success, nil
						}
					}
					now := time.Now()
					ki.LastEvaluatedAt = &now
					resAB, errAB := knowledge.RunABEval(ctx, ki, evalRunner)
					if errAB == nil && resAB != nil {
						ki.LastEvalResult = resAB
					}
				}
				_ = kStore.Save(ki)
			}

			return res
		}

		// If failed, rollback this round's patch before trying next round
		_ = activeSession.Rollback()
		lastFeedback = verdict.ActionableFeedback

		if !verdict.Approved {
			if verdict.ActionableFeedback == lastVerdictFeedback && verdict.ActionableFeedback != "" {
				consecutiveIdenticalCount++
				if consecutiveIdenticalCount >= maxIdentical && round < maxRounds {
					res.Error = fmt.Sprintf("early convergence loop termination: %d consecutive identical rejections encountered", consecutiveIdenticalCount)
					res.CostReport = costReport
					return res
				}
			} else {
				consecutiveIdenticalCount = 1
				lastVerdictFeedback = verdict.ActionableFeedback
			}
		}
	}

	res.CostReport = costReport
	if !res.Success && res.Error == "" {
		res.Error = fmt.Sprintf("failed to converge after %d rounds", maxRounds)
	}

	failedDetails := map[string]any{
		"storyId":          s.ID,
		"roundsRun":        res.RoundsRun,
		"success":          res.Success,
		"totalTokens":      costReport.TotalTokens,
		"dnaOverhead":      costReport.DNAOverheadTokens,
		"testCommandsHash": testCommandsHash,
		"error":            res.Error,
	}
	if opts != nil {
		coderRes := opts.CoderFamilyResolution
		if coderRes == "" && opts.Model != "" {
			_, coderRes = reviewer.ResolveModelFamilyWithDetails("", opts.Model)
		}
		criticRes := opts.ReviewerFamilyResolution
		if criticRes == "" && opts.ReviewerModel != "" {
			_, criticRes = reviewer.ResolveModelFamilyWithDetails("", opts.ReviewerModel)
		}
		failedDetails["coderModel"] = opts.Model
		failedDetails["coderFamily"] = opts.CoderFamily
		failedDetails["coderFamilyResolution"] = coderRes
		failedDetails["criticModel"] = opts.ReviewerModel
		failedDetails["criticFamily"] = opts.ReviewerFamily
		failedDetails["criticFamilyResolution"] = criticRes
	}
	if res.SkippedTargets != nil {
		failedDetails["skippedTargets"] = res.SkippedTargets
		failedDetails["targetSummary"] = res.TargetSummary
	}

	emitErr := audit.Default(repoCtx.RootDir).Emit(audit.AuditEvent{
		EventType: audit.EventCodeConvergence,
		Status:    "FAILED",
		Details:   failedDetails,
	})
	if emitErr != nil && res.Error == "" {
		res.Error = fmt.Sprintf("audit emission failed: %v", emitErr)
	}

	return res
}

// criteriaOf flattens a spec's Gherkin scenarios into one line each for the model reviewer.
func criteriaOf(s *spec.StorySpec) []string {
	var out []string
	for _, sc := range s.AcceptanceCriteria {
		out = append(out, fmt.Sprintf("%s: Given %s, When %s, Then %s", sc.Name, sc.Given, sc.When, sc.Then))
	}
	return out
}

func extractTouchedFilesFromDiff(diff string) []string {
	var files []string
	lines := strings.Split(diff, "\n")
	reDiff := regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
	rePlus := regexp.MustCompile(`^\+\+\+ b/(.*)$`)
	for _, line := range lines {
		if m := reDiff.FindStringSubmatch(line); len(m) > 2 {
			files = append(files, m[2])
		} else if m := rePlus.FindStringSubmatch(line); len(m) > 1 {
			if m[1] != "/dev/null" {
				files = append(files, m[1])
			}
		}
	}
	return files
}

func isScriptOrBuildFile(path string) bool {
	tfLower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(tfLower)
	ext := filepath.Ext(tfLower)
	if strings.Contains(base, "makefile") || strings.HasSuffix(tfLower, ".mk") || base == "tox.ini" {
		return true
	}
	if ext == ".sh" || ext == ".bash" || ext == ".zsh" || ext == ".rb" || ext == ".pl" || ext == ".bat" || ext == ".ps1" {
		return true
	}
	if base == "package.json" || base == "tsconfig.json" || strings.HasPrefix(base, "jest.config") || strings.HasPrefix(base, "vitest.config") || strings.HasPrefix(base, "karma.conf") {
		return true
	}
	if base == "conftest.py" || base == "pytest.ini" || base == "setup.cfg" || base == "pyproject.toml" || base == "setup.py" {
		return true
	}
	if base == "go.mod" || base == "go.work" || base == "go.sum" || base == "tools.go" {
		return true
	}
	if strings.HasPrefix(base, "build.gradle") || strings.HasPrefix(base, "settings.gradle") || strings.HasPrefix(tfLower, "gradle/") {
		return true
	}
	if base == "build.rs" || base == "cargo.toml" || base == "cargo.lock" {
		return true
	}
	if base == "pom.xml" {
		return true
	}
	if ext == ".yaml" || ext == ".yml" || ext == ".toml" || ext == ".ini" || ext == ".conf" || strings.HasPrefix(base, ".env") {
		return true
	}
	return false
}

// IsScriptOrBuildIndirection returns true if a test command exists and the patch touches:
// 1. Any non-test, non-source file (Makefile, package.json, Dockerfile, scripts/, configs, etc.)
//    unless it is on the explicit documentation/asset allowlist (.md, .txt, .rst, images, etc.).
// 2. OR a Go file that is not standard package source or *_test.go (tools.go, go.sum, go.mod, go.work, vendor/, or TestMain hooks).
func IsScriptOrBuildIndirection(cmdStr string, touchedFiles []string) bool {
	if len(touchedFiles) == 0 {
		return false
	}
	if strings.TrimSpace(cmdStr) == "" {
		return false
	}

	for _, tf := range touchedFiles {
		tfLower := strings.ToLower(filepath.ToSlash(tf))
		base := filepath.Base(tfLower)
		ext := filepath.Ext(tfLower)

		// 1. Special Go files and build metadata: tools.go, go.sum, go.mod, go.work, vendor/, TestMain hooks
		if base == "tools.go" || base == "go.sum" || base == "go.mod" || base == "go.work" ||
			strings.HasPrefix(tfLower, "vendor/") || strings.Contains(tfLower, "/vendor/") ||
			strings.Contains(tfLower, "testmain") {
			return true
		}

		// 2. Build & CI directories, scripts, and container definitions
		if strings.HasPrefix(tfLower, "scripts/") || strings.Contains(tfLower, "/scripts/") ||
			strings.HasPrefix(tfLower, "gradle/") || strings.Contains(tfLower, "/gradle/") ||
			strings.HasPrefix(tfLower, ".github/") || strings.Contains(tfLower, "/.github/") ||
			strings.HasPrefix(tfLower, ".circleci/") || strings.Contains(tfLower, "/.circleci/") ||
			strings.HasPrefix(tfLower, "build/") || strings.Contains(tfLower, "/build/") ||
			strings.HasPrefix(tfLower, "bin/") || strings.Contains(tfLower, "/bin/") ||
			strings.Contains(base, "makefile") || strings.HasSuffix(tfLower, ".mk") ||
			base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || base == "containerfile" {
			return true
		}

		// 3. Build, dependency, and test framework configurations
		if isScriptOrBuildFile(tf) {
			return true
		}

		// 4. Explicit Documentation and Static Asset Allowlist
		if ext == ".md" || ext == ".txt" || ext == ".rst" || ext == ".adoc" ||
			ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" ||
			ext == ".svg" || ext == ".ico" || ext == ".webp" || ext == ".pdf" {
			continue
		}

		// 5. Non-source files (configs, yaml, json, env, sh, etc.)
		isStandardSource := false
		switch ext {
		case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
			".py", ".rs", ".java", ".kt", ".kts", ".c", ".cpp", ".cc", ".cxx",
			".h", ".hpp", ".swift", ".m", ".mm", ".cs", ".proto", ".graphql", ".sql":
			isStandardSource = true
		}

		if !isStandardSource {
			return true
		}
	}
	return false
}

// scanWorkspaceForSecretLeaks scans all workspace files (including .txt, .md, source files, etc.)
// to ensure no sensitive environment variables, tokens, or canaries were written to the workspace
// before a commit is created.
func scanWorkspaceForSecretLeaks(rootDir string) error {
	sensitiveValues := make(map[string]string)
	for _, envStr := range os.Environ() {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k, val := parts[0], parts[1]
		valTrim := strings.TrimSpace(val)
		if len(valTrim) < 8 { // skip short common values
			continue
		}
		kUpper := strings.ToUpper(k)
		if strings.Contains(kUpper, "PUBLIC") {
			continue
		}
		isSensitive := strings.Contains(kUpper, "SECRET") || strings.Contains(kUpper, "TOKEN") ||
			strings.Contains(kUpper, "PRIVATE_KEY") || strings.Contains(kUpper, "PRIVKEY") ||
			strings.Contains(kUpper, "PASSWORD") || strings.Contains(kUpper, "AUTH") ||
			strings.Contains(kUpper, "CREDENTIAL") || strings.Contains(kUpper, "CANARY") ||
			strings.Contains(kUpper, "BEARER") || strings.Contains(kUpper, "API_KEY") ||
			(strings.Contains(kUpper, "KEY") && !strings.Contains(kUpper, "PUBLIC"))
		if isSensitive {
			sensitiveValues[valTrim] = k
		}
	}

	if len(sensitiveValues) == 0 {
		return nil
	}

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == ".artix" || info.Name() == ".kritix" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip large binaries (> 10MB)
		if info.Size() > 10*1024*1024 {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		contentStr := string(content)
		for secretVal, envKey := range sensitiveValues {
			if strings.Contains(contentStr, secretVal) {
				relPath, _ := filepath.Rel(rootDir, path)
				return fmt.Errorf("security canary leak detected: workspace file %q contains sensitive environment value from %s", relPath, envKey)
			}
		}
		return nil
	})
}

