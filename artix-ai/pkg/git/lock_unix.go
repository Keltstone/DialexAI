//go:build !windows

package git

import (
	"os"
	"syscall"
)

func lockFile(f *os.File) func() {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
}
