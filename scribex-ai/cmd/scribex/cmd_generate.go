package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"scribex/pkg/config"
	"scribex/pkg/doctypes"
	"scribex/pkg/generate"
	"scribex/pkg/llm"
)

type genFlags struct {
	types, include, exclude, backend, model, cli, resume *string
	maxCalls                                             *int
	maxWall                                              *time.Duration
	apply, confirmSend, branch, push, asJSON, enableGen  *bool
	prune                                                *bool
}

var gen genFlags

func registerGenFlags(fs *flag.FlagSet) {
	gen = genFlags{
		types:       fs.String("types", "", "generate: comma-separated doc type ids (default: all enabled)"),
		include:     fs.String("include", "", "generate: comma-separated include globs"),
		exclude:     fs.String("exclude", "", "generate: comma-separated exclude globs"),
		backend:     fs.String("backend", "", "generate: cli|ollama|anthropic|openai (default: ai.provider)"),
		model:       fs.String("model", "", "generate: model override"),
		cli:         fs.String("cli", "", "generate: vendor CLI override ("+strings.Join(config.CLINames, "|")+")"),
		resume:      fs.String("resume", "", "generate: run id of a saved plan"),
		maxCalls:    fs.Int("max-calls", 0, "generate: model-call ceiling (default ai.max_calls)"),
		maxWall:     fs.Duration("max-wall", 0, "generate: wall-time ceiling, e.g. 30m"),
		apply:       fs.Bool("apply", false, "generate: write docs (default: plan only)"),
		confirmSend: fs.Bool("confirm-send", false, "generate: allow source files to leave this machine (non-local backends)"),
		prune:       fs.Bool("prune", false, "generate: remove AI-managed orphan sections whose symbols were deleted"),
		branch:      fs.Bool("branch", false, "generate: commit the result to <prefix>/docs-<sha>"),
		push:        fs.Bool("push", false, "generate: also push that branch (never the current one)"),
		asJSON:      fs.Bool("json", false, "generate: print the plan as JSON"),
		enableGen:   fs.Bool("enable-generate", false, "serve: enable AI settings, doc types and Generate pages (admin, POST)"),
	}
}

func splitList(s string) []string {
	var o []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			o = append(o, p)
		}
	}
	return o
}

func generateCmd(root string) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if *gen.backend != "" {
		cfg.AI.Provider = *gen.backend
	}
	if *gen.model != "" {
		cfg.AI.Model = *gen.model
	}
	if *gen.cli != "" {
		cfg.AI.CLICommand = *gen.cli
	}
	if p := cfg.Validate(); len(p) > 0 {
		return fmt.Errorf("config: %s", strings.Join(p, "; "))
	}
	cat, err := doctypes.Load(root)
	if err != nil {
		return err
	}
	if probs := doctypes.Validate(root, cat); len(probs) > 0 {
		return fmt.Errorf("doctypes: %s", strings.Join(probs, "; "))
	}
	if cfg.AI.Review == "socratix" {
		for i := range cat.Types {
			if cat.Types[i].Grounding == "derived" {
				cat.Types[i].Review = "socratix"
			}
		}
	}

	var plan *generate.Plan
	if *gen.resume != "" {
		plan, err = generate.LoadPlan(root, *gen.resume)
	} else {
		plan, err = generate.MakePlan(root, cat, generate.Options{
			Root: root, Types: splitList(*gen.types), Include: splitList(*gen.include), Exclude: splitList(*gen.exclude), AI: cfg.AI,
		})
		if err == nil {
			err = generate.Save(root, plan)
		}
	}
	if err != nil {
		return err
	}
	if *gen.asJSON {
		b, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Print(plan.Markdown())
		fmt.Printf("\nPlan saved: %s/plan.json\n", generate.RunDir(plan.RunID))
	}
	if !*gen.apply {
		fmt.Fprintf(os.Stderr, "plan only. Apply with: scribex generate --resume %s --apply%s\n", plan.RunID, map[bool]string{true: "", false: " --confirm-send"}[plan.Local])
		return nil
	}
	model, err := llm.New(cfg.AI)
	if err != nil {
		return err
	}
	ao := generate.ApplyOptions{Root: root, AI: cfg.AI, Model: model, ConfirmSend: *gen.confirmSend, MaxCalls: *gen.maxCalls, MaxWall: *gen.maxWall, Prune: *gen.prune}
	if cfg.AI.Review == "socratix" || anyReview(cat) {
		if rv, rerr := generate.NewSocratix(root, cfg.AI.ReviewMode, "", ""); rerr != nil {
			fmt.Fprintln(os.Stderr, "review skipped:", rerr)
		} else {
			ao.Reviewer = rv
		}
	}
	res, err := generate.Apply(plan, cat, ao)
	if err != nil {
		return err
	}
	fmt.Printf("\nwrote %d doc(s), %d model call(s)\n", len(res.Written), res.Calls)
	for _, w := range res.Written {
		fmt.Println("  ", w)
	}
	if *gen.branch || *gen.push {
		if len(res.Written) == 0 {
			fmt.Println("nothing to publish")
		} else {
			pr, perr := generate.Publish(root, cfg.AI.BranchPrefix, plan.RunID, res.Written, *gen.push)
			if perr != nil {
				return perr
			}
			fmt.Printf("branch %s committed (%s)\n", pr.Branch, pr.Commit[:8])
			if pr.CompareURL != "" {
				fmt.Println("open the PR:", pr.CompareURL)
			}
		}
	}
	var bad []string
	bad = append(bad, res.Failed...)
	for _, i := range res.Issues {
		bad = append(bad, "check: "+i)
	}
	if res.Stopped != "" {
		bad = append(bad, "stopped: "+res.Stopped+" (resume with --resume "+plan.RunID+" --apply)")
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d problem(s):\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
	return nil
}

func anyReview(c doctypes.Catalog) bool {
	for _, t := range c.Types {
		if t.Enabled && t.Review == "socratix" {
			return true
		}
	}
	return false
}
