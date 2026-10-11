package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"artix/pkg/coder"
	"artix/pkg/sandbox"
)

func runReplay(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Error: task-id argument is required for artix replay\n")
		return 1
	}

	taskID := args[0]
	targetRound := -1

	for i := 1; i < len(args); i++ {
		if args[i] == "--round" && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				targetRound = n
			}
			i++
		}
	}

	traces, err := coder.ReadRoundTraces(cwd, taskID)
	if err != nil || len(traces) == 0 {
		fmt.Fprintf(stderr, "Error: no round traces found for task %s\n", taskID)
		return 1
	}

	var selectedTrace *coder.RoundTrace
	for i := range traces {
		if targetRound <= 0 || traces[i].Round == targetRound {
			selectedTrace = &traces[i]
		}
	}

	if selectedTrace == nil {
		fmt.Fprintf(stderr, "Error: round %d not found for task %s\n", targetRound, taskID)
		return 1
	}

	// Create a fresh worktree directory from cwd
	tmpDir, err := os.MkdirTemp("", "artix-replay-*")
	if err != nil {
		fmt.Fprintf(stderr, "Error creating temp worktree: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmpDir)

	// Clone or copy repository into fresh worktree
	cloneCmd := exec.Command("git", "clone", "--depth", "1", "file://"+cwd, tmpDir)
	if _, err := cloneCmd.CombinedOutput(); err != nil {
		cmdCopy := exec.Command("cp", "-R", cwd+"/.", tmpDir)
		_ = cmdCopy.Run()
	}

	// Apply patch if present
	if selectedTrace.Patch != "" {
		patchFile := filepath.Join(tmpDir, ".replay.patch")
		_ = os.WriteFile(patchFile, []byte(selectedTrace.Patch), 0644)

		applyCmd := exec.Command("git", "apply", "--whitespace=nowarn", patchFile)
		applyCmd.Dir = tmpDir
		if _, err := applyCmd.CombinedOutput(); err != nil {
			pCmd := exec.Command("patch", "-p1", "-i", patchFile)
			pCmd.Dir = tmpDir
			_ = pCmd.Run()
		}
		_ = os.Remove(patchFile)
	}

	// Run recorded sandbox command
	box := sandbox.NewSandbox(tmpDir)
	res := box.Run(context.Background(), selectedTrace.SandboxCommand, &sandbox.ExecOptions{
		Cwd: tmpDir,
	})

	if res.Stdout != "" {
		fmt.Fprintf(human, "%s", res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprintf(stderr, "%s", res.Stderr)
	}

	return res.ExitCode
}
