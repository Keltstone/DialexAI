// Package fsx holds filesystem helpers shared by every command that writes files.
package fsx

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"scribex/pkg/safepath"
)

// WriteAtomic writes data to path via temp file + fsync + rename in the same directory, so a crash
// never leaves a truncated file. An existing file keeps its mode; otherwise perm is used.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fsx: symlink not allowed: %s", path)
		}
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	if runtime.GOOS != "windows" {
		if d, err := os.Open(dir); err == nil {
			_ = d.Sync()
			_ = d.Close()
		}
	}
	return nil
}

// WriteFileIn writes data to root/rel atomically. It refuses: a rel that fails safepath.Join; a final component that is a symlink
// (Lstat) — we never replace or follow links; an existing target that is not a regular file; an existing target without the owner write bit (read-only means read-only).
// The parent directory is created (0o755) only after the safepath check passed. Mode: existing file's mode, else perm.
func WriteFileIn(root, rel string, data []byte, perm os.FileMode) error {
	target, err := safepath.Join(root, rel)
	if err != nil {
		return err
	}
	raw := filepath.Join(root, filepath.FromSlash(rel))
	if fi, err := os.Lstat(raw); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fsx: refusing to write to symlink %s", rel)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("fsx: refusing to write to non-regular file %s", rel)
		}
		if fi.Mode().Perm()&0o200 == 0 {
			return fmt.Errorf("fsx: refusing to write to read-only file %s", rel)
		}
		perm = fi.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	fi, err := os.Lstat(target)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fsx: refusing to write to symlink %s", rel)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("fsx: refusing to write to non-regular file %s", rel)
		}
		if fi.Mode().Perm()&0o200 == 0 {
			return fmt.Errorf("fsx: refusing to write to read-only file %s", rel)
		}
		perm = fi.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return WriteAtomic(target, data, perm)
}
