// Package diagram renders a Mermaid package-dependency graph for a Go module.
// ponytail: Go imports only; add other languages when needed.
package diagram

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var block = regexp.MustCompile(`(?s)<!--\s*@scribex:diagram\s*-->.*?<!--\s*@scribex:end\s*-->`)

// Mermaid returns a flowchart of intra-module package imports under root.
func Mermaid(root string) (string, error) {
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	mp := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(mod), "\n", 2)[0], "module"))
	edges := map[string]bool{}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "testdata") && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		from := filepath.ToSlash(rel)
		for _, im := range f.Imports {
			ip := strings.Trim(im.Path.Value, `"`)
			if ip == mp || strings.HasPrefix(ip, mp+"/") {
				if to := strings.TrimPrefix(strings.TrimPrefix(ip, mp), "/"); to != from {
					edges[node(from)+" --> "+node(to)] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(edges))
	for k := range edges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "```mermaid\nflowchart LR\n    " + strings.Join(keys, "\n    ") + "\n```", nil
}

func node(p string) string {
	if p == "." {
		p = "root"
	}
	return strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(p) + `["` + p + `"]`
}

// Replace rewrites the diagram block in md; reports whether one existed.
func Replace(md, diagram string) (string, bool) {
	if !block.MatchString(md) {
		return md, false
	}
	return block.ReplaceAllLiteralString(md, "<!-- @scribex:diagram -->\n"+diagram+"\n<!-- @scribex:end -->"), true
}
