//go:build windows

package trust

import "os"

func checkOwner(fi os.FileInfo, trustPath string) error {
	return nil
}
