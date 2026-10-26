// Package secrets keeps local secrets in .scribex/secrets.env (0600, gitignored). CI uses real env vars instead.
package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"scribex/pkg/config"
	"scribex/pkg/fsx"
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
	b, _ := os.ReadFile(filepath.Join(dir, "secrets.env"))
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok && config.EnvName.MatchString(k) {
			m[k] = v
		}
	}
	return m
}

// Load exports file secrets into the environment; real env vars win.
func Load(root string) {
	for k, v := range read(root) {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func Set(root, name, value string) error {
	dir, err := safepath.ScribexDir(root, true)
	if err != nil {
		return err
	}
	if !config.EnvName.MatchString(name) {
		return fmt.Errorf("name must be UPPER_SNAKE_CASE")
	}
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("value must be one non-empty line")
	}
	m := read(root)
	m[name] = value
	var b strings.Builder
	for _, k := range Names(root, m) {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	target := filepath.Join(dir, "secrets.env")
	if err := fsx.WriteAtomic(target, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return ignore(root)
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

func ignore(root string) error {
	p := filepath.Join(root, ".gitignore")
	b, _ := os.ReadFile(p)
	lines := strings.Split(string(b), "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) == ".scribex/secrets.env" {
			return nil
		}
	}
	content := string(b)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += ".scribex/secrets.env\n"
	return os.WriteFile(p, []byte(content), 0o644)
}
