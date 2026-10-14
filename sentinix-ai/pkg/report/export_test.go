package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"sentinix/pkg/advisories"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

func finding(rule, msg, ev string) store.Finding {
	return store.Finding{Finding: engines.Finding{Rule: rule, Message: msg, Evidence: ev, Severity: engines.High, CWE: "CWE-89"}, Status: "open"}
}

func TestCSVNeutralisesFormulas(t *testing.T) {
	var b bytes.Buffer
	CSV(&b, []store.Finding{finding("r", "=HYPERLINK(\"x\")", "")})
	if !strings.Contains(b.String(), "'=HYPERLINK") {
		t.Fatal(b.String())
	}
}

func TestSBOMHasVulnerabilityWithFix(t *testing.T) {
	var b bytes.Buffer
	f := finding("advisory/GHSA-1", "bad", "flask@2.0.0")
	f.Fix = "upgrade to 2.2.5 or later"
	if err := SBOM(&b, "p", []advisories.Component{{Ecosystem: "PyPI", Name: "Flask", Version: "2.0.0"}}, []store.Finding{f}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		BOMFormat string
		Vulns     []struct {
			ID, Recommendation string
			Affects            []struct{ Ref string }
		} `json:"vulnerabilities"`
	}
	json.Unmarshal(b.Bytes(), &out)
	if out.BOMFormat != "CycloneDX" || len(out.Vulns) != 1 || out.Vulns[0].Recommendation == "" {
		t.Fatalf("%s", b.String())
	}
}

func TestHTMLEscapesAndKeepsCoverage(t *testing.T) {
	var b bytes.Buffer
	f := finding("r", "<script>alert(1)</script>", "")
	HTML(&b, Data{Brand: Brand{Name: "Acme", Accent: "red;}</style><script>"}, Project: store.Project{Name: "p"}, Findings: []store.Finding{f},
		Scan: &store.Scan{Engines: "secrets"}})
	s := b.String()
	if strings.Contains(s, "<script>alert") || strings.Contains(s, "</style><script>") || !strings.Contains(s, "Not covered") || !strings.Contains(s, "A03 Injection") {
		t.Fatal(s)
	}
}
