// Package wiki manages an interlinked markdown wiki (GitHub-wiki model): [[wikilinks]], backlinks, lint, render.
// ponytail: links inside code fences are not skipped; add if it produces false positives.
package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Page struct {
	Key, File, Title string // File is relative to the wiki dir
	Tags             []string
	Body             string
}
type Wiki struct {
	Dir   string
	Pages map[string]*Page
	dups  []string
}

var (
	wikiLink = regexp.MustCompile(`\[\[([^\]|#]+)(#[^\]|]*)?(?:\|([^\]]*))?\]\]`)
	mdLink   = regexp.MustCompile(`\]\(([^)#:\s]+\.md)(?:#[^)]*)?\)`)
)

// Key normalizes a page name: "Getting Started", "getting-started.md" and "Getting_Started" are one page.
func Key(s string) string {
	s = strings.TrimSuffix(filepath.Base(strings.TrimSpace(s)), ".md")
	return strings.ToLower(strings.NewReplacer(" ", "-", "_", "-").Replace(s))
}

func Load(dir string) (*Wiki, error) {
	w := &Wiki{Dir: dir, Pages: map[string]*Page{}}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != dir {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		pg := parsePage(rel, string(b))
		if old, dup := w.Pages[pg.Key]; dup {
			w.dups = append(w.dups, fmt.Sprintf("duplicate page name %q: %s and %s", pg.Key, old.File, rel))
			return nil
		}
		w.Pages[pg.Key] = pg
		return nil
	})
	return w, err
}

func parsePage(rel, src string) *Page {
	p := &Page{Key: Key(rel), File: filepath.ToSlash(rel), Body: src}
	if strings.HasPrefix(src, "---\n") {
		if end := strings.Index(src[4:], "\n---"); end >= 0 {
			for _, l := range strings.Split(src[4:4+end], "\n") {
				k, v, _ := strings.Cut(l, ":")
				v = strings.TrimSpace(v)
				switch strings.TrimSpace(k) {
				case "title":
					p.Title = strings.Trim(v, `"'`)
				case "tags":
					for _, t := range strings.Split(strings.Trim(v, "[]"), ",") {
						if t = strings.TrimSpace(t); t != "" {
							p.Tags = append(p.Tags, t)
						}
					}
				}
			}
		}
	}
	if p.Title == "" {
		for _, l := range strings.Split(src, "\n") {
			if strings.HasPrefix(l, "# ") {
				p.Title = strings.TrimSpace(l[2:])
				break
			}
		}
	}
	if p.Title == "" {
		p.Title = strings.TrimSuffix(filepath.Base(rel), ".md")
	}
	return p
}

type ref struct {
	Key  string
	Line int
	Raw  string
}

func refs(p *Page) (out []ref) {
	for n, l := range strings.Split(p.Body, "\n") {
		for _, m := range wikiLink.FindAllStringSubmatch(l, -1) {
			out = append(out, ref{Key(m[1]), n + 1, m[0]})
		}
		for _, m := range mdLink.FindAllStringSubmatch(l, -1) {
			out = append(out, ref{Key(m[1]), n + 1, m[1]})
		}
	}
	return
}

// Backlinks maps page key -> sorted keys of pages linking to it.
func (w *Wiki) Backlinks() map[string][]string {
	set := map[string]map[string]bool{}
	for k, p := range w.Pages {
		for _, r := range refs(p) {
			if r.Key != k && w.Pages[r.Key] != nil {
				if set[r.Key] == nil {
					set[r.Key] = map[string]bool{}
				}
				set[r.Key][k] = true
			}
		}
	}
	out := map[string][]string{}
	for t, s := range set {
		for f := range s {
			out[t] = append(out[t], f)
		}
		sort.Strings(out[t])
	}
	return out
}

// Lint reports duplicates, broken links and orphans (pages nothing links to; home and _special pages exempt).
func (w *Wiki) Lint(home string) []string {
	out := append([]string{}, w.dups...)
	keys := w.sorted()
	for _, k := range keys {
		p := w.Pages[k]
		for _, r := range refs(p) {
			if w.Pages[r.Key] == nil {
				out = append(out, fmt.Sprintf("%s:%d broken link %s", p.File, r.Line, r.Raw))
			}
		}
	}
	bl := w.Backlinks()
	for _, k := range keys {
		if k != Key(home) && !strings.HasPrefix(filepath.Base(w.Pages[k].File), "_") && len(bl[k]) == 0 {
			out = append(out, fmt.Sprintf("%s orphan page (no inbound links)", w.Pages[k].File))
		}
	}
	return out
}

func (w *Wiki) sorted() []string {
	ks := make([]string, 0, len(w.Pages))
	for k := range w.Pages {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Search returns up to 20 "file:line: text" hits (title match reported at line 0).
func (w *Wiki) Search(q string) (out []string) {
	q = strings.ToLower(q)
	for _, k := range w.sorted() {
		p := w.Pages[k]
		if strings.Contains(strings.ToLower(p.Title), q) {
			out = append(out, p.File+": "+p.Title)
		}
		for n, l := range strings.Split(p.Body, "\n") {
			if strings.Contains(strings.ToLower(l), q) {
				out = append(out, fmt.Sprintf("%s:%d: %s", p.File, n+1, strings.TrimSpace(l)))
			}
		}
		if len(out) >= 20 {
			return out[:20]
		}
	}
	return
}

// New creates dir/<Title>.md with frontmatter; refuses to overwrite.
func New(dir, title string) (string, error) {
	path := filepath.Join(dir, strings.ReplaceAll(strings.TrimSpace(title), " ", "-")+".md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s exists", path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte("---\ntitle: "+title+"\ntags: []\n---\n# "+title+"\n\n"), 0o644)
}

// Render writes a publishable copy into out: wikilinks become relative links, backlinks footers,
// plus generated Home.md (if absent) and _Sidebar.md. Push out/ to <repo>.wiki.git for GitHub's wiki.
func (w *Wiki) Render(out, home string) error {
	bl := w.Backlinks()
	rel := func(from, to *Page) string {
		r, _ := filepath.Rel(filepath.Dir(from.File), to.File)
		return strings.ReplaceAll(filepath.ToSlash(r), " ", "%20")
	}
	write := func(file, body string) error {
		p := filepath.Join(out, file)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(body), 0o644)
	}
	for _, k := range w.sorted() {
		p := w.Pages[k]
		body := wikiLink.ReplaceAllStringFunc(p.Body, func(s string) string {
			m := wikiLink.FindStringSubmatch(s)
			t := w.Pages[Key(m[1])]
			if t == nil {
				return s
			}
			label := m[3]
			if label == "" {
				label = t.Title
			}
			return "[" + label + "](" + rel(p, t) + m[2] + ")"
		})
		if len(bl[k]) > 0 {
			var parts []string
			for _, f := range bl[k] {
				parts = append(parts, "["+w.Pages[f].Title+"]("+rel(p, w.Pages[f])+")")
			}
			body = strings.TrimRight(body, "\n") + "\n\n---\nLinked from: " + strings.Join(parts, ", ") + "\n"
		}
		if err := write(p.File, body); err != nil {
			return err
		}
	}
	list := func() string {
		var b strings.Builder
		for _, k := range w.sorted() {
			fmt.Fprintf(&b, "- [%s](%s)\n", w.Pages[k].Title, strings.ReplaceAll(w.Pages[k].File, " ", "%20"))
		}
		return b.String()
	}
	if w.Pages[Key(home)] == nil {
		if err := write(home+".md", "# "+home+"\n\n"+list()); err != nil {
			return err
		}
	}
	return write("_Sidebar.md", "**["+home+"]("+home+".md)**\n\n"+list())
}
