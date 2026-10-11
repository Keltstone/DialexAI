package coder

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"socratix/pkg/model"
	"socratix/pkg/runner"
)

// PatchRequest is what the loop hands a generator each round.
type PatchRequest struct {
	Round         int
	Feedback      string   // reviewer feedback from the previous round
	PriorFailures []string // compiler/test output from the previous round
	PromptContext *PromptContext
}

// PatchGenerator produces a unified diff for one round of the convergence loop.
type PatchGenerator func(ctx context.Context, req PatchRequest) (string, error)

// NewRunnerPatchGenerator asks a model (any engine AgentRunner: API or CLI) for the patch,
// using the coder's compiled prompt plus the previous round's feedback and failures.
func NewRunnerPatchGenerator(r runner.AgentRunner, agent model.Agent, dc *DomainCoder, base PromptContext) PatchGenerator {
	return NewRunnerPatchGeneratorWithTracker(r, agent, dc, base, nil)
}

// NewRunnerPatchGeneratorWithTracker allows registering a callback invoked with the actual provider usage.
func NewRunnerPatchGeneratorWithTracker(r runner.AgentRunner, agent model.Agent, dc *DomainCoder, base PromptContext, onUsage func(*ProviderUsage)) PatchGenerator {
	return func(ctx context.Context, req PatchRequest) (string, error) {
		pc := base
		pc.ReviewerFeedback = req.Feedback
		pc.PriorFailures = req.PriorFailures
		system, user := dc.CompilePrompt(&pc)

		reply, err := r.Respond(ctx, agent, pc.Spec.Title, user, system, nil, "")
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
			onUsage(&ProviderUsage{
				PromptTokens:     tin,
				CompletionTokens: tout,
				TotalTokens:      tin + tout,
			})
		}
		diff := ExtractUnifiedDiff(reply.Content)
		if diff == "" {
			return "", errors.New("model reply contained no unified diff")
		}
		return diff, nil
	}
}

var diffFence = regexp.MustCompile("(?s)```(?:diff|patch)?\\s*\\n(.*?)```")

// ExtractUnifiedDiff pulls a unified diff out of a model reply that may wrap it in markdown
// fences or prose. It returns "" if nothing that looks like a diff is present.
func ExtractUnifiedDiff(reply string) string {
	candidates := []string{reply}
	for _, m := range diffFence.FindAllStringSubmatch(reply, -1) {
		candidates = append([]string{m[1]}, candidates...) // prefer fenced blocks
	}
	for _, c := range candidates {
		lines := strings.Split(c, "\n")
		for i, l := range lines {
			if strings.HasPrefix(l, "diff --git ") || (strings.HasPrefix(l, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ")) {
				return strings.TrimRight(strings.Join(lines[i:], "\n"), "\n") + "\n"
			}
		}
	}
	return ""
}
