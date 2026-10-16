// Package engines defines the engine plugin interface and one finding schema.
package engines

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	Rule       string   `json:"rule"`
	Engine     string   `json:"engine"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Severity   Severity `json:"severity"`
	Message    string   `json:"message"`
	Evidence   string   `json:"evidence"` // masked, short
	CWE        string   `json:"cwe,omitempty"`
	Fix        string   `json:"fix,omitempty"` // remediation text
	Match      string   `json:"match,omitempty"`
	Occurrence int      `json:"occurrence,omitempty"`
}

// Fingerprint ignores the line number so baselines survive unrelated edits.
// It includes version prefix "v2:" and hashes rule, file, content hash, and occurrence.
func (f Finding) Fingerprint() string {
	var contentHash [32]byte
	if len(f.Match) == 64 {
		if b, err := hex.DecodeString(f.Match); err == nil && len(b) == 32 {
			copy(contentHash[:], b)
		} else {
			contentHash = sha256.Sum256([]byte(f.Match))
		}
	} else if f.Match != "" {
		contentHash = sha256.Sum256([]byte(f.Match))
	} else {
		contentHash = sha256.Sum256([]byte(f.Evidence))
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("v2|%s|%s|%x|%d", f.Rule, f.File, contentHash, f.Occurrence)))
	return "v2:" + hex.EncodeToString(h[:8])
}

type Engine interface {
	Name() string
	Scan(ctx context.Context, root string) ([]Finding, error)
}

// Bundled returns the built-in engine plus external engines found on PATH.
// Missing binaries are skipped; the report's coverage line says what ran.
func Bundled(extra ...Engine) []Engine {
	hasSecrets := false
	var es []Engine
	for _, e := range extra {
		if s, ok := e.(Secrets); ok {
			hasSecrets = true
			if s.SkippedFiles == nil {
				s.SkippedFiles = new([]string)
			}
			es = append(es, s)
		} else {
			es = append(es, e)
		}
	}
	if !hasSecrets {
		es = append([]Engine{Secrets{SkippedFiles: new([]string)}}, es...)
	}
	semgrepCfg := os.Getenv("SENTINIX_SEMGREP_CONFIG") // inside a sandbox point at rules baked into the image (/rules)
	if semgrepCfg == "" {
		semgrepCfg = "/rules"
	}
	osvCfg := os.Getenv("SENTINIX_OSV_CONFIG")
	if osvCfg == "" {
		osvCfg = "/dev/null"
	}
	for _, e := range []External{
		{"semgrep", []string{"scan", "--config", semgrepCfg, "--sarif", "--quiet", "--no-git-ignore", "--disable-nosem"}, nil},
		{"gitleaks", []string{"detect", "--no-git", "--report-format", "sarif", "--report-path", "/dev/stdout", "--no-banner", "--ignore-gitleaks-allow", "-s"}, nil},
		{"osv-scanner", []string{"--format", "sarif", "-r", "--config", osvCfg}, nil},
	} {
		// in a sandbox the binaries live in the image, not on the host
		if _, err := exec.LookPath(e.Bin); err == nil || Sandbox != nil {
			es = append(es, e)
		}
	}
	return es
}

// Sandbox, when set, builds the command that runs an external engine inside a container (see package sandbox).
// ponytail: package-level switch set once at startup; make it a per-engine option if scans ever differ.
var Sandbox func(root, bin string, args []string) *exec.Cmd

// External wraps a binary that emits SARIF on stdout. Root is appended last.
type External struct {
	Bin     string
	Args    []string
	Context context.Context
}

func (e External) Name() string { return e.Bin }

func (e External) Scan(ctx context.Context, root string) ([]Finding, error) {
	return e.ScanContext(ctx, root)
}

func (e External) ScanContext(ctx context.Context, root string) ([]Finding, error) {
	// Ignore in-repo .semgrepignore, .gitleaks.toml, .gitleaksignore, osv-scanner.toml by temporarily isolating them recursively
	inRepoIgnores := map[string]bool{
		".semgrepignore":   true,
		".gitleaks.toml":   true,
		".gitleaksignore":  true,
		"osv-scanner.toml": true,
	}
	var restores []func()
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !fi.IsDir() && inRepoIgnores[fi.Name()] {
			tmp := p + ".sentinix-isolated"
			if err := os.Rename(p, tmp); err == nil {
				restores = append(restores, func() {
					_ = os.Rename(tmp, p)
				})
			}
		}
		return nil
	})
	defer func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}()

	var cmd *exec.Cmd
	if Sandbox != nil {
		scmd := Sandbox(root, e.Bin, e.Args)
		cmd = exec.CommandContext(ctx, scmd.Path, scmd.Args[1:]...)
		cmd.Env = scmd.Env
		cmd.Dir = scmd.Dir
	} else {
		cmd = exec.CommandContext(ctx, e.Bin, append(append([]string{}, e.Args...), root)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")} // no server tokens in scan processes
	}
	cmd.WaitDelay = 200 * time.Millisecond
	out, err := cmd.Output()
	if len(out) == 0 && err != nil { // scanners exit non-zero when they find things
		return nil, fmt.Errorf("%s: %w", e.Bin, err)
	}
	return ParseSARIF(e.Bin, out)
}

func ParseSARIF(engine string, data []byte) ([]Finding, error) {
	type notification struct {
		Level   string `json:"level"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	var s struct {
		Runs []struct {
			Invocations []struct {
				ExecutionSuccessful        *bool          `json:"executionSuccessful"`
				ToolExecutionNotifications []notification `json:"toolExecutionNotifications"`
			} `json:"invocations"`
			ToolExecutionNotifications []notification `json:"toolExecutionNotifications"`
			Results                    []struct {
				RuleID              string                `json:"ruleId"`
				Level               string                `json:"level"`
				Message             struct{ Text string } `json:"message"`
				PartialFingerprints map[string]string     `json:"partialFingerprints"`
				Locations           []struct {
					Physical struct {
						Artifact struct{ URI string } `json:"artifactLocation"`
						Region   struct {
							StartLine int                   `json:"startLine"`
							Snippet   struct{ Text string } `json:"snippet"`
						} `json:"region"`
						ContextRegion struct {
							Snippet struct{ Text string } `json:"snippet"`
						} `json:"contextRegion"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: bad SARIF: %w", engine, err)
	}

	for _, r := range s.Runs {
		for _, notif := range r.ToolExecutionNotifications {
			return nil, fmt.Errorf("%s: did not fully run: %s", engine, notif.Message.Text)
		}
		for _, inv := range r.Invocations {
			for _, notif := range inv.ToolExecutionNotifications {
				return nil, fmt.Errorf("%s: did not fully run: %s", engine, notif.Message.Text)
			}
		}
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
			if fp, ok := x.PartialFingerprints["primaryLocationLineHash"]; ok && fp != "" {
				f.Match = fp
			} else if len(x.PartialFingerprints) > 0 {
				for _, v := range x.PartialFingerprints {
					if v != "" {
						f.Match = v
						break
					}
				}
			}
			if len(x.Locations) > 0 {
				loc := x.Locations[0]
				f.File = loc.Physical.Artifact.URI
				f.Line = loc.Physical.Region.StartLine
				snip := loc.Physical.Region.Snippet.Text
				if snip == "" {
					snip = loc.Physical.ContextRegion.Snippet.Text
				}
				if snip != "" {
					f.Evidence = Redact(snip)
					if f.Match == "" {
						h := sha256.Sum256([]byte(f.Evidence))
						f.Match = hex.EncodeToString(h[:])
					}
				}
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
	{"secret.github-token", regexp.MustCompile(`(gh[pousr]_[A-Za-z0-9_]{36,}|github_pat_[A-Za-z0-9_]{82})`)},
	{"secret.db-url", regexp.MustCompile(`(?i)[a-z0-9+.-]+://[^:\s]+:[^@\s]+@[^/\s]+/\S*`)},
	{"secret.high-entropy", regexp.MustCompile(`(?i)(bearer\s+[A-Za-z0-9_\-\.]{24,}|eyJ[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}|(?:entropy)\s*[:=]\s*["'][A-Za-z0-9+/=_\-\.]{20,}["'])`)},
	{"secret.generic-assignment", regexp.MustCompile(`(?i)(api[_-]?key|secret|passwd|password|token)\s*[:=]\s*["'][^"'\r\n]{16,}["']`)},
}

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****"
}

// Redact masks anything the secret rules recognise, for text that may leave the network (model prompts).
func Redact(s string) string {
	for _, r := range secretRules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string { return mask(m) })
	}
	return s
}
