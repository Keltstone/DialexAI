//go:build windows

package policy

import "os"

func isFileRootOwned(info os.FileInfo) bool {
	return false
}
