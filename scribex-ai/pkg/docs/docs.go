// Package docs is the repo-level engine: anchor checks, link scavenging, contract deltas.
package docs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"scribex/pkg/anchors"
	"scribex/pkg/symbols"
)

type Issue struct {
	File string
	anchors.Issue
}

func (i Issue) String() string { return i.File + i.Issue.String() }

type Report struct {
	Total  int // anchors seen
	Issues []Issue
}

// Delta is one public-symbol change between base and the working tree.
type Delta struct{ File, Sym, Kind string } // Kind: added | changed | removed

// Breaking reports whether consumers of the symbol can be affected.
func (d Delta) Breaking() bool { return d.Kind != "added" }

// Files lists markdown files under root.
func Files(root string) (out []string) {
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != root {
			if n := d.Name(); strings.HasPrefix(n, ".") || n == "node_modules" || n == "vendor" {
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

// Symbols returns the public symbols of a repo-relative file.
func Symbols(root, file string) map[string]string {
	src, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		return nil
	}
	return symbols.Extract(file, src)
}

// Check validates (and with sync, repairs) every anchor under root. Stale issues fixed by sync are not reported.
func Check(root string, sync bool, prose anchors.Prose) (Report, error) {
	cache := map[string]map[string]string{}
	look := func(file, sym string) (string, bool) {
		s, ok := cache[file]
		if !ok {
			s = Symbols(root, file)
			cache[file] = s
		}
		v, ok := s[sym]
		return v, ok
	}
	var r Report
	for _, f := range Files(root) {
		b, err := os.ReadFile(f)
		if err != nil {
			return r, err
		}
		r.Total += anchors.Count(string(b))
		issues, out := anchors.Process(string(b), look, sync, prose)
		for _, i := range issues {
			if !(sync && i.Stale) {
				r.Issues = append(r.Issues, Issue{f, i})
			}
		}
		if sync && out != string(b) {
			if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
				return r, err
			}
		}
	}
	return r, nil
}

var link = regexp.MustCompile(`\]\(([^)#:\s]+)(?:#[^)]*)?\)`)

// Links reports markdown links to repo-relative targets that do not exist.
func Links(root string) (out []Issue) {
	for _, f := range Files(root) {
		b, _ := os.ReadFile(f)
		for n, l := range strings.Split(string(b), "\n") {
			for _, m := range link.FindAllStringSubmatch(l, -1) {
				if _, err := os.Stat(filepath.Join(filepath.Dir(f), m[1])); err != nil {
					out = append(out, Issue{f, anchors.Issue{Line: n + 1, Ref: m[1], Msg: "broken link"}})
				}
			}
		}
	}
	return
}

// Refs maps "file#Sym" to the doc files anchoring it.
func Refs(root string) map[string][]string {
	re := regexp.MustCompile(`@scribex:anchor\(([^)\s]+)\)`)
	out := map[string][]string{}
	for _, f := range Files(root) {
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = append(out[m[1]], f)
		}
	}
	return out
}

// Deltas diffs public symbols of every file changed since base.
func Deltas(root, base string) ([]Delta, error) {
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", "--relative", base).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %w", base, err)
	}
	var ds []Delta
	for _, f := range strings.Fields(string(out)) {
		cur := Symbols(root, f)
		oldSrc, _ := exec.Command("git", "-C", root, "show", base+":./"+f).Output()
		old := symbols.Extract(f, oldSrc)
		for k, v := range cur {
			if ov, ok := old[k]; !ok {
				ds = append(ds, Delta{f, k, "added"})
			} else if ov != v {
				ds = append(ds, Delta{f, k, "changed"})
			}
		}
		for k := range old {
			if _, ok := cur[k]; !ok {
				ds = append(ds, Delta{f, k, "removed"})
			}
		}
	}
	sort.Slice(ds, func(i, j int) bool {
		return ds[i].File+ds[i].Sym < ds[j].File+ds[j].Sym
	})
	return ds, nil
}

// Markdown renders a PR-comment body.
func (r Report) Markdown(extra ...Issue) string {
	all := append(append([]Issue{}, r.Issues...), extra...)
	if len(all) == 0 {
		return fmt.Sprintf("Scribex: docs in sync (%d anchors checked).", r.Total)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Scribex found %d doc issue(s) (%d anchors):\n\n", len(all), r.Total)
	for _, i := range all {
		fmt.Fprintf(&b, "- `%s` line %d `%s`: %s\n", i.File, i.Line, i.Ref, i.Msg)
	}
	b.WriteString("\nRun `scribex sync` to refresh stale anchors.")
	return b.String()
}
