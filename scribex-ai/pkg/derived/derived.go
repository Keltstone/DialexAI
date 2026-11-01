// Package derived parses, renders and checks `<!-- @scribex:derived(...) -->` blocks: model-written
// sections that are summaries of code/specs rather than contracts. Each block records the source files
// it was written from; `check` reports it stale when a source changes. Nothing here claims correctness.
package derived

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"scribex/pkg/anchors"
	"scribex/pkg/safepath"
)

const (
	Banner  = "> AI draft from code and specs. Confirm before relying on it."
	endMark = "<!-- @scribex:end -->"

	CodeStaleSource = "stale-source"
	CodeDeadSource  = "dead-source"
	CodeDisputed    = "disputed"

	MaxSources  = 40
	maxHashSize = 4 << 20
	DisputedTag = "> Disputed:"
)

// Source is one file a block was written from, with the first 12 hex chars of its sha256.
type Source struct{ Path, Hash string }

// Block is one derived section.
type Block struct {
	ID        string // typeID:unit
	Sources   []Source
	Review    string // "" | socratix:<mode>:clean | socratix:<mode>:disputed | skipped:<reason>
	Confirmed string // YYYY-MM-DD when a human confirmed
	Body      string // prose without banner, disputed list or markers
	Disputed  []string
	Line      int // 1-based line of the opening marker (Parse only)
	End       int // 1-based line of the end marker, 0 if missing (Parse only)
}

var (
	open    = regexp.MustCompile(`^\s*<!--\s*@scribex:derived\(id=([A-Za-z0-9._:/-]{1,200})\)((?:\s+[a-z]+=\S*)*)\s*-->\s*$`)
	kvRe    = regexp.MustCompile(`([a-z]+)=(\S*)`)
	dateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reviewR = regexp.MustCompile(`^[a-z0-9:_-]{1,80}$`)
	hashRe  = regexp.MustCompile(`^[0-9a-f]{12}$`)
)

func encPath(p string) string {
	var b strings.Builder
	for _, c := range []byte(p) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("._/+-", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func decPath(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", false
		}
		var v byte
		if _, err := fmt.Sscanf(s[i+1:i+3], "%02X", &v); err != nil {
			return "", false
		}
		b.WriteByte(v)
		i += 2
	}
	return b.String(), true
}

// Render returns the canonical text of b (no trailing newline).
func Render(b Block) string {
	var h strings.Builder
	fmt.Fprintf(&h, "<!-- @scribex:derived(id=%s)", b.ID)
	if len(b.Sources) > 0 {
		var parts []string
		for _, s := range b.Sources {
			parts = append(parts, encPath(s.Path)+"@"+s.Hash)
		}
		h.WriteString(" sources=" + strings.Join(parts, ","))
	}
	if b.Review != "" {
		h.WriteString(" review=" + b.Review)
	}
	if b.Confirmed != "" {
		h.WriteString(" confirmed=" + b.Confirmed)
	}
	h.WriteString(" -->")
	lines := []string{h.String()}
	if b.Confirmed == "" {
		lines = append(lines, Banner)
	}
	for _, d := range b.Disputed {
		lines = append(lines, DisputedTag+" "+d)
	}
	if len(lines) > 1 {
		lines = append(lines, "")
	}
	lines = append(lines, strings.Trim(b.Body, "\n"), endMark)
	return strings.Join(lines, "\n")
}

// Parse returns the derived blocks in md (outside code fences) in order.
func Parse(md string) []Block {
	lines := strings.Split(md, "\n")
	fence := anchors.InFence(lines)
	var out []Block
	for i := 0; i < len(lines); i++ {
		if fence[i] {
			continue
		}
		m := open.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
		if m == nil {
			continue
		}
		b := Block{ID: m[1], Line: i + 1}
		bad := false
		for _, kv := range kvRe.FindAllStringSubmatch(m[2], -1) {
			switch kv[1] {
			case "sources":
				for _, part := range strings.Split(kv[2], ",") {
					at := strings.LastIndexByte(part, '@')
					if at <= 0 {
						bad = true
						continue
					}
					p, ok := decPath(part[:at])
					if !ok || !hashRe.MatchString(part[at+1:]) {
						bad = true
						continue
					}
					b.Sources = append(b.Sources, Source{p, part[at+1:]})
				}
			case "review":
				if !reviewR.MatchString(kv[2]) {
					bad = true
				}
				b.Review = kv[2]
			case "confirmed":
				if !dateRe.MatchString(kv[2]) {
					bad = true
				}
				b.Confirmed = kv[2]
			default:
				bad = true
			}
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == endMark {
				end = j
				break
			}
			if open.MatchString(strings.TrimRight(lines[j], "\r")) {
				break // nested/unterminated: stop at the next opener
			}
		}
		if bad || end < 0 {
			if end < 0 {
				b.End = 0
			}
			b.Body = ""
			if end >= 0 {
				b.End = end + 1
				i = end
			}
			b.Review = "invalid"
			out = append(out, b)
			continue
		}
		b.End = end + 1
		var body []string
		for _, l := range lines[i+1 : end] {
			l = strings.TrimRight(l, "\r")
			if strings.TrimSpace(l) == Banner {
				continue
			}
			if strings.HasPrefix(l, DisputedTag+" ") {
				b.Disputed = append(b.Disputed, strings.TrimPrefix(l, DisputedTag+" "))
				continue
			}
			body = append(body, l)
		}
		b.Body = strings.Trim(strings.Join(body, "\n"), "\n")
		out = append(out, b)
		i = end
	}
	return out
}

// HashFile returns the first 12 hex chars of sha256(file) for a repo-relative path inside root.
func HashFile(root, rel string) (string, error) {
	p, err := safepath.Join(root, rel)
	if err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s: not a regular file", rel)
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxHashSize+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// Issue is a finding about a derived block.
type Issue struct {
	Line  int
	Ref   string
	Msg   string
	Code  string
	Stale bool
}

// Check compares each block's recorded source hashes with the files under root. It makes no model calls.
func Check(root, md string) (out []Issue) {
	for _, b := range Parse(md) {
		if b.Review == "invalid" {
			out = append(out, Issue{Line: b.Line, Ref: b.ID, Msg: "malformed derived block (header or end marker)", Code: "bad-derived"})
			continue
		}
		var stale, dead []string
		for _, s := range b.Sources {
			h, err := HashFile(root, s.Path)
			switch {
			case err != nil:
				dead = append(dead, s.Path)
			case h != s.Hash:
				stale = append(stale, s.Path)
			}
		}
		sort.Strings(stale)
		sort.Strings(dead)
		if len(dead) > 0 {
			out = append(out, Issue{Line: b.Line, Ref: b.ID, Code: CodeDeadSource,
				Msg: "source file(s) missing: " + strings.Join(dead, ", ") + " (regenerate or confirm)"})
		}
		if len(stale) > 0 {
			out = append(out, Issue{Line: b.Line, Ref: b.ID, Code: CodeStaleSource, Stale: true,
				Msg: "source changed since this section was written: " + strings.Join(stale, ", ") + " (regenerate or confirm)"})
		}
		if len(b.Disputed) > 0 || strings.HasSuffix(b.Review, ":disputed") {
			out = append(out, Issue{Line: b.Line, Ref: b.ID, Code: CodeDisputed,
				Msg: fmt.Sprintf("council review disputed %d claim(s)", len(b.Disputed))})
		}
	}
	return
}

// Confirm re-stamps every block with the given id to the current source hashes, marks it confirmed on
// date and removes the banner. It returns the new document and whether anything changed.
func Confirm(root, md, id, date string) (string, bool, error) {
	if !dateRe.MatchString(date) {
		return md, false, fmt.Errorf("bad date %q", date)
	}
	lines := strings.Split(md, "\n")
	blocks := Parse(md)
	changed := false
	for k := len(blocks) - 1; k >= 0; k-- {
		b := blocks[k]
		if b.ID != id {
			continue
		}
		if b.Review == "invalid" || b.End == 0 {
			return md, false, fmt.Errorf("block %s is malformed", id)
		}
		var srcs []Source
		for _, s := range b.Sources {
			h, err := HashFile(root, s.Path)
			if err != nil {
				return md, false, fmt.Errorf("source %s: %w", s.Path, err)
			}
			srcs = append(srcs, Source{s.Path, h})
		}
		nb := b
		nb.Sources, nb.Confirmed, nb.Disputed = srcs, date, nil
		repl := strings.Split(Render(nb), "\n")
		lines = append(lines[:b.Line-1], append(repl, lines[b.End:]...)...)
		changed = true
	}
	if !changed {
		return md, false, fmt.Errorf("no derived block with id %q", id)
	}
	return strings.Join(lines, "\n"), true, nil
}

// CheckBody rejects bodies that could forge or break out of a block. The generator also runs the
// stricter prose validation; this is the last line of defence at render time.
func CheckBody(body string) error {
	low := strings.ToLower(body)
	switch {
	case strings.Contains(low, "@scribex"), strings.Contains(body, "<!--"), strings.Contains(body, "-->"):
		return fmt.Errorf("body contains a marker or HTML comment")
	case strings.TrimSpace(body) == "":
		return fmt.Errorf("body is empty")
	}
	for _, l := range strings.Split(body, "\n") {
		if strings.TrimSpace(l) == Banner || strings.HasPrefix(l, DisputedTag) {
			return fmt.Errorf("body contains reserved banner text")
		}
	}
	return nil
}
