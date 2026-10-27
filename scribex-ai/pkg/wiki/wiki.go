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
	"unicode"

	"scribex/pkg/fsx"
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
	count := 0
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		count++
		if count > 100000 {
			return fmt.Errorf("wiki page count exceeds 100,000 limit")
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
	if strings.HasPrefix(src, "---\n") || strings.HasPrefix(src, "---\r\n") {
		start := 4
		if strings.HasPrefix(src, "---\r\n") {
			start = 5
		}
		if end := strings.Index(src[start:], "\n---"); end >= 0 && end <= 64<<10 {
			for _, l := range strings.Split(src[start:start+end], "\n") {
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

// New creates relDir/<Title>.md with frontmatter under root; refuses to overwrite.
func New(root, relDir, title string) (string, error) {
	title = strings.TrimSpace(title)
	runes := []rune(title)
	if len(runes) < 1 || len(runes) > 120 {
		return "", fmt.Errorf("title must be 1-120 runes, got %d", len(runes))
	}
	for _, r := range runes {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("title contains control character")
		}
	}
	if strings.ContainsAny(title, "/\\:") {
		return "", fmt.Errorf("title cannot contain '/', '\\', or ':'")
	}
	if strings.Contains(title, "..") {
		return "", fmt.Errorf("title cannot contain '..'")
	}

	filename := strings.ReplaceAll(title, " ", "-") + ".md"
	relPath := filepath.Join(relDir, filename)
	targetPath := filepath.Join(root, relPath)
	if _, err := os.Stat(targetPath); err == nil {
		return "", fmt.Errorf("%s exists", relPath)
	}

	fmTitle := fmt.Sprintf("%q", title)
	body := fmt.Sprintf("---\ntitle: %s\ntags: []\n---\n# %s\n\n", fmTitle, title)

	if err := fsx.WriteFileIn(root, relPath, []byte(body), 0o644); err != nil {
		return "", err
	}
	return targetPath, nil
}

// Render writes a publishable copy into out: wikilinks become relative links, backlinks footers,
// plus generated Home.md (if absent) and _Sidebar.md. Push out/ to <repo>.wiki.git for GitHub's wiki.
func (w *Wiki) Render(out, home string) error {
	cleanOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	cleanSource, err := filepath.Abs(w.Dir)
	if err != nil {
		return err
	}

	// out must not be / or volume root
	if cleanOut == "/" || cleanOut == `\` || (filepath.VolumeName(cleanOut) != "" && cleanOut == filepath.VolumeName(cleanOut)+string(filepath.Separator)) {
		return fmt.Errorf("cannot render to root")
	}

	// out must not be inside wiki source dir
	relToSource, err := filepath.Rel(cleanSource, cleanOut)
	if err == nil && !strings.HasPrefix(relToSource, "..") {
		return fmt.Errorf("out directory %q cannot be inside wiki source dir %q", out, w.Dir)
	}

	// out must not be the repo root itself
	if fi, err := os.Stat(filepath.Join(cleanOut, ".git")); err == nil && fi.IsDir() {
		return fmt.Errorf("cannot render to repository root")
	}
	if fi, err := os.Stat(filepath.Join(cleanOut, ".scribex")); err == nil && fi.IsDir() {
		return fmt.Errorf("cannot render to repository root")
	}

	if err := os.MkdirAll(cleanOut, 0o755); err != nil {
		return err
	}

	bl := w.Backlinks()
	rel := func(from, to *Page) string {
		r, _ := filepath.Rel(filepath.Dir(from.File), to.File)
		return strings.ReplaceAll(filepath.ToSlash(r), " ", "%20")
	}
	write := func(file, body string) error {
		return fsx.WriteFileIn(cleanOut, file, []byte(body), 0o644)
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
