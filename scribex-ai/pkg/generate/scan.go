// Package generate plans and applies on-demand, whole-repo documentation generation. Plan makes no
// model calls; Apply writes only the outputs named in a plan and ends with a green `scribex check`.
package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"scribex/pkg/symbols"
)

const (
	maxSourceSize = 2 << 20
	maxSymbolsPer = 25 // symbols documented per model call
)

// SrcFile is one analysable source file.
type SrcFile struct {
	Rel    string
	Hash   string // first 12 hex of sha256, same as derived.HashFile
	Size   int64
	Parser string // ast | lexical
	Syms   []Sym  // public symbols in source order
	Module string
}

// Sym is a documentable public symbol.
type Sym struct {
	Key, Kind, Sig, Doc string
	Line                int
}

// Repo is the scanned source tree.
type Repo struct {
	Root  string
	Files []*SrcFile // sorted by Rel
	Skips []string   // "path: reason"
	Tests []string   // repo-relative test files (no symbols extracted); read only by test-oriented doc types
	byRel map[string]*SrcFile
}

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "build": true, "dist": true, "target": true, "out": true,
	"__pycache__": true, "venv": true, "env": true, "Pods": true, "DerivedData": true,
}

var testDirs = map[string]bool{"test": true, "tests": true, "androidTest": true, "__tests__": true, "testdata": true, "fixtures": true}

// testSrcDirs hold test sources worth reading; testdata and fixtures are skipped outright.
var testSrcDirs = map[string]bool{"test": true, "tests": true, "androidTest": true, "__tests__": true}

var moduleMarkers = map[string]bool{
	"build.gradle": true, "build.gradle.kts": true, "go.mod": true, "package.json": true,
	"pom.xml": true, "pyproject.toml": true, "setup.py": true,
}

func isTestFile(rel string) bool {
	b := filepath.Base(rel)
	for _, s := range []string{"_test.go", "Test.kt", "Tests.kt", "Test.java", "Tests.java", ".test.ts", ".spec.ts", ".test.tsx", ".spec.tsx", ".test.js", ".spec.js", "_test.py"} {
		if strings.HasSuffix(b, s) {
			return true
		}
	}
	return strings.HasPrefix(b, "test_") && strings.HasSuffix(b, ".py") || strings.HasSuffix(b, ".d.ts")
}

func inTestDir(rel string) bool {
	segs := strings.Split(rel, "/")
	for _, s := range segs[:len(segs)-1] {
		if testSrcDirs[s] {
			return true
		}
	}
	return false
}

// TestsFor returns test files that plausibly exercise the given source files: same directory, or a
// test whose name contains a source file's stem within the same top-level module.
func (r *Repo) TestsFor(files []*SrcFile) []string {
	dirs, stems := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		dirs[path.Dir(f.Rel)] = true
		b := path.Base(f.Rel)
		stems[strings.ToLower(strings.TrimSuffix(b, path.Ext(b)))] = true
	}
	var out []string
	for _, t := range r.Tests {
		tb := strings.ToLower(path.Base(t))
		hit := dirs[path.Dir(t)]
		for st := range stems {
			if !hit && len(st) >= 4 && strings.Contains(tb, st) {
				hit = true
			}
		}
		if hit {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

func hash12(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:12]
}

// ScanRepo walks root, extracting public symbols from every supported source file. It never follows
// symlinks, skips build/vendor/test trees, and records each skip with a reason.
func ScanRepo(root string) (*Repo, error) {
	walkRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	r := &Repo{Root: root, byRel: map[string]*SrcFile{}}
	markerDirs := map[string]bool{}
	err = filepath.WalkDir(walkRoot, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(walkRoot, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		name := d.Name()
		if d.Type()&os.ModeSymlink != 0 {
			r.Skips = append(r.Skips, rel+": symlink")
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || skipDirs[name] || (testDirs[name] && !testSrcDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		if moduleMarkers[name] {
			dir := filepath.ToSlash(filepath.Dir(rel))
			markerDirs[dir] = true
		}
		if !symbols.Supported(rel) {
			return nil
		}
		if isTest := isTestFile(rel) || inTestDir(rel); isTest {
			if fi, ierr := d.Info(); ierr == nil && fi.Mode().IsRegular() && fi.Size() <= maxSourceSize {
				r.Tests = append(r.Tests, rel)
			}
			return nil
		}
		fi, ierr := d.Info()
		if ierr != nil || !fi.Mode().IsRegular() {
			return nil
		}
		if fi.Size() > maxSourceSize {
			r.Skips = append(r.Skips, rel+": larger than 2 MiB")
			return nil
		}
		src, rerr := os.ReadFile(p)
		if rerr != nil {
			r.Skips = append(r.Skips, rel+": unreadable")
			return nil
		}
		sf := &SrcFile{Rel: rel, Hash: hash12(src), Size: fi.Size(), Parser: "lexical"}
		if strings.HasSuffix(rel, ".go") {
			sf.Parser = "ast"
		}
		sigs, serr := symbols.Extract(rel, src)
		if serr != nil {
			r.Skips = append(r.Skips, rel+": "+firstLine(serr.Error()))
			return nil
		}
		det, derr := symbols.ExtractDetailed(rel, src)
		if derr != nil {
			r.Skips = append(r.Skips, rel+": "+firstLine(derr.Error()))
			return nil
		}
		seen := map[string]bool{}
		for _, s := range det {
			sig, ok := sigs[s.Key]
			if !ok || seen[s.Key] {
				continue
			}
			seen[s.Key] = true
			sf.Syms = append(sf.Syms, Sym{Key: s.Key, Kind: s.Kind, Sig: sig, Doc: s.Doc, Line: s.Line})
		}
		r.Files = append(r.Files, sf)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Rel < r.Files[j].Rel })
	sort.Strings(r.Skips)
	sort.Strings(r.Tests)
	for _, f := range r.Files {
		f.Module = moduleOf(f.Rel, markerDirs)
		r.byRel[f.Rel] = f
	}
	return r, nil
}

// File returns the scanned file for a repo-relative path.
func (r *Repo) File(rel string) (*SrcFile, bool) { f, ok := r.byRel[rel]; return f, ok }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func moduleOf(rel string, markers map[string]bool) string {
	dir := filepath.ToSlash(filepath.Dir(rel))
	for d := dir; ; d = filepath.ToSlash(filepath.Dir(d)) {
		if markers[d] {
			if d == "." {
				return "root"
			}
			return d
		}
		if d == "." || d == "/" || d == "" {
			break
		}
	}
	if i := strings.IndexByte(rel, '/'); i > 0 {
		return rel[:i]
	}
	return "root"
}

// excerpt returns up to max bytes of a source file for prompts (head only), safe for a UTF-8 prompt.
func excerpt(root, rel string, max int) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if len(b) > max {
		b = b[:max]
		for len(b) > 0 && b[len(b)-1]&0xC0 == 0x80 { // do not cut a rune in half
			b = b[:len(b)-1]
		}
		if len(b) > 0 && b[len(b)-1] >= 0xC0 {
			b = b[:len(b)-1]
		}
		return string(b) + fmt.Sprintf("\n... [truncated at %d bytes]", max), nil
	}
	return string(b), nil
}
