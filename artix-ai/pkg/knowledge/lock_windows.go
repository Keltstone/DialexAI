//go:build windows

package knowledge

import "os"

func lockFile(f *os.File, lockPath string) (func(), error) {
	return func() {}, nil
}
