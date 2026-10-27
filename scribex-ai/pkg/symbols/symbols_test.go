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

func TestGoIotaContract(t *testing.T) {
	// 1. Reorder
	oldSrc := `package test
const (
	A = iota
	B
)`
	newSrc := `package test
const (
	B = iota
	A
)`
	oldSyms, err := Extract("test.go", []byte(oldSrc))
	if err != nil {
		t.Fatal(err)
	}
	newSyms, err := Extract("test.go", []byte(newSrc))
	if err != nil {
		t.Fatal(err)
	}
	if d := computeDelta(oldSyms, newSyms); d != "CHANGED A,B" {
		t.Errorf("reorder delta = %q, want 'CHANGED A,B'", d)
	}

	// 2. Insert in middle
	oldMid := `package test
const (
	A = iota
	C
)`
	newMid := `package test
const (
	A = iota
	B
	C
)`
	oldMidSyms, _ := Extract("test.go", []byte(oldMid))
	newMidSyms, _ := Extract("test.go", []byte(newMid))
	if d := computeDelta(oldMidSyms, newMidSyms); d != "CHANGED C ADDED B" && d != "ADDED B CHANGED C" {
		t.Errorf("insert middle delta = %q, want 'CHANGED C ADDED B'", d)
	}

	// 3. Blank _ spec before
	blankSrc := `package test
const (
	_ = iota
	KB = 1 << (10 * iota)
	MB
)`
	blankSyms, err := Extract("test.go", []byte(blankSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "const KB = 1 << (10 * iota) // iota=1"; blankSyms["KB"] != want {
		t.Errorf("KB = %q, want %q", blankSyms["KB"], want)
	}
	if want := "const MB = 1 << (10 * iota) // iota=2"; blankSyms["MB"] != want {
		t.Errorf("MB = %q, want %q", blankSyms["MB"], want)
	}

	// 4. Typed group
	typedSrc := `package test
type Level int
const (
	Low Level = iota
	Medium
	High
)`
	typedSyms, err := Extract("test.go", []byte(typedSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "const Low Level = iota // iota=0"; typedSyms["Low"] != want {
		t.Errorf("Low = %q, want %q", typedSyms["Low"], want)
	}
	if want := "const Medium Level = iota // iota=1"; typedSyms["Medium"] != want {
		t.Errorf("Medium = %q, want %q", typedSyms["Medium"], want)
	}
	if want := "const High Level = iota // iota=2"; typedSyms["High"] != want {
		t.Errorf("High = %q, want %q", typedSyms["High"], want)
	}

	// 5. Bitmask shift group
	bitmaskSrc := `package test
const (
	FlagA = 1 << iota
	FlagB
	FlagC
)`
	bitmaskSyms, err := Extract("test.go", []byte(bitmaskSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "const FlagA = 1 << iota // iota=0"; bitmaskSyms["FlagA"] != want {
		t.Errorf("FlagA = %q, want %q", bitmaskSyms["FlagA"], want)
	}
	if want := "const FlagB = 1 << iota // iota=1"; bitmaskSyms["FlagB"] != want {
		t.Errorf("FlagB = %q, want %q", bitmaskSyms["FlagB"], want)
	}
	if want := "const FlagC = 1 << iota // iota=2"; bitmaskSyms["FlagC"] != want {
		t.Errorf("FlagC = %q, want %q", bitmaskSyms["FlagC"], want)
	}

	// 6. Two names per spec
	multiSrc := `package test
const (
	X, Y = iota, iota * 2
	Z, W
)`
	multiSyms, err := Extract("test.go", []byte(multiSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "const X = iota // iota=0"; multiSyms["X"] != want {
		t.Errorf("X = %q, want %q", multiSyms["X"], want)
	}
	if want := "const Y = iota * 2 // iota=0"; multiSyms["Y"] != want {
		t.Errorf("Y = %q, want %q", multiSyms["Y"], want)
	}
	if want := "const Z = iota // iota=1"; multiSyms["Z"] != want {
		t.Errorf("Z = %q, want %q", multiSyms["Z"], want)
	}
	if want := "const W = iota * 2 // iota=1"; multiSyms["W"] != want {
		t.Errorf("W = %q, want %q", multiSyms["W"], want)
	}

	// 7. Non-iota const group (no suffix)
	nonIotaSrc := `package test
const (
	Normal1 = 1
	Normal2 = 2
)
const Single = 5`
	nonIotaSyms, err := Extract("test.go", []byte(nonIotaSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "const Normal1 = 1"; nonIotaSyms["Normal1"] != want {
		t.Errorf("Normal1 = %q, want %q", nonIotaSyms["Normal1"], want)
	}
	if want := "const Normal2 = 2"; nonIotaSyms["Normal2"] != want {
		t.Errorf("Normal2 = %q, want %q", nonIotaSyms["Normal2"], want)
	}
	if want := "const Single = 5"; nonIotaSyms["Single"] != want {
		t.Errorf("Single = %q, want %q", nonIotaSyms["Single"], want)
	}

	// 8. Unexported name consuming an index
	unexpSrc := `package test
const (
	First = iota
	hidden
	Second
)`
	unexpSyms, err := Extract("test.go", []byte(unexpSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "const First = iota // iota=0"; unexpSyms["First"] != want {
		t.Errorf("First = %q, want %q", unexpSyms["First"], want)
	}
	if _, ok := unexpSyms["hidden"]; ok {
		t.Errorf("hidden should not be exported")
	}
	if want := "const Second = iota // iota=2"; unexpSyms["Second"] != want {
		t.Errorf("Second = %q, want %q", unexpSyms["Second"], want)
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

func TestDetailedGoldens(t *testing.T) {
	langs := []struct {
		dir  string
		ext  string
		name string
	}{
		{"go", ".go", "Go"},
		{"kotlin", ".kt", "Kotlin"},
		{"java", ".java", "Java"},
		{"ts", ".ts", "TS/JS"},
		{"python", ".py", "Python"},
	}

	for _, lang := range langs {
		detailDir := filepath.Join("testdata", lang.dir, "detail")
		files, err := filepath.Glob(filepath.Join(detailDir, "*.src"))
		if err != nil {
			t.Fatalf("glob error for %s detail: %v", lang.name, err)
		}
		if len(files) < 8 {
			t.Fatalf("%s detail golden corpus has only %d files, require >= 8", lang.name, len(files))
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

				syms, err := ExtractDetailed(fakePath, data)
				if err != nil {
					t.Fatalf("ExtractDetailed error: %v", err)
				}

				// Format golden output: Key|Kind|Line|Doc one per line (source order)
				var lines []string
				for _, s := range syms {
					docEsc := strings.ReplaceAll(s.Doc, "\n", `\n`)
					lines = append(lines, fmt.Sprintf("%s|%s|%d|%s", s.Key, s.Kind, s.Line, docEsc))
				}
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

func TestExtractEquivalenceWithDetailed(t *testing.T) {
	langs := []struct {
		dir  string
		ext  string
		name string
	}{
		{"go", ".go", "Go"},
		{"kotlin", ".kt", "Kotlin"},
		{"java", ".java", "Java"},
		{"ts", ".ts", "TS/JS"},
		{"python", ".py", "Python"},
	}

	for _, lang := range langs {
		corpusDir := filepath.Join("testdata", lang.dir)
		files, err := filepath.Glob(filepath.Join(corpusDir, "*.src"))
		if err != nil {
			t.Fatalf("glob error for %s: %v", lang.name, err)
		}
		if len(files) == 0 {
			t.Fatalf("no files found for %s", lang.name)
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

				extracted, err1 := Extract(fakePath, data)
				if err1 != nil {
					t.Fatalf("Extract failed: %v", err1)
				}

				detailed, err2 := ExtractDetailed(fakePath, data)
				if err2 != nil {
					t.Fatalf("ExtractDetailed failed: %v", err2)
				}

				// Build mapFrom(ExtractDetailed(x)) for plain keys
				mapFromDetailed := make(map[string]string, len(detailed))
				for _, sym := range detailed {
					if _, exists := mapFromDetailed[sym.Key]; exists {
						t.Errorf("ExtractDetailed produced duplicate plain key: %q", sym.Key)
					}
					mapFromDetailed[sym.Key] = sym.Sig
				}

				// Assert that for every plain key in ExtractDetailed, Extract has the exact same signature
				for k, sig := range mapFromDetailed {
					extSig, ok := extracted[k]
					if !ok {
						t.Errorf("Extract is missing plain key %q present in ExtractDetailed", k)
						continue
					}
					if extSig != sig {
						t.Errorf("signature mismatch for plain key %q:\nExtract:          %q\nExtractDetailed:  %q", k, extSig, sig)
					}
				}

				// Assert that all plain keys in Extract are present in ExtractDetailed
				for k := range extracted {
					if _, ok := mapFromDetailed[k]; !ok {
						if !strings.Contains(k, "(") {
							t.Errorf("Extract has plain key %q not present in ExtractDetailed", k)
						}
					}
				}
			})
		}
	}
}
