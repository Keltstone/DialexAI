// Package manifest reads .scribex/manifest.yaml (a tiny YAML subset: no deps).
//
//	mappings:
//	  - page: "123"
//	    space: ENG
//	    title: Auth API
//	    symbols: [pkg/auth/jwt.go#VerifyToken]
package manifest

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"scribex/pkg/safepath"
)

type Mapping struct {
	Page, Space, Title string
	Symbols            []string
}

var spaceRe = regexp.MustCompile(`^[A-Za-z0-9~][A-Za-z0-9_~-]{0,254}$`)

// Validate checks all mappings according to Part A rules:
// - Page is ValidPageID (digits only, 1-20), duplicate Page values rejected
// - Space matches ^[A-Za-z0-9~][A-Za-z0-9_~-]{0,254}$
// - Title is non-empty, <= 255 runes, no control chars (incl newlines)
// - Symbols are <relpath>#<Sym> with exactly one #, and relpath passes CheckRel
// - Missing page is an error
func Validate(ms []Mapping) error {
	seenPages := make(map[string]bool)
	for i, m := range ms {
		entryNum := i + 1
		if m.Page == "" {
			return fmt.Errorf("manifest entry %d: field page is missing", entryNum)
		}
		if !safepath.ValidPageID(m.Page) {
			return fmt.Errorf("manifest entry %d: field page %q is invalid", entryNum, m.Page)
		}
		if seenPages[m.Page] {
			return fmt.Errorf("manifest entry %d: field page %q is duplicate", entryNum, m.Page)
		}
		seenPages[m.Page] = true

		if !spaceRe.MatchString(m.Space) {
			return fmt.Errorf("manifest entry %d: field space %q is invalid", entryNum, m.Space)
		}

		runes := []rune(m.Title)
		if len(runes) == 0 {
			return fmt.Errorf("manifest entry %d: field title is empty", entryNum)
		}
		if len(runes) > 255 {
			return fmt.Errorf("manifest entry %d: field title exceeds 255 runes", entryNum)
		}
		for _, r := range runes {
			if unicode.IsControl(r) {
				return fmt.Errorf("manifest entry %d: field title contains control characters", entryNum)
			}
		}

		for _, s := range m.Symbols {
			parts := strings.Split(s, "#")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("manifest entry %d: field symbols %q must be <relpath>#<Sym>", entryNum, s)
			}
			if err := safepath.CheckRel(parts[0]); err != nil {
				return fmt.Errorf("manifest entry %d: field symbols path %q: %w", entryNum, parts[0], err)
			}
		}
	}
	return nil
}

func Load(path string) ([]Mapping, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ms := Parse(string(b))
	if err := Validate(ms); err != nil {
		return nil, err
	}
	return ms, nil
}

func Parse(s string) (out []Mapping) {
	inSymbols := false
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' {
			continue
		}
		if inSymbols && strings.HasPrefix(l, "- ") {
			if len(out) > 0 {
				item := unq(strings.TrimSpace(l[2:]))
				if item != "" {
					out[len(out)-1].Symbols = append(out[len(out)-1].Symbols, item)
				}
			}
			continue
		}
		if strings.HasPrefix(l, "- ") {
			inSymbols = false
			out = append(out, Mapping{})
			l = strings.TrimSpace(l[2:])
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok || len(out) == 0 {
			continue
		}
		v = strings.TrimSpace(v)
		m := &out[len(out)-1]
		switch k {
		case "page":
			inSymbols = false
			m.Page = unq(v)
		case "space":
			inSymbols = false
			m.Space = unq(v)
		case "title":
			inSymbols = false
			m.Title = unq(v)
		case "symbols":
			if v == "" {
				inSymbols = true
			} else {
				inSymbols = false
				for _, x := range strings.Split(strings.Trim(v, "[]"), ",") {
					if x = unq(strings.TrimSpace(x)); x != "" {
						m.Symbols = append(m.Symbols, x)
					}
				}
			}
		default:
			inSymbols = false
		}
	}
	return
}

func unq(s string) string { return strings.Trim(s, `"'`) }
