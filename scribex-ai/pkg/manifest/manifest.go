// Package manifest reads .scribex/manifest.yaml (a tiny YAML subset: no deps).
//
//	mappings:
//	  - page: "123"
//	    space: ENG
//	    title: Auth API
//	    symbols: [pkg/auth/jwt.go#VerifyToken]
package manifest

import (
	"os"
	"strings"
)

type Mapping struct {
	Page, Space, Title string
	Symbols            []string
}

func Load(path string) ([]Mapping, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(b)), nil
}

func Parse(s string) (out []Mapping) {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' {
			continue
		}
		if strings.HasPrefix(l, "- ") {
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
			m.Page = unq(v)
		case "space":
			m.Space = unq(v)
		case "title":
			m.Title = unq(v)
		case "symbols":
			for _, x := range strings.Split(strings.Trim(v, "[]"), ",") {
				if x = unq(strings.TrimSpace(x)); x != "" {
					m.Symbols = append(m.Symbols, x)
				}
			}
		}
	}
	return
}

func unq(s string) string { return strings.Trim(s, `"'`) }
