package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Driver encapsulates git operations for a workspace.
type Driver struct {
	repoDir string
}

// NewDriver creates a git driver bound to repoDir.
func NewDriver(repoDir string) *Driver {
	return &Driver{repoDir: repoDir}
}

// RepoDir returns the repository directory path bound to this driver.
func (d *Driver) RepoDir() string {
	if d == nil {
		return ""
	}
	return d.repoDir
}

// WorkDir returns the repository directory path bound to this driver.
func (d *Driver) WorkDir() string {
	return d.RepoDir()
}


// GitStatusResult describes the git working tree status.
type GitStatusResult struct {
	Branch         string   `json:"branch"`
	HeadCommit     string   `json:"headCommit"`
	IsClean        bool     `json:"isClean"`
	ModifiedFiles  []string `json:"modifiedFiles"`
	StagedFiles    []string `json:"stagedFiles"`
	UntrackedFiles []string `json:"untrackedFiles"`
}

// Status returns the detailed git status of the working tree.
func (d *Driver) Status() (*GitStatusResult, error) {
	res := &GitStatusResult{
		IsClean:        true,
		ModifiedFiles:  make([]string, 0),
		StagedFiles:    make([]string, 0),
		UntrackedFiles: make([]string, 0),
	}

	branch, _ := d.runGit("rev-parse", "--abbrev-ref", "HEAD")
	res.Branch = strings.TrimSpace(branch)

	head, _ := d.runGit("rev-parse", "--short", "HEAD")
	res.HeadCommit = strings.TrimSpace(head)

	out, err := d.runGit("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		res.IsClean = false
		code := line[:2]
		file := strings.TrimSpace(line[3:])

		if strings.HasPrefix(code, "??") {
			res.UntrackedFiles = append(res.UntrackedFiles, file)
		} else {
			if code[0] != ' ' && code[0] != '?' {
				res.StagedFiles = append(res.StagedFiles, file)
			}
			if code[1] != ' ' && code[1] != '?' {
				res.ModifiedFiles = append(res.ModifiedFiles, file)
			}
		}
	}

	return res, nil
}

// Diff generates a unified diff of current changes.
func (d *Driver) Diff(stagedOnly bool, paths ...string) (string, error) {
	args := []string{"diff"}
	if stagedOnly {
		args = append(args, "--staged")
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	return d.runGit(args...)
}

// CreateBranch creates and checks out a new branch.
func (d *Driver) CreateBranch(name string) error {
	_, err := d.runGit("checkout", "-b", name)
	return err
}

// Checkout switches to an existing branch.
func (d *Driver) Checkout(name string) error {
	_, err := d.runGit("checkout", name)
	return err
}

// Commit creates a git commit for currently staged files.
func (d *Driver) Commit(message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}
	_, err := d.runGit("commit", "-m", message)
	if err != nil {
		return "", err
	}
	return d.HeadHash()
}

// CommitAll stages all changes and creates a commit.
func (d *Driver) CommitAll(message string) (string, error) {
	if _, err := d.runGit("add", "-A"); err != nil {
		return "", fmt.Errorf("git add failed: %w", err)
	}
	return d.Commit(message)
}

// CreateWorkingBranch generates a standardized task branch name and checks it out.
func (d *Driver) CreateWorkingBranch(taskID string) (string, error) {
	cleanID := strings.ToLower(strings.ReplaceAll(taskID, " ", "-"))
	branchName := fmt.Sprintf("artix/%s-%d", cleanID, time.Now().Unix())
	if err := d.CreateBranch(branchName); err != nil {
		return "", err
	}
	return branchName, nil
}

// HeadHash returns the full commit hash of HEAD.
func (d *Driver) HeadHash() (string, error) {
	out, err := d.runGit("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ResetHard resets the working tree and index to ref.
func (d *Driver) ResetHard(ref string) error {
	_, err := d.runGit("reset", "--hard", ref)
	return err
}

// Push pushes refspec to the specified remote.
func (d *Driver) Push(remote, refspec string) (string, error) {
	return d.runGit("push", remote, refspec)
}

// DeleteRemoteBranch deletes a branch on the remote.
func (d *Driver) DeleteRemoteBranch(remote, branch string) (string, error) {
	return d.runGit("push", remote, "--delete", branch)
}

func (d *Driver) runGit(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		stdout.Reset()
		stderr.Reset()
		cmd := exec.Command("git", append([]string{"-C", d.repoDir}, args...)...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err == nil {
			return stdout.String(), nil
		}
		if strings.Contains(stderr.String(), "index.lock") {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		break
	}

	return "", fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
}
