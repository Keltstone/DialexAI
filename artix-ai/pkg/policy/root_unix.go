//go:build !windows

package policy

import (
	"os"
	"syscall"
)

func isFileRootOwned(info os.FileInfo) bool {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Uid == 0
	}
	return false
}
