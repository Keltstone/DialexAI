package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RoundCheckpoint records a snapshot of code and feedback at an iteration round.
type RoundCheckpoint struct {
	Round      int       `json:"round"`
	CommitHash string    `json:"commitHash"`
	Diff       string    `json:"diff"`
	Feedback   string    `json:"feedback"`
	Approved   bool      `json:"approved"`
	Timestamp  time.Time `json:"timestamp"`
}

// ShadowWorktree represents an isolated, background git worktree.
type ShadowWorktree struct {
	TaskID      string            `json:"taskId"`
	Branch      string            `json:"branch"`
	BaseBranch  string            `json:"baseBranch"`
	BaseCommit  string            `json:"baseCommit"`
	Path        string            `json:"path"`
	Checkpoints []RoundCheckpoint `json:"checkpoints"`
	repoRoot    string
}

// WorktreeManager manages isolated shadow execution environments.
type WorktreeManager struct {
	repoRoot    string
	worktreeDir string
}

// NewWorktreeManager creates a worktree manager for a repository.
func NewWorktreeManager(repoRoot string) *WorktreeManager {
	wtDir := filepath.Join(repoRoot, ".artix", "worktrees")
	_ = os.MkdirAll(wtDir, 0755)

	return &WorktreeManager{
		repoRoot:    repoRoot,
		worktreeDir: wtDir,
	}
}

// CreateShadow creates a detached git worktree on a new branch and records the base commit hash.
func (m *WorktreeManager) CreateShadow(taskID, baseBranch string) (*ShadowWorktree, error) {
	if baseBranch == "" {
		baseBranch = "HEAD"
	}

	// Resolve the exact base commit hash for collision detection
	baseCommit := ""
	revCmd := exec.Command("git", "rev-parse", baseBranch)
	revCmd.Dir = m.repoRoot
	if out, err := revCmd.Output(); err == nil {
		baseCommit = strings.TrimSpace(string(out))
	}

	branchName := fmt.Sprintf("artix/shadow-%s-%d", taskID, time.Now().Unix())
	targetPath := filepath.Join(m.worktreeDir, taskID)

	// Remove any leftover worktree at target path
	_ = m.runGit("worktree", "remove", "--force", targetPath)
	_ = os.RemoveAll(targetPath)

	// git worktree add -b <branchName> <targetPath> <baseBranch>
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, targetPath, baseBranch)
	cmd.Dir = m.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to create shadow worktree: %v\nOutput: %s", err, string(out))
	}

	// Symlink .standards into shadow worktree if exists in root
	rootStandards := filepath.Join(m.repoRoot, ".standards")
	if info, err := os.Stat(rootStandards); err == nil && info.IsDir() {
		_ = os.Symlink(rootStandards, filepath.Join(targetPath, ".standards"))
	}

	return &ShadowWorktree{
		TaskID:      taskID,
		Branch:      branchName,
		BaseBranch:  baseBranch,
		BaseCommit:  baseCommit,
		Path:        targetPath,
		Checkpoints: make([]RoundCheckpoint, 0),
		repoRoot:    m.repoRoot,
	}, nil
}

// RecordCheckpoint saves an iteration snapshot commit inside the shadow worktree.
func (sw *ShadowWorktree) RecordCheckpoint(round int, feedback string, approved bool) (*RoundCheckpoint, error) {
	driver := NewDriver(sw.Path)
	diff, _ := driver.Diff(false)

	msg := fmt.Sprintf("checkpoint: round %d (Approved: %v)\n\nFeedback: %s", round, approved, feedback)
	hash, err := driver.CommitAll(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to record round checkpoint: %w", err)
	}

	cp := RoundCheckpoint{
		Round:      round,
		CommitHash: hash,
		Diff:       diff,
		Feedback:   feedback,
		Approved:   approved,
		Timestamp:  time.Now(),
	}

	sw.Checkpoints = append(sw.Checkpoints, cp)
	return &cp, nil
}

// MergeInto merges the shadow worktree branch into the target branch, performing pre-merge collision checks.
func (m *WorktreeManager) MergeInto(sw *ShadowWorktree, targetBranch string, squash bool) (string, error) {
	return m.MergeIntoContext(context.Background(), sw, targetBranch, squash)
}

// IsDirty checks if the repository has uncommitted changes or untracked files (excluding .artix internal files).
func (m *WorktreeManager) IsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = m.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return true // fail-closed if status cannot be determined
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) >= 2 {
			path := parts[len(parts)-1]
			if path == ".artix" || strings.HasPrefix(path, ".artix/") {
				continue
			}
		}
		return true
	}
	return false
}

// MergeIntoContext merges the shadow worktree branch into the target branch with context cancellation support.
// If the context is cancelled or the merge fails, any half-merged state is rolled back and cleaned up.
func (m *WorktreeManager) MergeIntoContext(ctx context.Context, sw *ShadowWorktree, targetBranch string, squash bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if targetBranch == "" {
		targetBranch = "HEAD"
	}

	// File-lock coordination to prevent concurrent collision between multiple artix processes
	lockPath := filepath.Join(m.repoRoot, ".artix", "merge.lock")
	_ = os.MkdirAll(filepath.Dir(lockPath), 0755)
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open merge lock at %s: %w", lockPath, err)
	}
	unlock := lockFile(lf)
	defer unlock()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Safety check: Never merge into a dirty tree to protect developer uncommitted work
	if m.IsDirty() {
		return "", fmt.Errorf("refusing to merge: working tree %s contains uncommitted changes or untracked files (merge refused to preserve developer work)", m.repoRoot)
	}

	// Record pre-merge HEAD commit
	revCmd := exec.Command("git", "rev-parse", "HEAD")
	revCmd.Dir = m.repoRoot
	preOut, err := revCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	preCommit := strings.TrimSpace(string(preOut))



	// Re-verification: Check if target branch has advanced since worktree was branched
	if sw.BaseCommit != "" {
		revCmd := exec.Command("git", "rev-parse", targetBranch)
		revCmd.Dir = m.repoRoot
		if currentOut, err := revCmd.Output(); err == nil {
			currentHead := strings.TrimSpace(string(currentOut))
			if currentHead != sw.BaseCommit {
				// Base has advanced; verify if it is an ancestor or divergent
				ancCmd := exec.Command("git", "merge-base", "--is-ancestor", sw.BaseCommit, currentHead)
				ancCmd.Dir = m.repoRoot
				if err := ancCmd.Run(); err != nil {
					return "", fmt.Errorf("concurrent worktree collision detected: target branch %s has divergent history from base commit %s; rebase required", targetBranch, sw.BaseCommit)
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Create ephemeral integration worktree so user's main checkout is never touched during merge
	intDir := filepath.Join(m.worktreeDir, fmt.Sprintf("int-%s-%d", sw.TaskID, time.Now().UnixNano()))
	addCmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", intDir, preCommit)
	addCmd.Dir = m.repoRoot
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to create integration worktree: %v\nOutput: %s", err, string(out))
	}
	defer func() {
		removeCmd := exec.Command("git", "worktree", "remove", "--force", intDir)
		removeCmd.Dir = m.repoRoot
		_ = removeCmd.Run()
		_ = os.RemoveAll(intDir)
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = m.repoRoot
		_ = pruneCmd.Run()
	}()

	args := []string{"merge"}
	if squash {
		args = append(args, "--squash")
	}
	args = append(args, sw.Branch)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = intDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to merge shadow branch %s: %v\nOutput: %s", sw.Branch, err, string(out))
	}

	if squash {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", fmt.Sprintf("feat: merge shadow worktree %s", sw.TaskID))
		commitCmd.Dir = intDir
		if out, err := commitCmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to commit squashed merge: %v\nOutput: %s", err, string(out))
		}
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Get latest commit hash from integration worktree
	revCmdPost := exec.Command("git", "rev-parse", "HEAD")
	revCmdPost.Dir = intDir
	hashBytes, err := revCmdPost.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get merged commit hash: %w", err)
	}
	mergedHash := strings.TrimSpace(string(hashBytes))

	// Fast-forward target in m.repoRoot
	ffCmd := exec.CommandContext(ctx, "git", "merge", "--ff-only", mergedHash)
	ffCmd.Dir = m.repoRoot
	if out, err := ffCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to fast-forward target to %s: %v\nOutput: %s", mergedHash, err, string(out))
	}

	return mergedHash, nil
}

// MergeIntoWithVerification merges the shadow branch, executes verifyCmds against the post-merge
// state, and if any test fails, automatically rolls back the merge and returns a collision error.
func (m *WorktreeManager) MergeIntoWithVerification(sw *ShadowWorktree, targetBranch string, squash bool, verifyCmds []string, runner func(cmdStr string) error) (string, error) {
	if m.IsDirty() {
		return "", fmt.Errorf("refusing to merge: working tree %s contains uncommitted changes or untracked files", m.repoRoot)
	}
	if targetBranch == "" {
		targetBranch = "HEAD"
	}

	lockPath := filepath.Join(m.repoRoot, ".artix", "merge.lock")
	_ = os.MkdirAll(filepath.Dir(lockPath), 0755)
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open merge lock at %s: %w", lockPath, err)
	}
	unlock := lockFile(lf)
	defer unlock()

	// Remember pre-merge commit hash
	revCmd := exec.Command("git", "rev-parse", "HEAD")
	revCmd.Dir = m.repoRoot
	preOut, err := revCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to determine HEAD commit before merge: %w", err)
	}
	preCommit := strings.TrimSpace(string(preOut))

	// Create ephemeral integration worktree
	intDir := filepath.Join(m.worktreeDir, fmt.Sprintf("int-verify-%s-%d", sw.TaskID, time.Now().UnixNano()))
	addCmd := exec.Command("git", "worktree", "add", "--detach", intDir, preCommit)
	addCmd.Dir = m.repoRoot
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to create integration worktree: %v\nOutput: %s", err, string(out))
	}
	defer func() {
		removeCmd := exec.Command("git", "worktree", "remove", "--force", intDir)
		removeCmd.Dir = m.repoRoot
		_ = removeCmd.Run()
		_ = os.RemoveAll(intDir)
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = m.repoRoot
		_ = pruneCmd.Run()
	}()

	args := []string{"merge"}
	if squash {
		args = append(args, "--squash")
	}
	args = append(args, sw.Branch)

	mergeCmd := exec.Command("git", args...)
	mergeCmd.Dir = intDir
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to merge shadow branch %s: %v\nOutput: %s", sw.Branch, err, string(out))
	}

	if squash {
		commitCmd := exec.Command("git", "commit", "-m", fmt.Sprintf("feat: merge shadow worktree %s", sw.TaskID))
		commitCmd.Dir = intDir
		if out, err := commitCmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to commit squashed merge: %v\nOutput: %s", err, string(out))
		}
	}

	revCmdPost := exec.Command("git", "rev-parse", "HEAD")
	revCmdPost.Dir = intDir
	hashBytes, err := revCmdPost.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get merged commit hash: %w", err)
	}
	mergedHash := strings.TrimSpace(string(hashBytes))

	// Execute post-merge verification suite against integration worktree or via runner
	if runner != nil && len(verifyCmds) > 0 {
		for _, cmdStr := range verifyCmds {
			if vErr := runner(cmdStr); vErr != nil {
				// Integration worktree is deleted by defer; main checkout was NEVER touched!
				return "", fmt.Errorf("post-merge test verification failed for %q: %w (merge rolled back to %s)", cmdStr, vErr, preCommit)
			}
		}
	}

	// Verification passed: Fast-forward target in m.repoRoot
	if m.IsDirty() {
		return "", fmt.Errorf("refusing to update checkout: working tree %s became dirty during verification", m.repoRoot)
	}
	ffCmd := exec.Command("git", "merge", "--ff-only", mergedHash)
	ffCmd.Dir = m.repoRoot
	if out, err := ffCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to fast-forward target to %s: %v\nOutput: %s", mergedHash, err, string(out))
	}

	return mergedHash, nil
}

// Remove cleans up the shadow worktree and removes its directory.
func (m *WorktreeManager) Remove(sw *ShadowWorktree) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", sw.Path)
	cmd.Dir = m.repoRoot
	_ = cmd.Run()
	_ = os.RemoveAll(sw.Path)

	// Prune dead worktree metadata
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = m.repoRoot
	return pruneCmd.Run()
}

// GarbageCollect removes orphaned or stale worktrees older than maxAge.
func (m *WorktreeManager) GarbageCollect(maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(m.worktreeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read worktrees directory: %w", err)
	}

	prunedCount := 0
	now := time.Now()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wtPath := filepath.Join(m.worktreeDir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}

		if maxAge <= 0 || now.Sub(info.ModTime()) > maxAge {
			_ = m.runGit("worktree", "remove", "--force", wtPath)
			_ = os.RemoveAll(wtPath)
			prunedCount++
		}
	}

	_ = m.runGit("worktree", "prune")
	return prunedCount, nil
}

func (m *WorktreeManager) runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoRoot
	return cmd.Run()
}
