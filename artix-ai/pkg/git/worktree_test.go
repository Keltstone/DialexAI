package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeManager_Lifecycle(t *testing.T) {
	tempRepo, err := os.MkdirTemp("", "kritix-wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempRepo)

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	// 1. Initialize repo
	runCmd(tempRepo, "git", "init", "-b", "main")
	runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
	runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
	_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Root Repo\n"), 0644)
	runCmd(tempRepo, "git", "add", ".")
	runCmd(tempRepo, "git", "commit", "-m", "initial commit")

	mgr := NewWorktreeManager(tempRepo)

	// 2. Create shadow worktree
	taskID := "task-xyz"
	sw, err := mgr.CreateShadow(taskID, "main")
	if err != nil {
		t.Fatalf("failed to create shadow worktree: %v", err)
	}

	if _, err := os.Stat(sw.Path); os.IsNotExist(err) {
		t.Fatalf("shadow worktree path does not exist: %s", sw.Path)
	}

	// 3. Make changes and record checkpoint 1 (Rejected)
	_ = os.WriteFile(filepath.Join(sw.Path, "feature.txt"), []byte("v1"), 0644)
	cp1, err := sw.RecordCheckpoint(1, "Missing tests", false)
	if err != nil {
		t.Fatalf("failed to record checkpoint 1: %v", err)
	}
	if cp1.Round != 1 || cp1.Approved != false || cp1.CommitHash == "" {
		t.Errorf("unexpected checkpoint 1: %+v", cp1)
	}

	// 4. Make changes and record checkpoint 2 (Approved)
	_ = os.WriteFile(filepath.Join(sw.Path, "feature.txt"), []byte("v2 with tests"), 0644)
	cp2, err := sw.RecordCheckpoint(2, "Tests pass", true)
	if err != nil {
		t.Fatalf("failed to record checkpoint 2: %v", err)
	}
	if cp2.Round != 2 || cp2.Approved != true {
		t.Errorf("unexpected checkpoint 2: %+v", cp2)
	}

	if len(sw.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints recorded, got %d", len(sw.Checkpoints))
	}

	// 5. Merge into main
	mergeHash, err := mgr.MergeInto(sw, "main", true)
	if err != nil {
		t.Fatalf("failed to merge shadow into main: %v", err)
	}
	if mergeHash == "" {
		t.Errorf("expected non-empty merge hash")
	}

	// Verify merged file exists in root repo
	content, err := os.ReadFile(filepath.Join(tempRepo, "feature.txt"))
	if err != nil || strings.TrimSpace(string(content)) != "v2 with tests" {
		t.Fatalf("merged content mismatch: %s (err: %v)", string(content), err)
	}

	// 6. Clean up shadow worktree
	if err := mgr.Remove(sw); err != nil {
		t.Fatalf("failed to remove shadow worktree: %v", err)
	}

	if _, err := os.Stat(sw.Path); !os.IsNotExist(err) {
		t.Errorf("expected shadow path to be deleted")
	}
}

func TestWorktreeManager_GarbageCollect(t *testing.T) {
	tempRepo := t.TempDir()

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	runCmd(tempRepo, "git", "init", "-b", "main")
	runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
	runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
	_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Root\n"), 0644)
	runCmd(tempRepo, "git", "add", ".")
	runCmd(tempRepo, "git", "commit", "-m", "initial commit")

	mgr := NewWorktreeManager(tempRepo)

	// Create 2 worktrees
	sw1, err := mgr.CreateShadow("task-old-01", "main")
	if err != nil {
		t.Fatalf("failed to create sw1: %v", err)
	}
	sw2, err := mgr.CreateShadow("task-old-02", "main")
	if err != nil {
		t.Fatalf("failed to create sw2: %v", err)
	}

	// GC with 0 duration should prune all worktrees
	pruned, err := mgr.GarbageCollect(0)
	if err != nil {
		t.Fatalf("GarbageCollect failed: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned worktrees, got %d", pruned)
	}

	if _, err := os.Stat(sw1.Path); !os.IsNotExist(err) {
		t.Errorf("expected sw1 to be removed")
	}
	if _, err := os.Stat(sw2.Path); !os.IsNotExist(err) {
		t.Errorf("expected sw2 to be removed")
	}
}

func TestWorktreeManager_MergeWithVerificationRollback(t *testing.T) {
	tempRepo := t.TempDir()

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	runCmd(tempRepo, "git", "init", "-b", "main")
	runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
	runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
	_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Root\n"), 0644)
	runCmd(tempRepo, "git", "add", ".")
	runCmd(tempRepo, "git", "commit", "-m", "initial commit")

	revCmd := exec.Command("git", "rev-parse", "HEAD")
	revCmd.Dir = tempRepo
	initialOut, _ := revCmd.Output()
	initialCommit := strings.TrimSpace(string(initialOut))

	mgr := NewWorktreeManager(tempRepo)
	sw, err := mgr.CreateShadow("task-verify-01", "main")
	if err != nil {
		t.Fatalf("CreateShadow failed: %v", err)
	}

	_ = os.WriteFile(filepath.Join(sw.Path, "broken.txt"), []byte("this breaks tests\n"), 0644)
	_, _ = sw.RecordCheckpoint(1, "added broken file", true)

	// Attempt merge with a failing test verification command
	failingRunner := func(cmdStr string) error {
		return fmt.Errorf("test suite failed: synthetic test failure")
	}

	_, err = mgr.MergeIntoWithVerification(sw, "main", true, []string{"fake-test"}, failingRunner)
	if err == nil {
		t.Fatal("expected MergeIntoWithVerification to fail when verification fails")
	}

	// Verify rollback happened: HEAD must be restored to initialCommit
	revCmd2 := exec.Command("git", "rev-parse", "HEAD")
	revCmd2.Dir = tempRepo
	postOut, _ := revCmd2.Output()
	postCommit := strings.TrimSpace(string(postOut))

	if postCommit != initialCommit {
		t.Fatalf("expected rollback to %s, but HEAD is at %s", initialCommit, postCommit)
	}

	// Verify broken.txt does NOT exist in main repo
	if _, err := os.Stat(filepath.Join(tempRepo, "broken.txt")); !os.IsNotExist(err) {
		t.Fatal("broken.txt should not exist in working directory after rollback")
	}
}

func TestWorktreeManager_DirtyCheckoutPreservationAcrossFailures(t *testing.T) {
	cases := []struct {
		name        string
		setupShadow func(t *testing.T, mgr *WorktreeManager, mainRepo string) *ShadowWorktree
		runMerge    func(ctx context.Context, mgr *WorktreeManager, sw *ShadowWorktree) error
	}{
		{
			name: "conflict",
			setupShadow: func(t *testing.T, mgr *WorktreeManager, mainRepo string) *ShadowWorktree {
				sw, err := mgr.CreateShadow("task-conflict", "main")
				if err != nil {
					t.Fatalf("CreateShadow failed: %v", err)
				}
				// Commit conflicting change on shadow branch
				_ = os.WriteFile(filepath.Join(sw.Path, "conflict_file.txt"), []byte("shadow version conflict\n"), 0644)
				_, _ = sw.RecordCheckpoint(1, "shadow conflict commit", true)

				// Commit conflicting change to main branch
				_ = os.WriteFile(filepath.Join(mainRepo, "conflict_file.txt"), []byte("main version conflicting edit\n"), 0644)
				cmdAdd := exec.Command("git", "add", "conflict_file.txt")
				cmdAdd.Dir = mainRepo
				_ = cmdAdd.Run()
				cmdCommit := exec.Command("git", "commit", "-m", "advance main with conflict")
				cmdCommit.Dir = mainRepo
				_ = cmdCommit.Run()
				return sw
			},
			runMerge: func(ctx context.Context, mgr *WorktreeManager, sw *ShadowWorktree) error {
				_, err := mgr.MergeIntoContext(ctx, sw, "main", false)
				return err
			},
		},
		{
			name: "context_cancel",
			setupShadow: func(t *testing.T, mgr *WorktreeManager, mainRepo string) *ShadowWorktree {
				sw, err := mgr.CreateShadow("task-cancel", "main")
				if err != nil {
					t.Fatalf("CreateShadow failed: %v", err)
				}
				_ = os.WriteFile(filepath.Join(sw.Path, "file.txt"), []byte("some content\n"), 0644)
				_, _ = sw.RecordCheckpoint(1, "checkpoint", true)
				return sw
			},
			runMerge: func(ctx context.Context, mgr *WorktreeManager, sw *ShadowWorktree) error {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // already cancelled
				_, err := mgr.MergeIntoContext(cancelCtx, sw, "main", false)
				return err
			},
		},
		{
			name: "squash_commit_failure",
			setupShadow: func(t *testing.T, mgr *WorktreeManager, mainRepo string) *ShadowWorktree {
				sw, err := mgr.CreateShadow("task-squash-fail", "main")
				if err != nil {
					t.Fatalf("CreateShadow failed: %v", err)
				}
				_ = os.WriteFile(filepath.Join(sw.Path, "squash_file.txt"), []byte("squash content\n"), 0644)
				_, _ = sw.RecordCheckpoint(1, "squash commit", true)
				// Install a failing pre-commit hook in repoRoot to force commit failure
				hooksDir := filepath.Join(mainRepo, ".git", "hooks")
				_ = os.MkdirAll(hooksDir, 0755)
				hookFile := filepath.Join(hooksDir, "pre-commit")
				_ = os.WriteFile(hookFile, []byte("#!/bin/sh\nexit 1\n"), 0755)
				return sw
			},
			runMerge: func(ctx context.Context, mgr *WorktreeManager, sw *ShadowWorktree) error {
				_, err := mgr.MergeIntoContext(ctx, sw, "main", true)
				return err
			},
		},
		{
			name: "post_merge_test_failure",
			setupShadow: func(t *testing.T, mgr *WorktreeManager, mainRepo string) *ShadowWorktree {
				sw, err := mgr.CreateShadow("task-postfail", "main")
				if err != nil {
					t.Fatalf("CreateShadow failed: %v", err)
				}
				_ = os.WriteFile(filepath.Join(sw.Path, "new_feature.txt"), []byte("feature code\n"), 0644)
				_, _ = sw.RecordCheckpoint(1, "feature", true)
				return sw
			},
			runMerge: func(ctx context.Context, mgr *WorktreeManager, sw *ShadowWorktree) error {
				failingRunner := func(cmdStr string) error {
					return fmt.Errorf("post-merge test failure")
				}
				_, err := mgr.MergeIntoWithVerification(sw, "main", true, []string{"test-suite"}, failingRunner)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tempRepo := t.TempDir()

			runCmd := func(dir string, name string, args ...string) {
				cmd := exec.Command(name, args...)
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
				}
			}

			runCmd(tempRepo, "git", "init", "-b", "main")
			runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
			runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
			_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Original Readme\n"), 0644)
			_ = os.WriteFile(filepath.Join(tempRepo, "conflict_file.txt"), []byte("main version\n"), 0644)
			runCmd(tempRepo, "git", "add", ".")
			runCmd(tempRepo, "git", "commit", "-m", "initial commit")

			mgr := NewWorktreeManager(tempRepo)
			sw := tc.setupShadow(t, mgr, tempRepo)

			// Now dirty the user's checkout with:
			// 1. Uncommitted edit on tracked file
			// 2. Untracked file
			modifiedContent := "# Developer uncommitted work in README\n"
			_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte(modifiedContent), 0644)

			untrackedContent := "Developer uncommitted untracked file\n"
			untrackedPath := filepath.Join(tempRepo, "untracked.txt")
			_ = os.WriteFile(untrackedPath, []byte(untrackedContent), 0644)

			// Attempt merge
			err := tc.runMerge(context.Background(), mgr, sw)
			if err == nil {
				t.Fatalf("expected merge to fail for case %s, but got nil", tc.name)
			}

			// Verify tracked modified file was NOT reverted
			readmeBytes, readErr := os.ReadFile(filepath.Join(tempRepo, "README.md"))
			if readErr != nil {
				t.Fatalf("failed to read README.md: %v", readErr)
			}
			if string(readmeBytes) != modifiedContent {
				t.Fatalf("DATA LOSS DETECTED: modified tracked file was clobbered! expected %q, got %q", modifiedContent, string(readmeBytes))
			}

			// Verify untracked file was NOT deleted by git clean -fd
			untrackedBytes, readUntrackedErr := os.ReadFile(untrackedPath)
			if readUntrackedErr != nil {
				t.Fatalf("DATA LOSS DETECTED: untracked file was deleted! error: %v", readUntrackedErr)
			}
			if string(untrackedBytes) != untrackedContent {
				t.Fatalf("untracked file content mismatch: expected %q, got %q", untrackedContent, string(untrackedBytes))
			}
		})
	}
}

func TestWorktreeManager_MergeLockFailure(t *testing.T) {
	tempRepo := t.TempDir()

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	runCmd(tempRepo, "git", "init", "-b", "main")
	runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
	runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
	_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Root\n"), 0644)
	runCmd(tempRepo, "git", "add", ".")
	runCmd(tempRepo, "git", "commit", "-m", "initial commit")

	mgr := NewWorktreeManager(tempRepo)
	sw, err := mgr.CreateShadow("task-lock-fail", "main")
	if err != nil {
		t.Fatalf("CreateShadow failed: %v", err)
	}

	// Make lock file uncreatable / unopenable by creating .artix/merge.lock as a directory with no permissions,
	// or making .artix read-only
	artixDir := filepath.Join(tempRepo, ".artix")
	_ = os.MkdirAll(artixDir, 0755)
	lockFileAsDir := filepath.Join(artixDir, "merge.lock")
	_ = os.MkdirAll(lockFileAsDir, 0555) // directory, so os.OpenFile will fail
	defer os.RemoveAll(lockFileAsDir)

	_, err = mgr.MergeIntoContext(context.Background(), sw, "main", false)
	if err == nil {
		t.Fatalf("expected MergeIntoContext to fail when merge.lock cannot be opened, but got nil")
	}
}

func TestN3_MergeRollbackCorrectness(t *testing.T) {
	tempRepo := t.TempDir()

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	runCmd(tempRepo, "git", "init", "-b", "main")
	runCmd(tempRepo, "git", "config", "user.name", "Test Admin")
	runCmd(tempRepo, "git", "config", "user.email", "admin@test.local")
	_ = os.WriteFile(filepath.Join(tempRepo, "README.md"), []byte("# Root\n"), 0644)
	runCmd(tempRepo, "git", "add", ".")
	runCmd(tempRepo, "git", "commit", "-m", "initial commit")

	mgr := NewWorktreeManager(tempRepo)

	// 1. TargetBranch="HEAD" must not create refs/heads/HEAD
	sw1, err := mgr.CreateShadow("task-head-ambig", "main")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(sw1.Path, "f1.txt"), []byte("data\n"), 0644)
	_, _ = sw1.RecordCheckpoint(1, "feat", true)

	failingRunner := func(cmd string) error { return fmt.Errorf("test failed") }
	_, _ = mgr.MergeIntoWithVerification(sw1, "HEAD", true, []string{"test"}, failingRunner)

	// Check if refs/heads/HEAD exists
	checkRef := exec.Command("git", "show-ref", "--verify", "refs/heads/HEAD")
	checkRef.Dir = tempRepo
	if err := checkRef.Run(); err == nil {
		t.Fatalf("SECURITY/CORRECTNESS VIOLATION: refs/heads/HEAD was created! Literal 'HEAD' branch exists.")
	}

	// 2. Squash merge with failing pre-commit hook must not leave changes staged
	sw2, err := mgr.CreateShadow("task-squash-hook", "main")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(sw2.Path, "squash_hook.txt"), []byte("staged data\n"), 0644)
	_, _ = sw2.RecordCheckpoint(1, "feat", true)

	// Failing pre-commit hook
	hooksDir := filepath.Join(tempRepo, ".git", "hooks")
	_ = os.MkdirAll(hooksDir, 0755)
	_ = os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755)

	_, _ = mgr.MergeInto(sw2, "main", true)

	// Check git status --porcelain: index must be completely clean (no staged files)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = tempRepo
	out, _ := statusCmd.Output()
	if strings.Contains(string(out), "squash_hook.txt") {
		t.Fatalf("CORRECTNESS VIOLATION: squash merge with failing pre-commit left changes staged: %s", string(out))
	}
}


