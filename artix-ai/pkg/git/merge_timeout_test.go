package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeInto_CancelledContext_LeavesRepoClean(t *testing.T) {
	repoDir := t.TempDir()

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cmd %s %v failed: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	runCmd(repoDir, "git", "init", "-b", "main")
	runCmd(repoDir, "git", "config", "user.name", "Tester")
	runCmd(repoDir, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".artix/\n"), 0644)
	runCmd(repoDir, "git", "add", ".gitignore")
	_ = os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("initial\n"), 0644)
	runCmd(repoDir, "git", "add", "file.txt")
	runCmd(repoDir, "git", "commit", "-m", "init")

	mgr := NewWorktreeManager(repoDir)
	sw, err := mgr.CreateShadow("test-task", "main")
	if err != nil {
		t.Fatalf("failed to create shadow worktree: %v", err)
	}

	// Add conflicting / long work in shadow worktree
	_ = os.WriteFile(filepath.Join(sw.Path, "file.txt"), []byte("shadow branch changes\n"), 0644)
	runCmd(sw.Path, "git", "commit", "-am", "shadow change")

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = mgr.MergeIntoContext(ctx, sw, "main", false)
	if err == nil {
		t.Fatalf("expected MergeIntoContext with cancelled context to return error")
	}

	// Verify repo is clean (no half-merged state)
	d := NewDriver(repoDir)
	diff, _ := d.Diff(false)
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("repo left with dirty diff after cancelled merge: %s", diff)
	}

	statusOut, _ := d.Status()
	if statusOut != nil && !statusOut.IsClean {
		t.Fatalf("repo left in dirty state! Status: %+v", statusOut)
	}
}
