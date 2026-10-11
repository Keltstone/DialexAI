//go:build !windows

package knowledge

import (
	"fmt"
	"os"
	"syscall"
)

func lockFile(f *os.File, lockPath string) (func(), error) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, fmt.Errorf("failed to acquire flock on %s: %w", lockPath, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}, nil
}
