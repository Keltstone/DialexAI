// Package secrets keeps local secrets in .scribex/secrets.env (0600, gitignored). CI uses real env vars instead.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scribex/pkg/config"
	"scribex/pkg/fsx"
	"scribex/pkg/gitx"
	"scribex/pkg/safepath"
)

func File(root string) string {
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		dir = filepath.Join(root, safepath.DirName)
	}
	return filepath.Join(dir, "secrets.env")
}

func read(root string) map[string]string {
	m := map[string]string{}
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		return m
	}
	p := filepath.Join(dir, "secrets.env")
	fi, err := os.Stat(p)
	if err != nil {
		return m
	}
	if fi.Mode().Perm()&0o077 != 0 && os.Getenv("CI") == "true" {
		return m
	}
	b, _ := os.ReadFile(p)
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok && config.EnvName.MatchString(k) {
			m[k] = v
		}
	}
	return m
}

// CheckPerms checks if secrets.env exists and is group/world-readable.
// If so, it prints a warning to stderr and returns false.
func CheckPerms(root string) bool {
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		return true
	}
	p := filepath.Join(dir, "secrets.env")
	fi, err := os.Stat(p)
	if err != nil {
		return true
	}
	if fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "warning: %s is group/world-readable (mode %04o); run chmod 600 %s\n", p, fi.Mode().Perm(), p)
		return false
	}
	return true
}

// Load exports file secrets into the environment; real env vars win.
func Load(root string) {
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		return
	}
	p := filepath.Join(dir, "secrets.env")
	fi, err := os.Stat(p)
	if err != nil {
		return
	}
	if fi.Mode().Perm()&0o077 != 0 && os.Getenv("CI") == "true" {
		return
	}
	for k, v := range read(root) {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func Set(root, name, value string) error {
	_, err := safepath.ScribexDir(root, true)
	if err != nil {
		return err
	}
	if !config.EnvName.MatchString(name) {
		return fmt.Errorf("name must be UPPER_SNAKE_CASE")
	}
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("value must be one non-empty line")
	}

	if err := ensureIgnored(root); err != nil {
		return err
	}

	m := read(root)
	m[name] = value
	var b strings.Builder
	for _, k := range Names(root, m) {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	return fsx.WriteFileIn(root, filepath.Join(safepath.DirName, "secrets.env"), []byte(b.String()), 0o600)
}

// Names lists stored secret names (sorted); m may be nil.
func Names(root string, m map[string]string) []string {
	if m == nil {
		m = read(root)
	}
	var ns []string
	for k := range m {
		ns = append(ns, k)
	}
	sort.Strings(ns)
	return ns
}

func ensureIgnored(root string) error {
	isGit := false
	if _, err := gitx.Run(root, "rev-parse", "--git-dir"); err == nil {
		isGit = true
		if _, err := gitx.Run(root, "check-ignore", "-q", "--", ".scribex/secrets.env"); err == nil {
			return nil
		}
	}

	gitIgnorePath := filepath.Join(root, ".gitignore")
	if fi, err := os.Lstat(gitIgnorePath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return errors.New("refusing to modify symlinked .gitignore")
		}
	}

	b, err := os.ReadFile(gitIgnorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(b)
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == ".scribex/secrets.env" {
			return nil
		}
	}

	eol := "\n"
	if strings.Count(content, "\r\n")*2 > strings.Count(content, "\n") {
		eol = "\r\n"
	}

	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += eol
	}
	content += ".scribex/secrets.env" + eol

	if err := fsx.WriteFileIn(root, ".gitignore", []byte(content), 0o644); err != nil {
		if !isGit && os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}
