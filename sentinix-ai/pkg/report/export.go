package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"sentinix/pkg/advisories"
	"sentinix/pkg/store"
)

// CSV writes one row per finding. Cells starting with = + - @ are prefixed so spreadsheets never run them.
func CSV(w io.Writer, fs []store.Finding) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "rule", "severity", "status", "file", "line", "cwe", "owner", "ticket", "message", "fix"})
	safe := func(s string) string {
		if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
			return "'" + s
		}
		return s
	}
	for _, f := range fs {
		cw.Write([]string{f.ID, f.Rule, f.Severity.String(), f.Status, safe(f.File), fmt.Sprint(f.Line), f.CWE, safe(f.Owner), f.Ticket, safe(f.Message), safe(f.Fix)})
	}
	cw.Flush()
	return cw.Error()
}

func purl(c advisories.Component) string {
	switch c.Ecosystem {
	case "Go":
		return "pkg:golang/" + c.Name + "@v" + c.Version
	case "npm":
		return "pkg:npm/" + url.PathEscape(c.Name) + "@" + c.Version
	case "PyPI":
		return "pkg:pypi/" + strings.ToLower(c.Name) + "@" + c.Version
	case "Maven":
		g, a, _ := strings.Cut(c.Name, ":")
		return "pkg:maven/" + g + "/" + a + "@" + c.Version
	}
	return "pkg:generic/" + c.Name + "@" + c.Version
}

// SBOM writes CycloneDX 1.5 JSON. Open advisory findings become vulnerabilities with their fixed versions.
func SBOM(w io.Writer, project string, comps []advisories.Component, fs []store.Finding) error {
	type comp struct {
		Type    string `json:"type"`
		BOMRef  string `json:"bom-ref"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Purl    string `json:"purl"`
	}
	refs := map[string]string{} // name@version -> purl
	cs := []comp{}
	for _, c := range comps {
		p := purl(c)
		if _, dup := refs[c.Name+"@"+c.Version]; dup {
			continue
		}
		refs[c.Name+"@"+c.Version] = p
		cs = append(cs, comp{"library", p, c.Name, c.Version, p})
	}
	vulns := []map[string]any{}
	for _, f := range fs {
		id, ok := strings.CutPrefix(f.Rule, "advisory/")
		if !ok || f.Status != "open" {
			continue
		}
		v := map[string]any{"id": id, "ratings": []any{map[string]string{"severity": f.Severity.String()}},
			"description": f.Message, "affects": []any{map[string]string{"ref": refs[f.Evidence]}}}
		if f.Fix != "" {
			v["recommendation"] = f.Fix
		}
		vulns = append(vulns, v)
	}
	return json.NewEncoder(w).Encode(map[string]any{
		"bomFormat": "CycloneDX", "specVersion": "1.5", "version": 1,
		"metadata":        map[string]any{"component": map[string]string{"type": "application", "name": project}},
		"components":      cs,
		"vulnerabilities": vulns,
	})
}
