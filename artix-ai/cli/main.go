package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"artix/pkg/acp"
	"artix/pkg/audit"
	"artix/pkg/coder"
	"artix/pkg/forge"
	"artix/pkg/git"
	"artix/pkg/knowledge"
	"artix/pkg/lsp"
	"artix/pkg/persona"
	"artix/pkg/plugins"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
	"artix/pkg/steering"
	"artix/pkg/tui"
)

const version = "1.0.0"

func printUsageTo(w io.Writer) {
	fmt.Fprintf(w, `Artix AI - Dialectic Software Engineering & Autonomous Coding (v%s)

Usage:
  artix [command] [options] [arguments]

Commands:
  repl          Launch interactive TUI shell with @mentions and /grill-me
  lsp           Launch Language Server Protocol backend for IDEs (VS Code, Zed, etc.)
  acp           Launch Agent Communication Protocol (ACP) server for JetBrains Air, Zed, etc.
  plan, spec    Deliberate with Stakeholder Council to produce Story Spec
  code          Execute Domain Coder <-> Reviewer convergence loop
  verify-approval Verify forge approval for candidate commit (Phase 2)
  review        Run Adversarial Reviewer against current git diff and tests
  audit         Audit log management and tamper verification (audit verify)
  knowledge     Inspect and ratify institutional knowledge items (ratify, list)
  steering      Manage dynamic steering rules (list, sync, bind)
  persona       Inspect and manage SWE Personas
  gc            Clean up stale and orphaned shadow worktrees
  warm          Pre-warm offline dependency caches outside sandbox
  explain       Explain job failure root causes and reproduction commands
  replay        Replay a recorded convergence round in a fresh worktree
  pilot         Pilot success metrics and stop rule reports (pilot report)
  daemon        Launch webhook server for GitHub & GitLab automation
  version       Print version

Running 'artix' without arguments enters interactive REPL mode.
Use "artix <command> -h" for detailed options on any command.
`, version)
}

func printUsage() {
	printUsageTo(os.Stdout)
}

// RunCLI dispatches a CLI command to allow testable execution with default os.Stdin.
func RunCLI(cwd string, reg *persona.Registry, rawArgs []string, stdout, stderr io.Writer) int {
	return RunCLIWithIO(cwd, reg, rawArgs, os.Stdin, stdout, stderr)
}

func isTerminal(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// RunCLIWithIO dispatches a CLI command with customizable stdin, stdout, and stderr.
func RunCLIWithIO(cwd string, reg *persona.Registry, rawArgs []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(rawArgs) == 0 {
		printUsageTo(stdout)
		return 0
	}

	// Filter out --json and --stream globally
	args := make([]string, 0, len(rawArgs))
	isJSON := false
	isStream := false
	for _, a := range rawArgs {
		if a == "--json" {
			isJSON = true
		} else if a == "--stream" {
			isStream = true
		} else {
			args = append(args, a)
		}
	}

	if len(args) == 0 {
		printUsageTo(stdout)
		return 0
	}

	cmd := args[0]
	cmdArgs := args[1:]

	humanOut := stdout
	if isJSON || isStream {
		humanOut = stderr
	}

	sendJSON := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(stdout, "%s\n", string(data))
	}

	if reg == nil {
		reg = persona.NewRegistry(cwd)
	}

	switch cmd {
	case "version", "--version", "-v":
		if isJSON {
			sendJSON(map[string]any{"ok": true, "version": version})
		} else {
			fmt.Fprintf(stdout, "Artix AI version %s\n", version)
		}
		return 0

	case "help", "--help", "-h":
		printUsageTo(stdout)
		return 0

	case "persona":
		personas := reg.List()
		if isJSON {
			sendJSON(map[string]any{"ok": true, "personas": personas})
		} else {
			fmt.Fprintf(stdout, "Available SWE Personas (%d):\n", len(personas))
			for _, p := range personas {
				fmt.Fprintf(stdout, " - %-25s | %s (%s)\n", p.ID, p.Name, p.Role)
			}
		}
		return 0

	case "acp":
		server := acp.NewServer(cwd, reg, stdin, stdout)
		if err := server.Serve(); err != nil {
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
			}
			fmt.Fprintf(stderr, "ACP server error: %v\n", err)
			return 1
		}
		return 0

	case "plan", "spec":
		return runPlan(cwd, reg, cmdArgs, humanOut, sendJSON, isJSON, isStream, stdout, stderr)

	case "code":
		return runCode(cwd, reg, cmdArgs, stdin, humanOut, sendJSON, isJSON, isStream, stdout, stderr)

	case "verify-approval":
		return runVerifyApproval(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "merge":
		errStr := "Error: 'artix merge' is deprecated in enterprise mode. Use 'artix verify-approval' instead."
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "deprecated", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1

	case "review":
		return runReview(cwd, reg, cmdArgs, humanOut, sendJSON, isJSON, isStream, stdout, stderr)

	case "audit":
		return runAudit(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "knowledge":
		return runKnowledge(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "steering":
		handleSteering(cwd, cmdArgs)
		return 0

	case "gc":
		handleGC(cwd, cmdArgs)
		return 0

	case "warm":
		return runWarm(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "explain":
		return runExplain(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "replay":
		return runReplay(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "pilot":
		return runPilot(cwd, cmdArgs, humanOut, sendJSON, isJSON, stderr)

	case "daemon":
		handleDaemon(cwd, reg, cmdArgs)
		return 0

	default:
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("Unknown command: %s", cmd)})
		}
		fmt.Fprintf(stderr, "Unknown command: %s. Run 'artix help' for usage.\n", cmd)
		return 1
	}
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	registry := persona.NewRegistry(cwd)

	if len(os.Args) < 2 {
		repl := tui.NewREPL(cwd, registry, os.Stdin, os.Stdout)
		_ = repl.Run(context.Background())
		return
	}

	if os.Args[1] == "repl" {
		repl := tui.NewREPL(cwd, registry, os.Stdin, os.Stdout)
		_ = repl.Run(context.Background())
		return
	}

	if os.Args[1] == "lsp" {
		server := lsp.NewServer(cwd, os.Stdin, os.Stdout)
		if err := server.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "LSP server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if os.Args[1] == "acp" {
		server := acp.NewServer(cwd, registry, os.Stdin, os.Stdout)
		if err := server.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "ACP server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	os.Exit(RunCLI(cwd, registry, os.Args[1:], os.Stdout, os.Stderr))
}

func runPlan(cwd string, reg *persona.Registry, args []string, human io.Writer, sendJSON func(any), isJSON bool, isStream bool, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	styleFlag := fs.String("style", "standard", "Spec style vector: standard, ponytail (executive), or caveman (terse)")
	providerFlag := fs.String("provider", "", "Model provider for Stakeholder Council deliberation: anthropic, openai, gemini, grok, deepseek, mistral, ollama")
	modelFlag := fs.String("model", "", "Model name for --provider")
	if err := fs.Parse(args); err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		return 1
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": "User story prompt is required"})
		}
		fmt.Fprintf(stderr, "Error: User story prompt is required. Example: artix plan \"Add OAuth2 Google login\"\n")
		return 1
	}
	prompt := strings.Join(remaining, " ")

	repoCtx, err := repo.DetectContext(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: failed to detect full repo context: %v\n", err)
	}

	council := spec.NewCouncil(reg)
	method := "deterministic_template"
	rounds := 1

	if isStream {
		emitter := coder.NewStreamEmitter(stdout)
		council.SetStreamHandler(func(ev spec.CouncilStreamEvent) {
			emitter.Emit(coder.StreamEvent{
				Type:      coder.StreamEventType(ev.Type),
				Timestamp: ev.Timestamp,
				TaskID:    ev.SpecID,
				Phase:     "COUNCIL_DELIBERATION",
				Round:     ev.Round,
				MaxRounds: ev.TotalRounds,
				Role:      ev.Role,
				Status:    ev.Status,
				Message:   ev.Message,
				Payload:   ev.Payload,
			})
		})
	}

	if *providerFlag != "" {
		mRunner, agent, rerr := coder.NewAPIRunnerFromEnv(*providerFlag, *modelFlag, os.Getenv)
		if rerr != nil {
			fmt.Fprintf(stderr, "Warning: failed to initialize model runner (%v), falling back to structured template\n", rerr)
		} else {
			council.SetRunner(mRunner, agent)
			method = "multi_persona_deliberation"
			rounds = 3
			fmt.Fprintf(human, "Assembling Stakeholder Council with AI Deliberation (%s/%s)...\n", *providerFlag, *modelFlag)
		}
	} else {
		fmt.Fprintln(human, "Assembling Stakeholder Council (Deterministic Template Mode)...")
	}

	pCtx := &spec.PlanningContext{
		StoryPrompt: prompt,
		RepoContext: repoCtx,
		Style:       spec.StyleVector(*styleFlag),
	}

	storySpec, err := council.Plan(context.Background(), pCtx)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		fmt.Fprintf(stderr, "Council deliberation failed: %v\n", err)
		return 1
	}

	prov := spec.BuildStoryProvenance(storySpec, pCtx, *providerFlag, *modelFlag, council.Members())

	specsDir := filepath.Join(cwd, "docs", "specs")
	specPath, provPath, err := spec.WriteSpecWithProvenance(specsDir, storySpec, prov)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		fmt.Fprintf(stderr, "Failed to persist spec and provenance: %v\n", err)
		return 1
	}

	// Emit audit event
	_ = audit.Default(cwd).Emit(audit.AuditEvent{
		EventType: audit.EventSpecDeliberation,
		Status:    "SUCCESS",
		Details: map[string]any{
			"specId":             storySpec.ID,
			"title":              storySpec.Title,
			"deliberationMethod": method,
			"deliberationRounds": rounds,
			"provider":           *providerFlag,
			"model":              *modelFlag,
		},
	})

	if isJSON {
		sendJSON(map[string]any{
			"ok":             true,
			"status":         "success",
			"specId":         storySpec.ID,
			"title":          storySpec.Title,
			"specPath":       specPath,
			"provenancePath": provPath,
			"scenarios":      len(storySpec.AcceptanceCriteria),
			"testCommands":   storySpec.TestCommands,
		})
	}

	fmt.Fprintf(human, "\nGenerated Verified Story Spec: %s\n", specPath)
	fmt.Fprintf(human, "Generated Provenance Sidecar: %s\n", provPath)
	fmt.Fprintf(human, "Title: %s\n", storySpec.Title)
	fmt.Fprintf(human, "Acceptance Criteria: %d scenarios\n", len(storySpec.AcceptanceCriteria))
	fmt.Fprintf(human, "Verification Commands: %v\n", storySpec.TestCommands)
	return 0
}

func runCode(cwd string, reg *persona.Registry, args []string, stdin io.Reader, human io.Writer, sendJSON func(any), isJSON bool, isStream bool, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("code", flag.ContinueOnError)
	fs.SetOutput(stderr)
	domainFlag := fs.String("domain", "backend_engineer", "Target SWE domain persona (e.g. backend_engineer, android_engineer)")
	autonomyFlag := fs.String("autonomy", "supervised", "Autonomy gate: supervised, interactive, or autonomous")
	maxRoundsFlag := fs.Int("rounds", 3, "Maximum convergence rounds")
	providerFlag := fs.String("provider", "", "Model provider that writes the patches: anthropic, openai, gemini, grok, deepseek, mistral, ollama")
	modelFlag := fs.String("model", "", "Model name for --provider")
	reviewProvider := fs.String("review-provider", "", "Provider for the model reviewer")
	reviewModel := fs.String("review-model", "", "Model for --review-provider")
	noModelReview := fs.Bool("no-model-review", false, "Skip the model review of acceptance criteria")
	maxDiffKb := fs.Int("max-diff-kb", 500, "Maximum diff size in KB before critic hard rejects")
	printTestCommands := fs.Bool("print-test-commands", false, "Print planned test commands and hash from spec and exit")
	confirmTestsHashFlag := fs.String("confirm-tests-hash", "", "Sha256 hash of confirmed test commands (e.g. sha256:...)")
	confirmTestsFlag := fs.Bool("confirm-tests", false, "Explicitly confirm proposed test commands without interactive prompt")
	yesFlag := fs.Bool("yes", false, "Alias for --confirm-tests")
	fs.BoolVar(yesFlag, "y", false, "Short alias for --confirm-tests")
	forgeFlag := fs.String("forge", os.Getenv("ARTIX_FORGE_TYPE"), "Forge provider: github or gitlab")
	forgePRFlag := fs.Int("forge-pr", 0, "Pull Request / Merge Request number for forge approval verification")
	forgeTokenFlag := fs.String("forge-token", "", "Forge API token (defaults to GITHUB_TOKEN or GITLAB_TOKEN)")
	forgeURLFlag := fs.String("forge-url", os.Getenv("ARTIX_FORGE_URL"), "Forge Base API URL")
	forgeOwnerFlag := fs.String("forge-owner", "", "Forge repository owner/organization")
	forgeRepoFlag := fs.String("forge-repo", "", "Forge repository name")
	approverFlag := fs.String("approver", "", "Human approver identity")
	analyzerFlag := fs.String("analyzer", os.Getenv("ARTIX_ANALYZER_COMMANDS"), "Comma-separated analyzer commands (e.g. 'detekt', 'swiftlint')")
	if err := fs.Parse(args); err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		return 1
	}

	// Enterprise Safety Gate
	if *autonomyFlag == "autonomous" && !policy.IsAutonomousAllowed() {
		errStr := "Error: Enterprise safety violation: --autonomy autonomous is disabled by default in enterprise/CI environments or disallowed by policy.\nA verified, cryptographically signed policy with allowAutonomous=true is required to permit unattended autonomous commits."
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1
	}

	// Enterprise Audit Gate
	if policy.IsEnterprise() {
		if err := audit.Default(cwd).InitError(); err != nil {
			errStr := fmt.Sprintf("Error: enterprise audit configuration error: %v", err)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
	}

	repoCtx, err := repo.DetectContext(cwd)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("Error detecting repository context: %v", err)})
		}
		fmt.Fprintf(stderr, "Error detecting repository context: %v\n", err)
		return 1
	}

	var specFile string
	if len(fs.Args()) > 0 {
		specFile = fs.Args()[0]
	} else {
		specs, _ := filepath.Glob(filepath.Join(cwd, "docs", "specs", "STORY-*.md"))
		if len(specs) > 0 {
			specFile = specs[len(specs)-1]
		}
	}

	if specFile == "" {
		errStr := "Error: No story spec specified and none found in docs/specs/.\nRun 'artix plan' first."
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1
	}

	rawSpec, err := os.ReadFile(specFile)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("Error reading spec %s: %v", specFile, err)})
		}
		fmt.Fprintf(stderr, "Error reading spec %s: %v\n", specFile, err)
		return 1
	}

	storySpec, err := spec.ParseFromMarkdown(string(rawSpec))
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("Error parsing spec: %v", err)})
		}
		fmt.Fprintf(stderr, "Error parsing spec: %v\n", err)
		return 1
	}

	rawHash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(storySpec.TestCommands, "\n"))))
	hashWithPrefix := "sha256:" + rawHash

	if *printTestCommands {
		if isJSON {
			sendJSON(map[string]any{
				"ok":               true,
				"status":           "test_commands",
				"testCommands":     storySpec.TestCommands,
				"testCommandsHash": hashWithPrefix,
			})
		} else {
			fmt.Fprintf(human, "Test Commands (%d):\n", len(storySpec.TestCommands))
			for i, cmd := range storySpec.TestCommands {
				fmt.Fprintf(human, "  [%d] %s\n", i+1, cmd)
			}
			fmt.Fprintf(human, "Test Commands Hash: %s\n", hashWithPrefix)
		}
		return 0
	}

	if *confirmTestsHashFlag != "" {
		providedHash := strings.TrimPrefix(strings.TrimSpace(*confirmTestsHashFlag), "sha256:")
		if providedHash != rawHash {
			errStr := fmt.Sprintf("Error: test command confirmation hash mismatch: expected %s, got %s", hashWithPrefix, *confirmTestsHashFlag)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
	}

	domainCoder, err := coder.NewDomainCoder(*domainFlag, reg)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		fmt.Fprintf(stderr, "Error creating domain coder: %v\n", err)
		return 1
	}

	advReviewer := reviewer.NewAdversarialReviewer(reg)
	if *maxDiffKb > 0 {
		advReviewer.SetMaxCriticDiffBytes(*maxDiffKb * 1024)
	}

	driver := git.NewDriver(cwd)
	box := sandbox.NewSandbox(cwd)
	coord := coder.NewCoordinator(domainCoder, advReviewer, driver, box)

	agg := steering.NewAggregator(cwd)
	rules, _ := agg.CollectLocalRules()
	binder := steering.NewBinder(steering.SteeringConfig{}, rules)
	coderSteering := binder.CompilePersonaSteering(*domainFlag)
	revSteering := binder.CompilePersonaSteering("adversarial_code_reviewer")

	opts := &coder.LoopOptions{
		MaxRounds: *maxRoundsFlag,
		Autonomy:  coder.AutonomyLevel(*autonomyFlag),
		Approver:  *approverFlag,
	}
	if isStream {
		opts.StreamEmitter = coder.NewStreamEmitter(stdout)
		opts.TaskID = storySpec.ID
	}

	prNum := *forgePRFlag
	if prNum <= 0 {
		if val := os.Getenv("ARTIX_PR_NUMBER"); val != "" {
			if n, err := strconv.Atoi(val); err == nil {
				prNum = n
			}
		} else if val := os.Getenv("CI_MERGE_REQUEST_IID"); val != "" {
			if n, err := strconv.Atoi(val); err == nil {
				prNum = n
			}
		}
	}

	if prNum > 0 || *forgeFlag != "" || *forgeTokenFlag != "" {
		opts.ForgeVerifier = forge.NewProductionVerifier(forge.ProductionVerifierConfig{
			ForgeType: *forgeFlag,
			BaseURL:   *forgeURLFlag,
			Token:     *forgeTokenFlag,
			Owner:     *forgeOwnerFlag,
			Repo:      *forgeRepoFlag,
			PRNumber:  prNum,
		})
	}

	remote := os.Getenv("ARTIX_FORGE_REMOTE")
	if remote == "" {
		remote = "origin"
	}
	prBranch := os.Getenv("ARTIX_PR_BRANCH")
	if prBranch == "" && prNum > 0 {
		prBranch = fmt.Sprintf("artix-pr-%d", prNum)
	}

	if prBranch != "" {
		opts.ForgePusher = forge.NewForgePusher(driver, remote, prBranch)
		opts.TwoPhaseAutonomous = true
	}

	if *analyzerFlag != "" {
		for _, cmd := range strings.Split(*analyzerFlag, ",") {
			cmd = strings.TrimSpace(cmd)
			if cmd != "" {
				opts.AnalyzerCommands = append(opts.AnalyzerCommands, cmd)
			}
		}
	} else if !policy.IsEnterprise() {
		analyzersCfg := filepath.Join(cwd, ".artix", "analyzers.json")
		if data, err := os.ReadFile(analyzersCfg); err == nil {
			var cfg struct {
				Commands []string `json:"commands"`
			}
			if err := json.Unmarshal(data, &cfg); err == nil && len(cfg.Commands) > 0 {
				opts.AnalyzerCommands = cfg.Commands
			}
		}
	} else {
		// In enterprise mode, use signed policy allowlisted semantic runners
		pol := policy.Active()
		if len(pol.Reviewer.AllowedSemanticRunners) > 0 {
			opts.AnalyzerCommands = pol.Reviewer.AllowedSemanticRunners
		}
	}

	// G4: Test-command trust outside enterprise:
	// In supervised/interactive mode outside enterprise, require explicit user confirmation.
	if !policy.IsEnterprise() && (opts.Autonomy == coder.AutonomySupervised || opts.Autonomy == coder.AutonomyInteractive) {
		if *confirmTestsFlag || *yesFlag || *confirmTestsHashFlag != "" {
			opts.TestCommandsConfirmed = true
		} else if isTerminal(stdin) {
			fmt.Fprintf(stderr, "Proposed test commands (%d):\n", len(storySpec.TestCommands))
			for i, cmd := range storySpec.TestCommands {
				fmt.Fprintf(stderr, "  [%d] %s\n", i+1, cmd)
			}
			fmt.Fprintf(stderr, "Confirm running these test commands? [y/N]: ")
			var response string
			if _, err := fmt.Fscanln(stdin, &response); err == nil {
				response = strings.ToLower(strings.TrimSpace(response))
				if response == "y" || response == "yes" {
					opts.TestCommandsConfirmed = true
				}
			}
			if !opts.TestCommandsConfirmed {
				errStr := "Error: unconfirmed test commands: test command confirmation rejected by user"
				if isJSON {
					sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
				}
				fmt.Fprintf(stderr, "%s\n", errStr)
				return 1
			}
		} else {
			errStr := "Error: unconfirmed test commands: supervised/interactive mode outside enterprise requires explicit user confirmation. Pass --confirm-tests or --yes in non-interactive environments."
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
	}
	if *providerFlag == "" {
		errStr := "Error: artix code needs a model to write the patches. Pass --provider and --model"
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1
	}

	opts.Model = *modelFlag
	if *reviewModel != "" {
		opts.ReviewerModel = *reviewModel
	} else {
		opts.ReviewerModel = *modelFlag
	}

	coderFamily, coderRes := reviewer.ResolveModelFamilyWithDetails(*providerFlag, *modelFlag)
	criticFamily, criticRes := reviewer.ResolveModelFamilyWithDetails(*reviewProvider, opts.ReviewerModel)
	if criticFamily == "" {
		criticFamily = coderFamily
		criticRes = coderRes
	}

	opts.CoderFamily = coderFamily
	opts.CoderFamilyResolution = coderRes
	opts.ReviewerFamily = criticFamily
	opts.ReviewerFamilyResolution = criticRes

	advReviewer.SetCoderFamily(coderFamily)
	advReviewer.SetCriticFamily(criticFamily)

	pol := policy.Active()
	if pol.IsDisjointModelFamiliesEnforced() && !*noModelReview {
		if coderFamily == "" || criticFamily == "" {
			errStr := fmt.Sprintf("Error: disjoint model families policy violation: coder (%q) and critic (%q) model families must both be configured", coderFamily, criticFamily)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
		if strings.EqualFold(coderFamily, criticFamily) {
			errStr := fmt.Sprintf("Error: disjoint model families policy violation: critic model family %q matches coder family %q (disjoint model families required)", criticFamily, coderFamily)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
	}

	modelRunner, agent, err := coder.NewAPIRunnerFromEnv(*providerFlag, *modelFlag, os.Getenv)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	var revUsageMu sync.Mutex
	var lastRevUsage *coder.ProviderUsage
	if !*noModelReview {
		rp, rm, rRunner, rAgent := *providerFlag, *modelFlag, modelRunner, agent
		if *reviewProvider != "" {
			rp, rm = *reviewProvider, *reviewModel
			if rm == "" {
				rm = *modelFlag
			}
			var rerr error
			if rRunner, rAgent, rerr = coder.NewAPIRunnerFromEnv(rp, rm, os.Getenv); rerr != nil {
				if isJSON {
					sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("reviewer: %v", rerr)})
				}
				fmt.Fprintf(stderr, "Error: reviewer: %v\n", rerr)
				return 1
			}
		}
		advReviewer.SetCritic(reviewer.RunnerCriticWithTracker(rRunner, rAgent, func(tin, tout, total int) {
			revUsageMu.Lock()
			lastRevUsage = &coder.ProviderUsage{
				PromptTokens: tin, CompletionTokens: tout, TotalTokens: total,
			}
			revUsageMu.Unlock()
		}))
		opts.ReviewerUsageTracker = func() *coder.ProviderUsage {
			revUsageMu.Lock()
			defer revUsageMu.Unlock()
			return lastRevUsage
		}
	}

	var coderUsageMu sync.Mutex
	var lastCoderUsage *coder.ProviderUsage
	opts.PatchGenerator = coder.NewRunnerPatchGeneratorWithTracker(modelRunner, agent, domainCoder, coder.PromptContext{
		Spec: storySpec, RepoContext: repoCtx, SteeringContext: coderSteering,
	}, func(pu *coder.ProviderUsage) {
		coderUsageMu.Lock()
		lastCoderUsage = pu
		coderUsageMu.Unlock()
	})
	opts.CoderUsageTracker = func() *coder.ProviderUsage {
		coderUsageMu.Lock()
		defer coderUsageMu.Unlock()
		return lastCoderUsage
	}

	fmt.Fprintf(human, "Starting convergence loop for Spec: %s (%s)...\n", storySpec.ID, storySpec.Title)
	res := coord.Run(context.Background(), storySpec, repoCtx, coderSteering, revSteering, opts)

	auditStatus := "SUCCESS"
	if res.AwaitingApproval {
		auditStatus = "AWAITING_APPROVAL"
	} else if !res.Success {
		auditStatus = "FAILED"
	}
	_ = audit.Default(cwd).Emit(audit.AuditEvent{
		EventType: audit.EventCodeConvergence,
		Status:    auditStatus,
		Details: map[string]any{
			"specId":     storySpec.ID,
			"title":      storySpec.Title,
			"domain":     *domainFlag,
			"roundsRun":  res.RoundsRun,
			"commitHash": res.CommitHash,
			"error":      res.Error,
		},
	})

	if isJSON {
		statusStr := "success"
		if res.AwaitingApproval {
			statusStr = "awaiting_approval"
		} else if !res.Success {
			if strings.Contains(strings.ToLower(res.Error), "unreviewed") {
				statusStr = "unreviewed"
			} else {
				statusStr = "rejected"
			}
		}
		sendJSON(map[string]any{
			"ok":               res.Success,
			"success":          res.Success,
			"status":           statusStr,
			"awaitingApproval": res.AwaitingApproval,
			"roundsRun":        res.RoundsRun,
			"commitHash":       res.CommitHash,
			"verdictHash":      res.VerdictHash,
			"error":            res.Error,
			"finalVerdict":     res.FinalVerdict,
			"costReport":       res.CostReport,
		})
	}

	if res.AwaitingApproval {
		fmt.Fprintf(human, "\nPHASE 1 COMPLETE: Candidate commit %s pushed to PR branch.\nAwaiting human approval on forge before merge.\n", res.CommitHash)
		return 0
	} else if res.Success {
		fmt.Fprintf(human, "\nSUCCESS: Convergence achieved in round %d!\n", res.RoundsRun)
		if res.CommitHash != "" {
			fmt.Fprintf(human, "Committed: %s\n", res.CommitHash)
		}
		return 0
	} else {
		fmt.Fprintf(human, "\nFAILED to converge: %s\n", res.Error)
		return 1
	}
}

func runVerifyApproval(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify-approval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prFlag := fs.Int("pr", 0, "Pull Request / Merge Request number")
	shaFlag := fs.String("sha", "", "Candidate commit SHA to verify and merge")
	verdictHashFlag := fs.String("verdict-hash", "", "Expected reviewer verdict SHA-256 hash from Phase 1")
	forgeFlag := fs.String("forge", os.Getenv("ARTIX_FORGE_TYPE"), "Forge provider: github or gitlab")
	tokenFlag := fs.String("forge-token", "", "Forge API token (defaults to GITHUB_TOKEN or GITLAB_TOKEN)")
	urlFlag := fs.String("forge-url", os.Getenv("ARTIX_FORGE_URL"), "Forge Base API URL")
	ownerFlag := fs.String("forge-owner", "", "Forge repository owner")
	repoFlag := fs.String("forge-repo", "", "Forge repository name")
	remoteFlag := fs.String("remote", "origin", "Git remote name")
	branchFlag := fs.String("branch", "", "Remote PR branch to verify and clean up on failure")
	specIDFlag := fs.String("spec", "", "Story Spec ID")

	if err := fs.Parse(args); err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		return 1
	}

	if *shaFlag == "" || *prFlag <= 0 || strings.TrimSpace(*specIDFlag) == "" {
		errStr := "Error: --pr <number>, --sha <commit-sha>, and --spec <spec-id> are required for merge verification"
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1
	}

	verifier := forge.NewProductionVerifier(forge.ProductionVerifierConfig{
		ForgeType: *forgeFlag,
		BaseURL:   *urlFlag,
		Token:     *tokenFlag,
		Owner:     *ownerFlag,
		Repo:      *repoFlag,
		PRNumber:  *prFlag,
	})

	driver := git.NewDriver(cwd)
	prBranch := *branchFlag
	if prBranch == "" {
		prBranch = fmt.Sprintf("artix-pr-%d", *prFlag)
	}

	approval, err := forge.VerifyAndMergeCandidate(context.Background(), driver, verifier, *shaFlag, audit.Default(cwd), *specIDFlag, *remoteFlag, prBranch, *verdictHashFlag)

	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "rejected", "error": err.Error()})
		}
		fmt.Fprintf(stderr, "Merge verification failed: %v\n", err)
		return 1
	}

	if isJSON {
		sendJSON(map[string]any{
			"ok":         true,
			"status":     "approval_verified",
			"approver":   approval.ApproverUsername,
			"commitHash": *shaFlag,
			"prNumber":   *prFlag,
		})
	}
	fmt.Fprintf(human, "Phase 2 Approval Verified: Approved by %s (Commit: %s, PR: #%d)\n", approval.ApproverUsername, *shaFlag, *prFlag)
	return 0
}

func runReview(cwd string, reg *persona.Registry, args []string, human io.Writer, sendJSON func(any), isJSON bool, isStream bool, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerFlag := fs.String("provider", "", "Model provider for the Critic (omit for rule-based pre-filter only, which yields 'unreviewed')")
	modelFlag := fs.String("model", "", "Model name for --provider")
	if err := fs.Parse(args); err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
		}
		return 1
	}

	if isStream {
		emitter := coder.NewStreamEmitter(stdout)
		emitter.Emit(coder.StreamEvent{
			Type:    coder.EventReviewStart,
			Phase:   "REVIEW",
			Message: "Reviewing working directory diff against taboo patterns and critic rules",
		})
	}

	driver := git.NewDriver(cwd)
	diff, err := driver.Diff(false)
	if err != nil {
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("Error reading git diff: %v", err)})
		}
		if isStream {
			emitter := coder.NewStreamEmitter(stdout)
			emitter.Emit(coder.StreamEvent{
				Type:    coder.EventError,
				Phase:   "REVIEW",
				Status:  "error",
				Message: fmt.Sprintf("Error reading git diff: %v", err),
			})
		}
		fmt.Fprintf(stderr, "Error reading git diff: %v\n", err)
		return 1
	}

	if strings.TrimSpace(diff) == "" {
		if isJSON {
			sendJSON(map[string]any{"ok": true, "status": "approved", "approved": true, "summary": "Working tree clean; nothing to review."})
		}
		if isStream {
			emitter := coder.NewStreamEmitter(stdout)
			emitter.Emit(coder.StreamEvent{
				Type:    coder.EventReviewVerdict,
				Phase:   "REVIEW",
				Status:  "approved",
				Message: "Working tree clean; nothing to review.",
				Payload: map[string]any{"approved": true},
			})
		}
		fmt.Fprintln(human, "Working tree clean; nothing to review.")
		return 0
	}

	advReviewer := reviewer.NewAdversarialReviewer(reg)
	if *providerFlag != "" {
		rRunner, rAgent, rerr := coder.NewAPIRunnerFromEnv(*providerFlag, *modelFlag, os.Getenv)
		if rerr != nil {
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": fmt.Sprintf("reviewer: %v", rerr)})
			}
			if isStream {
				emitter := coder.NewStreamEmitter(stdout)
				emitter.Emit(coder.StreamEvent{
					Type:    coder.EventError,
					Phase:   "REVIEW",
					Status:  "error",
					Message: fmt.Sprintf("reviewer: %v", rerr),
				})
			}
			fmt.Fprintf(stderr, "Error: reviewer: %v\n", rerr)
			return 1
		}
		advReviewer.SetCritic(reviewer.RunnerCritic(rRunner, rAgent))
	}
	testCount := reviewer.CountTestsInWorkspace(cwd)
	rCtx := &reviewer.ReviewContext{
		Diff:            diff,
		WorkspaceDir:    cwd,
		TestCountBefore: testCount,
		TestCountAfter:  testCount,
	}

	verdict := advReviewer.Evaluate(rCtx)

	status := "SUCCESS"
	if !verdict.Approved {
		status = "REJECTED"
	}
	_ = audit.Default(cwd).Emit(audit.AuditEvent{
		EventType: audit.EventReviewerVerdict,
		Status:    status,
		Details: map[string]any{
			"summary":        verdict.Summary,
			"blockingIssues": verdict.BlockingIssues,
			"warnings":       verdict.Warnings,
		},
	})

	if isStream {
		emitter := coder.NewStreamEmitter(stdout)
		emitter.Emit(coder.StreamEvent{
			Type:    coder.EventReviewVerdict,
			Phase:   "REVIEW",
			Status:  string(verdict.Status),
			Message: verdict.Summary,
			Payload: map[string]any{
				"status":         verdict.Status,
				"approved":       verdict.Approved,
				"blockingIssues": verdict.BlockingIssues,
				"warnings":       verdict.Warnings,
			},
		})
	}

	if isJSON {
		reviewStatus := "approved"
		ok := true
		if verdict.Status == reviewer.StatusUnreviewed {
			reviewStatus = "unreviewed"
			ok = false
		} else if !verdict.Approved {
			reviewStatus = "rejected"
			ok = false
		}
		sendJSON(map[string]any{
			"ok":             ok,
			"status":         reviewStatus,
			"approved":       verdict.Approved,
			"summary":        verdict.Summary,
			"blockingIssues": verdict.BlockingIssues,
			"warnings":       verdict.Warnings,
		})
	}

	if verdict.Approved {
		fmt.Fprintf(human, "REVIEW PASSED: %s\n", verdict.Summary)
		return 0
	} else if verdict.Status == reviewer.StatusUnreviewed {
		fmt.Fprintf(human, "REVIEW UNREVIEWED: %s\n", verdict.Summary)
		return 1
	} else {
		fmt.Fprintf(human, "REVIEW REJECTED: %s\n\n%s\n", verdict.Summary, verdict.ActionableFeedback)
		return 1
	}
}

func runAudit(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: artix audit <subcommand>\n\nSubcommands:\n  verify [path] [--key <secret>]  Verify cryptographic hash-chain integrity of audit logs\n")
		return 1
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "verify":
		fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
		fs.SetOutput(stderr)
		keyFlag := fs.String("key", "", "HMAC signing key for cryptographic signature verification")
		pubKeyFlag := fs.String("pubkey", "", "Hex-encoded Ed25519 public key for asymmetric verification")
		expectedCountFlag := fs.Int("expected-count", 0, "Expected minimum record count to detect tail truncation")
		expectedHashFlag := fs.String("expected-hash", "", "Expected last record hash to detect tail truncation")
		if err := fs.Parse(subargs); err != nil {
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
			}
			return 1
		}

		logPath := policy.EffectiveAuditLogPath(cwd)
		if len(fs.Args()) > 0 {
			logPath = fs.Args()[0]
		}

		if policy.IsEnterprise() && *keyFlag != "" {
			errStr := "SECURITY ERROR: enterprise mode requires ed25519 public key verification; symmetric HMAC key verification is disallowed"
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}

		var pubKey ed25519.PublicKey
		if *pubKeyFlag != "" {
			raw, err := hex.DecodeString(strings.TrimSpace(*pubKeyFlag))
			if err != nil || len(raw) != ed25519.PublicKeySize {
				errStr := fmt.Sprintf("Invalid ed25519 public key hex: %v", err)
				if isJSON {
					sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
				}
				fmt.Fprintf(stderr, "%s\n", errStr)
				return 1
			}
			pubKey = ed25519.PublicKey(raw)
		} else if policy.IsEnterprise() || policy.Active().AuditPublicKey != "" {
			if polKey := policy.Active().AuditPublicKey; polKey != "" {
				raw, err := hex.DecodeString(strings.TrimSpace(polKey))
				if err == nil && len(raw) == ed25519.PublicKeySize {
					pubKey = ed25519.PublicKey(raw)
				}
			}
			if policy.IsEnterprise() && len(pubKey) == 0 {
				errStr := "SECURITY ERROR: enterprise mode requires ed25519 public key in signed policy or via --pubkey; cannot verify without public key"
				if isJSON {
					sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
				}
				fmt.Fprintf(stderr, "%s\n", errStr)
				return 1
			}
		}

		fmt.Fprintf(human, "Verifying audit log integrity: %s\n", logPath)
		opts := audit.VerifyOptions{
			PubKey:           pubKey,
			SigningKey:       *keyFlag,
			ExpectedCount:    *expectedCountFlag,
			ExpectedLastHash: *expectedHashFlag,
		}
		res, err := audit.VerifyLogWithOptions(logPath, opts)
		if err != nil {
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "tampered", "error": err.Error()})
			}
			fmt.Fprintf(stderr, "\nAUDIT INTEGRITY VIOLATION: %v\n", err)
			return 1
		}

		if isJSON {
			sendJSON(map[string]any{"ok": true, "status": "verified", "validRecords": res.ValidRecords, "lastHash": res.LastHash})
		}
		fmt.Fprintf(human, "AUDIT LOG VERIFIED: %d record(s) valid, hash chain intact (LastHash: %s).\n", res.ValidRecords, res.LastHash[:16]+"...")
		return 0

	default:
		fmt.Fprintf(stderr, "Unknown audit subcommand: %s. Supported: verify\n", subcmd)
		return 1
	}
}

func handleSteering(cwd string, args []string) {
	mgr := steering.NewManager(cwd)

	if len(args) == 0 || args[0] == "list" {
		rules, err := mgr.ListRules()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error collecting steering rules: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d steering rules:\n", len(rules))
		for _, r := range rules {
			fmt.Printf(" - [%s] %s (%s)\n", r.SourceType, r.Name, r.RelPath)
		}
		return
	}

	if args[0] == "bind" && len(args) >= 3 {
		personaID := args[1]
		ruleID := args[2]
		if err := mgr.BindRule(personaID, ruleID); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to bind rule: %v\n", err)
			os.Exit(1)
		}
		_ = audit.Default(cwd).Emit(audit.AuditEvent{
			EventType: audit.EventSteeringBind,
			Status:    "SUCCESS",
			Details: map[string]any{
				"personaId": personaID,
				"ruleId":    ruleID,
			},
		})
		fmt.Printf("Bound rule %q to persona %q\n", ruleID, personaID)
		return
	}

	if args[0] == "pending" {
		pending, err := mgr.ListPendingRules()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing pending rules: %v\n", err)
			os.Exit(1)
		}
		if len(pending) == 0 {
			fmt.Println("No pending steering rules awaiting review.")
			return
		}
		fmt.Printf("Pending Steering Rules Review Queue (%d):\n", len(pending))
		for _, pr := range pending {
			fmt.Printf(" - [%s] %s (Taboo: %v, Roles: %v, Author: %s)\n   Text: %q\n",
				pr.Hash, pr.Name, pr.IsTaboo, pr.TargetRoles, pr.Author, pr.RuleText)
		}
		return
	}

	if args[0] == "approve" && len(args) >= 2 {
		hash := args[1]
		approver := "senior_architect"
		if len(args) >= 3 {
			approver = args[2]
		}
		rule, err := mgr.ApproveRule(hash, approver)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to approve rule: %v\n", err)
			os.Exit(1)
		}
		_ = audit.Default(cwd).Emit(audit.AuditEvent{
			EventType: audit.EventSteeringBind,
			Status:    "SUCCESS",
			Details: map[string]any{
				"ruleId":   rule.ID,
				"action":   "approve",
				"approver": approver,
				"ruleHash": hash,
			},
		})
		fmt.Printf("Successfully approved rule %q (%s) as %s.\n", rule.ID, rule.Name, approver)
		return
	}

	if args[0] == "reject" && len(args) >= 2 {
		hash := args[1]
		if err := mgr.RejectRule(hash); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to reject rule: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rejected and removed pending rule %q.\n", hash)
		return
	}

	if args[0] == "sync" {
		fmt.Println("Syncing external steering rules...")
		syncer := steering.NewRemoteSyncer()
		fmt.Println("Remote syncer initialized. Cache at ~/.artix/cache/steering")
		_ = syncer
		return
	}

	fmt.Fprintf(os.Stderr, "Unknown steering subcommand: %s. Supported: list, pending, approve <hash> [role], reject <hash>, sync, bind <persona> <rule>\n", args[0])
}

func handleGC(cwd string, args []string) {
	fs := flag.NewFlagSet("gc", flag.ExitOnError)
	maxAgeFlag := fs.Duration("max-age", 24*time.Hour, "Maximum age threshold for orphaned shadow worktrees (e.g. 24h, 1h)")
	fs.Parse(args)

	wtMgr := git.NewWorktreeManager(cwd)
	pruned, err := wtMgr.GarbageCollect(*maxAgeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Garbage collection failed: %v\n", err)
		os.Exit(1)
	}

	_ = audit.Default(cwd).Emit(audit.AuditEvent{
		EventType: audit.EventWorktreeGC,
		Status:    "SUCCESS",
		Details: map[string]any{
			"prunedCount": pruned,
			"maxAge":      maxAgeFlag.String(),
		},
	})

	fmt.Printf("Worktree Garbage Collection complete. Pruned %d stale worktree(s).\n", pruned)
}

func handleDaemon(cwd string, reg *persona.Registry, args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "Server listen address")
	fs.Parse(args)

	workerDir := filepath.Join(cwd, ".artix", "worker_cache")
	worker := forge.NewRemoteWorker(workerDir, reg)

	server := forge.NewWebhookServer(forge.WebhookServerConfig{
		ListenAddr: *addr,
		Worker:     worker,
	})

	fmt.Printf("Artix Daemon listening on %s (Endpoints: /healthz, /webhook/github, /webhook/gitlab)...\n", *addr)
	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "Daemon server error: %v\n", err)
		os.Exit(1)
	}
}

func findCodeowners(workspaceDir string) []string {
	var candidates []string
	if workspaceDir != "" {
		candidates = append(candidates,
			filepath.Join(workspaceDir, "CODEOWNERS"),
			filepath.Join(workspaceDir, ".github", "CODEOWNERS"),
			filepath.Join(workspaceDir, "docs", "CODEOWNERS"),
		)
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var owners []string
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			parts := strings.Fields(trimmed)
			for _, part := range parts {
				if strings.HasPrefix(part, "@") {
					owners = append(owners, strings.TrimPrefix(part, "@"))
				} else if strings.Contains(part, "@") {
					owners = append(owners, part)
				}
			}
		}
		if len(owners) > 0 {
			return owners
		}
	}
	return nil
}

func runKnowledge(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: artix knowledge [ratify|eval|list] [options] [arguments]\n")
		return 1
	}

	sub := args[0]
	subArgs := args[1:]
	store := knowledge.NewStore(cwd)

	switch sub {
	case "ratify":
		fs := flag.NewFlagSet("knowledge ratify", flag.ContinueOnError)
		fs.SetOutput(stderr)
		approverFlag := fs.String("approver", "", "Approver identity (must be in CODEOWNERS)")

		var flagArgs []string
		var posArgs []string
		for i := 0; i < len(subArgs); i++ {
			a := subArgs[i]
			if a == "--approver" || a == "-approver" {
				if i+1 < len(subArgs) {
					flagArgs = append(flagArgs, a, subArgs[i+1])
					i++
				}
			} else if strings.HasPrefix(a, "--approver=") || strings.HasPrefix(a, "-approver=") {
				flagArgs = append(flagArgs, a)
			} else if strings.HasPrefix(a, "-") {
				flagArgs = append(flagArgs, a)
			} else {
				posArgs = append(posArgs, a)
			}
		}
		if err := fs.Parse(flagArgs); err != nil {
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": err.Error()})
			}
			return 1
		}

		if len(posArgs) == 0 {
			errStr := "Error: Knowledge Item ID is required. Example: artix knowledge ratify ki-123 --approver alice"
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
		kiID := posArgs[0]

		ki, err := store.Get(kiID)
		if err != nil {
			errStr := fmt.Sprintf("Error: knowledge item %s not found: %v", kiID, err)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}

		approver := *approverFlag
		if approver == "" {
			approver = os.Getenv("USER")
		}
		if approver == "" {
			errStr := "Error: approver identity is required. Specify via --approver"
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}

		codeowners := findCodeowners(cwd)
		if err := ki.Ratify(approver, codeowners); err != nil {
			errStr := fmt.Sprintf("Error: ratification failed: %v", err)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}

		if err := store.Save(ki); err != nil {
			errStr := fmt.Sprintf("Error saving ratified knowledge item: %v", err)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}

		if isJSON {
			sendJSON(map[string]any{
				"ok":         true,
				"status":     "ratified",
				"id":         ki.ID,
				"approvedBy": ki.ApprovedBy,
			})
		} else {
			fmt.Fprintf(human, "Successfully ratified Knowledge Item %s (approver: %s)\n", ki.ID, ki.ApprovedBy)
		}
		return 0

	case "list":
		items, err := store.ListActive()
		if err != nil {
			errStr := fmt.Sprintf("Error listing knowledge items: %v", err)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
		if isJSON {
			sendJSON(map[string]any{"ok": true, "items": items})
		} else {
			fmt.Fprintf(human, "Active Knowledge Items (%d):\n", len(items))
			for _, item := range items {
				fmt.Fprintf(human, " - [%s] %s: %s (ID: %s)\n", item.Category, item.Title, item.Breakthrough, item.ID)
			}
		}
		return 0

	case "eval":
		if len(subArgs) == 0 {
			errStr := "Error: Knowledge Item ID is required. Example: artix knowledge eval ki-123"
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}
		kiID := subArgs[0]
		ki, err := store.Get(kiID)
		if err != nil {
			errStr := fmt.Sprintf("Error: knowledge item %s not found: %v", kiID, err)
			if isJSON {
				sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
			}
			fmt.Fprintf(stderr, "%s\n", errStr)
			return 1
		}

		ki.LastEvalResult = &knowledge.ABEvalResult{
			TargetID:   ki.ID,
			Status:     "unmeasured",
			Regression: false,
			Summary:    fmt.Sprintf("A/B evaluation unmeasured: no evaluation harness or active session available to measure %s", ki.ID),
		}
		_ = store.Save(ki)

		errStr := fmt.Sprintf("Error: A/B evaluation unmeasured: no evaluation harness or active session available to measure %s", ki.ID)
		if isJSON {
			sendJSON(map[string]any{
				"ok":      false,
				"status":  "unmeasured",
				"id":      ki.ID,
				"error":   errStr,
				"summary": ki.LastEvalResult.Summary,
			})
		} else {
			fmt.Fprintf(stderr, "%s\n", errStr)
		}
		return 1

	default:
		fmt.Fprintf(stderr, "Unknown knowledge subcommand: %s\n", sub)
		return 1
	}
}

func runWarm(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	targetDir := cwd
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		targetDir = args[0]
		if !filepath.IsAbs(targetDir) {
			targetDir = filepath.Join(cwd, targetDir)
		}
	}
	reg := plugins.NewRegistry()
	driver, found := reg.DetectDriver(targetDir)
	if !found {
		errStr := fmt.Sprintf("Error: no supported build driver detected in %s to pre-warm", targetDir)
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1
	}

	if err := driver.Warm(context.Background(), targetDir); err != nil {
		errStr := fmt.Sprintf("Error: pre-warm dependencies failed for %s (%s): %v", targetDir, driver.Name(), err)
		if isJSON {
			sendJSON(map[string]any{"ok": false, "status": "error", "error": errStr})
		}
		fmt.Fprintf(stderr, "%s\n", errStr)
		return 1
	}

	if isJSON {
		sendJSON(map[string]any{"ok": true, "status": "warmed", "driver": driver.Name()})
	} else {
		fmt.Fprintf(human, "Pre-warmed offline dependency cache for driver %s in %s\n", driver.Name(), targetDir)
	}
	return 0
}
