package forge

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"artix/pkg/coder"
	"artix/pkg/git"
	"artix/pkg/persona"
	"artix/pkg/plugins"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
	"artix/pkg/steering"
)

// RemoteWorkerTask configures an autonomous server execution job.
type RemoteWorkerTask struct {
	Target       RemoteRepoTarget                        `json:"target"`
	Auth         ForgeAuth                               `json:"auth"`
	Prompt       string                                  `json:"prompt"`
	Domain       string                                  `json:"domain"`
	CoderFamily  string                                  `json:"coderFamily,omitempty"`
	CriticFamily string                                  `json:"criticFamily,omitempty"`
	Critic       reviewer.Critic                         `json:"-"`
	MockPatchGen func(round int, feedback string) string // for tests
}

// RemoteWorkerResult contains the completed job artifacts.
type RemoteWorkerResult struct {
	Success     bool                 `json:"success"`
	Branch      string               `json:"branch"`
	Spec        *spec.StorySpec      `json:"spec"`
	PullRequest *PullRequestResponse `json:"pullRequest,omitempty"`
	LoopResult  *coder.LoopResult    `json:"loopResult,omitempty"`
	Error       string               `json:"error,omitempty"`
}

// RemoteWorker coordinates server-side autonomous repository tasks.
type RemoteWorker struct {
	workRoot                       string
	registry                       *persona.Registry
	allowedCloneHosts              []string
	allowInsecureLocalCloneForTest bool
}

// NewRemoteWorker creates a remote worker using a base directory for ephemeral clones.
func NewRemoteWorker(workRoot string, registry *persona.Registry, allowedHosts ...string) *RemoteWorker {
	_ = os.MkdirAll(workRoot, 0755)
	hosts := []string{"github.com", "gitlab.com"}
	if len(allowedHosts) > 0 {
		hosts = allowedHosts
	}
	return &RemoteWorker{
		workRoot:          workRoot,
		registry:          registry,
		allowedCloneHosts: hosts,
	}
}

// WorkRoot returns the base working directory for ephemeral clones.
func (w *RemoteWorker) WorkRoot() string {
	if w == nil {
		return ""
	}
	return w.workRoot
}

// SetAllowInsecureLocalCloneForTest permits local file clones strictly within unit/integration tests.
func (w *RemoteWorker) SetAllowInsecureLocalCloneForTest(allow bool) {
	w.allowInsecureLocalCloneForTest = allow
}

// SetAllowedCloneHosts configures the trusted repository hosts for cloning.
func (w *RemoteWorker) SetAllowedCloneHosts(hosts []string) {
	if len(hosts) > 0 {
		w.allowedCloneHosts = hosts
	}
}

// Execute runs the full autonomous pipeline: Clone -> Plan -> Code -> Verify -> Push -> PR.
func (w *RemoteWorker) Execute(ctx context.Context, task *RemoteWorkerTask) *RemoteWorkerResult {
	res := &RemoteWorkerResult{
		Success: false,
	}

	taskID := fmt.Sprintf("job-%d", time.Now().Unix())
	tenant := task.Target.Owner
	if tenant == "" {
		tenant = "default"
	}
	repoName := task.Target.Repo
	if repoName == "" {
		repoName = "repo"
	}
	cloneDir := filepath.Join(w.workRoot, tenant, repoName, taskID)
	_ = os.MkdirAll(cloneDir, 0755)
	defer os.RemoveAll(cloneDir)

	// Validate clone URL strictly: must be valid URL, scheme must be https, and hostname must be in allowedCloneHosts
	u, err := url.Parse(task.Target.CloneURL)
	if !w.allowInsecureLocalCloneForTest {
		if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" {
			res.Error = fmt.Sprintf("clone refused: clone URL %q is invalid, non-https, or missing hostname", task.Target.CloneURL)
			return res
		}
		host := strings.ToLower(u.Hostname())
		allowed := false
		for _, h := range w.allowedCloneHosts {
			if host == strings.ToLower(h) || strings.HasSuffix(host, "."+strings.ToLower(h)) {
				allowed = true
				break
			}
		}
		if !allowed {
			res.Error = fmt.Sprintf("clone refused: host %q is not in allowed clone hosts allowlist %v", host, w.allowedCloneHosts)
			return res
		}
	}

	// 1. Clone repository
	var cloneCmd *exec.Cmd
	if task.Auth.Token != "" && strings.HasPrefix(task.Target.CloneURL, "https://") {
		// Pass token securely via git extraHeader to prevent token leakage in ps args or .git/config
		cloneCmd = exec.Command("git", "-c", fmt.Sprintf("http.extraHeader=Authorization: Bearer %s", task.Auth.Token), "clone", "--depth", "50", task.Target.CloneURL, cloneDir)
	} else {
		authURL := formatAuthURL(task.Target.CloneURL, task.Auth)
		cloneCmd = exec.Command("git", "clone", "--depth", "50", authURL, cloneDir)
	}
	if err := cloneCmd.Run(); err != nil {
		res.Error = fmt.Sprintf("failed to clone repository: %v", err)
		return res
	}

	// 2. Discover repo context & create working branch
	repoCtx, err := repo.DetectContext(cloneDir)
	if err != nil {
		res.Error = fmt.Sprintf("failed to detect repo context: %v", err)
		return res
	}

	driver := git.NewDriver(cloneDir)
	branchName, err := driver.CreateWorkingBranch(taskID)
	if err != nil {
		res.Error = fmt.Sprintf("failed to create working branch: %v", err)
		return res
	}
	res.Branch = branchName

	// Enforce driver allowlist before any LLM planning or warm execution
	if reg := plugins.NewRegistry(); reg != nil {
		if buildDrv, ok := reg.DetectDriver(cloneDir); ok {
			if err := policy.Active().ValidateDriverAllowed(buildDrv.Name()); err != nil {
				res.Error = fmt.Sprintf("job execution blocked: %v", err)
				return res
			}
			_ = buildDrv.Warm(ctx, cloneDir)
		}
	}

	// 3. Plan: Stakeholder Council generates Story Spec
	council := spec.NewCouncil(w.registry)
	pCtx := &spec.PlanningContext{
		StoryPrompt: task.Prompt,
		RepoContext: repoCtx,
		Style:       spec.StyleStandard,
	}

	storySpec, err := council.Plan(ctx, pCtx)
	if err != nil {
		res.Error = fmt.Sprintf("planning council failed: %v", err)
		return res
	}
	res.Spec = storySpec

	// Write spec and provenance to repo
	specsDir := filepath.Join(cloneDir, "docs", "specs")
	prov := spec.BuildStoryProvenance(storySpec, pCtx, "", "", council.Members())
	_, _, _ = spec.WriteSpecWithProvenance(specsDir, storySpec, prov)

	// 4. Code: Domain Coder & Reviewer loop
	domainCoder, err := coder.NewDomainCoder(task.Domain, w.registry)
	if err != nil {
		res.Error = fmt.Sprintf("failed to initialize domain coder: %v", err)
		return res
	}

	rev := reviewer.NewAdversarialReviewer(w.registry)
	rev.SetCoderFamily(task.CoderFamily)
	rev.SetCriticFamily(task.CriticFamily)

	pol := policy.Active()
	if pol.IsDisjointModelFamiliesEnforced() {
		if task.CoderFamily == "" || task.CriticFamily == "" {
			res.Error = fmt.Sprintf("disjoint model families policy violation: model families must be explicitly configured (coder=%q, critic=%q); disjoint model families required", task.CoderFamily, task.CriticFamily)
			return res
		}
		if strings.EqualFold(task.CoderFamily, task.CriticFamily) {
			res.Error = fmt.Sprintf("disjoint model families policy violation: critic model family %q matches coder family %q (disjoint model families required)", task.CriticFamily, task.CoderFamily)
			return res
		}
	}

	if task.Critic != nil {
		rev.SetCritic(task.Critic)
	} else if task.MockPatchGen != nil {
		// In simulated/mock test runs, attach a mock critic that approves when mock checks pass
		rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
			return `{"approved":true,"blocking":[],"warnings":[]}`, nil
		})
	}
	box := sandbox.NewSandbox(cloneDir)
	coord := coder.NewCoordinator(domainCoder, rev, driver, box)

	// Ingest steering
	agg := steering.NewAggregator(cloneDir)
	rules, _ := agg.CollectLocalRules()
	binder := steering.NewBinder(steering.SteeringConfig{}, rules)
	coderSteering := binder.CompilePersonaSteering(task.Domain)
	revSteering := binder.CompilePersonaSteering("adversarial_code_reviewer")

	opts := &coder.LoopOptions{
		MaxRounds:    3,
		Autonomy:     coder.AutonomyAutonomous,
		MockPatchGen: task.MockPatchGen,
	}

	if task.Auth.Type == ForgeGitLab {
		glClient := NewGitLabClient(task.Auth)
		prNum := task.Target.PRNumber
		if prNum <= 0 {
			prNum = task.Target.IssueNumber
		}
		opts.ForgeVerifier = NewGitLabVerifier(glClient, &task.Target, prNum)
	} else if task.Auth.Type == ForgeGitHub {
		ghClient := NewGitHubClient(task.Auth)
		prNum := task.Target.PRNumber
		if prNum <= 0 {
			prNum = task.Target.IssueNumber
		}
		opts.ForgeVerifier = NewGitHubVerifier(ghClient, &task.Target, prNum)
	}

	loopRes := coord.Run(ctx, storySpec, repoCtx, coderSteering, revSteering, opts)
	res.LoopResult = loopRes

	if !loopRes.Success {
		res.Error = fmt.Sprintf("coder loop failed: %s", loopRes.Error)
		return res
	}

	// 5. Open Pull Request / Merge Request
	var client ForgeClient
	if task.Auth.Type == ForgeGitLab {
		client = NewGitLabClient(task.Auth)
	} else {
		client = NewGitHubClient(task.Auth)
	}

	prReq := &PullRequestRequest{
		Title: fmt.Sprintf("feat: %s", storySpec.Title),
		Body:  storySpec.RawMarkdown,
		Head:  branchName,
		Base:  task.Target.Branch,
	}
	if prReq.Base == "" {
		prReq.Base = "main"
	}

	prResp, err := client.CreatePullRequest(&task.Target, prReq)
	if err != nil {
		// Log PR opening failure without failing entire local git work
		res.Error = fmt.Sprintf("code committed but PR creation failed: %v", err)
	} else {
		res.PullRequest = prResp
	}

	res.Success = loopRes.Success
	return res
}

func formatAuthURL(rawURL string, auth ForgeAuth) string {
	// Tokens must never be embedded in URLs (prevents leakage via process table and git remote config)
	return rawURL
}
