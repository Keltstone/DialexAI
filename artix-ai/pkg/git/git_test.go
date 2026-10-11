package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestGitRepo(t *testing.T) (string, *Driver) {
	tempDir, err := os.MkdirTemp("", "kritix_git_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", tempDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user for commits
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@kritix.ai").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Kritix Test").Run()

	// Create initial file & commit
	initialFile := filepath.Join(tempDir, "README.md")
	_ = os.WriteFile(initialFile, []byte("# Test Project\nInitial content\n"), 0644)

	driver := NewDriver(tempDir)
	_, err = driver.CommitAll("chore: initial commit")
	if err != nil {
		t.Fatalf("initial commit failed: %v", err)
	}

	return tempDir, driver
}

func TestGitDriverOperations(t *testing.T) {
	dir, driver := setupTestGitRepo(t)
	defer os.RemoveAll(dir)

	// 1. Test Status
	status, err := driver.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !status.IsClean {
		t.Errorf("expected clean status after commit")
	}

	// 2. Test Branching
	branchName, err := driver.CreateWorkingBranch("add-auth")
	if err != nil {
		t.Fatalf("CreateWorkingBranch failed: %v", err)
	}
	if !strings.HasPrefix(branchName, "artix/add-auth-") {
		t.Errorf("unexpected branch name: %s", branchName)
	}

	// 3. Test Modifying and Diff
	file := filepath.Join(dir, "README.md")
	_ = os.WriteFile(file, []byte("# Test Project\nModified content\n"), 0644)

	diff, err := driver.Diff(false)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "Modified content") {
		t.Errorf("diff does not contain expected change")
	}

	// 4. Test Commit
	commitHash, err := driver.CommitAll("feat: update readme")
	if err != nil {
		t.Fatalf("CommitAll failed: %v", err)
	}
	if commitHash == "" {
		t.Errorf("expected non-empty commit hash")
	}
}

func TestPatchSessionApplyAndRollback(t *testing.T) {
	dir, driver := setupTestGitRepo(t)
	defer os.RemoveAll(dir)

	originalContent := "# Test Project\nInitial content\n"

	// Create a patch that changes line 2
	patch := `--- a/README.md
+++ b/README.md
@@ -1,2 +1,2 @@
 # Test Project
-Initial content
+Patched content
`

	session := NewPatchSession(dir, patch)

	// 1. Test Apply
	if err := session.Apply(); err != nil {
		t.Fatalf("session.Apply failed: %v", err)
	}

	file := filepath.Join(dir, "README.md")
	readBack, _ := os.ReadFile(file)
	if !strings.Contains(string(readBack), "Patched content") {
		t.Errorf("expected file to have patched content")
	}

	// Check git diff confirms patch
	diff, _ := driver.Diff(false)
	if !strings.Contains(diff, "Patched content") {
		t.Errorf("expected git diff to reflect patch")
	}

	// 2. Test Rollback
	if err := session.Rollback(); err != nil {
		t.Fatalf("session.Rollback failed: %v", err)
	}

	readBackAfterRollback, _ := os.ReadFile(file)
	if string(readBackAfterRollback) != originalContent {
		t.Errorf("expected file to be restored to %q, got %q", originalContent, string(readBackAfterRollback))
	}

	status, _ := driver.Status()
	if !status.IsClean {
		t.Errorf("expected working tree to be clean after rollback")
	}
}

func TestPatchSession_CannotLandPatchWritingGitHooks(t *testing.T) {
	dir, driver := setupTestGitRepo(t)
	defer os.RemoveAll(dir)

	// Ensure .git/hooks directory exists
	hooksDir := filepath.Join(dir, ".git", "hooks")
	_ = os.MkdirAll(hooksDir, 0755)

	canaryPath := filepath.Join(dir, "hook_canary.txt")
	defer os.Remove(canaryPath)

	// Patch that attempts to write a pre-commit hook into .git/hooks/pre-commit
	maliciousPatch := `--- /dev/null
+++ b/.git/hooks/pre-commit
@@ -0,0 +1,3 @@
+#!/bin/sh
+echo HOOK_PWNED > hook_canary.txt
+exit 0
`

	session := NewPatchSession(dir, maliciousPatch)

	// Applying this patch MUST fail
	err := session.Apply()
	if err == nil {
		t.Fatalf("session.Apply must reject patches writing to .git/hooks, but succeeded")
	}

	// Verify hook file does not exist
	hookFile := filepath.Join(hooksDir, "pre-commit")
	if _, err := os.Stat(hookFile); err == nil {
		t.Fatalf(".git/hooks/pre-commit was written by patch session")
	}

	// Commit a new file to verify hook is not executed
	testFile := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(testFile, []byte("content"), 0644)
	if _, err := driver.CommitAll("another commit"); err != nil {
		t.Fatalf("driver.CommitAll failed: %v", err)
	}

	if _, err := os.Stat(canaryPath); err == nil {
		t.Fatalf("malicious pre-commit hook ran during commit!")
	}
}

