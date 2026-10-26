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

// Join returns root/rel after proving the result, with symlinks resolved, stays inside root.
// rel may not exist yet (writes); the deepest existing ancestor is resolved instead.
func Join(root, rel string) (string, error) {
	if rel == "" || len(rel) > maxLen {
		return "", errors.New("safepath: empty or oversized path")
	}
	if strings.ContainsRune(rel, 0) {
		return "", errors.New("safepath: NUL in path")
	}
	if strings.HasPrefix(strings.ToLower(rel), "file:") {
		return "", errors.New("safepath: URL scheme not allowed")
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", fmt.Errorf("safepath: absolute path %q not allowed", rel)
	}
	for _, c := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if c == ".." {
			return "", fmt.Errorf("safepath: %q escapes root", rel)
		}
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
