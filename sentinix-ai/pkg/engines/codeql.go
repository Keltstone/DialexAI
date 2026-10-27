package engines

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CodeQL adds real data-flow (taint) analysis by driving the CodeQL CLI: one database per detected language,
// the security-extended suite, SARIF back into the normal finding pipeline. Opt-in (SENTINIX_CODEQL=1) because
// it is slow; the query packs must already be installed (no network is used here). Databases are built with
// --build-mode=none where the language supports it, so the scanned project's build is never executed.
type CodeQL struct{ Bin string }

// language -> (codeql language id, query pack, extra create flags)
var codeqlLangs = map[string]struct {
	id, pack string
	noBuild  bool
}{
	".kt": {"java-kotlin", "java", true}, ".java": {"java-kotlin", "java", true},
	".py": {"python", "python", true},
	".js": {"javascript-typescript", "javascript", true}, ".ts": {"javascript-typescript", "javascript", true},
	".cs": {"csharp", "csharp", true},
	".go": {"go", "go", false},
}

func (c CodeQL) Name() string { return "codeql" }

func (c CodeQL) Scan(ctx context.Context, root string) ([]Finding, error) {
	bin := c.Bin
	if bin == "" {
		bin = "codeql"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("%w: codeql not installed", ErrEngineUnavailable)
	}
	langs := map[string]string{} // language id -> pack
	noBuild := map[string]bool{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "build") {
			return filepath.SkipDir
		}
		if l, ok := codeqlLangs[filepath.Ext(p)]; ok && d.Type().IsRegular() {
			langs[l.id], noBuild[l.id] = l.pack, l.noBuild
		}
		return nil
	})
	ids := make([]string, 0, len(langs))
	for id := range langs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	tmp, err := os.MkdirTemp("", "sentinix-codeql-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	var all []Finding
	for _, id := range ids {
		db, out := filepath.Join(tmp, id+"-db"), filepath.Join(tmp, id+".sarif")
		create := []string{"database", "create", db, "--language=" + id, "--source-root=" + root}
		if noBuild[id] {
			create = append(create, "--build-mode=none")
		}
		suite := fmt.Sprintf("codeql/%s-queries:codeql-suites/%s-security-extended.qls", langs[id], langs[id])
		for _, args := range [][]string{create, {"database", "analyze", db, suite, "--format=sarif-latest", "--output=" + out}} {
			if b, err := exec.CommandContext(ctx, path, args...).CombinedOutput(); err != nil {
				return all, fmt.Errorf("codeql %s (%s): %v: %s", args[1], id, err, truncateErr(b))
			}
		}
		data, err := os.ReadFile(out)
		if err != nil {
			return all, err
		}
		fs, err := ParseSARIFWithCategory("codeql", data, CategorySecurity)
		if err != nil {
			return all, err
		}
		all = append(all, fs...)
	}
	return all, nil
}

func truncateErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[len(s)-200:]
	}
	return s
}
