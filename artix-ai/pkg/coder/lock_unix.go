//go:build !windows

package coder

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

func validateFileOwnership(info os.FileInfo, path string) error {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != os.Getuid() {
			return fmt.Errorf("ledger file %s owned by uid %d, expected current user uid %d (refusing untrusted ledger)", path, stat.Uid, os.Getuid())
		}
	}
	return nil
}
