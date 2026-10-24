// Package trace exports the Jira <-> PR <-> symbol <-> doc traceability matrix (SOC2 / ISO 27001 evidence).
package trace

import (
	"encoding/csv"
	"io"
	"strings"

	"scribex/pkg/docs"
	"scribex/pkg/manifest"
)

func Write(w io.Writer, jira, pr string, ds []docs.Delta, refs map[string][]string, maps []manifest.Mapping) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"jira", "pr", "file", "symbol", "change", "repo_docs", "confluence_pages"})
	for _, d := range ds {
		ref := d.File + "#" + d.Sym
		var pages []string
		for _, m := range maps {
			for _, s := range m.Symbols {
				if s == ref {
					pages = append(pages, m.Page)
				}
			}
		}
		cw.Write([]string{jira, pr, d.File, d.Sym, d.Kind, strings.Join(refs[ref], ";"), strings.Join(pages, ";")})
	}
	cw.Flush()
	return cw.Error()
}
