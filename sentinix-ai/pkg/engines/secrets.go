package engines

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Secrets is the dependency-free built-in engine: no binary, no network.
type Secrets struct{}

func (Secrets) Name() string { return "secrets" }

func (Secrets) Scan(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "node_modules" || n == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if info, e := d.Info(); e != nil || info.Size() > 1<<20 {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return nil
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, p)
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			if strings.Contains(line, "sentinix:ignore") {
				continue
			}
			for _, r := range secretRules {
				if m := r.re.FindString(line); m != "" && !(r.id == "secret.generic-assignment" && placeholder.MatchString(m)) {
					sev := High
					if isTestPath(rel) {
						sev = Low // fixtures are usually fake; still reported so a real key is not hidden
					}
					out = append(out, Finding{Rule: r.id, Engine: "secrets", File: rel, Line: n,
						Severity: sev, Message: "possible hardcoded secret", Evidence: mask(m)})
				}
			}
		}
		return nil
	})
	return out, err
}

// placeholder values are documentation and stubs, not credentials. Only the generic rule uses this:
// a real AWS key or private key is never excused by a word in it.
var placeholder = regexp.MustCompile(`(?i)(example|fake|dummy|mock|stub|demo|sample|secure-random|placeholder|changeme|your[-_]|x{6,}|<[^>]+>|\$\{)`)

func isTestPath(p string) bool {
	for _, m := range []string{"/test/", "commonTest", "androidTest", "iosTest", "jvmTest", "_test.go", "Test.kt", "Test.java", "fixtures", "testdata"} {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}
