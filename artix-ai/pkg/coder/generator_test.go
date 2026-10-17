package coder

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
	"socratix/pkg/model"
	"socratix/pkg/runner"
)

const patchTo = `--- a/counter.txt
+++ b/counter.txt
@@ -1 +1 @@
-0
+%s
`

func patch(v string) string { return strings.Replace(patchTo, "%s", v, 1) }

type fakeModel struct {
	replies []string
	err     error
	calls   []string // the user prompt of each call
}

func (f *fakeModel) Respond(_ context.Context, _ model.Agent, _ string, commonContext, _ string, _ []model.DebateMessage, _ string) (runner.AgentReply, error) {
	f.calls = append(f.calls, commonContext)
	if f.err != nil {
		return runner.AgentReply{}, f.err
	}
	i := len(f.calls) - 1
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return runner.AgentReply{Content: f.replies[i]}, nil
}

func TestExtractUnifiedDiff(t *testing.T) {
	d := patch("1")
	for name, reply := range map[string]string{
		"bare":           d,
		"fenced diff":    "Here you go:\n```diff\n" + d + "```\nDone.",
		"fenced no lang": "```\n" + d + "```",
		"prose before":   "I changed the counter.\n\n" + d,
		"git header":     "diff --git a/counter.txt b/counter.txt\n" + d,
	} {
		got := ExtractUnifiedDiff(reply)
		if !strings.Contains(got, "--- a/counter.txt") || !strings.Contains(got, "+1") || strings.Contains(got, "Done.") || strings.Contains(got, "```") {
			t.Errorf("%s: got %q", name, got)
		}
	}
	for _, none := range []string{"", "I cannot help with that.", "--- just a rule\nno plus line"} {
		if got := ExtractUnifiedDiff(none); got != "" {
			t.Errorf("%q should yield no diff, got %q", none, got)
		}
	}
}

func liveFixture(t *testing.T) (*ConvergenceCoordinator, *DomainCoder, *spec.StorySpec, *repo.RepositoryContext, string) {
	t.Helper()
	dir, driver := setupTestRepo(t)
	t.Cleanup(func() { os.RemoveAll(dir) })
	reg := persona.NewRegistry("")
	dc, _ := NewDomainCoder("backend_engineer", reg)
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	coord := NewCoordinator(dc, rev, driver, sandbox.NewSandbox(dir))
	s := &spec.StorySpec{ID: "S-1", Title: "Set counter to 2", TestCommands: []string{"grep -q '^2$' counter.txt"}}
	return coord, dc, s, &repo.RepositoryContext{RootDir: dir}, dir
}

func TestLoopWithModelBackedGeneratorConvergesAndFeedsFailuresBack(t *testing.T) {
	coord, dc, s, rc, dir := liveFixture(t)
	fm := &fakeModel{replies: []string{"```diff\n" + patch("1") + "```", "```diff\n" + patch("2") + "```"}}
	gen := NewRunnerPatchGenerator(fm, model.NewAgent(model.ProviderAnthropic, "m"), dc, PromptContext{Spec: s})

	res := coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{MaxRounds: 3, PatchGenerator: gen})
	if !res.Success || res.RoundsRun != 2 {
		t.Fatalf("want success in 2 rounds, got %+v", res)
	}
	if len(fm.calls) != 2 {
		t.Fatalf("model should be called once per round, got %d", len(fm.calls))
	}
	if strings.Contains(fm.calls[0], "REVIEWER") {
		t.Error("round 1 has no feedback yet")
	}
	if !strings.Contains(fm.calls[1], "CRITICAL FEEDBACK FROM ADVERSARIAL REVIEWER") || !strings.Contains(fm.calls[1], "PREVIOUS COMPILER / TEST SUITE FAILURES") {
		t.Errorf("round 2 prompt must carry reviewer feedback and test failures: %s", fm.calls[1])
	}
	if b, _ := os.ReadFile(dir + "/counter.txt"); strings.TrimSpace(string(b)) != "2" {
		t.Errorf("patch not left applied: %q", b)
	}
}

func TestLoopReportsConfigurationAndGeneratorErrors(t *testing.T) {
	coord, _, s, rc, _ := liveFixture(t)

	res := coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{MaxRounds: 2})
	if res.Success || !strings.Contains(res.Error, "no patch generator configured") {
		t.Errorf("missing generator must be an explicit configuration error: %+v", res)
	}
	res = coord.Run(context.Background(), s, rc, nil, nil, nil)
	if res.Success || !strings.Contains(res.Error, "no patch generator configured") {
		t.Errorf("nil options: %+v", res)
	}

	boom := func(context.Context, PatchRequest) (string, error) { return "", errors.New("rate limited") }
	res = coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{PatchGenerator: boom})
	if res.Success || !strings.Contains(res.Error, "rate limited") || !strings.Contains(res.Error, "round 1") {
		t.Errorf("generator error must surface: %+v", res)
	}

	noDiff := NewRunnerPatchGenerator(&fakeModel{replies: []string{"sorry, no"}}, model.NewAgent(model.ProviderAnthropic, "m"), mustCoder(t), PromptContext{Spec: s})
	res = coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{PatchGenerator: noDiff})
	if res.Success || !strings.Contains(res.Error, "no unified diff") {
		t.Errorf("reply without a diff: %+v", res)
	}
}

func mustCoder(t *testing.T) *DomainCoder {
	dc, err := NewDomainCoder("backend_engineer", persona.NewRegistry(""))
	if err != nil {
		t.Fatal(err)
	}
	return dc
}

func TestAutonomousModeRefusesToCommitUnverifiedChange(t *testing.T) {
	policy.SetActivePolicyForTest(&policy.Policy{
		AllowAutonomous: true,
	})
	defer policy.ResetTestPolicy()
	coord, _, s, rc, _ := liveFixture(t)
	s.TestCommands = nil // nothing verifies the change
	res := coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{
		Autonomy:     AutonomyAutonomous,
		MockPatchGen: func(int, string) string { return patch("2") },
	})
	if res.CommitHash != "" || !strings.Contains(res.Error, "autonomous commit refused") {
		t.Fatalf("must not auto-commit with no tests: %+v", res)
	}
}

func TestInteractiveStopRollsBackThePatch(t *testing.T) {
	coord, _, s, rc, dir := liveFixture(t)
	res := coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{
		MockPatchGen: func(int, string) string { return patch("2") },
		OnIteration:  func(int, string, *reviewer.ReviewVerdict) bool { return false },
	})
	if res.Success || !strings.Contains(res.Error, "stopped by user") {
		t.Fatalf("got %+v", res)
	}
	if b, _ := os.ReadFile(dir + "/counter.txt"); strings.TrimSpace(string(b)) != "0" {
		t.Errorf("stopping must roll the working tree back, counter=%q", b)
	}
}

func TestLoopGivesUpAfterMaxRoundsAndLeavesTreeClean(t *testing.T) {
	coord, _, s, rc, dir := liveFixture(t)
	res := coord.Run(context.Background(), s, rc, nil, nil, &LoopOptions{
		MaxRounds:    2,
		MockPatchGen: func(int, string) string { return patch("1") }, // never satisfies the test
	})
	if res.Success || !strings.Contains(res.Error, "failed to converge after 2 rounds") || res.RoundsRun != 2 {
		t.Fatalf("got %+v", res)
	}
	if b, _ := os.ReadFile(dir + "/counter.txt"); strings.TrimSpace(string(b)) != "0" {
		t.Errorf("failed rounds must be rolled back, counter=%q", b)
	}
}

func TestNewAPIRunnerFromEnv(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	r, agent, err := NewAPIRunnerFromEnv(" Anthropic ", "claude-x", env(map[string]string{"ANTHROPIC_API_KEY": "k"}))
	if err != nil || r == nil || agent.Provider != model.ProviderAnthropic || agent.RunMode != model.RunModeAPI || agent.Model != "claude-x" {
		t.Fatalf("got %+v %+v %v", r, agent, err)
	}
	if _, _, err := NewAPIRunnerFromEnv("ollama", "llama3", env(nil)); err != nil {
		t.Errorf("ollama needs no key: %v", err)
	}
	for name, args := range map[string][3]string{
		"missing key":      {"openai", "gpt", ""},
		"unknown provider": {"skynet", "m", "k"},
		"missing model":    {"anthropic", " ", "k"},
	} {
		_, _, err := NewAPIRunnerFromEnv(args[0], args[1], env(map[string]string{"ANTHROPIC_API_KEY": args[2], "OPENAI_API_KEY": args[2]}))
		if err == nil {
			t.Errorf("%s: expected error", name)
		} else if strings.Contains(err.Error(), "k\"") {
			t.Errorf("%s: error must never echo a key: %v", name, err)
		}
	}
}
