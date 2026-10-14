package packs

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"sentinix/pkg/engines"
)

// Engine runs the packs whose stacks match what the repo contains.
type Engine struct {
	Packs    []*Pack
	Disabled map[string]bool             // full rule ids, e.g. "android/webview-js"; changes go through the approval queue
	Override map[string]engines.Severity // per-rule severity
}

func (Engine) Name() string { return "packs" }

func skipDir(n string) bool { return n == ".git" || n == "node_modules" || n == "build" }

// DetectStacks looks at file names and build files only.
func DetectStacks(root string) map[string]bool {
	st := map[string]bool{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		n := d.Name()
		if d.IsDir() {
			if skipDir(n) {
				return filepath.SkipDir
			}
			if n == "commonMain" {
				st["kmp"], st["kotlin"] = true, true
			}
			return nil
		}
		switch {
		case n == "AndroidManifest.xml":
			st["android"] = true
		case strings.HasSuffix(n, ".kt"), strings.HasSuffix(n, ".kts"):
			st["kotlin"] = true
			if strings.HasPrefix(n, "build.gradle") {
				b, _ := os.ReadFile(p)
				s := string(b)
				if strings.Contains(s, "multiplatform") {
					st["kmp"] = true
				}
				if strings.Contains(s, "com.android.") {
					st["android"] = true
				}
			}
		}
		return nil
	})
	return st
}

func (e Engine) active(st map[string]bool) []*Pack {
	var out []*Pack
	for _, p := range e.Packs {
		on := len(p.Stacks) == 0
		for _, s := range p.Stacks {
			on = on || st[s]
		}
		if on {
			out = append(out, p)
		}
	}
	return out
}

func (e Engine) Scan(root string) ([]engines.Finding, error) {
	active := e.active(DetectStacks(root))
	var out []engines.Finding
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if info, e := d.Info(); e != nil || info.Size() > 1<<20 {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		var content string
		for _, pk := range active {
			if content == "" {
				b, _ := os.ReadFile(p)
				content = string(b)
			}
			for _, f := range pk.match(rel, content, "") {
				if e.Disabled[f.Rule] {
					continue
				}
				if s, ok := e.Override[f.Rule]; ok {
					f.Severity = s
				}
				out = append(out, f)
			}
		}
		return nil
	})
	return out, err
}

// ReadKeys reads base64 ed25519 public keys, one per line.
func ReadKeys(path string) ([]ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ks []ed25519.PublicKey
	for _, l := range strings.Fields(string(data)) {
		b, err := base64.StdEncoding.DecodeString(l)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%s: bad public key", path)
		}
		ks = append(ks, b)
	}
	return ks, nil
}

// NewEngine returns the built-in packs plus any signed packs in dir (dir "" = built-ins only).
func NewEngine(dir, trustFile string) (Engine, error) {
	ps, err := Builtin()
	if err != nil || dir == "" {
		return Engine{Packs: ps}, err
	}
	keys, err := ReadKeys(trustFile)
	if err != nil {
		return Engine{}, err
	}
	extra, err := LoadDir(dir, keys)
	return Engine{Packs: append(ps, extra...)}, err
}
