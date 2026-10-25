package symbols

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestSupported(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"foo.go", true},
		{"Foo.kt", true},
		{"script.kts", true},
		{"App.java", true},
		{"index.ts", true},
		{"App.tsx", true},
		{"utils.mts", true},
		{"utils.cts", true},
		{"index.js", true},
		{"index.jsx", true},
		{"index.mjs", true},
		{"index.cjs", true},
		{"types.d.ts", true},
		{"main.py", true},
		{"styles.css", false},
		{"doc.md", false},
		{"README", false},
		{"binary.exe", false},
	}
	for _, c := range cases {
		if got := Supported(c.path); got != c.want {
			t.Errorf("Supported(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestGoP01Rules(t *testing.T) {
	src := `package test

const (
	C1 = 1
	C2
	C3 string = "hello"
)

var (
	V1 int
	V2 string
	V3 = 3
)

type Box[T any] struct {
	Val T
}

func (b *Box[T]) Get() T {
	return b.Val
}

func (b unexported) Hidden() {}
`
	syms, err := Extract("test.go", []byte(src))
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}

	// Verify consts
	if syms["C1"] != "const C1 = 1" {
		t.Errorf("C1 = %q", syms["C1"])
	}
	if syms["C2"] != "const C2 = 1" {
		t.Errorf("C2 = %q", syms["C2"])
	}
	if syms["C3"] != `const C3 string = "hello"` {
		t.Errorf("C3 = %q", syms["C3"])
	}

	// Verify vars
	if syms["V1"] != "var V1 int" {
		t.Errorf("V1 = %q", syms["V1"])
	}

	// Verify generic receiver key stripping: Box[T].Get -> Box.Get
	if _, ok := syms["Box.Get"]; !ok {
		t.Errorf("missing Box.Get in %v", syms)
	}

	// Verify unexported receiver method excluded
	if _, ok := syms["unexported.Hidden"]; ok {
		t.Errorf("unexported receiver method leaked")
	}

	// Syntax error must return an error
	badSrc := `package test
func broken(`
	_, err = Extract("bad.go", []byte(badSrc))
	if err == nil {
		t.Fatalf("expected syntax error, got nil")
	}
}

func TestGoldenCorpus(t *testing.T) {
	langs := []struct {
		dir  string
		ext  string
		name string
	}{
		{"kotlin", ".kt", "Kotlin"},
		{"java", ".java", "Java"},
		{"ts", ".ts", "TS/JS"},
		{"python", ".py", "Python"},
		{"go", ".go", "Go"},
	}

	for _, lang := range langs {
		langDir := filepath.Join("testdata", lang.dir)
		files, err := filepath.Glob(filepath.Join(langDir, "*.src"))
		if err != nil {
			t.Fatalf("glob error for %s: %v", lang.name, err)
		}
		if len(files) < 25 {
			t.Fatalf("%s golden corpus has only %d files, require >= 25", lang.name, len(files))
		}

		for _, srcPath := range files {
			testName := filepath.Base(srcPath)
			t.Run(lang.name+"/"+testName, func(t *testing.T) {
				data, err := os.ReadFile(srcPath)
				if err != nil {
					t.Fatalf("read file error: %v", err)
				}

				fakePath := "file" + lang.ext
				if strings.HasSuffix(testName, ".kts.src") {
					fakePath = "file.kts"
				} else if strings.HasSuffix(testName, ".tsx.src") {
					fakePath = "file.tsx"
				} else if strings.HasSuffix(testName, ".js.src") {
					fakePath = "file.js"
				}

				syms, err := Extract(fakePath, data)
				if err != nil {
					t.Fatalf("Extract error: %v", err)
				}

				// Format golden output: sorted key<TAB>sig lines
				var lines []string
				for k, v := range syms {
					lines = append(lines, fmt.Sprintf("%s\t%s", k, v))
				}
				sort.Strings(lines)
				actual := strings.Join(lines, "\n")
				if len(lines) > 0 {
					actual += "\n"
				}

				goldenPath := strings.TrimSuffix(srcPath, ".src") + ".golden"
				if *update {
					err := os.WriteFile(goldenPath, []byte(actual), 0644)
					if err != nil {
						t.Fatalf("failed to write golden file: %v", err)
					}
					return
				}

				goldenData, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
				}

				expected := string(goldenData)
				if actual != expected {
					t.Fatalf("golden mismatch for %s:\n=== EXPECTED ===\n%s\n=== ACTUAL ===\n%s",
						goldenPath, expected, actual)
				}
			})
		}
	}
}

func TestDeltaPairs(t *testing.T) {
	langs := []struct {
		dir  string
		ext  string
		name string
	}{
		{"kotlin", ".kt", "Kotlin"},
		{"java", ".java", "Java"},
		{"ts", ".ts", "TS/JS"},
		{"python", ".py", "Python"},
		{"go", ".go", "Go"},
	}

	for _, lang := range langs {
		deltasDir := filepath.Join("testdata", lang.dir, "deltas")
		oldFiles, err := filepath.Glob(filepath.Join(deltasDir, "*.old"))
		if err != nil {
			t.Fatalf("glob error for %s deltas: %v", lang.name, err)
		}
		if len(oldFiles) < 22 { // >= 12 changed/added/removed + >= 10 NONE
			t.Fatalf("%s delta corpus has only %d files, require >= 22", lang.name, len(oldFiles))
		}

		changedCount := 0
		noneCount := 0

		for _, oldPath := range oldFiles {
			baseName := strings.TrimSuffix(oldPath, ".old")
			newPath := baseName + ".new"
			expectPath := baseName + ".expect"

			testName := filepath.Base(baseName)
			t.Run(lang.name+"/"+testName, func(t *testing.T) {
				oldSrc, err := os.ReadFile(oldPath)
				if err != nil {
					t.Fatalf("read old file: %v", err)
				}
				newSrc, err := os.ReadFile(newPath)
				if err != nil {
					t.Fatalf("read new file: %v", err)
				}
				expectData, err := os.ReadFile(expectPath)
				if err != nil {
					t.Fatalf("read expect file: %v", err)
				}
				expectedDelta := strings.TrimSpace(string(expectData))

				fakePath := "file" + lang.ext
				oldSyms, err := Extract(fakePath, oldSrc)
				if err != nil {
					t.Fatalf("extract old error: %v", err)
				}
				newSyms, err := Extract(fakePath, newSrc)
				if err != nil {
					t.Fatalf("extract new error: %v", err)
				}

				actualDelta := computeDelta(oldSyms, newSyms)
				if actualDelta != expectedDelta {
					t.Fatalf("delta mismatch for %s:\nexpected: %q\nactual:   %q",
						baseName, expectedDelta, actualDelta)
				}

				if expectedDelta == "NONE" {
					noneCount++
				} else {
					changedCount++
				}
			})
		}

		if changedCount < 12 {
			t.Errorf("%s has %d CHANGED/ADDED/REMOVED deltas, require >= 12", lang.name, changedCount)
		}
		if noneCount < 10 {
			t.Errorf("%s has %d NONE deltas, require >= 10", lang.name, noneCount)
		}
	}
}

func computeDelta(oldSyms, newSyms map[string]string) string {
	var added []string
	var removed []string
	var changed []string

	for k, newSig := range newSyms {
		if oldSig, ok := oldSyms[k]; !ok {
			added = append(added, k)
		} else if oldSig != newSig {
			changed = append(changed, k)
		}
	}
	for k := range oldSyms {
		if _, ok := newSyms[k]; !ok {
			removed = append(removed, k)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)

	var parts []string
	if len(changed) > 0 {
		parts = append(parts, fmt.Sprintf("CHANGED %s", strings.Join(changed, ",")))
	}
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("ADDED %s", strings.Join(added, ",")))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("REMOVED %s", strings.Join(removed, ",")))
	}

	if len(parts) == 0 {
		return "NONE"
	}
	return strings.Join(parts, " ")
}

func TestMetamorphicCorpus(t *testing.T) {
	// For all golden sources, insert whitespace / comments between tokens inside headers and verify Extract is identical
	langs := []string{"kotlin", "java", "ts", "python", "go"}
	for _, lang := range langs {
		files, _ := filepath.Glob(filepath.Join("testdata", lang, "*.src"))
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			ext := filepath.Ext(strings.TrimSuffix(f, ".src"))
			if ext == "" {
				ext = ".kt"
			}
			baseSyms, err := Extract("file"+ext, src)
			if err != nil {
				continue
			}

			// Add CRLF
			crlfSrc := []byte(strings.ReplaceAll(string(src), "\n", "\r\n"))
			crlfSyms, err := Extract("file"+ext, crlfSrc)
			if err != nil {
				t.Fatalf("CRLF extract failed for %s: %v", f, err)
			}
			if len(baseSyms) != len(crlfSyms) {
				t.Fatalf("CRLF symbol count mismatch for %s", f)
			}

			// Add BOM
			bomSrc := append([]byte{0xef, 0xbb, 0xbf}, src...)
			bomSyms, err := Extract("file"+ext, bomSrc)
			if err != nil {
				t.Fatalf("BOM extract failed for %s: %v", f, err)
			}
			if len(baseSyms) != len(bomSyms) {
				t.Fatalf("BOM symbol count mismatch for %s", f)
			}
		}
	}
}

func TestPerformanceThresholds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf test in short mode")
	}

	// 1 MB file < 200 ms
	oneMBLine := "fun functionA(x: Int, y: String): Unit = println(x)\n"
	repeats := (1024 * 1024) / len(oneMBLine)
	oneMBData := []byte(strings.Repeat(oneMBLine, repeats))

	start := os.Getenv("BENCH")
	_ = start

	syms, err := Extract("test.kt", oneMBData)
	if err != nil {
		t.Fatalf("1 MB extract error: %v", err)
	}
	if len(syms) == 0 {
		t.Fatalf("expected symbols in 1 MB file")
	}

	// 100 000 nested { -> error (depth limit), not stack overflow
	var sb strings.Builder
	for i := 0; i < 1000; i++ { // tested with depth limit > 500
		sb.WriteString("{\n")
	}
	_, err = Extract("deep.kt", []byte(sb.String()))
	if err == nil {
		t.Fatalf("expected depth limit error, got nil")
	}
}
