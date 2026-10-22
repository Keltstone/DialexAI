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
	"sync"
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

const (
	CategorySecurity      = "security"
	CategoryArchitecture  = "architecture"
	CategoryStyle         = "style"
	CategoryAccessibility = "accessibility"
	CategoryQuality       = "quality"
)

var ValidCategories = []string{
	CategorySecurity,
	CategoryArchitecture,
	CategoryStyle,
	CategoryAccessibility,
	CategoryQuality,
}

func IsValidCategory(c string) bool {
	for _, v := range ValidCategories {
		if c == v {
			return true
		}
	}
	return false
}

type Finding struct {
	Rule       string   `json:"rule"`
	Engine     string   `json:"engine"`
	Category   string   `json:"category"`
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

func NormalizeCategory(c string) string {
	if c == "" || !IsValidCategory(c) {
		return CategorySecurity
	}
	return c
}

func (f Finding) NormalizedCategory() string {
	return NormalizeCategory(f.Category)
}

var (
	reCommentBlock = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reCommentXML   = regexp.MustCompile(`(?s)<!--.*?-->`)
	reCommentLine  = regexp.MustCompile(`(?m)(^\s*//.*$|\s+//.*$)`)
	reWhitespace   = regexp.MustCompile(`[ \t\r\n]+`)
	reSpacePunct   = regexp.MustCompile(`\s*([=/(,),:\[\]{}<>;])\s*`)
)

func NormalizeFindingContent(s string) string {
	s = reCommentBlock.ReplaceAllString(s, "")
	s = reCommentXML.ReplaceAllString(s, "")
	s = reCommentLine.ReplaceAllString(s, "")
	s = reWhitespace.ReplaceAllString(s, " ")
	s = reSpacePunct.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

func NormalizeFindingPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	return filepath.Base(p)
}

// Fingerprint ignores the line number so baselines survive unrelated edits.
// It includes version prefix "v2:" and hashes rule, file, content hash, and occurrence.
func (f Finding) Fingerprint() string {
	var contentHash [32]byte
	if len(f.Match) == 64 {
		if b, err := hex.DecodeString(f.Match); err == nil && len(b) == 32 {
			copy(contentHash[:], b)
		} else {
			norm := NormalizeFindingContent(f.Match)
			contentHash = sha256.Sum256([]byte(norm))
		}
	} else if f.Match != "" {
		norm := NormalizeFindingContent(f.Match)
		contentHash = sha256.Sum256([]byte(norm))
	} else {
		norm := NormalizeFindingContent(f.Evidence)
		contentHash = sha256.Sum256([]byte(norm))
	}
	normFile := NormalizeFindingPath(f.File)
	h := sha256.Sum256([]byte(fmt.Sprintf("v2|%s|%s|%x|%d", f.Rule, normFile, contentHash, f.Occurrence)))
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

var versionCache sync.Map // bin -> version; `semgrep --version` costs ~0.8s and EngineMeta runs several times per PR
// ponytail: cached for process life (key includes resolved path); a binary upgraded in place needs a restart.

func binVersion(bin string, arg string) string {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "unknown"
	}
	if v, ok := versionCache.Load(path); ok {
		return v.(string)
	}
	ver := "unknown"
	if out, err := exec.Command(path, arg).Output(); err == nil {
		ver = strings.TrimSpace(string(out))
	}
	versionCache.Store(path, ver)
	return ver
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
		ver := binVersion("semgrep", "--version")
		return EngineMetadata{Name: "semgrep", Version: ver, RulesetHash: hashStr, Status: "active"}
	case "gitleaks":
		ver := binVersion("gitleaks", "version")
		return EngineMetadata{Name: "gitleaks", Version: ver, RulesetHash: "default", Status: "active"}
	case "osv-scanner":
		ver := binVersion("osv-scanner", "--version")
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

func copyTreeForScan(src, dst string, keep func(rel string) bool) error {
	inRepoIgnores := map[string]bool{
		".semgrepignore":   true,
		".gitleaks.toml":   true,
		".gitleaksignore":  true,
		"osv-scanner.toml": true,
	}
	// ponytail: 16 workers is a guess; tune if copy still dominates on slow disks
	jobs := make(chan copyJob, 256)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				data, err := os.ReadFile(j.src)
				if err == nil {
					err = os.WriteFile(j.dst, data, j.mode)
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
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
		if keep != nil && !keep(filepath.ToSlash(rel)) {
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
		jobs <- copyJob{p, target, fi.Mode()}
		return nil
	})
	close(jobs)
	wg.Wait()
	if err == nil {
		err = firstErr
	}
	return err
}

// isDependencyFile reports whether osv-scanner could read this file. Deliberately broad.
// ponytail: filename allow-list; an ecosystem missing here silently loses SCA coverage, so extend it when osv-scanner gains one.
func isDependencyFile(rel string) bool {
	b := rel[strings.LastIndex(rel, "/")+1:]
	switch b {
	case "package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "go.mod", "go.sum",
		"pom.xml", "Pipfile", "Pipfile.lock", "pyproject.toml", "Gemfile", "composer.json", "deps.json", "packages.lock.json",
		"Package.resolved", "Podfile.lock", "pubspec.yaml", "conanfile.txt", "conanfile.py", "mix.exs", "renv.lock", "bun.lockb":
		return true
	}
	for _, suf := range []string{".gradle", ".gradle.kts", ".lock", ".lockfile", ".toml", ".csproj", ".cdx.json", ".spdx.json", ".cdx.xml", ".spdx"} {
		if strings.HasSuffix(b, suf) {
			return true
		}
	}
	return strings.HasPrefix(b, "requirements") && strings.HasSuffix(b, ".txt")
}

type scopeKey struct{}

// WithScope limits per-file engines (semgrep, gitleaks) to these repo-relative paths.
// osv-scanner always sees the whole tree: manifests/lockfiles are cross-file. A nil set means no scoping.
func WithScope(ctx context.Context, files map[string]bool) context.Context {
	return context.WithValue(ctx, scopeKey{}, files)
}

func ScopeFrom(ctx context.Context) map[string]bool {
	m, _ := ctx.Value(scopeKey{}).(map[string]bool)
	return m
}

type copyJob struct {
	src, dst string
	mode     os.FileMode
}

// removeAllParallel deletes top-level entries concurrently; unlink-heavy trees are syscall-bound.
func removeAllParallel(dir string) {
	ents, _ := os.ReadDir(dir)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, e := range ents {
		wg.Add(1)
		sem <- struct{}{}
		go func(p string) {
			defer wg.Done()
			os.RemoveAll(p)
			<-sem
		}(filepath.Join(dir, e.Name()))
	}
	wg.Wait()
	os.RemoveAll(dir)
}

func (e External) ScanContext(ctx context.Context, root string) ([]Finding, error) {
	if e.Unavailable {
		return nil, fmt.Errorf("%w: %s has no configuration", ErrEngineUnavailable, e.Bin)
	}

	scope := ScopeFrom(ctx)
	var keep func(string) bool
	if e.Bin == "osv-scanner" {
		keep = isDependencyFile // manifests/lockfiles are cross-file: all of them, not just the changed ones
	} else if scope != nil {
		if len(scope) == 0 {
			return nil, nil // nothing changed: nothing for a per-file engine to scan
		}
		keep = func(rel string) bool { return scope[rel] }
	}
	isoDir, err := os.MkdirTemp("", "sentinix-isolated-*")
	if err != nil {
		return nil, err
	}
	defer removeAllParallel(isoDir)

	if err := copyTreeForScan(root, isoDir, keep); err != nil {
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
	realIso, _ := filepath.EvalSymlinks(isoDir)
	realIsoClean := strings.TrimPrefix(filepath.ToSlash(realIso), "/")
	isoDirClean := strings.TrimPrefix(filepath.ToSlash(isoDir), "/")
	for i := range fs {
		cleanFile := strings.TrimPrefix(fs[i].File, "file://")
		cleanFile = strings.TrimPrefix(filepath.ToSlash(cleanFile), "/")
		if realIsoClean != "" {
			cleanFile = strings.TrimPrefix(cleanFile, realIsoClean)
			cleanFile = strings.TrimPrefix(cleanFile, "/")
		}
		cleanFile = strings.TrimPrefix(cleanFile, isoDirClean)
		cleanFile = strings.TrimPrefix(cleanFile, "/")
		fs[i].File = cleanFile
	}
	return fs, nil
}

func MapSARIFSeverityAndCategory(tool, ruleID, level, message, defaultCategory string) (Severity, string) {
	t := strings.ToLower(tool)
	ruleLower := strings.ToLower(ruleID)
	msgLower := strings.ToLower(message)
	lvl := strings.ToLower(level)

	cat := defaultCategory

	switch {
	case strings.Contains(t, "detekt") || strings.Contains(t, "ktlint"):
		if cat == "" {
			cat = CategoryStyle
		}
		// Spec section 3: Detekt / ktlint warning, style -> low
		return Low, cat

	case strings.Contains(t, "android-lint") || strings.Contains(t, "androidlint") || strings.Contains(t, "lint"):
		isA11y := strings.Contains(ruleLower, "accessibility") ||
			strings.Contains(ruleLower, "contentdescription") ||
			strings.Contains(ruleLower, "touchtarget") ||
			strings.Contains(ruleLower, "clickableview") ||
			strings.Contains(msgLower, "accessibility") ||
			strings.Contains(msgLower, "contentdescription")
		if isA11y {
			if cat == "" {
				cat = CategoryAccessibility
			}
			// Spec section 3: Android Lint a11y error -> high; warning -> medium
			if lvl == "error" {
				return High, cat
			}
			return Medium, cat
		}

		isSec := strings.Contains(ruleLower, "security") ||
			strings.Contains(ruleLower, "exported") ||
			strings.Contains(ruleLower, "cleartext") ||
			strings.Contains(ruleLower, "trustall") ||
			strings.Contains(ruleLower, "debuggable")
		if isSec {
			if cat == "" {
				cat = CategorySecurity
			}
			if lvl == "error" {
				return High, cat
			}
			return Medium, cat
		}

		if cat == "" {
			cat = CategoryQuality
		}
		// Spec section 3: Lint other error / warning -> medium / low
		if lvl == "error" {
			return Medium, cat
		}
		return Low, cat

	case strings.Contains(t, "konsist"):
		if cat == "" {
			cat = CategoryArchitecture
		}
		// Spec section 3: Konsist failure (architecture) -> high
		return High, cat

	default:
		if cat == "" {
			cat = CategorySecurity
		}
		switch lvl {
		case "error":
			return High, cat
		case "note":
			return Low, cat
		default:
			return Medium, cat
		}
	}
}

func ParseSARIF(engine string, data []byte) ([]Finding, error) {
	return ParseSARIFWithCategory(engine, data, "")
}

func ParseSARIFWithCategory(engine string, data []byte, defaultCategory string) ([]Finding, error) {
	type notification struct {
		Level   string `json:"level"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	var s struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Name string `json:"name"`
				} `json:"driver"`
			} `json:"tool"`
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
		toolName := engine
		if r.Tool.Driver.Name != "" {
			toolName = r.Tool.Driver.Name
		}
		for _, x := range r.Results {
			sev, cat := MapSARIFSeverityAndCategory(toolName, x.RuleID, x.Level, x.Message.Text, defaultCategory)
			f := Finding{Rule: x.RuleID, Engine: engine, Category: cat, Message: x.Message.Text, Severity: sev}
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
				uri := loc.Physical.Artifact.URI
				uri = strings.TrimPrefix(uri, "file://")
				cleanFile := filepath.ToSlash(filepath.Clean(uri))
				if strings.Contains(uri, "..") || strings.HasPrefix(cleanFile, "../") || cleanFile == ".." || (filepath.IsAbs(uri) && strings.Contains(uri, "..")) {
					// Drop hostile result per finding instead of failing whole report
					continue
				}
				if !filepath.IsAbs(uri) {
					cleanFile = strings.TrimPrefix(cleanFile, "/")
				}
				f.File = cleanFile
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

// reAssignedSecret masks the value of secret-looking assignments regardless of length or entropy
// (`password = "hunter2"`), which the vendor rules and the entropy fallback both miss.
var reAssignedSecret = regexp.MustCompile(`(?i)((?:pass(?:word|wd)?|pwd|secret|token|api[_-]?key|auth(?:orization)?|credential)s?["']?\s*[:=]\s*["']?)([^\s"',;)]{4,})`)

var rePEMBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`)

var reStringConcat = regexp.MustCompile(`"([^"\n]+)"\s*\+\s*"([^"\n]+)"`)

var wordRe = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{20,}`)

func RedactSafe(s string) (string, error) {
	if len(secretRules) == 0 {
		return "", fmt.Errorf("redaction unavailable: no rules loaded")
	}
	// Detect split string concatenations (e.g. "AKIA" + "IOSFODNN7EXAMPLQ") attempting to evade rules
	s = reStringConcat.ReplaceAllStringFunc(s, func(m string) string {
		sub := reStringConcat.FindStringSubmatch(m)
		if len(sub) == 3 {
			combined := sub[1] + sub[2]
			for _, r := range secretRules {
				if r.Regex.MatchString(combined) {
					return mask(combined)
				}
			}
			if len(combined) >= 20 && ShannonEntropy(combined) >= 3.6 {
				return mask(combined)
			}
		}
		return m
	})
	for _, r := range secretRules {
		s = r.Regex.ReplaceAllStringFunc(s, func(m string) string { return mask(m) })
	}
	s = rePEMBlock.ReplaceAllString(s, "-----BEGIN PRIVATE KEY-----****")
	s = reAssignedSecret.ReplaceAllString(s, "${1}****")
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
