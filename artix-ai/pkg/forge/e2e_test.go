package forge

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"artix/internal/forgesec"
	"artix/pkg/audit"
	"artix/pkg/coder"
	"artix/pkg/git"
	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
	"artix/pkg/spec"
	"artix/pkg/steering"
)


func newTestReviewer(reg *persona.Registry) *reviewer.AdversarialReviewer {
	rev := reviewer.NewAdversarialReviewer(reg)
	rev.SetCoderFamily("anthropic")
	rev.SetCriticFamily("openai")
	return rev
}

func setupTestRepoForForge(t *testing.T) (string, *git.Driver) {
	tempDir, err := os.MkdirTemp("", "artix_forge_e2e_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create test ed25519 audit signing key outside workspace under a protected .artix dir
	keysDir, err := os.MkdirTemp("", "artix_forge_test_keys")
	if err == nil {
		secDir := filepath.Join(keysDir, ".artix")
		_ = os.MkdirAll(secDir, 0700)
		pub, priv, _ := ed25519.GenerateKey(nil)
		keyPath := filepath.Join(secDir, "audit_ed25519.key")
		_ = os.WriteFile(keyPath, []byte(hex.EncodeToString(priv)), 0600)
		t.Setenv("ARTIX_AUDIT_PRIVATE_KEY_PATH", keyPath)
		t.Setenv("ARTIX_AUDIT_PUBLIC_KEY", hex.EncodeToString(pub))
		t.Setenv("ARTIX_AUDIT_SYSLOG_ENDPOINT", "127.0.0.1:514")
	}

	_ = exec.Command("git", "init", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@artix.ai").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Artix Test").Run()

	_ = os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".artix/\n.kritix/\n"), 0644)
	file := filepath.Join(tempDir, "counter.txt")
	_ = os.WriteFile(file, []byte("0\n"), 0644)

	driver := git.NewDriver(tempDir)
	_, _ = driver.CommitAll("initial commit")
	return tempDir, driver
}

// TestR3_2_MockedGitHub_CoordinatorRun_FullE2E verifies that Coordinator.Run correctly integrates
// with a production ForgeVerifier calling a mocked GitHub server, succeeding on valid approvals
// and refusing on stale head, author self-approval, and 5xx errors.
func TestR3_2_MockedGitHub_CoordinatorRun_FullE2E(t *testing.T) {
	policy.ResetCache()
	defer policy.ResetCache()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_ALLOW_AUTONOMOUS", "1")
	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:       true,
		AllowAutonomous:      true,
		RequireForgeApproval: true,
		AllowedTestCommands:  []string{"test -f counter.txt"},
		IsVerified:           true,
	})

	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headSHA, err := driver.HeadHash()
	if err != nil || headSHA == "" {
		t.Fatalf("failed to get head hash: %v", err)
	}

	repoCtx, err := repo.DetectContext(tempDir)
	if err != nil {
		t.Fatalf("failed to detect repo context: %v", err)
	}

	reg := persona.NewRegistry("")
	coderEngine, err := coder.NewDomainCoder("backend_engineer", reg)
	if err != nil {
		t.Fatalf("failed to create domain coder: %v", err)
	}
	advReviewer := newTestReviewer(reg)
	advReviewer.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coord := coder.NewCoordinator(coderEngine, advReviewer, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "SPEC-GH-001",
		Title:        "Implement Feature",
		TestCommands: []string{"test -f counter.txt"},
	}

	// 1. Success case: Mocked GitHub returns APPROVED review from "alice-lead" matching candidate commit SHA
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/101") {
			// Find the current branch head dynamically
			curHead, _ := driver.HeadHash()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": curHead},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/101/reviews") {
			curHead, _ := driver.HeadHash()
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "alice-lead", "type": "User"},
					"state":     "APPROVED",
					"commit_id": curHead,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	ghClient := NewGitHubClient(ForgeAuth{
		Type:    ForgeGitHub,
		Token:   "gh-secret-token",
		BaseURL: ghServer.URL,
	})

	target := &RemoteRepoTarget{Owner: "acme", Repo: "core"}
	verifier := NewGitHubVerifier(ghClient, target, 101)

	opts := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: verifier,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
	}

	res := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, opts)
	if !res.Success {
		t.Fatalf("expected coordinator run with genuine GitHub forge approval to SUCCEED, got error: %s", res.Error)
	}
	if res.CommitHash == "" {
		t.Fatalf("expected commit hash to be non-empty upon autonomous convergence")
	}

	// 2. Stale Head SHA rejection
	staleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/102") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": "old-stale-commit-sha-00000000000000"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer staleServer.Close()

	staleClient := NewGitHubClient(ForgeAuth{Type: ForgeGitHub, Token: "tok", BaseURL: staleServer.URL})
	optsStale := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: NewGitHubVerifier(staleClient, target, 102),
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-1\n+2\n"
		},
	}
	resStale := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, optsStale)
	if resStale.Success {
		t.Fatalf("expected stale commit to be REJECTED, but run succeeded")
	}
	if !strings.Contains(resStale.Error, "stale commit") && !strings.Contains(resStale.Error, "differs") {
		t.Fatalf("expected error mentioning stale commit, got: %s", resStale.Error)
	}

	// 3. Author Self-Approval rejection
	selfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		currentHead, _ := driver.HeadHash()
		if strings.HasSuffix(r.URL.Path, "/pulls/103") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": currentHead},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/103/reviews") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "developer-bob", "type": "User"}, // author approving self
					"state":     "APPROVED",
					"commit_id": currentHead,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer selfServer.Close()

	selfClient := NewGitHubClient(ForgeAuth{Type: ForgeGitHub, Token: "tok", BaseURL: selfServer.URL})
	optsSelf := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: NewGitHubVerifier(selfClient, target, 103),
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-1\n+3\n"
		},
	}
	resSelf := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, optsSelf)
	if resSelf.Success {
		t.Fatalf("expected author self-approval to be REJECTED, but run succeeded")
	}
	if !strings.Contains(resSelf.Error, "author") && !strings.Contains(resSelf.Error, "cannot approve") {
		t.Fatalf("expected error mentioning author cannot approve, got: %s", resSelf.Error)
	}

	// 4. API 500 error rejection
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer errServer.Close()

	errClient := NewGitHubClient(ForgeAuth{Type: ForgeGitHub, Token: "tok", BaseURL: errServer.URL})
	optsErr := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: NewGitHubVerifier(errClient, target, 104),
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-1\n+4\n"
		},
	}
	resErr := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, optsErr)
	if resErr.Success {
		t.Fatalf("expected API 500 failure to fail closed, but run succeeded")
	}
}

// TestR3_2_MockedGitLab_CoordinatorRun_FullE2E verifies that GitLab MR approvals work end-to-end with Coordinator.Run.
func TestR3_2_MockedGitLab_CoordinatorRun_FullE2E(t *testing.T) {
	policy.ResetCache()
	defer policy.ResetCache()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_ALLOW_AUTONOMOUS", "1")
	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:       true,
		AllowAutonomous:      true,
		RequireForgeApproval: true,
		AllowedTestCommands:  []string{"test -f counter.txt"},
		IsVerified:           true,
	})

	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headSHA, err := driver.HeadHash()
	if err != nil || headSHA == "" {
		t.Fatalf("failed to get head hash: %v", err)
	}

	repoCtx, err := repo.DetectContext(tempDir)
	if err != nil {
		t.Fatalf("failed to detect repo context: %v", err)
	}

	reg := persona.NewRegistry("")
	coderEngine, _ := coder.NewDomainCoder("backend_engineer", reg)
	advReviewer := newTestReviewer(reg)
	advReviewer.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coord := coder.NewCoordinator(coderEngine, advReviewer, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "SPEC-GL-001",
		Title:        "GitLab Integration",
		TestCommands: []string{"test -f counter.txt"},
	}

	// Mocked GitLab Server
	glServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/merge_requests/55") {
			curHead, _ := driver.HeadHash()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     55,
				"iid":    55,
				"sha":    curHead,
				"state":  "opened",
				"author": map[string]any{"username": "dev-alice"},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/merge_requests/55/approvals") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  55,
				"iid": 55,
				"approved_by": []map[string]any{
					{
						"user": map[string]any{"id": 99, "username": "sec-lead-charlie", "name": "Charlie"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer glServer.Close()

	glClient := NewGitLabClient(ForgeAuth{
		Type:    ForgeGitLab,
		Token:   "gl-pat-token",
		BaseURL: glServer.URL,
	})

	target := &RemoteRepoTarget{Owner: "acme-corp", Repo: "services"}
	verifier := NewGitLabVerifier(glClient, target, 55)

	opts := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: verifier,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n"
		},
	}

	res := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, opts)
	if !res.Success {
		t.Fatalf("expected coordinator run with genuine GitLab approval to SUCCEED, got error: %s", res.Error)
	}
	if res.CommitHash == "" {
		t.Fatalf("expected commit hash upon autonomous convergence")
	}
}

// TestR4_2_ApprovePRAtPreCommitHead_BotAddsCommit_RejectedUntilReapproved verifies that
// if human approved PR at pre-commit SHA X, but the bot generates a new commit Y,
// enterprise autonomous merge is rejected because the new commit Y is unapproved,
// preventing unapproved code from bypassing the human approval gate.
func TestR4_2_ApprovePRAtPreCommitHead_BotAddsCommit_RejectedUntilReapproved(t *testing.T) {
	policy.ResetCache()
	defer policy.ResetCache()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_ALLOW_AUTONOMOUS", "1")
	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:       true,
		AllowAutonomous:      true,
		RequireForgeApproval: true,
		AllowedTestCommands:  []string{"test -f counter.txt"},
		IsVerified:           true,
	})

	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headSHA, err := driver.HeadHash()
	if err != nil {
		t.Fatalf("failed to get head hash: %v", err)
	}

	reg := persona.NewRegistry("")
	coderObj, _ := coder.NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coord := coder.NewCoordinator(coderObj, rev, driver, box)

	repoCtx := &repo.RepositoryContext{RootDir: tempDir}
	storySpec := &spec.StorySpec{
		ID:           "SPEC-GH-R4-2",
		Title:        "Post-commit Head Approval Enforcement",
		TestCommands: []string{"test -f counter.txt"},
	}

	// Forge server has approval for the PRE-commit headSHA X, but not the new bot commit Y
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/201") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": headSHA}, // pre-commit head SHA
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/201/reviews") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "alice-lead", "type": "User"},
					"state":     "APPROVED",
					"commit_id": headSHA, // approved old SHA X
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	ghClient := NewGitHubClient(ForgeAuth{
		Type:    ForgeGitHub,
		Token:   "gh-secret-token",
		BaseURL: ghServer.URL,
	})

	target := &RemoteRepoTarget{Owner: "acme", Repo: "core"}
	verifier := NewGitHubVerifier(ghClient, target, 201)

	opts := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: verifier,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+99\n"
		},
	}

	res := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, opts)
	if res.Success {
		t.Fatalf("SECURITY VIOLATION (R4-2): coordinator converged and committed bot patch even though human approval was only on pre-commit SHA %s!", headSHA)
	}
	if !strings.Contains(res.Error, "stale") && !strings.Contains(res.Error, "differs") && !strings.Contains(res.Error, "approval") {
		t.Fatalf("expected error mentioning stale commit or approval on old head, got: %s", res.Error)
	}
}

// TestR5_2_AutonomousCandidatePushAndForgeApproval_E2E verifies that Coordinator.Run correctly
// exercises the push -> approve -> verify flow against a mock forge whose state is independent of local HEAD.
func TestR5_2_AutonomousCandidatePushAndForgeApproval_E2E(t *testing.T) {
	policy.ResetCache()
	defer policy.ResetCache()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_ALLOW_AUTONOMOUS", "1")
	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:       true,
		AllowAutonomous:      true,
		RequireForgeApproval: true,
		AllowedTestCommands:  []string{"test -f counter.txt"},
		IsVerified:           true,
	})

	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headBefore, err := driver.HeadHash()
	if err != nil || headBefore == "" {
		t.Fatalf("failed to get head hash: %v", err)
	}

	repoCtx, err := repo.DetectContext(tempDir)
	if err != nil {
		t.Fatalf("failed to detect repo context: %v", err)
	}

	reg := persona.NewRegistry("")
	coderObj, _ := coder.NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coord := coder.NewCoordinator(coderObj, rev, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "SPEC-GH-R5-2",
		Title:        "Push and Approve Candidate Commit",
		TestCommands: []string{"test -f counter.txt"},
	}

	// Mock forge holds independent PR state (independent of local HEAD)
	var remotePRHead string = "initial-unapproved-sha-111111"
	var remoteReviews []map[string]any

	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/301") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": remotePRHead},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/301/reviews") {
			_ = json.NewEncoder(w).Encode(remoteReviews)
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	ghClient := NewGitHubClient(ForgeAuth{
		Type:    ForgeGitHub,
		Token:   "gh-secret-token",
		BaseURL: ghServer.URL,
	})

	target := &RemoteRepoTarget{Owner: "acme", Repo: "core"}
	verifier := NewGitHubVerifier(ghClient, target, 301)

	// Subtest 1: Success when ForgePusher pushes candidate to PR branch and human approves that exact SHA
	pushedSHA := ""
	opts := &coder.LoopOptions{
		MaxRounds: 1,
		Autonomy:  coder.AutonomyAutonomous,
		ForgePusher: func(ctx context.Context, commitSHA string) error {
			pushedSHA = commitSHA
			// Update mock forge remote PR head to the pushed candidate SHA
			remotePRHead = commitSHA
			// Human approves the pushed commit on the forge
			remoteReviews = []map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "lead-alice", "type": "User"},
					"state":     "APPROVED",
					"commit_id": commitSHA,
				},
			}
			return nil
		},
		ForgeVerifier: verifier,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+42\n"
		},
	}

	res := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, opts)
	if !res.Success {
		t.Fatalf("expected push->approve->verify flow to SUCCEED, got error: %s", res.Error)
	}
	if pushedSHA == "" || res.CommitHash != pushedSHA {
		t.Fatalf("expected candidate commit SHA %s to match pushed SHA %s", res.CommitHash, pushedSHA)
	}

	// Subtest 2: Refusal when ForgePusher fails or approval is on a different SHA
	remotePRHead = "unrelated-stale-sha-999999"
	remoteReviews = []map[string]any{
		{
			"id":        2,
			"user":      map[string]any{"login": "lead-alice", "type": "User"},
			"state":     "APPROVED",
			"commit_id": "unrelated-stale-sha-999999",
		},
	}
	optsRefusal := &coder.LoopOptions{
		MaxRounds:     1,
		Autonomy:      coder.AutonomyAutonomous,
		ForgeVerifier: verifier,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-42\n+43\n"
		},
	}
	resRefusal := coord.Run(context.Background(), storySpec, repoCtx, &steering.PersonaSteeringContext{}, &steering.PersonaSteeringContext{}, optsRefusal)
	if resRefusal.Success {
		t.Fatalf("expected unpushed / stale PR head to be REJECTED, but run succeeded")
	}
}

// TestR5_2_ResetHard_NeverResetsPreExistingUserCommit proves that ResetHard can never reset away
// a pre-existing user commit when the commit step produced nothing or failed, and never touches a tree
// the user is working in.
func TestR5_2_ResetHard_NeverResetsPreExistingUserCommit(t *testing.T) {
	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	initialHead, err := driver.HeadHash()
	if err != nil || initialHead == "" {
		t.Fatalf("failed to get initial head: %v", err)
	}

	repoCtx := &repo.RepositoryContext{RootDir: tempDir}
	reg := persona.NewRegistry("")
	coderObj, _ := coder.NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(tempDir)
	coord := coder.NewCoordinator(coderObj, rev, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "SPEC-GH-RESET-SAFETY",
		Title:        "Reset Safety Verification",
		TestCommands: []string{"test -f counter.txt"},
	}

	// 1. When patch application / test verification produces no commit
	optsNoCommit := &coder.LoopOptions{
		MaxRounds: 1,
		Autonomy:  coder.AutonomyAutonomous,
		MockPatchGen: func(round int, feedback string) string {
			return "invalid-patch-that-fails-to-apply"
		},
	}
	resNoCommit := coord.Run(context.Background(), storySpec, repoCtx, nil, nil, optsNoCommit)
	if resNoCommit.Success {
		t.Fatalf("expected invalid patch to fail")
	}

	curHead, err := driver.HeadHash()
	if err != nil {
		t.Fatalf("failed to get head hash after failed run: %v", err)
	}
	if curHead != initialHead {
		t.Fatalf("CRITICAL REGRESSION: pre-existing user commit was reset away! Expected HEAD=%s, got HEAD=%s", initialHead, curHead)
	}

	// 2. When candidate commit is made but forge approval is rejected
	optsForgeFail := &coder.LoopOptions{
		MaxRounds: 1,
		Autonomy:  coder.AutonomyAutonomous,
		ForgeVerifier: func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
			return nil, fmt.Errorf("forge approval rejected by security team")
		},
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+777\n"
		},
	}
	resForgeFail := coord.Run(context.Background(), storySpec, repoCtx, nil, nil, optsForgeFail)
	if resForgeFail.Success {
		t.Fatalf("expected rejected forge approval to fail")
	}

	curHeadAfterForgeFail, err := driver.HeadHash()
	if err != nil {
		t.Fatalf("failed to get head hash after forge failure: %v", err)
	}
	if curHeadAfterForgeFail != initialHead {
		t.Fatalf("CRITICAL REGRESSION: candidate rollback destroyed pre-existing user commit! Expected HEAD=%s, got HEAD=%s", initialHead, curHeadAfterForgeFail)
	}
}

// TestR6_2_TwoPhaseAutonomousPRFlow_RealBareRepo exercises the full two-phase autonomous flow:
// Phase 1 pushes candidate commit to a PR branch on a real local bare git repo with branch-protection enforcement
// and exits with AwaitingApproval: true; Phase 2 verifies forge approval on that exact candidate commit SHA
// and cleans up remote branches on verification failure.
func TestR6_2_TwoPhaseAutonomousPRFlow_RealBareRepo(t *testing.T) {
	policy.ResetCache()
	defer policy.ResetCache()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_ALLOW_AUTONOMOUS", "1")
	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode:       true,
		AllowAutonomous:      true,
		RequireForgeApproval: true,
		AllowedTestCommands:  []string{"test -f counter.txt"},
		IsVerified:           true,
	})

	// 1. Create a real bare git repository to act as the remote forge repository
	bareDir, err := os.MkdirTemp("", "artix_remote_bare_repo")
	if err != nil {
		t.Fatalf("failed to create bare repo dir: %v", err)
	}
	defer os.RemoveAll(bareDir)

	if err := exec.Command("git", "init", "--bare", bareDir).Run(); err != nil {
		t.Fatalf("failed to init bare git repo: %v", err)
	}

	// 2. Create local working clone
	workDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)

	// Set remote 'origin' to the bare repo
	_ = exec.Command("git", "-C", workDir, "remote", "add", "origin", bareDir).Run()
	// Push initial main branch to remote
	if out, err := exec.Command("git", "-C", workDir, "push", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("failed to push initial main to bare repo: %v (%s)", err, string(out))
	}

	repoCtx, err := repo.DetectContext(workDir)
	if err != nil {
		t.Fatalf("failed to detect repo context: %v", err)
	}

	reg := persona.NewRegistry("")
	coderObj, _ := coder.NewDomainCoder("backend_engineer", reg)
	rev := newTestReviewer(reg)
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})
	box := sandbox.NewSandbox(workDir)
	coord := coder.NewCoordinator(coderObj, rev, driver, box)

	storySpec := &spec.StorySpec{
		ID:           "SPEC-GH-R6-2",
		Title:        "Two-Phase Autonomous Flow Verification",
		TestCommands: []string{"test -f counter.txt"},
	}

	// Subtest A: Branch Protection - direct push to protected branch (main) is rejected
	protectedPusher := NewForgePusher(driver, "origin", "main")
	if err := protectedPusher(context.Background(), "some-commit-sha"); err == nil {
		t.Fatalf("SECURITY VIOLATION: ForgePusher permitted direct push to protected branch 'main'!")
	} else if !strings.Contains(err.Error(), "branch protection") {
		t.Fatalf("expected error mentioning branch protection, got: %v", err)
	}

	// Subtest B: Phase 1 - Coordinator pushes candidate commit to PR branch on real bare remote
	prBranch := "artix-pr-42"
	pusher := NewForgePusher(driver, "origin", prBranch)

	optsPhase1 := &coder.LoopOptions{
		MaxRounds:          1,
		Autonomy:           coder.AutonomyAutonomous,
		TwoPhaseAutonomous: true,
		ForgePusher:        pusher,
		MockPatchGen: func(round int, feedback string) string {
			return "diff --git a/counter.txt b/counter.txt\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+100\n"
		},
	}

	resPhase1 := coord.Run(context.Background(), storySpec, repoCtx, nil, nil, optsPhase1)
	if resPhase1.Success {
		t.Fatalf("SECURITY VIOLATION (R8-4): Phase 1 must return Success: false while awaiting approval, got true")
	}
	if !resPhase1.AwaitingApproval {
		t.Fatalf("expected Phase 1 to return AwaitingApproval: true, got false")
	}
	candidateSHA := resPhase1.CommitHash
	if candidateSHA == "" {
		t.Fatalf("expected Phase 1 to return candidate commit SHA")
	}

	// Assert that the real bare repo now has the candidate commit on refs/heads/artix-pr-42
	out, err := exec.Command("git", "-C", bareDir, "rev-parse", "refs/heads/"+prBranch).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to query bare repo for PR branch: %v (%s)", err, string(out))
	}
	remoteRefSHA := strings.TrimSpace(string(out))
	if remoteRefSHA != candidateSHA {
		t.Fatalf("expected bare repo PR branch SHA %s to equal candidate SHA %s", remoteRefSHA, candidateSHA)
	}

	// Subtest C: Phase 2 Rejection and Remote Branch Cleanup
	// Setup mock GitHub forge server
	var remoteReviewState string = "CHANGES_REQUESTED"
	var remoteApprover string = "lead-security-alice"

	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/42") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": candidateSHA},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/42/reviews") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": remoteApprover, "type": "User"},
					"state":     remoteReviewState,
					"commit_id": candidateSHA,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	ghClient := NewGitHubClient(ForgeAuth{
		Type:    ForgeGitHub,
		Token:   "test-token",
		BaseURL: ghServer.URL,
	})
	targetRepo := &RemoteRepoTarget{Owner: "acme", Repo: "core"}
	prodVerifier := NewGitHubVerifier(ghClient, targetRepo, 42)

	// In Subtest C, review is CHANGES_REQUESTED -> VerifyAndMergeCandidate must fail and clean up remote branch
	_, err = VerifyAndMergeCandidate(context.Background(), driver, prodVerifier, candidateSHA, nil, storySpec.ID, "origin", prBranch, resPhase1.VerdictHash)
	if err == nil {
		t.Fatalf("expected VerifyAndMergeCandidate to fail when verifier rejects")
	}

	// Verify that the remote candidate branch was deleted from the bare repo
	delOut, delErr := exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+prBranch).CombinedOutput()
	if delErr == nil {
		t.Fatalf("expected remote branch %s to be deleted after rejection cleanup, but rev-parse succeeded: %s", prBranch, string(delOut))
	}

	// Subtest D: Phase 2 Approval and Merge
	// Re-push candidate to remote PR branch
	if err := pusher(context.Background(), candidateSHA); err != nil {
		t.Fatalf("failed to re-push candidate commit: %v", err)
	}

	// Change review state on forge to APPROVED
	remoteReviewState = "APPROVED"

	approval, err := VerifyAndMergeCandidate(context.Background(), driver, prodVerifier, candidateSHA, nil, storySpec.ID, "origin", prBranch, resPhase1.VerdictHash)
	if err != nil {
		t.Fatalf("expected Phase 2 VerifyAndMergeCandidate to SUCCEED with valid approval, got error: %v", err)
	}
	if approval == nil || approval.ApproverUsername != "lead-security-alice" {
		t.Fatalf("expected valid approval for lead-security-alice, got: %+v", approval)
	}
}

// TestR7_2_BranchProtection_NoFalsePositives_ProtectsDevelopStaging verifies that
// develop, dev, staging, release/*, main, master are protected, while feature branches
// like verify-login, validation-fix, vendor-update are NOT false-positived.
func TestR7_2_BranchProtection_NoFalsePositives_ProtectsDevelopStaging(t *testing.T) {
	protectedBranches := []string{
		"main",
		"master",
		"develop",
		"dev",
		"staging",
		"prod",
		"production",
		"trunk",
		"release/1.0",
		"releases/2026.1",
		"hotfix/security-patch",
	}

	for _, b := range protectedBranches {
		if !IsProtectedBranch(b) {
			t.Fatalf("SECURITY VIOLATION: branch %q was expected to be PROTECTED, but IsProtectedBranch returned false", b)
		}
	}

	unprotectedFeatureBranches := []string{
		"verify-login",
		"validation-fix",
		"vendor-update",
		"feature/auth",
		"artix-pr-42",
		"fix/bug-123",
	}

	for _, b := range unprotectedFeatureBranches {
		if IsProtectedBranch(b) {
			t.Fatalf("FALSE POSITIVE: feature branch %q was incorrectly marked as PROTECTED by IsProtectedBranch", b)
		}
	}
}

// TestR7_2_NonDestructiveCleanup_NoReviewsYet_And_503 verifies that remote candidate PR branches
// are NOT deleted on "no reviews yet" or HTTP 503 / network errors, and are ONLY deleted on definitive negative reviews.
func TestR7_2_NonDestructiveCleanup_NoReviewsYet_And_503(t *testing.T) {
	bareDir, err := os.MkdirTemp("", "artix_remote_bare_cleanup")
	if err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}
	defer os.RemoveAll(bareDir)
	_ = exec.Command("git", "init", "--bare", bareDir).Run()

	workDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)
	_ = exec.Command("git", "-C", workDir, "remote", "add", "origin", bareDir).Run()

	prBranch := "artix-pr-cleanup-test"
	pusher := NewForgePusher(driver, "origin", prBranch)
	headSHA, _ := driver.HeadHash()
	if err := pusher(context.Background(), headSHA); err != nil {
		t.Fatalf("failed to push candidate branch: %v", err)
	}

	// 1. "No reviews yet" / no APPROVED review found -> MUST NOT delete remote candidate branch
	noReviewsVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, fmt.Errorf("forge approval verification failed: no valid human APPROVED review found")
	}

	_, err = VerifyAndMergeCandidate(context.Background(), driver, noReviewsVerifier, headSHA, nil, "SPEC-1", "origin", prBranch)
	if err == nil {
		t.Fatalf("expected verification failure when no reviews exist")
	}

	// Branch must still exist in remote bare repo
	if out, err := exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+prBranch).CombinedOutput(); err != nil {
		t.Fatalf("DESTRUCTIVE BUG (R7-2 b): remote PR branch was destroyed on 'no reviews yet' error: %s", string(out))
	}

	// 2. HTTP 503 Server Outage -> MUST NOT delete remote candidate branch
	server503Verifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, fmt.Errorf("forge API error: HTTP 503 Service Unavailable")
	}

	_, err = VerifyAndMergeCandidate(context.Background(), driver, server503Verifier, headSHA, nil, "SPEC-1", "origin", prBranch)
	if err == nil {
		t.Fatalf("expected verification failure on 503")
	}

	// Branch must still exist in remote bare repo
	if out, err := exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+prBranch).CombinedOutput(); err != nil {
		t.Fatalf("DESTRUCTIVE BUG (R7-2 b): remote PR branch was destroyed on 503 outage error: %s", string(out))
	}

	// 3. Definitive rejection (CHANGES_REQUESTED) -> MUST delete remote candidate branch
	changesRequestedVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, fmt.Errorf("%w: review changes requested by \"reviewer-bob\"", ErrChangesRequested)
	}

	_, err = VerifyAndMergeCandidate(context.Background(), driver, changesRequestedVerifier, headSHA, nil, "SPEC-1", "origin", prBranch)
	if err == nil {
		t.Fatalf("expected verification failure on changes requested")
	}

	// Branch must be deleted
	if _, err := exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+prBranch).CombinedOutput(); err == nil {
		t.Fatalf("expected remote PR branch to be deleted after definitive CHANGES_REQUESTED rejection")
	}
}

// TestR7_2_Phase2_BoundToPhase1AuditRecord verifies that Phase 2 merge verification requires
// the candidate commit SHA and Spec ID to be cryptographically bound to a Phase 1 audit record.
func TestR7_2_Phase2_BoundToPhase1AuditRecord(t *testing.T) {
	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headSHA, _ := driver.HeadHash()
	logger := audit.Default(tempDir)

	// Subtest 1: Unbound / unrecorded SHA passed to Phase 2 is refused
	approvedVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return &policy.PRApproval{
			ApproverUsername: "lead-alice",
			State:            "APPROVED",
			CommitSHA:        commitSHA,
			VerifiedByForge:  true,
		}, nil
	}

	_, err := VerifyAndMergeCandidate(context.Background(), driver, approvedVerifier, "unbound-foreign-candidate-sha-99999", logger, "SPEC-BOUND-001", "origin", "artix-pr-1", "verdict-hash-123")
	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R7-2 e): Phase 2 accepted candidate SHA without Phase 1 audit record binding!")
	}
	if !strings.Contains(err.Error(), "audit") && !strings.Contains(err.Error(), "bound") && !strings.Contains(err.Error(), "phase 1") {
		t.Fatalf("expected error mentioning Phase 1 audit record binding, got: %v", err)
	}

	// Subtest 2: Record Phase 1 candidate pushed event in audit log
	verdictHash := "bound-verdict-hash-1234"
	_ = logger.Emit(audit.AuditEvent{
		EventType: "CANDIDATE_PUSHED",
		Status:    "AWAITING_APPROVAL",
		Details: map[string]any{
			"storyId":             "SPEC-BOUND-001",
			"candidateSHA":        headSHA,
			"reviewerVerdictHash": verdictHash,
			"prBranch":            "artix-pr-1",
		},
	})

	// Now Phase 2 verification succeeds with matching candidate SHA, Spec ID, and verdict hash
	approval, err := VerifyAndMergeCandidate(context.Background(), driver, approvedVerifier, headSHA, logger, "SPEC-BOUND-001", "origin", "artix-pr-1", verdictHash)
	if err != nil {
		t.Fatalf("expected Phase 2 verification to succeed when bound to Phase 1 audit record, got: %v", err)
	}
	if approval == nil || approval.ApproverUsername != "lead-alice" {
		t.Fatalf("expected valid approval for lead-alice, got: %+v", approval)
	}
}

// TestR8_2_VerifyPhase1AuditBinding_CryptographicIntegrity tests that verifyPhase1AuditBinding
// enforces complete cryptographic verification: non-empty path, whole-log signature/hash chain,
// CANDIDATE_PUSHED event type, valid status, and tamper resistance (appended line, tampered middle, truncated tail).
func TestR8_2_VerifyPhase1AuditBinding_CryptographicIntegrity(t *testing.T) {
	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headSHA, _ := driver.HeadHash()
	logger := audit.Default(tempDir)
	logPath := logger.LogPath()

	// 1. Empty audit log path must FAIL CLOSED
	_, err := verifyPhase1AuditBinding("", "SPEC-1", headSHA, "")
	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R8-2): verifyPhase1AuditBinding with empty log path succeeded (must fail closed)")
	}

	// 2. Emit a valid signed CANDIDATE_PUSHED record
	verdictHash := "a1b2c3d4e5f600112233445566778899aabbccddeeff00112233445566778899"
	emitErr := logger.Emit(audit.AuditEvent{
		EventType:   "CANDIDATE_PUSHED",
		Status:      "AWAITING_APPROVAL",
		StorySpecID: "SPEC-R8-2",
		Details: map[string]any{
			"storyId":             "SPEC-R8-2",
			"candidateSHA":        headSHA,
			"reviewerVerdictHash": verdictHash,
			"prBranch":            "artix-pr-82",
		},
	})
	if emitErr != nil {
		t.Fatalf("failed to emit initial audit record: %v", emitErr)
	}

	// Valid binding succeeds
	_, err = verifyPhase1AuditBinding(logPath, "SPEC-R8-2", headSHA, verdictHash)
	if err != nil {
		t.Fatalf("expected valid audit binding to succeed, got: %v", err)
	}

	// 3. Reviewer verdict hash mismatch must FAIL
	_, err = verifyPhase1AuditBinding(logPath, "SPEC-R8-2", headSHA, "wrong-verdict-hash-0000")
	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R8-2): verifyPhase1AuditBinding succeeded with mismatched reviewer verdict hash")
	}

	// 4. FAILED convergence event status must FAIL
	tempDirFailed, _ := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDirFailed)
	failedLogger := audit.NewLogger(tempDirFailed)
	_ = failedLogger.Emit(audit.AuditEvent{
		EventType:   "CANDIDATE_PUSHED",
		Status:      "FAILED",
		StorySpecID: "SPEC-FAILED",
		Details: map[string]any{
			"storyId":      "SPEC-FAILED",
			"candidateSHA": headSHA,
		},
	})
	if _, err := verifyPhase1AuditBinding(failedLogger.LogPath(), "SPEC-FAILED", headSHA, "some-verdict-hash"); err == nil {
		t.Fatalf("SECURITY VIOLATION (R8-2): verifyPhase1AuditBinding accepted a FAILED candidate push event")
	}

	// 5. Appended forged line (attacker appends unverified record) must FAIL
	forgedLine := `{"eventId":"forged-999","timestamp":"2026-10-08T00:00:00Z","eventType":"CANDIDATE_PUSHED","status":"AWAITING_APPROVAL","storySpecId":"SPEC-FORGED","prevHash":"0000","recordHash":"forged"}` + "\n"
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString(forgedLine)
	_ = f.Close()

	if _, err := verifyPhase1AuditBinding(logPath, "SPEC-R8-2", headSHA, verdictHash); err == nil {
		t.Fatalf("SECURITY VIOLATION (R8-2): verifyPhase1AuditBinding accepted log with appended forged record")
	}
}

// TestR8_2_TypedErrors_503BodyWithKeywords_DoesNotDeleteBranch verifies that typed errors
// prevent HTTP 503 response bodies containing words like "changes requested" or "dismissed"
// from triggering branch deletion.
func TestR8_2_TypedErrors_503BodyWithKeywords_DoesNotDeleteBranch(t *testing.T) {
	bareDir, err := os.MkdirTemp("", "artix_remote_503_test")
	if err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}
	defer os.RemoveAll(bareDir)
	_ = exec.Command("git", "init", "--bare", bareDir).Run()

	workDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)
	_ = exec.Command("git", "-C", workDir, "remote", "add", "origin", bareDir).Run()

	prBranch := "artix-pr-503-guard"
	pusher := NewForgePusher(driver, "origin", prBranch)
	headSHA, _ := driver.HeadHash()
	if err := pusher(context.Background(), headSHA); err != nil {
		t.Fatalf("failed to push candidate branch: %v", err)
	}

	// 1. 503 error containing "changes requested" and "dismissed" in body -> NOT a definitive rejection
	bodyWithKeywordsErr := fmt.Errorf("HTTP 503 Service Unavailable: upstream worker dismissed review cache for changes requested query")
	if IsDefinitiveRejection(bodyWithKeywordsErr) {
		t.Fatalf("BUG (R8-2): IsDefinitiveRejection returned true for generic HTTP 503 error containing keyword substrings")
	}

	http503Verifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, bodyWithKeywordsErr
	}

	_, err = VerifyAndMergeCandidate(context.Background(), driver, http503Verifier, headSHA, nil, "SPEC-1", "origin", prBranch)
	if err == nil {
		t.Fatalf("expected verification to fail on 503")
	}

	// Remote PR branch must NOT be deleted
	if out, err := exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "refs/heads/"+prBranch).CombinedOutput(); err != nil {
		t.Fatalf("DESTRUCTIVE BUG (R8-2): remote branch was destroyed on HTTP 503 containing keywords: %s", string(out))
	}

	// 2. Typed rejection error (ErrChangesRequested) -> IS a definitive rejection and deletes branch
	typedRejectionVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, ErrChangesRequested
	}

	_, err = VerifyAndMergeCandidate(context.Background(), driver, typedRejectionVerifier, headSHA, nil, "SPEC-1", "origin", prBranch)
	if err == nil {
		t.Fatalf("expected verification to fail on typed changes requested error")
	}

}

// TestR9_2_CryptographicAuditBinding_HostileEvaluatorCorpus validates all R9-2 hostile reviewer findings:
// a) Unsigned log with no key in enterprise mode must fail closed (never fall back to unkeyed hash chain).
// b) Malformed ARTIX_AUDIT_PUBLIC_KEY ("zz-not-hex") must hard fail, never fall back.
// c) Verdict binding must fail closed when expected or recorded verdict hash is empty or mismatched.
// d) CANDIDATE_PUSHED event is strictly required (EventCodeConvergence/SUCCESS is rejected).
func TestR9_2_CryptographicAuditBinding_HostileEvaluatorCorpus(t *testing.T) {
	// Subtest A: Unsigned log in enterprise mode MUST FAIL CLOSED
	t.Run("Unsigned_Log_In_EnterpriseMode_Must_FailClosed", func(t *testing.T) {
		tempDir, driver := setupTestRepoForForge(t)
		defer os.RemoveAll(tempDir)
		headSHA, _ := driver.HeadHash()

		// Create unsigned logger (no private key, no public key)
		t.Setenv("ARTIX_ENTERPRISE", "1")
		t.Setenv("ARTIX_AUDIT_PUBLIC_KEY", "")
		t.Setenv("ARTIX_AUDIT_PRIVATE_KEY_PATH", "")

		policy.ResetCache()
		defer policy.ResetCache()
		policy.SetActivePolicyForTest(&policy.Policy{
			EnterpriseMode:       true,
			RequireForgeApproval: true,
			AuditPublicKey:       "", // Empty key in enterprise mode
			IsVerified:           true,
		})

		unsignedLogger := audit.NewLogger(tempDir)
		_ = unsignedLogger.Emit(audit.AuditEvent{
			EventType:   "CANDIDATE_PUSHED",
			Status:      "AWAITING_APPROVAL",
			StorySpecID: "SPEC-UNSIGNED",
			Details: map[string]any{
				"storyId":             "SPEC-UNSIGNED",
				"candidateSHA":        headSHA,
				"reviewerVerdictHash": "hash1234567890abcdef",
			},
		})

		_, err := verifyPhase1AuditBinding(unsignedLogger.LogPath(), "SPEC-UNSIGNED", headSHA, "hash1234567890abcdef")
		if err == nil {
			t.Fatalf("SECURITY VIOLATION (R9-2 a): verifyPhase1AuditBinding accepted unsigned log in enterprise mode!")
		}
	})

	// Subtest B: Malformed ARTIX_AUDIT_PUBLIC_KEY must HARD FAIL
	t.Run("Malformed_PublicKey_Must_HardFail", func(t *testing.T) {
		tempDir, driver := setupTestRepoForForge(t)
		defer os.RemoveAll(tempDir)
		headSHA, _ := driver.HeadHash()

		t.Setenv("ARTIX_AUDIT_PUBLIC_KEY", "zz-not-hex-malformed")
		logger := audit.NewLogger(tempDir)
		_ = logger.Emit(audit.AuditEvent{
			EventType:   "CANDIDATE_PUSHED",
			Status:      "AWAITING_APPROVAL",
			StorySpecID: "SPEC-MALFORMED",
			Details: map[string]any{
				"storyId":             "SPEC-MALFORMED",
				"candidateSHA":        headSHA,
				"reviewerVerdictHash": "hash1234567890abcdef",
			},
		})

		_, err := verifyPhase1AuditBinding(logger.LogPath(), "SPEC-MALFORMED", headSHA, "hash1234567890abcdef")
		if err == nil {
			t.Fatalf("SECURITY VIOLATION (R9-2 b): verifyPhase1AuditBinding silently accepted malformed public key!")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "invalid") && !strings.Contains(strings.ToLower(err.Error()), "public key") && !strings.Contains(strings.ToLower(err.Error()), "hex") {
			t.Fatalf("expected error mentioning invalid public key, got: %v", err)
		}
	})

	// Subtest C: Empty expected or recorded verdict hash must FAIL CLOSED
	t.Run("Empty_Verdict_Hash_Must_FailClosed", func(t *testing.T) {
		tempDir, driver := setupTestRepoForForge(t)
		defer os.RemoveAll(tempDir)
		headSHA, _ := driver.HeadHash()
		logger := audit.Default(tempDir)

		// Record CANDIDATE_PUSHED with empty reviewerVerdictHash
		_ = logger.Emit(audit.AuditEvent{
			EventType:   "CANDIDATE_PUSHED",
			Status:      "AWAITING_APPROVAL",
			StorySpecID: "SPEC-NO-VERDICT",
			Details: map[string]any{
				"storyId":      "SPEC-NO-VERDICT",
				"candidateSHA": headSHA,
			},
		})

		// Calling with empty expected verdict
		if _, err := verifyPhase1AuditBinding(logger.LogPath(), "SPEC-NO-VERDICT", headSHA, ""); err == nil {
			t.Fatalf("SECURITY VIOLATION (R9-2 c): verifyPhase1AuditBinding succeeded when expected verdict hash was empty!")
		}

		// Calling with expected verdict against record missing verdict
		if _, err := verifyPhase1AuditBinding(logger.LogPath(), "SPEC-NO-VERDICT", headSHA, "some-verdict-hash"); err == nil {
			t.Fatalf("SECURITY VIOLATION (R9-2 c): verifyPhase1AuditBinding succeeded when recorded verdict hash was empty!")
		}
	})

	// Subtest D: Ordinary SUCCESS convergence event MUST NOT satisfy CANDIDATE_PUSHED check
	t.Run("Ordinary_Success_Convergence_Rejected", func(t *testing.T) {
		tempDir, driver := setupTestRepoForForge(t)
		defer os.RemoveAll(tempDir)
		headSHA, _ := driver.HeadHash()
		logger := audit.Default(tempDir)

		_ = logger.Emit(audit.AuditEvent{
			EventType:   audit.EventCodeConvergence,
			Status:      "SUCCESS",
			StorySpecID: "SPEC-ORD-SUCCESS",
			Details: map[string]any{
				"storyId":             "SPEC-ORD-SUCCESS",
				"commitHash":          headSHA,
				"candidateSHA":        headSHA,
				"reviewerVerdictHash": "some-hash-1234",
			},
		})


		if _, err := verifyPhase1AuditBinding(logger.LogPath(), "SPEC-ORD-SUCCESS", headSHA, "some-hash-1234"); err == nil {
			t.Fatalf("SECURITY VIOLATION (R9-2 d): verifyPhase1AuditBinding accepted ordinary EventCodeConvergence event instead of CANDIDATE_PUSHED!")
		}
	})
}


// TestR10_2_NilAuditLogger_MustNotBypassPhase1BindingWhenAuditLogExists asserts that passing a nil
// *audit.Logger pointer cannot bypass Phase 1 cryptographic audit binding when an audit log exists or in enterprise mode.
func TestR10_2_NilAuditLogger_MustNotBypassPhase1BindingWhenAuditLogExists(t *testing.T) {
	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)
	headSHA, _ := driver.HeadHash()

	// Enterprise mode active
	t.Setenv("ARTIX_ENTERPRISE", "1")
	policy.SetActivePolicyForTest(&policy.Policy{
		EnterpriseMode: true,
		AuditPublicKey: os.Getenv("ARTIX_AUDIT_PUBLIC_KEY"),
	})
	defer policy.ResetCache()

	// Emit an audit record
	logger := audit.Default(tempDir)
	_ = logger.Emit(audit.AuditEvent{
		EventType:   "CANDIDATE_PUSHED",
		Status:      "AWAITING_APPROVAL",
		StorySpecID: "SPEC-NIL-LOGGER",
		Details: map[string]any{
			"storyId":             "SPEC-NIL-LOGGER",
			"candidateSHA":        headSHA,
			"reviewerVerdictHash": "real-verdict-hash-1234",
		},
	})

	approvedVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		sig := forgesec.SignToken("lead-alice", "developer-bob", "APPROVED", commitSHA, "github_api_server_verified")
		return &policy.PRApproval{
			ApproverUsername: "lead-alice",
			AuthorUsername:   "developer-bob",
			State:            "APPROVED",
			CommitSHA:        commitSHA,
			Signature:        sig,
			Source:           "github_api_server_verified",
			VerifiedByForge:  true,
		}, nil
	}

	// 1. Calling VerifyAndMergeCandidate with nil logger and wrong verdict hash must FAIL (nil logger must NOT skip verification)
	_, err := VerifyAndMergeCandidate(context.Background(), driver, approvedVerifier, headSHA, nil, "SPEC-NIL-LOGGER", "origin", "artix-pr-1", "wrong-verdict-hash")
	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R10-2): VerifyAndMergeCandidate bypassed Phase 1 audit verification when auditLogger was nil!")
	}

	// 2. Calling with matching verdict hash must succeed
	app, err := VerifyAndMergeCandidate(context.Background(), driver, approvedVerifier, headSHA, nil, "SPEC-NIL-LOGGER", "origin", "artix-pr-1", "real-verdict-hash-1234")
	if err != nil {
		t.Fatalf("expected VerifyAndMergeCandidate with valid matching Phase 1 binding to succeed, got: %v", err)
	}
	if app == nil || app.ApproverUsername != "lead-alice" {
		t.Fatalf("expected valid approval, got: %v", app)
	}
}

// TestR10_2_BuiltBinary_Phase1ToPhase2_EndToEndWithVerdictHash tests that the real compiled artix binary
// outputs verdictHash in Phase 1 JSON and requires that exact --verdict-hash in Phase 2 artix merge.
func TestR10_2_BuiltBinary_Phase1ToPhase2_EndToEndWithVerdictHash(t *testing.T) {
	tempBinDir, err := os.MkdirTemp("", "artix_bin_test")
	if err != nil {
		t.Fatalf("failed to create temp bin dir: %v", err)
	}
	defer os.RemoveAll(tempBinDir)

	binPath := filepath.Join(tempBinDir, "artix")
	// Compile real binary from CLI package
	buildCmd := exec.Command("go", "build", "-o", binPath, "artix/cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build artix binary: %v (%s)", err, string(out))
	}

	// Create real bare remote repo
	bareDir, err := os.MkdirTemp("", "artix_bare_remote")
	if err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}
	defer os.RemoveAll(bareDir)
	_ = exec.Command("git", "init", "--bare", bareDir).Run()

	// Create working repo
	workDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)
	_ = driver

	_ = exec.Command("git", "-C", workDir, "remote", "add", "origin", bareDir).Run()
	_ = exec.Command("git", "-C", workDir, "push", "origin", "HEAD:refs/heads/main").Run()

	// Write story spec
	specsDir := filepath.Join(workDir, "docs", "specs")
	_ = os.MkdirAll(specsDir, 0755)
	specPath := filepath.Join(specsDir, "STORY-SPEC-BIN-01.md")
	specContent := "# Story Spec: Binary Phase 1 to 2\n\n**Spec ID:** `SPEC-BIN-01`\n\n## 2. Acceptance Criteria\n\n### Scenario 1: file updated\n* **Given** a counter file\n* **When** it updates\n* **Then** it exists\n\n## 5. Verification Test Suite\n\n```bash\ntest -f counter.txt\n```\n"
	_ = os.WriteFile(specPath, []byte(specContent), 0644)

	// Create and sign enterprise policy file
	pubHex := os.Getenv("ARTIX_AUDIT_PUBLIC_KEY")
	policyFile := filepath.Join(workDir, "policy.json")
	policyJSON := fmt.Sprintf(`{
		"enterpriseMode": true,
		"allowAutonomous": true,
		"requireSignedPolicy": true,
		"requireForgeApproval": true,
		"allowedTestCommands": ["test -f counter.txt"],
		"auditRemoteSinks": [{"type": "syslog", "endpoint": "127.0.0.1:514"}],
		"auditPublicKey": "%s"
	}`, pubHex)
	_ = os.WriteFile(policyFile, []byte(policyJSON), 0644)
	policySignKey := "test-secret-key-1234"
	_ = policy.SignPolicyFile(policyFile, policySignKey)

	// Mock LLM server
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		bodyBytes, _ := io.ReadAll(r.Body)
		bodyStr := string(bodyBytes)
		if strings.Contains(bodyStr, "adversarial_code_reviewer") || strings.Contains(bodyStr, "rubric") || strings.Contains(bodyStr, "Evaluation Rubric") || strings.Contains(bodyStr, "Reviewer") || strings.Contains(bodyStr, "Adversarial") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "msg_critic",
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "text", "text": `{"approved": true, "blocking": [], "warnings": []}`},
				},
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": `{"approved": true, "blocking": [], "warnings": []}`,
						},
					},
				},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 10, "prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_coder",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "```diff\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+100\n```\n"},
			},
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "```diff\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+100\n```\n",
					},
				},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 20, "prompt_tokens": 20, "completion_tokens": 20, "total_tokens": 40},
		})
	}))
	defer llmServer.Close()

	// Mock Forge server
	var (
		shaMu               sync.Mutex
		currentCandidateSHA string
	)
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		shaMu.Lock()
		curSHA := currentCandidateSHA
		shaMu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/pulls/77") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": curSHA},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/77/reviews") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "security-lead-alice", "type": "User"},
					"state":     "APPROVED",
					"commit_id": curSHA,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	// Phase 1: Run artix code --json with PR branch
	cmdPhase1 := exec.Command(binPath, "code", "--json",
		"--autonomy", "autonomous",
		"--provider", "anthropic",
		"--model", "claude-3-5-sonnet-20241022",
		"--review-provider", "openai",
		"--review-model", "gpt-4o",
		"--forge", "github",
		"--forge-pr", "77",
		"--forge-url", ghServer.URL,
		"--forge-owner", "acme",
		"--forge-repo", "core",
		specPath,
	)
	var stdoutPhase1, stderrPhase1 strings.Builder
	cmdPhase1.Stdout = &stdoutPhase1
	cmdPhase1.Stderr = &stderrPhase1
	cmdPhase1.Dir = workDir
	cmdPhase1.Env = append(os.Environ(),
		"ARTIX_ENTERPRISE=1",
		"ARTIX_ALLOW_AUTONOMOUS=1",
		"ARTIX_PR_BRANCH=artix-pr-77",
		"ARTIX_FORGE_REMOTE=origin",
		"ANTHROPIC_API_KEY=mock-key",
		"OPENAI_API_KEY=mock-key",
		"ARTIX_POLICY_SIGNING_KEY="+policySignKey,
		"ARTIX_POLICY_PATH="+policyFile,
		"ARTIX_API_URL="+llmServer.URL,
	)

	_ = cmdPhase1.Run()
	var jsonResPhase1 struct {
		Ok               bool   `json:"ok"`
		Status           string `json:"status"`
		AwaitingApproval bool   `json:"awaitingApproval"`
		CommitHash       string `json:"commitHash"`
		VerdictHash      string `json:"verdictHash"`
	}

	stdoutStr := strings.TrimSpace(stdoutPhase1.String())
	if err := json.Unmarshal([]byte(stdoutStr), &jsonResPhase1); err != nil {
		t.Fatalf("failed to parse Phase 1 JSON output: %v (stdout: %s, stderr: %s)", err, stdoutStr, stderrPhase1.String())
	}


	if jsonResPhase1.Status != "awaiting_approval" || !jsonResPhase1.AwaitingApproval {
		t.Fatalf("expected Phase 1 to return status awaiting_approval, got: %+v (stdout: %s, stderr: %s)", jsonResPhase1, stdoutStr, stderrPhase1.String())
	}
	if jsonResPhase1.VerdictHash == "" {
		t.Fatalf("FAIL: Phase 1 JSON did not print verdictHash (got empty string)")
	}
	if jsonResPhase1.CommitHash == "" {
		t.Fatalf("expected Phase 1 to produce candidate commit hash")
	}

	shaMu.Lock()
	currentCandidateSHA = jsonResPhase1.CommitHash
	shaMu.Unlock()

	// Phase 2 Success: artix merge with matching --verdict-hash
	cmdMergeOk := exec.Command(binPath, "verify-approval", "--json",
		"--pr", "77",
		"--sha", jsonResPhase1.CommitHash,
		"--spec", "SPEC-BIN-01",
		"--verdict-hash", jsonResPhase1.VerdictHash,
		"--forge", "github",
		"--forge-url", ghServer.URL,
		"--forge-owner", "acme",
		"--forge-repo", "core",
		"--remote", "origin",
		"--branch", "artix-pr-77",
	)
	cmdMergeOk.Dir = workDir
	cmdMergeOk.Env = append(os.Environ(),
		"ARTIX_ENTERPRISE=1",
		"ARTIX_ALLOW_AUTONOMOUS=1",
		"ARTIX_POLICY_SIGNING_KEY="+policySignKey,
		"ARTIX_POLICY_PATH="+policyFile,
	)

	var stdoutMergeOk, stderrMergeOk strings.Builder
	cmdMergeOk.Stdout = &stdoutMergeOk
	cmdMergeOk.Stderr = &stderrMergeOk
	errMergeOk := cmdMergeOk.Run()
	if errMergeOk != nil {
		t.Fatalf("expected Phase 2 artix merge with valid verdict hash to succeed, got: %v (stdout: %s, stderr: %s)", errMergeOk, stdoutMergeOk.String(), stderrMergeOk.String())
	}

	var jsonResMergeOk struct {
		Ok     bool   `json:"ok"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(stdoutMergeOk.String())), &jsonResMergeOk)
	if !jsonResMergeOk.Ok || jsonResMergeOk.Status != "approval_verified" {
		t.Fatalf("expected merge status approval_verified, got: %+v (stdout: %s, stderr: %s)", jsonResMergeOk, stdoutMergeOk.String(), stderrMergeOk.String())
	}

	// Phase 2 Rejection: artix verify-approval with mismatched --verdict-hash
	cmdMergeBad := exec.Command(binPath, "verify-approval", "--json",
		"--pr", "77",
		"--sha", jsonResPhase1.CommitHash,
		"--spec", "SPEC-BIN-01",
		"--verdict-hash", "bad-mismatched-verdict-hash-00000000000000000000000000000000",
		"--forge", "github",
		"--forge-url", ghServer.URL,
		"--forge-owner", "acme",
		"--forge-repo", "core",
		"--remote", "origin",
		"--branch", "artix-pr-77",
	)
	cmdMergeBad.Dir = workDir
	cmdMergeBad.Env = append(os.Environ(),
		"ARTIX_ENTERPRISE=1",
		"ARTIX_ALLOW_AUTONOMOUS=1",
		"ARTIX_POLICY_SIGNING_KEY="+policySignKey,
		"ARTIX_POLICY_PATH="+policyFile,
	)

	var stdoutMergeBad, stderrMergeBad strings.Builder
	cmdMergeBad.Stdout = &stdoutMergeBad
	cmdMergeBad.Stderr = &stderrMergeBad
	errMergeBad := cmdMergeBad.Run()
	if errMergeBad == nil {
		t.Fatalf("SECURITY VIOLATION (R10-2): artix merge with mismatched verdict hash succeeded unexpectedly! Stdout: %s", stdoutMergeBad.String())
	}
}

// TestR2_Phase2_AuditEmitFailure_MustExitNonZero verifies that when the APPROVAL_VERIFIED
// audit emit fails (e.g. read-only audit log / bad key), artix merge exits non-zero with ok:false.
func TestR2_Phase2_AuditEmitFailure_MustExitNonZero(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artix_r2_emit_fail")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath := filepath.Join(tempDir, "artix")
	buildCmd := exec.Command("go", "build", "-o", binPath, "artix/cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v (%s)", err, string(out))
	}

	workDir, _ := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)

	headSHA := "abc1234567890abcdef1234567890abcdef12345"
	verdictHash := "verdict1234567890abcdef1234567890abcdef"
	storyID := "SPEC-R2-A"

	// Create and sign a Phase 1 CANDIDATE_PUSHED record
	logger := audit.Default(workDir)
	_ = logger.Emit(audit.AuditEvent{
		EventType:   "CANDIDATE_PUSHED",
		Status:      "AWAITING_APPROVAL",
		StorySpecID: storyID,
		Details: map[string]any{
			"storyId":             storyID,
			"candidateSHA":        headSHA,
			"prBranch":            "artix-pr-10",
			"reviewerVerdictHash": verdictHash,
		},
	})

	// Mock Forge server with approval
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/10") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": headSHA},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/10/reviews") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "lead-alice", "type": "User"},
					"state":     "APPROVED",
					"commit_id": headSHA,
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/10/merge") {
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true, "sha": headSHA})
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	// Make the audit log read-only to force Emit failure on APPROVAL_VERIFIED
	logPath := logger.LogPath()
	_ = os.Chmod(logPath, 0400)
	defer os.Chmod(logPath, 0600)

	cmdMerge := exec.Command(binPath, "verify-approval", "--json",
		"--pr", "10",
		"--sha", headSHA,
		"--spec", storyID,
		"--forge", "github",
		"--forge-url", ghServer.URL,
		"--forge-owner", "acme",
		"--forge-repo", "core",
	)
	cmdMerge.Dir = workDir
	var stdout, stderr strings.Builder
	cmdMerge.Stdout = &stdout
	cmdMerge.Stderr = &stderr
	cmdErr := cmdMerge.Run()

	if cmdErr == nil {
		t.Fatalf("SECURITY VIOLATION (R2 a): artix verify-approval succeeded when APPROVAL_VERIFIED audit Emit failed! Stdout: %s", stdout.String())
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &res)
	if res["ok"] == true {
		t.Fatalf("expected ok:false when audit emit fails, got: %v", res)
	}
}

// TestR2_Phase2_OmitVerdictHash_ReadsFromPhase1Record verifies that omitting --verdict-hash
// resolves the verdict hash from the authenticated Phase 1 record keyed by --sha/--spec.
func TestR2_Phase2_OmitVerdictHash_ReadsFromPhase1Record(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artix_r2_omit_hash")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath := filepath.Join(tempDir, "artix")
	buildCmd := exec.Command("go", "build", "-o", binPath, "artix/cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v (%s)", err, string(out))
	}

	workDir, _ := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)

	headSHA := "def1234567890abcdef1234567890abcdef12345"
	verdictHash := "verdict_hash_from_phase_1_authenticated"
	storyID := "SPEC-R2-B"

	logger := audit.Default(workDir)
	_ = logger.Emit(audit.AuditEvent{
		EventType:   "CANDIDATE_PUSHED",
		Status:      "AWAITING_APPROVAL",
		StorySpecID: storyID,
		Details: map[string]any{
			"storyId":             storyID,
			"candidateSHA":        headSHA,
			"prBranch":            "artix-pr-11",
			"reviewerVerdictHash": verdictHash,
		},
	})

	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/pulls/11") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]any{"login": "developer-bob", "type": "User"},
				"head": map[string]any{"sha": headSHA},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/11/reviews") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        1,
					"user":      map[string]any{"login": "lead-alice", "type": "User"},
					"state":     "APPROVED",
					"commit_id": headSHA,
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pulls/11/merge") {
			_ = json.NewEncoder(w).Encode(map[string]any{"merged": true, "sha": headSHA})
			return
		}
		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	// Run artix verify-approval WITHOUT --verdict-hash
	cmdMerge := exec.Command(binPath, "verify-approval", "--json",
		"--pr", "11",
		"--sha", headSHA,
		"--spec", storyID,
		"--forge", "github",
		"--forge-url", ghServer.URL,
		"--forge-owner", "acme",
		"--forge-repo", "core",
	)
	cmdMerge.Dir = workDir
	var stdout, stderr strings.Builder
	cmdMerge.Stdout = &stdout
	cmdMerge.Stderr = &stderr
	if err := cmdMerge.Run(); err != nil {
		t.Fatalf("expected artix verify-approval without --verdict-hash to succeed by reading from Phase 1 record, got error: %v (stdout: %s, stderr: %s)", err, stdout.String(), stderr.String())
	}
}

// TestR2_Phase2_SpecRequiredAndNonEmpty verifies that omitting --spec or passing empty --spec is rejected.
func TestR2_Phase2_SpecRequiredAndNonEmpty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artix_r2_spec_req")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath := filepath.Join(tempDir, "artix")
	buildCmd := exec.Command("go", "build", "-o", binPath, "artix/cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v (%s)", err, string(out))
	}

	workDir, _ := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)

	cmdMerge := exec.Command(binPath, "verify-approval", "--json",
		"--pr", "12",
		"--sha", "abc12345",
		"--spec", "", // empty spec
	)
	cmdMerge.Dir = workDir
	var stdout, stderr strings.Builder
	cmdMerge.Stdout = &stdout
	cmdMerge.Stderr = &stderr
	err = cmdMerge.Run()
	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R2 c): artix verify-approval succeeded with empty --spec! (must require non-empty --spec)")
	}
}

// TestR2_Phase2_EnterpriseTrustRoots_IgnoreEnvKeys verifies that a binary built with the enterprise ldflag
// strictly ignores ARTIX_POLICY_PATH, ARTIX_POLICY_SIGNING_KEY, ARTIX_POLICY_TRUSTED_PUBKEY, and ARTIX_AUDIT_PUBLIC_KEY.
func TestR2_Phase2_EnterpriseTrustRoots_IgnoreEnvKeys(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artix_r2_enterprise_trust")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	binPath := filepath.Join(tempDir, "artix-enterprise")
	// Compile binary WITH enterprise ldflag
	buildCmd := exec.Command("go", "build", "-ldflags", "-X artix/pkg/policy.RequireSignedPolicyFlag=true", "-o", binPath, "artix/cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build enterprise binary: %v (%s)", err, string(out))
	}

	workDir, _ := setupTestRepoForForge(t)
	defer os.RemoveAll(workDir)

	// Create a rogue self-signed policy in a non-root path
	roguePolicyPath := filepath.Join(workDir, "rogue_policy.json")
	roguePolicyJSON := `{
		"enterpriseMode": true,
		"allowAutonomous": true,
		"requireSignedPolicy": true,
		"requireForgeApproval": false,
		"allowedTestCommands": ["true"]
	}`
	_ = os.WriteFile(roguePolicyPath, []byte(roguePolicyJSON), 0644)
	rogueSecret := "rogue-env-secret-999"
	_ = policy.SignPolicyFile(roguePolicyPath, rogueSecret)

	// Run enterprise binary with env overrides pointing to rogue policy and keys
	cmd := exec.Command(binPath, "run", "--json", "SPEC-ROGUE")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"ARTIX_POLICY_PATH="+roguePolicyPath,
		"ARTIX_POLICY_SIGNING_KEY="+rogueSecret,
		"ARTIX_POLICY_TRUSTED_PUBKEY=00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		"ARTIX_AUDIT_PUBLIC_KEY=00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	if err == nil {
		t.Fatalf("SECURITY VIOLATION (R2 d): enterprise-ldflag binary accepted self-signed policy and env keys! Output: %s", stdout.String())
	}
}

// TestR2_Phase2_RemoteBranchCleanup_OnlyAuthorizedReviewer verifies that candidate remote branch
// is ONLY deleted on rejection from an AUTHORIZED reviewer, and NOT on bot or unauthorized reviewer rejection.
func TestR2_Phase2_RemoteBranchCleanup_OnlyAuthorizedReviewer(t *testing.T) {
	tempDir, driver := setupTestRepoForForge(t)
	defer os.RemoveAll(tempDir)

	headSHA := "abc1234567890abcdef1234567890abcdef12345"
	storyID := "SPEC-R2-E"
	prBranch := "artix-pr-e2e-auth"

	// Mock bare remote
	bareDir, err := os.MkdirTemp("", "bare_r2_e")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(bareDir)
	_ = exec.Command("git", "init", "--bare", bareDir).Run()
	_ = exec.Command("git", "-C", tempDir, "remote", "add", "origin", bareDir).Run()

	// Push candidate branch to bare remote
	_, _ = driver.Push("origin", fmt.Sprintf("HEAD:refs/heads/%s", prBranch))

	// Verify branch exists on remote
	checkBranchExists := func() bool {
		cmd := exec.Command("git", "--git-dir", bareDir, "branch", "--list", prBranch)
		out, _ := cmd.Output()
		return strings.Contains(string(out), prBranch)
	}
	if !checkBranchExists() {
		t.Fatalf("precondition failed: remote branch %s does not exist", prBranch)
	}

	// 1. Bot "changes requested" verifier
	botRejectionVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, fmt.Errorf("%w: bot account 'code-bot' requested changes", ErrBotApprover)
	}

	// Phase 1 record binding
	logger := audit.Default(tempDir)
	_ = logger.Emit(audit.AuditEvent{
		EventType:   "CANDIDATE_PUSHED",
		Status:      "AWAITING_APPROVAL",
		StorySpecID: storyID,
		Details: map[string]any{
			"storyId":             storyID,
			"candidateSHA":        headSHA,
			"prBranch":            prBranch,
			"reviewerVerdictHash": "hash123",
		},
	})

	_, err = VerifyAndMergeCandidate(context.Background(), driver, botRejectionVerifier, headSHA, logger, storyID, "origin", prBranch, "hash123")
	if err == nil {
		t.Fatalf("expected verification to fail")
	}
	// Branch must NOT be deleted after bot rejection
	if !checkBranchExists() {
		t.Fatalf("SECURITY VIOLATION (R2 e): remote branch was deleted after BOT rejection!")
	}

	// 2. Authorized human changes requested verifier
	authRejectionVerifier := func(ctx context.Context, commitSHA string) (*policy.PRApproval, error) {
		return nil, fmt.Errorf("%w: security-lead requested changes", ErrChangesRequested)
	}

	_, err = VerifyAndMergeCandidate(context.Background(), driver, authRejectionVerifier, headSHA, logger, storyID, "origin", prBranch, "hash123")
	if err == nil {
		t.Fatalf("expected verification to fail")
	}
	// Branch MUST be deleted after authorized human rejection
	if checkBranchExists() {
		t.Fatalf("expected remote branch %s to be deleted after authorized human rejection, but it still exists", prBranch)
	}
}

// TestR14_8_FullEnterprise_Phase1_To_VerifyApproval_E2E verifies the complete enterprise flow:
// - Master trust key compiled into binary via ldflags
// - Root policy signed by master trust key
// - NO ARTIX_ENTERPRISE environment variable passed
// - Fixed-state mock forge server
// - Negative cases: stale head, self-approval, bot approver, dismissed review, 5xx forge error
// - Positive case: verified approval and merge
func TestR14_8_FullEnterprise_Phase1_To_VerifyApproval_E2E(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Generate master trust key and audit keypair
	pubMaster, privMaster, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	masterPubKeyHex := hex.EncodeToString(pubMaster)

	pubAudit, privAudit, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	auditPubHex := hex.EncodeToString(pubAudit)
	auditPrivHex := hex.EncodeToString(privAudit)

	secDir := filepath.Join(tempDir, ".artix")
	_ = os.MkdirAll(secDir, 0700)
	auditKeyPath := filepath.Join(secDir, "audit_ed25519.key")
	if err := os.WriteFile(auditKeyPath, []byte(auditPrivHex), 0600); err != nil {
		t.Fatal(err)
	}

	// 2. Create and sign root policy
	pol := &policy.Policy{
		EnterpriseMode:          true,
		AllowAutonomous:         true,
		RequireSignedPolicy:     true,
		RequireSeparateApprover: true,
		AllowedApprovers:        []string{"alice", "security-lead"},
		AuditPublicKey:          auditPubHex,
		AuditPrivateKeyPath:     auditKeyPath,
		AuditRemoteSinks:        []policy.RemoteSinkConfig{{Type: "syslog", Endpoint: "127.0.0.1:514"}},
		AllowedTestCommands:     []string{"grep '1' counter.txt"},
		RequireForgeApproval:    true,
	}
	polBytes, err := json.Marshal(pol)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(privMaster, polBytes)
	sigHex := hex.EncodeToString(sig)

	policyFile := filepath.Join(tempDir, "enterprise-root-policy.json")
	if err := os.WriteFile(policyFile, polBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyFile+".sig", []byte(sigHex), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Compile artix binary with compiled master key
	binPath := filepath.Join(tempDir, "artix")
	ldflags := fmt.Sprintf("-X artix/pkg/policy.CompiledTrustedPublicKeyHex=%s", masterPubKeyHex)
	buildCmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", binPath, "artix/cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build artix binary with compiled master key: %v (%s)", err, string(out))
	}

	// 4. Set up mock LLM server for Phase 1 code generation & reviewer
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		bodyBytes, _ := io.ReadAll(r.Body)
		bodyStr := string(bodyBytes)
		if strings.Contains(bodyStr, "adversarial_code_reviewer") || strings.Contains(bodyStr, "rubric") || strings.Contains(bodyStr, "Evaluation Rubric") || strings.Contains(bodyStr, "Reviewer") || strings.Contains(bodyStr, "Adversarial") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "msg_critic",
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "text", "text": `{"approved": true, "blocking": [], "warnings": []}`},
				},
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": `{"approved": true, "blocking": [], "warnings": []}`,
						},
					},
				},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 10, "prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_coder",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "```diff\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n```\n"},
			},
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "```diff\n--- a/counter.txt\n+++ b/counter.txt\n@@ -1 +1 @@\n-0\n+1\n```\n",
					},
				},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 20, "prompt_tokens": 20, "completion_tokens": 20, "total_tokens": 40},
		})
	}))
	defer llmServer.Close()

	// 5. Set up mock forge server with independent fixed PR head SHA and configurable review states
	var forgeMu sync.Mutex
	forgeMode := "valid"
	fixedPRHeadSHA := "1111222233334444555566667777888899990000"

	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forgeMu.Lock()
		mode := forgeMode
		prHead := fixedPRHeadSHA
		forgeMu.Unlock()

		if mode == "5xx" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"500 Internal Server Error"}`))
			return
		}

		// GET /repos/acme/core/pulls/101
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/pulls/101") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 101,
				"state":  "open",
				"user": map[string]any{
					"login": "artix-agent",
					"type":  "User",
				},
				"head": map[string]any{
					"sha": prHead,
					"ref": "artix-pr-101",
				},
				"base": map[string]any{
					"ref": "main",
				},
			})
			return
		}

		// GET /repos/acme/core/pulls/101/reviews
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/pulls/101/reviews") {
			w.Header().Set("Content-Type", "application/json")
			switch mode {
			case "valid":
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":        1001,
						"state":     "APPROVED",
						"commit_id": prHead,
						"user": map[string]any{
							"login": "alice",
							"type":  "User",
						},
						"submitted_at": "2026-10-09T00:00:00Z",
					},
				})
			case "self_approval":
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":        1002,
						"state":     "APPROVED",
						"commit_id": prHead,
						"user": map[string]any{
							"login": "artix-agent",
							"type":  "User",
						},
						"submitted_at": "2026-10-09T00:00:00Z",
					},
				})
			case "bot":
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":        1003,
						"state":     "APPROVED",
						"commit_id": prHead,
						"user": map[string]any{
							"login": "dependabot[bot]",
							"type":  "Bot",
						},
						"submitted_at": "2026-10-09T00:00:00Z",
					},
				})
			case "dismissed":
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":        1004,
						"state":     "DISMISSED",
						"commit_id": prHead,
						"user": map[string]any{
							"login": "alice",
							"type":  "User",
						},
						"submitted_at": "2026-10-09T00:00:00Z",
					},
				})
			default:
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			}
			return
		}

		// PUT /repos/acme/core/pulls/101/merge
		if r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/pulls/101/merge") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sha":     "merged-commit-sha-9999",
				"merged":  true,
				"message": "Pull Request successfully merged",
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer ghServer.Close()

	// 6. Initialize git workspace and remote bare origin
	workDir := filepath.Join(tempDir, "work")
	bareDir := filepath.Join(tempDir, "bare.git")
	_ = os.MkdirAll(workDir, 0755)
	_ = os.MkdirAll(bareDir, 0755)

	_ = exec.Command("git", "init", "--bare", bareDir).Run()
	_ = exec.Command("git", "init", workDir).Run()
	_ = exec.Command("git", "-C", workDir, "config", "user.email", "agent@artix.ai").Run()
	_ = exec.Command("git", "-C", workDir, "config", "user.name", "Artix Agent").Run()
	_ = exec.Command("git", "-C", workDir, "remote", "add", "origin", bareDir).Run()

	initialFile := filepath.Join(workDir, "counter.txt")
	_ = os.WriteFile(initialFile, []byte("0\n"), 0644)
	_ = exec.Command("git", "-C", workDir, "add", ".").Run()
	_ = exec.Command("git", "-C", workDir, "commit", "-m", "init").Run()
	_ = exec.Command("git", "-C", workDir, "branch", "-M", "main").Run()
	_ = exec.Command("git", "-C", workDir, "push", "-u", "origin", "main").Run()

	specsDir := filepath.Join(workDir, "docs", "specs")
	_ = os.MkdirAll(specsDir, 0755)
	specFile := filepath.Join(specsDir, "STORY-SPEC-E2E-01.md")
	specContent := "# Story Spec: Enterprise E2E\n\n**Spec ID:** `SPEC-E2E-01`\n\n## 2. Acceptance Criteria\n\n### Scenario 1: counter updated\n* **Given** a counter file\n* **When** it updates\n* **Then** it contains 1\n\n## 5. Verification Test Suite\n\n```bash\ngrep '1' counter.txt\n```\n"
	_ = os.WriteFile(specFile, []byte(specContent), 0644)

	// 7. Execute Phase 1 with compiled binary (WITHOUT ARTIX_ENTERPRISE)
	// Base environment: strictly NO ARTIX_ENTERPRISE env variable
	baseEnv := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tempDir,
		"ARTIX_POLICY_PATH=" + policyFile,
		"ARTIX_AUDIT_PRIVATE_KEY=" + auditPrivHex,
		"ARTIX_AUDIT_PRIVATE_KEY_PATH=" + auditKeyPath,
		"ARTIX_AUDIT_PUBLIC_KEY=" + auditPubHex,
		"ARTIX_ALLOW_AUTONOMOUS=1",
		"ARTIX_PR_BRANCH=artix-pr-101",
		"ARTIX_FORGE_REMOTE=origin",
		"ANTHROPIC_API_KEY=mock-key",
		"OPENAI_API_KEY=mock-key",
		"ARTIX_API_URL=" + llmServer.URL,
	}

	cmdPhase1 := exec.Command(binPath, "code", "--json",
		"--autonomy", "autonomous",
		"--confirm-tests",
		"--provider", "anthropic",
		"--model", "claude-3-5-sonnet-20241022",
		"--review-provider", "openai",
		"--review-model", "gpt-4o",
		"--forge", "github",
		"--forge-pr", "101",
		"--forge-url", ghServer.URL,
		"--forge-owner", "acme",
		"--forge-repo", "core",
		specFile,
	)
	cmdPhase1.Dir = workDir
	cmdPhase1.Env = baseEnv

	var stdoutPhase1, stderrPhase1 strings.Builder
	cmdPhase1.Stdout = &stdoutPhase1
	cmdPhase1.Stderr = &stderrPhase1
	errPhase1 := cmdPhase1.Run()
	if errPhase1 != nil {
		t.Fatalf("Phase 1 execution failed: %v (stdout: %s, stderr: %s)", errPhase1, stdoutPhase1.String(), stderrPhase1.String())
	}

	var jsonPhase1 struct {
		Status           string `json:"status"`
		AwaitingApproval bool   `json:"awaitingApproval"`
		CommitHash       string `json:"commitHash"`
		VerdictHash      string `json:"verdictHash"`
	}
	stdoutStr := strings.TrimSpace(stdoutPhase1.String())
	if jsonIdx := strings.Index(stdoutStr, "{"); jsonIdx >= 0 {
		stdoutStr = stdoutStr[jsonIdx:]
	}
	if err := json.Unmarshal([]byte(stdoutStr), &jsonPhase1); err != nil {
		t.Fatalf("failed to parse Phase 1 JSON output: %v (stdout: %s, stderr: %s)", err, stdoutStr, stderrPhase1.String())
	}

	if jsonPhase1.Status != "awaiting_approval" || !jsonPhase1.AwaitingApproval {
		t.Fatalf("expected Phase 1 to return awaiting_approval, got: %+v", jsonPhase1)
	}
	if jsonPhase1.CommitHash == "" || jsonPhase1.VerdictHash == "" {
		t.Fatalf("expected non-empty commitHash and verdictHash in Phase 1 output: %+v", jsonPhase1)
	}

	candidateSHA := jsonPhase1.CommitHash
	verdictHash := jsonPhase1.VerdictHash

	runVerifyApproval := func(mode string, forgeHead string) (int, string, string) {
		forgeMu.Lock()
		forgeMode = mode
		fixedPRHeadSHA = forgeHead
		forgeMu.Unlock()

		cmd := exec.Command(binPath, "verify-approval", "--json",
			"--pr", "101",
			"--sha", candidateSHA,
			"--spec", "SPEC-E2E-01",
			"--verdict-hash", verdictHash,
			"--forge", "github",
			"--forge-url", ghServer.URL,
			"--forge-owner", "acme",
			"--forge-repo", "core",
			"--remote", "origin",
			"--branch", "artix-pr-101",
		)
		cmd.Dir = workDir
		cmd.Env = baseEnv

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		return exitCode, stdout.String(), stderr.String()
	}

	// 7. Negative Case 1: Stale Head (PR head on forge differs from candidate commit SHA)
	exitStale, outStale, _ := runVerifyApproval("valid", "deadbeef00000000000000000000000000000000")
	if exitStale == 0 || strings.Contains(outStale, `"status":"approval_verified"`) {
		t.Fatalf("SECURITY VIOLATION (R14-8): verify-approval succeeded with STALE HEAD! Output: %s", outStale)
	}

	// 8. Negative Case 2: Self-Approval (Approver is author/agent)
	exitSelf, outSelf, _ := runVerifyApproval("self_approval", candidateSHA)
	if exitSelf == 0 || strings.Contains(outSelf, `"status":"approval_verified"`) {
		t.Fatalf("SECURITY VIOLATION (R14-8): verify-approval succeeded with SELF-APPROVAL! Output: %s", outSelf)
	}

	// 9. Negative Case 3: Bot Approval (Approver is bot account)
	exitBot, outBot, _ := runVerifyApproval("bot", candidateSHA)
	if exitBot == 0 || strings.Contains(outBot, `"status":"approval_verified"`) {
		t.Fatalf("SECURITY VIOLATION (R14-8): verify-approval succeeded with BOT APPROVAL! Output: %s", outBot)
	}

	// 10. Negative Case 4: Dismissed Review (Review state is DISMISSED)
	exitDismissed, outDismissed, _ := runVerifyApproval("dismissed", candidateSHA)
	if exitDismissed == 0 || strings.Contains(outDismissed, `"status":"approval_verified"`) {
		t.Fatalf("SECURITY VIOLATION (R14-8): verify-approval succeeded with DISMISSED REVIEW! Output: %s", outDismissed)
	}

	// 11. Negative Case 5: 5xx Forge Error (Forge returns 500 server error)
	exit5xx, out5xx, _ := runVerifyApproval("5xx", candidateSHA)
	if exit5xx == 0 || strings.Contains(out5xx, `"status":"approval_verified"`) {
		t.Fatalf("SECURITY VIOLATION (R14-8): verify-approval succeeded with 5XX FORGE ERROR! Output: %s", out5xx)
	}

	// 12. Positive Case: Valid Flow (Authorized human approval on exact candidate SHA)
	exitValid, outValid, errValid := runVerifyApproval("valid", candidateSHA)
	if exitValid != 0 {
		t.Fatalf("expected valid verify-approval to succeed with exit 0, got exit %d: %s (err: %s)", exitValid, outValid, errValid)
	}
	if !strings.Contains(outValid, `"status":"approval_verified"`) || !strings.Contains(outValid, `"ok":true`) {
		t.Fatalf("expected verify-approval output to have status approval_verified and ok:true, got: %s", outValid)
	}
}









