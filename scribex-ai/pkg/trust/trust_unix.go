//go:build !windows

package trust

import (
	"fmt"
	"os"
	"syscall"
)

func checkOwner(fi os.FileInfo, trustPath string) error {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		if stat.Uid != uint32(os.Getuid()) {
			return fmt.Errorf("trust file %s is not owned by current user (uid %d != %d)", trustPath, stat.Uid, os.Getuid())
		}
	}
	return nil
}
