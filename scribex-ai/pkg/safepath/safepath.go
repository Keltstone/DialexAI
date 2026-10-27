// Package safepath confines untrusted relative paths (anchors, manifest page ids, wiki links) to a root directory.
package safepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxLen = 4096

var pageID = regexp.MustCompile(`^[0-9]{1,20}$`)

// ValidPageID reports whether s is a plain numeric Confluence page id.
func ValidPageID(s string) bool { return pageID.MatchString(s) }

// CheckRel performs pure lexical checks on rel: no abs, no .., no NUL, no file:, length <= 4096.
func CheckRel(rel string) error {
	if rel == "" || len(rel) > maxLen {
		return errors.New("safepath: empty or oversized path")
	}
	if strings.ContainsRune(rel, 0) {
		return errors.New("safepath: NUL in path")
	}
	if strings.HasPrefix(strings.ToLower(rel), "file:") {
		return errors.New("safepath: URL scheme not allowed")
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return fmt.Errorf("safepath: absolute path %q not allowed", rel)
	}
	for _, c := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if c == ".." {
			return fmt.Errorf("safepath: %q escapes root", rel)
		}
	}
	return nil
}

// Join returns root/rel after proving the result, with symlinks resolved, stays inside root.
// rel may not exist yet (writes); the deepest existing ancestor is resolved instead.
func Join(root, rel string) (string, error) {
	if err := CheckRel(rel); err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	got, err := resolve(filepath.Join(realRoot, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/"))))
	if err != nil {
		return "", err
	}
	if r, err := filepath.Rel(realRoot, got); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("safepath: %q resolves outside root", rel)
	}
	return got, nil
}

const DirName = ".scribex"

var ErrScribexDir = errors.New(".scribex must be a real directory inside the repository")

// ScribexDir returns the absolute path of <root>/.scribex after proving: it is not a symlink (Lstat),
// it resolves inside root (EvalSymlinks + Rel), and if it exists it is a directory.
// create=true makes it (0o755) when missing.
func ScribexDir(root string, create bool) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}

	target := filepath.Join(root, DirName)
	fi, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			if !create {
				return "", os.ErrNotExist
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			fi, err = os.Lstat(target)
			if err != nil {
				return "", err
			}
		} else {
			return "", ErrScribexDir
		}
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		return "", ErrScribexDir
	}
	if !fi.IsDir() {
		return "", ErrScribexDir
	}

	realDir, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", ErrScribexDir
	}
	rel, err := filepath.Rel(realRoot, realDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrScribexDir
	}

	return filepath.Abs(target)
}

// resolve is EvalSymlinks that tolerates a missing tail.
func resolve(p string) (string, error) {
	r, err := filepath.EvalSymlinks(p)
	if err == nil {
		return r, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	fi, lerr := os.Lstat(p)
	if lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, rerr := os.Readlink(p)
		if rerr == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(p), target)
			}
			return resolve(target)
		}
	}
	parent, err := resolve(filepath.Dir(p))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(p)), nil
}
