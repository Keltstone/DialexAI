//go:build windows

package coder

import "os"

func lockFile(f *os.File, lockPath string) (func(), error) {
	return func() {}, nil
}

func validateFileOwnership(info os.FileInfo, path string) error {
	return nil
}
