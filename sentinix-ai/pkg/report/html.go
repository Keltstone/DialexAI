package report

import (
	"html/template"
	"io"
	"regexp"
	"strings"
	"time"

	"sentinix/pkg/store"
)

type Brand struct {
	Name   string
	Accent string // #rrggbb
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// owasp maps common CWEs to OWASP Top 10 (2021) categories for the auditor view.
var owasp = map[string]string{
	"CWE-89": "A03 Injection", "CWE-79": "A03 Injection", "CWE-862": "A01 Broken Access Control", "CWE-352": "A01 Broken Access Control",
	"CWE-276": "A01 Broken Access Control", "CWE-200": "A01 Broken Access Control", "CWE-327": "A02 Cryptographic Failures",
	"CWE-319": "A02 Cryptographic Failures", "CWE-312": "A02 Cryptographic Failures", "CWE-295": "A07 Identification and Authentication Failures",
	"CWE-942": "A05 Security Misconfiguration", "CWE-489": "A05 Security Misconfiguration", "CWE-749": "A05 Security Misconfiguration",
	"CWE-926": "A05 Security Misconfiguration", "CWE-530": "A05 Security Misconfiguration", "CWE-532": "A09 Logging and Monitoring Failures",
}

type Data struct {
	Brand    Brand
	Project  store.Project
	Scan     *store.Scan // latest scan, for the coverage statement
	Findings []store.Finding
	Posture  store.Posture
	Now      time.Time
}

type row struct {
	store.Finding
	OWASP string
}

var page = template.Must(template.New("r").Funcs(template.FuncMap{
	"sev": func(f store.Finding) string { return f.Severity.String() },
	"day": func(t int64) string { return time.Unix(t, 0).UTC().Format("2006-01-02") },
}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Brand.Name}} security report - {{.Project.Name}}</title>
<style>
:root{--accent:{{.Accent}};--bg:#fff;--fg:#1a1a1a;--mute:#666;--line:#ddd}
@media (prefers-color-scheme:dark){:root{--bg:#161616;--fg:#eee;--mute:#aaa;--line:#333}}
body{font:15px/1.5 system-ui,sans-serif;background:var(--bg);color:var(--fg);max-width:960px;margin:0 auto;padding:24px 16px}
h1{border-bottom:3px solid var(--accent);padding-bottom:8px}
table{border-collapse:collapse;width:100%;font-size:13px}th,td{border-bottom:1px solid var(--line);padding:6px;text-align:left;vertical-align:top}
.sev{font-weight:600;text-transform:uppercase;font-size:11px}.critical,.high{color:#c0392b}.medium{color:#b9770e}.low,.info{color:var(--mute)}
.cov{border-left:4px solid var(--accent);padding:4px 12px;color:var(--mute)}
@media print{body{max-width:none;padding:0}a{color:inherit}tr{break-inside:avoid}}
</style></head><body>
<h1>{{.Brand.Name}} - {{.Project.Name}}</h1>
<p>{{.Project.Repo}} @ {{.Project.Branch}} - generated {{.Now.Format "2006-01-02 15:04 MST"}}</p>
<h2>Summary</h2>
<p>Open findings: critical {{index .Posture.Open 4}}, high {{index .Posture.Open 3}}, medium {{index .Posture.Open 2}}, low {{index .Posture.Open 1}}, info {{index .Posture.Open 0}}.
{{if .Posture.AvgFixDays}}Average time to fix: {{printf "%.1f" .Posture.AvgFixDays}} days.{{end}}</p>
<h2>Coverage</h2>
<p class="cov">{{if .Scan}}Last scan {{day .Scan.Started}} on commit {{.Scan.Commit}} ({{.Scan.Mode}}). Engines run: {{.Scan.Engines}}. Rule packs: {{.Scan.Packs}}.{{else}}No scan recorded.{{end}}
A clean report only means these checks found nothing; it is not a statement that the product is secure. Not covered: engines that were not installed, logic flaws, runtime and infrastructure issues.</p>
<h2>Findings</h2>
<table><tr><th>Severity</th><th>Rule</th><th>Where</th><th>Status</th><th>Standards</th><th>Fix</th></tr>
{{range .Rows}}<tr><td class="sev {{sev .Finding}}">{{sev .Finding}}</td><td>{{.Rule}}<br>{{.Message}}</td><td>{{.File}}:{{.Line}}</td><td>{{.Status}}{{if .Owner}}<br>{{.Owner}}{{end}}{{if .AcceptedReason}}<br>accepted: {{.AcceptedReason}} (until {{day .AcceptedUntil}}){{end}}</td><td>{{.CWE}}{{if .OWASP}}<br>{{.OWASP}}{{end}}</td><td>{{.Fix}}</td></tr>
{{else}}<tr><td colspan="6">No findings in this view.</td></tr>{{end}}</table>
</body></html>`))

// HTML writes a print-friendly report (use the browser's Save as PDF). html/template escapes every value.
func HTML(w io.Writer, d Data) error {
	if d.Brand.Name == "" {
		d.Brand.Name = "Sentinix"
	}
	accent := d.Brand.Accent
	if !hexColor.MatchString(accent) {
		accent = "#0b6bcb"
	}
	var rows []row
	for _, f := range d.Findings {
		rows = append(rows, row{f, owasp[strings.ToUpper(f.CWE)]})
	}
	if d.Now.IsZero() {
		d.Now = time.Now()
	}
	return page.Execute(w, struct {
		Data
		Accent template.CSS
		Rows   []row
	}{d, template.CSS(accent), rows})
}
