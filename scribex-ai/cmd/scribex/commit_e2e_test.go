package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scribex/pkg/gitx"
)

func setupSyncRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if _, err := gitx.Run(repo, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.Run(repo, "config", "user.name", "Sync Tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.Run(repo, "config", "user.email", "sync@example.com"); err != nil {
		t.Fatal(err)
	}

	// Code file
	code := "package main\n\nfunc Hello() string { return \"hello\" }\n"
	_ = os.WriteFile(filepath.Join(repo, "main.go"), []byte(code), 0o644)

	// Doc file
	doc := "# Doc\n\n<!-- @scribex:anchor(main.go#Hello) -->\n"
	_ = os.WriteFile(filepath.Join(repo, "doc.md"), []byte(doc), 0o644)

	if _, err := gitx.Run(repo, "add", "main.go", "doc.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.Run(repo, "commit", "-m", "initial commit"); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestCommitE2E_SafeSyncCommit(t *testing.T) {
	// 1. Happy path + second run nothing to commit + no amend
	t.Run("happy_path_and_second_run", func(t *testing.T) {
		repo := setupSyncRepo(t)
		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("sync -commit failed: %v, out: %s", err, string(out))
		}

		// Doc was committed
		headShow, _ := gitx.Run(repo, "show", "--stat", "HEAD")
		if !strings.Contains(string(headShow), "doc.md") {
			t.Fatalf("expected doc.md in commit, got:\n%s", string(headShow))
		}

		// Second run: nothing to commit, no new commit
		headBefore, _ := gitx.Run(repo, "rev-parse", "HEAD")
		c2 := exec.Command(binaryPath, "sync", "-commit")
		c2.Dir = repo
		out2, err := c2.CombinedOutput()
		if err != nil {
			t.Fatalf("second sync failed: %v, out: %s", err, string(out2))
		}
		if !strings.Contains(string(out2), "nothing to commit") {
			t.Fatalf("expected 'nothing to commit', got: %s", string(out2))
		}
		headAfter, _ := gitx.Run(repo, "rev-parse", "HEAD")
		if strings.TrimSpace(string(headBefore)) != strings.TrimSpace(string(headAfter)) {
			t.Fatalf("HEAD changed on second run: %s != %s", headBefore, headAfter)
		}
	})

	// 2. Unrelated staged wip.md stays staged and is NOT in commit
	t.Run("staged_wip_stays_staged", func(t *testing.T) {
		repo := setupSyncRepo(t)
		_ = os.WriteFile(filepath.Join(repo, "wip.md"), []byte("work in progress"), 0o644)
		if _, err := gitx.Run(repo, "add", "wip.md"); err != nil {
			t.Fatal(err)
		}

		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("sync -commit failed: %v, out: %s", err, string(out))
		}

		// Verify wip.md is still staged
		cached, _ := gitx.Run(repo, "diff", "--cached", "--name-only")
		if !strings.Contains(string(cached), "wip.md") {
			t.Fatalf("wip.md should remain staged, got cached: %s", string(cached))
		}

		// Verify wip.md is NOT in HEAD commit
		showStat, _ := gitx.Run(repo, "show", "--stat", "HEAD")
		if strings.Contains(string(showStat), "wip.md") {
			t.Fatalf("wip.md was erroneously committed in HEAD:\n%s", string(showStat))
		}
		if !strings.Contains(string(showStat), "doc.md") {
			t.Fatalf("doc.md was not committed in HEAD:\n%s", string(showStat))
		}
	})

	// 3. Unrelated unstaged edit in another .md stays unstaged and uncommitted
	t.Run("unstaged_edit_stays_unstaged", func(t *testing.T) {
		repo := setupSyncRepo(t)
		_ = os.WriteFile(filepath.Join(repo, "other.md"), []byte("# Other\ninitial"), 0o644)
		_, _ = gitx.Run(repo, "add", "other.md")
		_, _ = gitx.Run(repo, "commit", "-m", "add other.md")

		// Modify other.md unstaged
		_ = os.WriteFile(filepath.Join(repo, "other.md"), []byte("# Other\nmodified unstaged"), 0o644)

		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("sync -commit failed: %v, out: %s", err, string(out))
		}

		// other.md must still have unstaged diff
		diff, _ := gitx.Run(repo, "diff", "--name-only")
		if !strings.Contains(string(diff), "other.md") {
			t.Fatalf("other.md unstaged changes were lost or committed")
		}
		showHead, _ := gitx.Run(repo, "show", "--stat", "HEAD")
		if strings.Contains(string(showHead), "other.md") {
			t.Fatalf("other.md was unexpectedly committed in HEAD")
		}
	})

	// 4. Pre-dirty doc that sync also changes -> refusal, doc left as synced on disk, no commit
	t.Run("pre_dirty_doc_refused", func(t *testing.T) {
		repo := setupSyncRepo(t)
		// Modify doc.md without staging
		_ = os.WriteFile(filepath.Join(repo, "doc.md"), []byte("# Doc\nUnstaged pre-dirty edit\n<!-- @scribex:anchor(main.go#Hello) -->\n"), 0o644)

		headBefore, _ := gitx.Run(repo, "rev-parse", "HEAD")
		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err == nil {
			t.Fatalf("expected refusal when doc had pre-existing uncommitted changes")
		}
		expectedErr := "had uncommitted changes before sync"
		if !strings.Contains(string(out), expectedErr) {
			t.Fatalf("expected %q in error output, got: %s", expectedErr, string(out))
		}

		// No commit created
		headAfter, _ := gitx.Run(repo, "rev-parse", "HEAD")
		if strings.TrimSpace(string(headBefore)) != strings.TrimSpace(string(headAfter)) {
			t.Fatalf("commit was created despite pre-dirty refusal")
		}

		// Doc was left synced on disk
		content, _ := os.ReadFile(filepath.Join(repo, "doc.md"))
		if !strings.Contains(string(content), "sig=") {
			t.Fatalf("doc was not updated on disk: %s", string(content))
		}
	})

	// 5. Detached HEAD refusal
	t.Run("detached_head_refusal", func(t *testing.T) {
		repo := setupSyncRepo(t)
		head, _ := gitx.Run(repo, "rev-parse", "HEAD")
		_, _ = gitx.Run(repo, "checkout", strings.TrimSpace(string(head)))

		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error on detached HEAD")
		}
		if !strings.Contains(string(out), "refusing to commit on detached HEAD") {
			t.Fatalf("expected 'refusing to commit on detached HEAD', got: %s", string(out))
		}
	})

	// 6. Missing git refusal with rc != 0
	t.Run("git_missing_refusal", func(t *testing.T) {
		repo := setupSyncRepo(t)
		emptyDir := t.TempDir()
		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		c.Env = []string{"PATH=" + emptyDir}
		out, err := c.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error when git is missing, got rc 0")
		}
		if !strings.Contains(string(out), "git not found") {
			t.Fatalf("expected 'git not found' in output, got: %s", string(out))
		}
	})

	// 7. Missing identity refusal and env identity works
	t.Run("identity_refusal_and_env", func(t *testing.T) {
		repo := setupSyncRepo(t)
		// Unset user.name and user.email in local config
		_, _ = gitx.Run(repo, "config", "--unset", "user.name")
		_, _ = gitx.Run(repo, "config", "--unset", "user.email")

		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		// Neutralize global git config by setting HOME to an empty temp dir
		emptyHome := t.TempDir()
		c.Env = append(os.Environ(), "HOME="+emptyHome, "SCRIBEX_GIT_NAME=", "SCRIBEX_GIT_EMAIL=")
		out, err := c.CombinedOutput()
		if err == nil {
			t.Fatalf("expected missing identity refusal, got nil")
		}
		if !strings.Contains(string(out), "git identity not configured") {
			t.Fatalf("expected 'git identity not configured' in error, got: %s", string(out))
		}

		// Reset doc.md so it needs syncing
		_ = os.WriteFile(filepath.Join(repo, "doc.md"), []byte("# Doc\n\n<!-- @scribex:anchor(main.go#Hello) -->\n"), 0o644)

		// Now supply identity via env
		c2 := exec.Command(binaryPath, "sync", "-commit")
		c2.Dir = repo
		c2.Env = append(os.Environ(),
			"HOME="+emptyHome,
			"SCRIBEX_GIT_NAME=Env Author",
			"SCRIBEX_GIT_EMAIL=env@example.com",
		)
		out2, err := c2.CombinedOutput()
		if err != nil {
			t.Fatalf("sync -commit with env identity failed: %v, out: %s", err, string(out2))
		}

		// Verify author in commit
		author, _ := gitx.Run(repo, "log", "-1", "--format=%an <%ae>")
		if strings.TrimSpace(string(author)) != "Env Author <env@example.com>" {
			t.Fatalf("expected author 'Env Author <env@example.com>', got: %q", string(author))
		}
	})

	// 8. Filename with glob characters ([a]*.md) committed correctly
	t.Run("glob_filename_committed", func(t *testing.T) {
		repo := setupSyncRepo(t)
		globFile := "[a]test.md"
		_ = os.WriteFile(filepath.Join(repo, globFile), []byte("# Glob\n<!-- @scribex:anchor(main.go#Hello) -->\n"), 0o644)
		_, _ = gitx.Run(repo, "add", "--literal-pathspecs", globFile)
		_, _ = gitx.Run(repo, "commit", "-m", "add glob file")

		// Touch main.go signature
		_ = os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc Hello(name string) string { return \"hello \" + name }\n"), 0o644)

		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("sync -commit failed on glob filename: %v, out: %s", err, string(out))
		}

		showStat, _ := gitx.Run(repo, "show", "--stat", "HEAD")
		if !strings.Contains(string(showStat), "[a]test.md") {
			t.Fatalf("glob file [a]test.md was not committed in HEAD:\n%s", string(showStat))
		}
	})

	// 9. Dead anchor present -> no commit, rc != 0
	t.Run("dead_anchor_refusal", func(t *testing.T) {
		repo := setupSyncRepo(t)
		_ = os.WriteFile(filepath.Join(repo, "dead.md"), []byte("# Dead\n<!-- @scribex:anchor(main.go#NoSuchSymbol) -->\n"), 0o644)
		_, _ = gitx.Run(repo, "add", "dead.md")
		_, _ = gitx.Run(repo, "commit", "-m", "add dead.md")

		headBefore, _ := gitx.Run(repo, "rev-parse", "HEAD")
		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error when dead anchor is present, got nil")
		}
		if !strings.Contains(string(out), "dead anchor") {
			t.Fatalf("expected 'dead anchor' in output, got: %s", string(out))
		}
		headAfter, _ := gitx.Run(repo, "rev-parse", "HEAD")
		if strings.TrimSpace(string(headBefore)) != strings.TrimSpace(string(headAfter)) {
			t.Fatalf("commit was created despite dead anchor")
		}
	})

	// 10. Hostile hooks and fsmonitor: marker absent
	t.Run("hostile_hooks_neutralised", func(t *testing.T) {
		repo := setupSyncRepo(t)
		marker := filepath.Join(t.TempDir(), "hook_ran_marker")

		// Create hostile hook script
		hookScript := filepath.Join(repo, "evil_hook.sh")
		scriptContent := "#!/bin/sh\ntouch " + marker + "\n"
		_ = os.WriteFile(hookScript, []byte(scriptContent), 0o755)

		// Set core.hooksPath and core.fsmonitor in .git/config
		_, _ = gitx.Run(repo, "config", "core.hooksPath", repo)
		_, _ = gitx.Run(repo, "config", "core.fsmonitor", hookScript)

		// Also place in .git/hooks/pre-commit
		_ = os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755)
		_ = os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte(scriptContent), 0o755)

		c := exec.Command(binaryPath, "sync", "-commit")
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("sync -commit failed: %v, out: %s", err, string(out))
		}

		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("hostile git hook was executed: marker file %s exists", marker)
		}
	})
}
