// Package engines defines the engine plugin interface and one finding schema.
package engines

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type Severity int

const (
	Info Severity = iota
	Low
	Medium
	High
	Critical
)

var sevNames = []string{"info", "low", "medium", "high", "critical"}

func (s Severity) String() string { return sevNames[s] }

func ParseSeverity(v string) (Severity, error) {
	for i, n := range sevNames {
		if n == strings.ToLower(v) {
			return Severity(i), nil
		}
	}
	return 0, fmt.Errorf("unknown severity %q", v)
}

type Finding struct {
	Rule     string   `json:"rule"`
	Engine   string   `json:"engine"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Evidence string   `json:"evidence"` // masked, short
	CWE      string   `json:"cwe,omitempty"`
	Fix      string   `json:"fix,omitempty"` // remediation text
}

// Fingerprint ignores the line number so baselines survive unrelated edits.
func (f Finding) Fingerprint() string {
	h := sha256.Sum256([]byte(f.Rule + "|" + f.File + "|" + f.Evidence))
	return hex.EncodeToString(h[:8])
}

type Engine interface {
	Name() string
	Scan(root string) ([]Finding, error)
}

// Bundled returns the built-in engine plus external engines found on PATH.
// Missing binaries are skipped; the report's coverage line says what ran.
func Bundled(extra ...Engine) []Engine {
	es := append([]Engine{Secrets{}}, extra...)
	for _, e := range []External{
		{"semgrep", []string{"scan", "--config", "auto", "--sarif", "--quiet"}},
		{"gitleaks", []string{"detect", "--no-git", "--report-format", "sarif", "--report-path", "/dev/stdout", "--no-banner", "-s"}},
		{"osv-scanner", []string{"--format", "sarif", "-r"}},
	} {
		if _, err := exec.LookPath(e.Bin); err == nil {
			es = append(es, e)
		}
	}
	return es
}

// External wraps a binary that emits SARIF on stdout. Root is appended last.
type External struct {
	Bin  string
	Args []string
}

func (e External) Name() string { return e.Bin }

func (e External) Scan(root string) ([]Finding, error) {
	cmd := exec.Command(e.Bin, append(append([]string{}, e.Args...), root)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")} // no server tokens in scan processes
	out, err := cmd.Output()
	if len(out) == 0 && err != nil { // scanners exit non-zero when they find things
		return nil, fmt.Errorf("%s: %w", e.Bin, err)
	}
	return ParseSARIF(e.Bin, out)
}

func ParseSARIF(engine string, data []byte) ([]Finding, error) {
	var s struct {
		Runs []struct {
			Results []struct {
				RuleID    string                `json:"ruleId"`
				Level     string                `json:"level"`
				Message   struct{ Text string } `json:"message"`
				Locations []struct {
					Physical struct {
						Artifact struct{ URI string }    `json:"artifactLocation"`
						Region   struct{ StartLine int } `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: bad SARIF: %w", engine, err)
	}
	var fs []Finding
	for _, r := range s.Runs {
		for _, x := range r.Results {
			f := Finding{Rule: x.RuleID, Engine: engine, Message: x.Message.Text, Severity: Medium}
			switch x.Level {
			case "error":
				f.Severity = High
			case "note":
				f.Severity = Low
			}
			if len(x.Locations) > 0 {
				f.File = x.Locations[0].Physical.Artifact.URI
				f.Line = x.Locations[0].Physical.Region.StartLine
			}
			fs = append(fs, f)
		}
	}
	return fs, nil
}

var secretRules = []struct {
	id string
	re *regexp.Regexp
}{
	{"secret.aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"secret.private-key", regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`)},
	{"secret.generic-assignment", regexp.MustCompile(`(?i)(api[_-]?key|secret|passwd|password|token)\s*[:=]\s*["'][A-Za-z0-9/+_\-]{16,}["']`)},
}

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****"
}
