// Package packs loads, verifies and runs declarative rule packs.
// A pack is data only: it cannot execute anything or touch the network.
package packs

import (
	"bytes"
	"crypto/ed25519"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sentinix/pkg/engines"
)

type Manifest struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Category   string   `json:"category,omitempty"` // default pack category
	Stacks     []string `json:"stacks"` // empty = every stack
	MinVersion string   `json:"min_version,omitempty"`
	Author     string   `json:"author,omitempty"`
}

type Rule struct {
	ID           string   `json:"id"`
	Category     string   `json:"category,omitempty"` // category override
	Severity     string   `json:"severity"`
	CWE          string   `json:"cwe,omitempty"`
	Message      string   `json:"message"`
	Remediation  string   `json:"remediation,omitempty"`
	Files        []string `json:"files"`                   // basename globs
	PathContains string   `json:"path_contains,omitempty"` // e.g. "commonMain"
	Pattern      string   `json:"pattern"`
	Unless       string   `json:"unless,omitempty"`    // skip when the matched text matches this
	Multiline    bool     `json:"multiline,omitempty"` // apply to the whole file, not line by line

	re, unless *regexp.Regexp
	sev        engines.Severity
}

// Sample is a good/bad code pair. Every rule needs one before a pack loads.
type Sample struct {
	Rule string `json:"rule"`
	File string `json:"file"` // name the sample is scanned as
	Bad  string `json:"bad"`
	Good string `json:"good"`
}

type Pack struct {
	Manifest
	Rules   []*Rule  `json:"rules"`
	Samples []Sample `json:"tests"`
}

// Signed is the on-disk format: Body is the exact bytes that were signed.
type Signed struct {
	Body json.RawMessage `json:"body"`
	Sig  string          `json:"sig"`
}

func Sign(body []byte, key ed25519.PrivateKey) ([]byte, error) {
	var c bytes.Buffer // json.Marshal compacts RawMessage, so sign the compact form
	if err := json.Compact(&c, body); err != nil {
		return nil, err
	}
	body = c.Bytes()
	return json.Marshal(Signed{Body: body, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(key, body))})
}

// Parse compiles the rules and runs the pack's own tests.
func Parse(body []byte) (*Pack, error) {
	var p Pack
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	if p.Name == "" || p.Version == "" {
		return nil, fmt.Errorf("pack needs name and version")
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		var err error
		if r.sev, err = engines.ParseSeverity(r.Severity); err != nil {
			return nil, fmt.Errorf("%s: %w", r.ID, err)
		}
		pre := ""
		if r.Multiline {
			pre = "(?s)"
		}
		pat := r.Pattern
		pat = strings.ReplaceAll(pat, `\(\)`, `\(\s*\)`)
		if r.re, err = regexp.Compile(pre + pat); err != nil {
			return nil, fmt.Errorf("%s: %w", r.ID, err)
		}
		if r.Unless != "" {
			if r.unless, err = regexp.Compile(pre + r.Unless); err != nil {
				return nil, fmt.Errorf("%s: %w", r.ID, err)
			}
		}
		seen[r.ID] = false
	}
	for _, s := range p.Samples {
		if _, ok := seen[s.Rule]; !ok {
			return nil, fmt.Errorf("test for unknown rule %q", s.Rule)
		}
		seen[s.Rule] = true
		if n := len(p.match(s.File, s.Bad, s.Rule)); n == 0 {
			return nil, fmt.Errorf("rule %s: bad sample not detected", s.Rule)
		}
		if n := len(p.match(s.File, s.Good, s.Rule)); n != 0 {
			return nil, fmt.Errorf("rule %s: good sample flagged", s.Rule)
		}
	}
	for id, tested := range seen {
		if !tested {
			return nil, fmt.Errorf("rule %s has no test", id)
		}
	}
	return &p, nil
}

// Load verifies the signature against any trusted key, then parses.
func Load(data []byte, trusted []ed25519.PublicKey) (*Pack, error) {
	var s Signed
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(s.Sig)
	if err != nil {
		return nil, fmt.Errorf("bad signature encoding")
	}
	for _, k := range trusted {
		if ed25519.Verify(k, s.Body, sig) {
			return Parse(s.Body)
		}
	}
	return nil, fmt.Errorf("pack signature not trusted")
}

//go:embed builtin/*.json
var builtin embed.FS

// Builtin packs ship inside the binary, so they need no signature.
func Builtin() ([]*Pack, error) {
	var ps []*Pack
	err := fs.WalkDir(builtin, "builtin", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := builtin.ReadFile(p)
		pk, err := Parse(b)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		ps = append(ps, pk)
		return nil
	})
	return ps, err
}

// LoadDir loads every signed *.json pack in dir. One bad pack aborts: a half-loaded rule set is worse than none.
func LoadDir(dir string, trusted []ed25519.PublicKey) ([]*Pack, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var ps []*Pack
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		p, err := Load(b, trusted)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func (r *Rule) applies(file string) bool {
	if r.PathContains != "" && !strings.Contains(file, r.PathContains) {
		return false
	}
	base := filepath.Base(file)
	for _, g := range r.Files {
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
	}
	return false
}

func (r *Rule) hit(f engines.Finding, text string, line int) (engines.Finding, bool) {
	if r.unless != nil && r.unless.MatchString(text) {
		return f, false
	}
	text = strings.TrimSpace(text)
	if len(text) > 80 {
		text = text[:80]
	}
	f.Line, f.Evidence = line, text
	return f, true
}

// match runs the pack's rules (optionally just one) on a single file's content.
func (p *Pack) match(file, content, only string) []engines.Finding {
	var out []engines.Finding
	for _, r := range p.Rules {
		if (only != "" && r.ID != only) || !r.applies(file) {
			continue
		}
		cat := r.Category
		if cat == "" {
			cat = p.Category
		}
		if cat == "" {
			cat = engines.CategorySecurity
		}
		base := engines.Finding{Rule: p.Name + "/" + r.ID, Engine: "pack:" + p.Name + "@" + p.Version, Category: cat, File: file,
			Severity: r.sev, Message: r.Message, CWE: r.CWE, Fix: r.Remediation}
		if r.Multiline {
			for _, m := range r.re.FindAllStringIndex(content, -1) {
				if f, ok := r.hit(base, content[m[0]:m[1]], 1+strings.Count(content[:m[0]], "\n")); ok {
					out = append(out, f)
				}
			}
			continue
		}
		for i, l := range strings.Split(content, "\n") {
			m := r.re.FindString(l)
			if m == "" {
				normL := engines.NormalizeFindingContent(l)
				if r.re.FindString(normL) != "" {
					m = l
				}
			}
			if m != "" {
				if f, ok := r.hit(base, l, i+1); ok {
					out = append(out, f)
				}
			}
		}
	}
	return out
}
