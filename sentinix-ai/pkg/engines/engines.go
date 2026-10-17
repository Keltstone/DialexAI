// Package engines defines the engine plugin interface and one finding schema.
package engines

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	Source     string   `json:"source,omitempty"`
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
var ErrEngineUnavailable = errors.New("ENGINE_UNAVAILABLE")

type EngineMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	RulesetHash string `json:"ruleset_hash"`
	Status      string `json:"status"` // "active", "ENGINE_UNAVAILABLE"
}

func resolveSemgrepConfig() (string, bool) {
	if cfg := os.Getenv("SENTINIX_SEMGREP_CONFIG"); cfg != "" {
		if _, err := os.Stat(cfg); err == nil {
			return cfg, true
		}
	}
	if _, err := os.Stat("/rules"); err == nil {
		return "/rules", true
	}
	for _, p := range []string{"rules/default.yml", "../rules/default.yml", "../../rules/default.yml"} {
		if _, err := os.Stat(p); err == nil {
			if abs, err := filepath.Abs(p); err == nil {
				return abs, true
			}
			return p, true
		}
	}
	return "", false
}

func EngineMeta(name string) EngineMetadata {
	switch name {
	case "secrets":
		h := sha256.Sum256([]byte(GitleaksRulesVersion))
		return EngineMetadata{Name: "secrets", Version: GitleaksRulesVersion, RulesetHash: hex.EncodeToString(h[:8]), Status: "active"}
	case "packs":
		return EngineMetadata{Name: "packs", Version: "1.0.0", RulesetHash: "builtin", Status: "active"}
	case "semgrep":
		cfg, ok := resolveSemgrepConfig()
		if !ok {
			return EngineMetadata{Name: "semgrep", Version: "unknown", RulesetHash: "none", Status: "ENGINE_UNAVAILABLE"}
		}
		var hashStr string
		if b, err := os.ReadFile(cfg); err == nil {
			h := sha256.Sum256(b)
			hashStr = hex.EncodeToString(h[:8])
		} else {
			hashStr = "rules"
		}
		ver := "unknown"
		if out, err := exec.Command("semgrep", "--version").Output(); err == nil {
			ver = strings.TrimSpace(string(out))
		}
		return EngineMetadata{Name: "semgrep", Version: ver, RulesetHash: hashStr, Status: "active"}
	case "gitleaks":
		ver := "unknown"
		if out, err := exec.Command("gitleaks", "version").Output(); err == nil {
			ver = strings.TrimSpace(string(out))
		}
		return EngineMetadata{Name: "gitleaks", Version: ver, RulesetHash: "default", Status: "active"}
	case "osv-scanner":
		ver := "unknown"
		if out, err := exec.Command("osv-scanner", "--version").Output(); err == nil {
			ver = strings.TrimSpace(string(out))
		}
		return EngineMetadata{Name: "osv-scanner", Version: ver, RulesetHash: "default", Status: "active"}
	default:
		return EngineMetadata{Name: name, Version: "1.0.0", RulesetHash: "default", Status: "active"}
	}
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
	semgrepCfg, semgrepHasConfig := resolveSemgrepConfig()
	osvCfg := os.Getenv("SENTINIX_OSV_CONFIG")
	if osvCfg == "" {
		osvCfg = "/dev/null"
	}
	for _, e := range []External{
		{"semgrep", []string{"scan", "--config", semgrepCfg, "--sarif", "--quiet", "--no-git-ignore", "--disable-nosem"}, nil, !semgrepHasConfig},
		{"gitleaks", []string{"detect", "--no-git", "--report-format", "sarif", "--report-path", "-", "--no-banner", "--ignore-gitleaks-allow", "-s"}, nil, false},
		{"osv-scanner", []string{"--format", "sarif", "-r", "--config", osvCfg}, nil, false},
	} {
		// in a sandbox the binaries live in the image, not on the host
		if _, err := exec.LookPath(e.Bin); err == nil || Sandbox != nil {
			es = append(es, e)
		}
	}
	return es
}

// Sandbox, when set, builds the command that runs an external engine inside a container (see package sandbox).
var Sandbox func(root, bin string, args []string) *exec.Cmd

// External wraps a binary that emits SARIF on stdout. Root is appended last.
type External struct {
	Bin         string
	Args        []string
	Context     context.Context
	Unavailable bool
}

func (e External) Name() string { return e.Bin }

func (e External) Scan(ctx context.Context, root string) ([]Finding, error) {
	return e.ScanContext(ctx, root)
}

func copyTreeForScan(src, dst string) error {
	inRepoIgnores := map[string]bool{
		".semgrepignore":   true,
		".gitleaks.toml":   true,
		".gitleaksignore":  true,
		"osv-scanner.toml": true,
	}
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if inRepoIgnores[fi.Name()] {
			return nil
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if filepath.IsAbs(linkTarget) || strings.HasPrefix(linkTarget, "..") {
				return nil
			}
			return os.Symlink(linkTarget, target)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, fi.Mode())
	})
}

func (e External) ScanContext(ctx context.Context, root string) ([]Finding, error) {
	if e.Unavailable {
		return nil, fmt.Errorf("%w: %s has no configuration", ErrEngineUnavailable, e.Bin)
	}

	isoDir, err := os.MkdirTemp("", "sentinix-isolated-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(isoDir)

	if err := copyTreeForScan(root, isoDir); err != nil {
		return nil, err
	}

	// Place an empty .semgrepignore in the isolated copy to override default ignore patterns
	_ = os.WriteFile(filepath.Join(isoDir, ".semgrepignore"), []byte{}, 0o644)

	var cmd *exec.Cmd
	if Sandbox != nil {
		scmd := Sandbox(isoDir, e.Bin, e.Args)
		cmd = exec.CommandContext(ctx, scmd.Path, scmd.Args[1:]...)
		cmd.Env = scmd.Env
		cmd.Dir = scmd.Dir
	} else {
		cmd = exec.CommandContext(ctx, e.Bin, append(append([]string{}, e.Args...), isoDir)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")} // no server tokens in scan processes
	}
	cmd.WaitDelay = 200 * time.Millisecond
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if len(out) == 0 && err != nil { // scanners exit non-zero when they find things
		if e.Bin == "osv-scanner" && (strings.Contains(stderr.String(), "No package sources found") || strings.Contains(err.Error(), "128")) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w: %s", e.Bin, err, stderr.String())
	}
	fs, err := ParseSARIF(e.Bin, out)
	if err != nil {
		return nil, err
	}
	for i := range fs {
		cleanFile := strings.TrimPrefix(fs[i].File, "file://")
		cleanFile = strings.TrimPrefix(cleanFile, isoDir)
		cleanFile = strings.TrimPrefix(cleanFile, "/")
		fs[i].File = cleanFile
	}
	return fs, nil
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

var secretRules = gitleaksRules

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****"
}

// Redact masks anything the secret rules recognise, for text that may leave the network (model prompts).
func Redact(s string) string {
	res, _ := RedactSafe(s)
	return res
}

var wordRe = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{20,}`)

func RedactSafe(s string) (string, error) {
	if len(secretRules) == 0 {
		return "", fmt.Errorf("redaction unavailable: no rules loaded")
	}
	for _, r := range secretRules {
		s = r.Regex.ReplaceAllStringFunc(s, func(m string) string { return mask(m) })
	}
	// High entropy fallback for remaining unmasked candidate tokens
	s = wordRe.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "****") {
			return m
		}
		if len(m) >= 20 && ShannonEntropy(m) >= 3.6 {
			return mask(m)
		}
		return m
	})
	return s, nil
}

var savedRules []SecretRule

func CorruptRulesForTest() {
	savedRules = secretRules
	secretRules = nil
}

func RestoreRulesForTest() {
	if savedRules != nil {
		secretRules = savedRules
		savedRules = nil
	}
}



