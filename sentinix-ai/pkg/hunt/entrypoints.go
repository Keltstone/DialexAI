// Package hunt is the opt-in LLM audit engine: deterministic entry-point discovery, then one small
// model call per entry point and vulnerability class. Output is advisory (never blocks).
package hunt

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Entry struct {
	Kinds []string // component | deeplink | route
	Name  string
	File  string // slash path relative to root
}

var (
	exportedRe = regexp.MustCompile(`<(?:activity|service|receiver|provider)[^>]*android:name="\.?([\w.]+)"[^>]*android:exported="true"`)
	deeplinkRe = regexp.MustCompile(`intent\??\.data`)
	routeRe    = regexp.MustCompile(`\b(?:get|post|put|delete|patch)\("/`)
	skipDirs   = map[string]bool{".git": true, "node_modules": true, "build": true}
)

// Entrypoints walks root and lists exported components, deeplink handlers and HTTP routes.
func Entrypoints(root string) ([]Entry, error) {
	var src, exported []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() {
			return nil
		}
		switch {
		case d.Name() == "AndroidManifest.xml":
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range exportedRe.FindAllStringSubmatch(string(b), -1) {
				exported = append(exported, m[1][strings.LastIndex(m[1], ".")+1:])
			}
		case strings.HasSuffix(p, ".kt"), strings.HasSuffix(p, ".java"):
			src = append(src, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, p := range src {
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 1<<20 {
			continue
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		var kinds []string
		for _, e := range exported {
			if e == name {
				kinds = append(kinds, "component")
			}
		}
		if deeplinkRe.Match(b) {
			kinds = append(kinds, "deeplink")
		}
		if routeRe.Match(b) {
			kinds = append(kinds, "route")
		}
		if len(kinds) > 0 {
			out = append(out, Entry{kinds, name, rel})
		}
	}
	return out, nil
}
