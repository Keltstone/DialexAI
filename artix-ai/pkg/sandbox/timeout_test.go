package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTimeout_KillsEntireProcessGroup_NoSurvivor(t *testing.T) {
	box := NewSandbox(t.TempDir())

	// Command that spawns a background grandchild process that would survive if only parent was killed.
	// The grandchild writes its pid to a file, then sleeps.
	tmpDir := t.TempDir()
	pidFile := tmpDir + "/child.pid"
	cmdStr := fmt.Sprintf("sh -c '(sleep 300) & echo $! > %s; sleep 300'", pidFile)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	res := box.Run(ctx, cmdStr, &ExecOptions{
		Timeout: 200 * time.Millisecond,
	})

	if !res.TimedOut {
		t.Fatalf("expected command to time out, got: %+v", res)
	}

	// Read child PID if created
	out, err := exec.Command("pgrep", "-f", "sleep 300").CombinedOutput()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		t.Fatalf("process group survivor detected! pgrep found: %s", string(out))
	}
}
