// scribex: keep docs in sync with code. init | check [-base ref] | sync | diagram
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"scribex/pkg/anchors"
	"scribex/pkg/diagram"
	"scribex/pkg/symbols"
)

var version = "1.0.0"

const usage = "usage: scribex init | check [-C dir] [-base ref] | sync [-C dir] [-commit] | diagram [-C dir] | version"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := fs.String("C", ".", "repo root")
	base := fs.String("base", "", "git ref to diff against (impact gate)")
	commit := fs.Bool("commit", false, "sync: git-commit changed docs as a companion commit")
	fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "version", "--version":
		fmt.Println("Scribex AI version", version)
	case "init":
		err = initCmd(*root)
	case "check":
		err = check(*root, *base, false)
	case "sync":
		if err = check(*root, "", true); err == nil && *commit {
			err = companion(*root)
		}
	case "diagram":
		err = diagramCmd(*root)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "scribex:", err)
		os.Exit(1)
	}
}

// companion commits only .md files touched by sync; no-op when clean.
func companion(root string) error {
	if out, _ := exec.Command("git", "-C", root, "status", "--porcelain", "--", "*.md").Output(); len(out) == 0 {
		return nil
	}
	if err := exec.Command("git", "-C", root, "add", "--", "*.md").Run(); err != nil {
		return err
	}
	return exec.Command("git", "-C", root, "commit", "-m", "docs: scribex sync", "--", "*.md").Run()
}

func initCmd(root string) error {
	dir := filepath.Join(root, ".scribex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m := filepath.Join(dir, "manifest.yaml")
	if _, err := os.Stat(m); err == nil {
		return nil
	}
	return os.WriteFile(m, []byte("# page-to-symbol mappings for external docs (Phase 3)\nmappings: []\n"), 0o644)
}

// docFiles lists markdown files under root.
func docFiles(root string) (out []string) {
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != root {
			switch n := d.Name(); {
			case strings.HasPrefix(n, "."), n == "node_modules", n == "vendor":
				return filepath.SkipDir
			}
		}
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	return
}

func check(root, base string, sync bool) error {
	if base != "" {
		if changed, err := contractDelta(root, base); err != nil {
			return err
		} else if len(changed) == 0 {
			fmt.Println("impact gate: no public contract change; 0 tokens, nothing to do")
			return nil
		} else {
			fmt.Println("impact gate: contract delta in:", strings.Join(changed, ", "))
		}
	}
	cache := map[string]map[string]string{}
	look := func(file, sym string) (string, bool) {
		syms, ok := cache[file]
		if !ok {
			src, err := os.ReadFile(filepath.Join(root, file))
			if err == nil {
				syms = symbols.Extract(file, src)
			}
			cache[file] = syms
		}
		s, ok := syms[sym]
		return s, ok
	}
	bad := 0
	for _, f := range docFiles(root) {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		issues, out := anchors.Process(string(b), look, sync)
		for _, i := range issues {
			fmt.Println(f + i.String())
			if i.Msg[0] == 's' && strings.HasPrefix(i.Msg, "stale") && sync {
				continue // fixed by sync
			}
			bad++
		}
		if sync && out != string(b) {
			if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
				return err
			}
			fmt.Println("synced", f)
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d doc issue(s)", bad)
	}
	return nil
}

// contractDelta returns files whose public signatures differ from base.
func contractDelta(root, base string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", "--relative", base).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %w", base, err)
	}
	var changed []string
	for _, f := range strings.Fields(string(out)) {
		cur, _ := os.ReadFile(filepath.Join(root, f))
		old, _ := exec.Command("git", "-C", root, "show", base+":./"+f).Output()
		if !sameSyms(symbols.Extract(f, old), symbols.Extract(f, cur)) {
			changed = append(changed, f)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func sameSyms(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func diagramCmd(root string) error {
	d, err := diagram.Mermaid(root)
	if err != nil {
		return err
	}
	n := 0
	for _, f := range docFiles(root) {
		b, _ := os.ReadFile(f)
		if out, ok := diagram.Replace(string(b), d); ok {
			if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
				return err
			}
			fmt.Println("updated", f)
			n++
		}
	}
	if n == 0 {
		fmt.Println(d)
	}
	return nil
}
