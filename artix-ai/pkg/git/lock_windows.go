//go:build windows

package git

import "os"

func lockFile(f *os.File) func() {
	return func() {
		_ = f.Close()
	}
}
