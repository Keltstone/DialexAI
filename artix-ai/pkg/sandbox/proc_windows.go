//go:build windows

package sandbox

import (
	"os/exec"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	cmd.WaitDelay = time.Second
}
