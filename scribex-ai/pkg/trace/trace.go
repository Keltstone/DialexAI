// Package trace exports the Jira <-> PR <-> symbol <-> doc traceability matrix (SOC2 / ISO 27001 evidence).
package trace

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"scribex/pkg/docs"
	"scribex/pkg/fsx"
	"scribex/pkg/manifest"
)

const Schema = "scribex-trace/1"

// Options configures evidence generation.
type Options struct {
	Version           string
	BaseSHA           string
	HeadSHA           string
	HeadCommitTimeUTC string
	Jira              string
	JiraVerified      bool
	PR                string
	Deltas            []docs.Delta
	Refs              map[string][]string
	Mappings          []manifest.Mapping
}

// Meta describes the JSON sidecar f.csv.json.
type Meta struct {
	Schema         string   `json:"schema"`
	Status         string   `json:"status"`
	Rows           int      `json:"rows"`
	CSVSHA256      string   `json:"csv_sha256"`
	GeneratedAtUTC string   `json:"generated_at_utc"`
	ScribexVersion string   `json:"scribex_version"`
	BaseSHA        string   `json:"base_sha"`
	HeadSHA        string   `json:"head_sha"`
	JiraVerified   bool     `json:"jira_verified"`
	UnknownFiles   []string `json:"unknown_files"`
}

// Cell neutralises CSV formula injection and strips/replaces unescaped newlines with spaces.
func Cell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")

	trimmed := strings.TrimLeft(s, " ")
	if len(trimmed) > 0 {
		switch trimmed[0] {
		case '=', '+', '-', '@', '\t', '\r':
			return "'" + s
		}
	}
	return s
}

type rowItem struct {
	file     string
	symbol   string
	change   string
	repoDocs string
	pages    string
}

func dedupeAndSort(items []string) string {
	if len(items) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var unique []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" && !seen[it] {
			seen[it] = true
			unique = append(unique, it)
		}
	}
	sort.Strings(unique)
	return strings.Join(unique, ";")
}

func dedupeAndSortPages(maps []manifest.Mapping, ref string) string {
	var pages []string
	for _, m := range maps {
		for _, s := range m.Symbols {
			if s == ref {
				pages = append(pages, m.Page)
			}
		}
	}
	return dedupeAndSort(pages)
}

// NormalizeChange maps delta kind to added|modified|deleted|none.
func NormalizeChange(kind string) string {
	switch kind {
	case "added":
		return "added"
	case "changed", "modified":
		return "modified"
	case "removed", "deleted":
		return "deleted"
	case "none", "":
		return "none"
	default:
		return kind
	}
}

// Generate builds the deterministic, formula-neutralised CSV evidence bytes and reports the data row count.
func Generate(opts Options) ([]byte, int, error) {
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)

	header := []string{
		"schema", "scribex_version", "base_sha", "head_sha", "head_commit_time_utc",
		"jira", "jira_verified", "pr", "file", "symbol", "change", "repo_docs", "confluence_pages",
	}
	if err := cw.Write(header); err != nil {
		return nil, 0, err
	}

	jvStr := "false"
	if opts.JiraVerified {
		jvStr = "true"
	}

	if len(opts.Deltas) == 0 {
		zeroRow := []string{
			Cell(Schema),
			Cell(opts.Version),
			Cell(opts.BaseSHA),
			Cell(opts.HeadSHA),
			Cell(opts.HeadCommitTimeUTC),
			Cell(opts.Jira),
			Cell(jvStr),
			Cell(opts.PR),
			Cell(""),
			Cell(""),
			Cell("none"),
			Cell(""),
			Cell(""),
		}
		if err := cw.Write(zeroRow); err != nil {
			return nil, 0, err
		}
		cw.Flush()
		return buf.Bytes(), 1, cw.Error()
	}

	var items []rowItem
	for _, d := range opts.Deltas {
		ref := d.File + "#" + d.Sym
		var docsList []string
		if opts.Refs != nil {
			docsList = opts.Refs[ref]
		}
		repoDocs := dedupeAndSort(docsList)
		pages := dedupeAndSortPages(opts.Mappings, ref)

		items = append(items, rowItem{
			file:     d.File,
			symbol:   d.Sym,
			change:   NormalizeChange(d.Kind),
			repoDocs: repoDocs,
			pages:    pages,
		})
	}

	// Deterministic sorting by (file, symbol, change)
	sort.Slice(items, func(i, j int) bool {
		if items[i].file != items[j].file {
			return items[i].file < items[j].file
		}
		if items[i].symbol != items[j].symbol {
			return items[i].symbol < items[j].symbol
		}
		return items[i].change < items[j].change
	})

	for _, it := range items {
		row := []string{
			Cell(Schema),
			Cell(opts.Version),
			Cell(opts.BaseSHA),
			Cell(opts.HeadSHA),
			Cell(opts.HeadCommitTimeUTC),
			Cell(opts.Jira),
			Cell(jvStr),
			Cell(opts.PR),
			Cell(it.file),
			Cell(it.symbol),
			Cell(it.change),
			Cell(it.repoDocs),
			Cell(it.pages),
		}
		if err := cw.Write(row); err != nil {
			return nil, 0, err
		}
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return nil, 0, err
	}

	return buf.Bytes(), len(items), nil
}

// WriteFiles safely writes both the CSV and the JSON sidecar using atomic file writes.
func WriteFiles(csvPath string, csvBytes, metaBytes []byte) error {
	for _, p := range []string{csvPath, csvPath + ".json"} {
		if fi, err := os.Lstat(p); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to write to symlink %s", p)
			}
			if fi.IsDir() {
				return fmt.Errorf("refusing to write to directory %s", p)
			}
		}
	}

	if err := fsx.WriteAtomic(csvPath, csvBytes, 0o644); err != nil {
		return err
	}
	if err := fsx.WriteAtomic(csvPath+".json", metaBytes, 0o644); err != nil {
		return err
	}
	return nil
}

// Write generates and writes trace evidence to w using default/fallback metadata.
func Write(w io.Writer, jiraKey, pr string, ds []docs.Delta, refs map[string][]string, maps []manifest.Mapping) error {
	opts := Options{
		Version:           "dev",
		BaseSHA:           "base",
		HeadSHA:           "head",
		HeadCommitTimeUTC: "2026-01-01T00:00:00Z",
		Jira:              jiraKey,
		JiraVerified:      false,
		PR:                pr,
		Deltas:            ds,
		Refs:              refs,
		Mappings:          maps,
	}
	b, _, err := Generate(opts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
