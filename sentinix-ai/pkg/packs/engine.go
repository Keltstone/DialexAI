package packs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"sentinix/pkg/engines"
)

var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".webp": true, ".pdf": true, ".zip": true, ".tar": true, ".gz": true,
	".7z": true, ".rar": true, ".exe": true, ".dll": true, ".so": true,
	".dylib": true, ".class": true, ".jar": true, ".war": true, ".ear": true,
	".aar": true, ".bin": true, ".iso": true, ".dmg": true, ".mp3": true,
	".mp4": true, ".mov": true, ".avi": true, ".wasm": true, ".pyc": true,
	".o": true, ".a": true, ".keystore": true, ".jks": true,
}

func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return binaryExts[ext]
}

// Engine runs the packs whose stacks match what the repo contains.
type Engine struct {
	Packs        []*Pack
	Disabled     map[string]bool             // full rule ids, e.g. "android/webview-js"; changes go through the approval queue
	Override     map[string]engines.Severity // per-rule severity
	SkippedFiles []string
}

func (Engine) Name() string { return "packs" }

func (e Engine) Skipped() []string {
	if len(e.SkippedFiles) > 0 {
		return e.SkippedFiles
	}
	return nil
}

func skipDir(n string) bool { return n == ".git" || n == "node_modules" || n == "build" }

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) != -1
}

// DetectStacks looks at file names and build files only.
func DetectStacks(root string) (map[string]bool, []string) {
	st := map[string]bool{}
	var skipped []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			skipped = append(skipped, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		n := d.Name()
		if d.IsDir() {
			if n == ".git" {
				return filepath.SkipDir
			}
			if n == "node_modules" || n == "build" {
				skipped = append(skipped, rel)
				return filepath.SkipDir
			}
			if n == "commonMain" {
				st["kmp"], st["kotlin"] = true, true
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			skipped = append(skipped, rel)
			return nil
		}
		if info.Size() > 1<<20 {
			skipped = append(skipped, rel)
			return nil
		}

		// Check binary
		f, err := os.Open(p)
		if err == nil {
			head := make([]byte, 8000)
			nR, _ := io.ReadFull(f, head)
			f.Close()
			if nR > 0 && isBinary(head[:nR]) {
				skipped = append(skipped, rel)
				return nil
			}
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
	return st, skipped
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

func (e Engine) Scan(ctx context.Context, root string) ([]engines.Finding, error) {
	fs, _, err := e.ScanWithSkipped(ctx, root)
	return fs, err
}

func (e Engine) ScanWithSkipped(ctx context.Context, root string) ([]engines.Finding, []string, error) {
	st, skipped := DetectStacks(root)
	active := e.active(st)
	var out []engines.Finding
	occMap := map[string]int{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}

		info, err := os.Lstat(p)
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			skipped = append(skipped, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if d.Name() == "node_modules" || d.Name() == "build" {
				skipped = append(skipped, rel)
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			skipped = append(skipped, rel)
			return nil
		}
		if info.Size() > 1<<20 {
			skipped = append(skipped, rel)
			return nil
		}

		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		if isBinary(b) {
			if !isBinaryFile(rel) {
				out = append(out, engines.Finding{
					Rule:     "file/nul-byte",
					Engine:   "packs",
					Category: engines.CategorySecurity,
					File:     rel,
					Line:     1,
					Severity: engines.Info,
					Message:  "file contains NUL bytes",
				})
			} else {
				skipped = append(skipped, rel)
				return nil
			}
		}
		content := string(b)

		for _, pk := range active {
			for _, f := range pk.match(rel, content, "") {
				if e.Disabled[f.Rule] {
					continue
				}
				if s, ok := e.Override[f.Rule]; ok {
					f.Severity = s
				}
				m := f.Match
				if m == "" {
					m = f.Evidence
				}
				normM := engines.NormalizeFindingContent(m)
				mHash := sha256.Sum256([]byte(normM))
				occKey := fmt.Sprintf("%s|%x", f.Rule, mHash)
				f.Occurrence = occMap[occKey]
				occMap[occKey]++
				out = append(out, f)
			}
		}
		return nil
	})
	return out, skipped, err
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

// Versions lists "name@version" for the loaded packs, recorded with every scan.
func (e Engine) Versions() string {
	var v []string
	for _, p := range e.Packs {
		v = append(v, p.Name+"@"+p.Version)
	}
	return strings.Join(v, ",")
}
