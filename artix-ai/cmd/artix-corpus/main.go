package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"artix/pkg/persona"
	"artix/pkg/reviewer"
	"artix/pkg/sandbox"
)

type pinnedModule struct {
	pkg     string
	version string
}

// Source 2: Pinned first-tier standard ecosystem modules
var pinnedModules = []pinnedModule{
	{"golang.org/x/sync", "v0.7.0"},
	{"github.com/google/uuid", "v1.6.0"},
	{"go.uber.org/zap", "v1.27.0"},
	{"golang.org/x/crypto", "v0.25.0"},
	{"github.com/stretchr/testify", "v1.9.0"},
}

// Source 3: Independent unpinned-but-fixed second set
var secondSetModules = []pinnedModule{
	{"github.com/spf13/cobra", "v1.8.1"},
	{"golang.org/x/sys", "v0.22.0"},
	{"github.com/pkg/errors", "v0.9.1"},
}

type modDownloadInfo struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Dir     string `json:"Dir"`
}

type regressionItem struct {
	source   string
	relPath  string
	expected string
	got      string
	reason   string
}

type sourceStats struct {
	sourceName    string
	version       string
	totalFiles    int
	passedFiles   int
	rejectedFiles int
	reasonCounts  map[string]int
}

func main() {
	outCSV := flag.String("output", "docs/pilot/benign_corpus_results.csv", "Output CSV file path")
	baselineCSV := flag.String("baseline", "docs/pilot/benign_corpus_results.csv", "Baseline CSV file path for regression detection")
	updateBaseline := flag.Bool("update", false, "Update baseline CSV without failing on regressions")
	gorootTarget := flag.Float64("goroot-target", 5.0, "Absolute FPR target percentage for GOROOT")
	pinnedTarget := flag.Float64("pinned-target", 5.0, "Absolute FPR target percentage for each pinned module")
	testifyTarget := flag.Float64("testify-target", 10.0, "Absolute FPR target percentage for testify")
	secondSetTarget := flag.Float64("second-set-target", 15.0, "Absolute FPR target percentage for second fixed set")
	flag.Parse()

	// Load existing committed CSV baseline for regression comparison
	baselineMap := make(map[string]string) // key: source + ":" + relPath -> verdict
	if *baselineCSV != "" {
		if bf, err := os.Open(*baselineCSV); err == nil {
			r := csv.NewReader(bf)
			records, _ := r.ReadAll()
			_ = bf.Close()
			for i, row := range records {
				if i == 0 || len(row) < 4 {
					continue
				}
				key := row[0] + ":" + row[2]
				baselineMap[key] = row[3]
			}
		}
	}

	gorootBytes, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get GOROOT: %v\n", err)
		os.Exit(1)
	}
	goroot := strings.TrimSpace(string(gorootBytes))
	goVerBytes, err := exec.Command("go", "version").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get go version: %v\n", err)
		os.Exit(1)
	}
	goVersion := strings.TrimSpace(string(goVerBytes))

	fmt.Printf("=== Artix AI: Three-Source Independent Benign Corpus Evaluation ===\n")
	fmt.Printf("Source 1 (GOROOT): %s (%s)\n", goroot, goVersion)
	fmt.Printf("Source 2 (Pinned Set 1): %d modules\n", len(pinnedModules))
	fmt.Printf("Source 3 (Fixed Set 2): %d modules\n", len(secondSetModules))
	fmt.Printf("Committed Baseline CSV: %s (%d files indexed)\n\n", *baselineCSV, len(baselineMap))

	_ = os.MkdirAll(filepath.Dir(*outCSV), 0755)
	f, err := os.Create(*outCSV)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create CSV %s: %v\n", *outCSV, err)
		os.Exit(1)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	_ = writer.Write([]string{"source", "version", "file", "verdict", "blocking_reason"})

	rev := reviewer.NewAdversarialReviewer(persona.NewRegistry(""))
	rev.SetCritic(func(ctx context.Context, prompt string) (string, error) {
		return `{"approved":true,"blocking":[],"warnings":[]}`, nil
	})

	var allRegressions []regressionItem

	// 1. Source 1: GOROOT src test files (minus testdata)
	gorootSrc := filepath.Join(goroot, "src")
	var gorootTestFiles []string
	err = filepath.Walk(gorootSrc, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "unreadable file/dir in GOROOT: %s: %v\n", path, err)
			os.Exit(1)
		}
		if info.IsDir() {
			name := info.Name()
			if name == "testdata" || name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			gorootTestFiles = append(gorootTestFiles, path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error walking GOROOT: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Discovered %d test files in GOROOT/src.\n", len(gorootTestFiles))

	gorootStats := sourceStats{
		sourceName:   "GOROOT",
		version:      goVersion,
		reasonCounts: make(map[string]int),
	}

	evalFiles(gorootTestFiles, gorootSrc, "GOROOT", goVersion, rev, writer, &gorootStats, baselineMap, &allRegressions)

	// 2. Source 2: Pinned third-party modules (Set 1)
	var set1StatsList []sourceStats
	totalSet1Files, totalSet1Passed, totalSet1Rejected := 0, 0, 0
	for _, pm := range pinnedModules {
		st := evalModule(pm, rev, writer, baselineMap, &allRegressions)
		set1StatsList = append(set1StatsList, st)
		totalSet1Files += st.totalFiles
		totalSet1Passed += st.passedFiles
		totalSet1Rejected += st.rejectedFiles
	}

	// 3. Source 3: Independent unpinned-but-fixed second set (Set 2)
	var set2StatsList []sourceStats
	totalSet2Files, totalSet2Passed, totalSet2Rejected := 0, 0, 0
	for _, pm := range secondSetModules {
		st := evalModule(pm, rev, writer, baselineMap, &allRegressions)
		set2StatsList = append(set2StatsList, st)
		totalSet2Files += st.totalFiles
		totalSet2Passed += st.passedFiles
		totalSet2Rejected += st.rejectedFiles
	}

	writer.Flush()

	// Print Per-Source Summary Report
	fmt.Printf("\n===================================================\n")
	fmt.Printf("=== BENIGN CORPUS EVALUATION PER-SOURCE FPR REPORT ===\n")
	fmt.Printf("===================================================\n")

	gorootFPR := 0.0
	if gorootStats.totalFiles > 0 {
		gorootFPR = (float64(gorootStats.rejectedFiles) / float64(gorootStats.totalFiles)) * 100.0
	}
	fmt.Printf("\n[SOURCE 1: GOROOT] %s\n", goVersion)
	fmt.Printf("  Total files: %d | Approved: %d | Rejected: %d (FPR: %.2f%%)\n",
		gorootStats.totalFiles, gorootStats.passedFiles, gorootStats.rejectedFiles, gorootFPR)

	set1FPR := 0.0
	if totalSet1Files > 0 {
		set1FPR = (float64(totalSet1Rejected) / float64(totalSet1Files)) * 100.0
	}
	fmt.Printf("\n[SOURCE 2: PINNED MODULES SET 1] Combined FPR: %.2f%%\n", set1FPR)
	for _, ms := range set1StatsList {
		subFPR := 0.0
		if ms.totalFiles > 0 {
			subFPR = (float64(ms.rejectedFiles) / float64(ms.totalFiles)) * 100.0
		}
		fmt.Printf("  - %s@%s: %d total, %d passed, %d rejected (%.2f%% FPR)\n",
			ms.sourceName, ms.version, ms.totalFiles, ms.passedFiles, ms.rejectedFiles, subFPR)
	}

	set2FPR := 0.0
	if totalSet2Files > 0 {
		set2FPR = (float64(totalSet2Rejected) / float64(totalSet2Files)) * 100.0
	}
	fmt.Printf("\n[SOURCE 3: FIXED MODULES SET 2] Combined FPR: %.2f%%\n", set2FPR)
	for _, ms := range set2StatsList {
		subFPR := 0.0
		if ms.totalFiles > 0 {
			subFPR = (float64(ms.rejectedFiles) / float64(ms.totalFiles)) * 100.0
		}
		fmt.Printf("  - %s@%s: %d total, %d passed, %d rejected (%.2f%% FPR)\n",
			ms.sourceName, ms.version, ms.totalFiles, ms.passedFiles, ms.rejectedFiles, subFPR)
	}

	grandTotal := gorootStats.totalFiles + totalSet1Files + totalSet2Files
	grandPassed := gorootStats.passedFiles + totalSet1Passed + totalSet2Passed
	grandRejected := gorootStats.rejectedFiles + totalSet1Rejected + totalSet2Rejected
	grandFPR := 0.0
	if grandTotal > 0 {
		grandFPR = (float64(grandRejected) / float64(grandTotal)) * 100.0
	}

	fmt.Printf("\n[GRAND TOTAL ACROSS ALL THREE SOURCES]\n")
	fmt.Printf("  Total files evaluated: %d\n", grandTotal)
	fmt.Printf("  Approved (True Negatives): %d\n", grandPassed)
	fmt.Printf("  Rejected (False Positives): %d (Overall FPR: %.2f%%)\n", grandRejected, grandFPR)
	fmt.Printf("  Results CSV saved to: %s\n", *outCSV)

	// Target and Gap Evaluation (R14-9)
	type targetCheck struct {
		name   string
		target float64
		actual float64
		gap    float64
		passed bool
	}
	var targetChecks []targetCheck

	gorootGap := gorootFPR - *gorootTarget
	targetChecks = append(targetChecks, targetCheck{
		name:   "Source 1: GOROOT",
		target: *gorootTarget,
		actual: gorootFPR,
		gap:    gorootGap,
		passed: gorootFPR <= *gorootTarget,
	})

	for _, ms := range set1StatsList {
		subFPR := 0.0
		if ms.totalFiles > 0 {
			subFPR = (float64(ms.rejectedFiles) / float64(ms.totalFiles)) * 100.0
		}
		target := *pinnedTarget
		if strings.Contains(ms.sourceName, "testify") {
			target = *testifyTarget
		}
		targetChecks = append(targetChecks, targetCheck{
			name:   fmt.Sprintf("Source 2: %s@%s", ms.sourceName, ms.version),
			target: target,
			actual: subFPR,
			gap:    subFPR - target,
			passed: subFPR <= target,
		})
	}

	for _, ms := range set2StatsList {
		subFPR := 0.0
		if ms.totalFiles > 0 {
			subFPR = (float64(ms.rejectedFiles) / float64(ms.totalFiles)) * 100.0
		}
		target := *secondSetTarget
		targetChecks = append(targetChecks, targetCheck{
			name:   fmt.Sprintf("Source 3: %s@%s", ms.sourceName, ms.version),
			target: target,
			actual: subFPR,
			gap:    subFPR - target,
			passed: subFPR <= target,
		})
	}

	fmt.Printf("\n===================================================\n")
	fmt.Printf("=== ABSOLUTE FPR TARGETS & GAP ANALYSIS (R14-9) ===\n")
	fmt.Printf("===================================================\n")
	hasTargetFailure := false
	for _, tc := range targetChecks {
		statusStr := "MET"
		if !tc.passed {
			statusStr = "FAILED"
			hasTargetFailure = true
		}
		fmt.Printf("  [%-6s] %-45s Target: <=%5.2f%% | Actual: %5.2f%% | Gap: %+6.2f%%\n",
			statusStr, tc.name, tc.target, tc.actual, tc.gap)
	}

	if !*updateBaseline && hasTargetFailure {
		fmt.Fprintf(os.Stderr, "\nFAILURE: One or more corpus sources exceeded absolute FPR targets.\n")
		os.Exit(1)
	}

	// Check for regressions vs committed baseline CSV
	if !*updateBaseline && len(baselineMap) > 0 && len(allRegressions) > 0 {
		fmt.Fprintf(os.Stderr, "\nFAILURE: %d benign corpus regression(s) vs committed baseline CSV detected:\n", len(allRegressions))
		for _, reg := range allRegressions {
			fmt.Fprintf(os.Stderr, "  - [%s] %s: expected %s, got %s (%s)\n", reg.source, reg.relPath, reg.expected, reg.got, reg.reason)
		}
		os.Exit(1)
	}

	fmt.Printf("\nSUCCESS: Benign corpus evaluation completed: all absolute FPR targets met and 0 regressions vs committed baseline CSV.\n")
}

func evalModule(pm pinnedModule, rev *reviewer.AdversarialReviewer, writer *csv.Writer, baseline map[string]string, regressions *[]regressionItem) sourceStats {
	modCoord := fmt.Sprintf("%s@%s", pm.pkg, pm.version)
	fmt.Printf("\nDownloading module %s...\n", modCoord)
	out, err := exec.Command("go", "mod", "download", "-json", modCoord).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to download %s: %v\n", modCoord, err)
		os.Exit(1)
	}

	var info modDownloadInfo
	if err := json.Unmarshal(out, &info); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse mod download json for %s: %v\n", modCoord, err)
		os.Exit(1)
	}

	var modTestFiles []string
	err = filepath.Walk(info.Dir, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "unreadable file in module %s: %s: %v\n", modCoord, path, err)
			os.Exit(1)
		}
		if fileInfo.IsDir() {
			name := fileInfo.Name()
			if name == "testdata" || name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			modTestFiles = append(modTestFiles, path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error walking module %s: %v\n", modCoord, err)
		os.Exit(1)
	}

	fmt.Printf("Discovered %d test files in %s.\n", len(modTestFiles), modCoord)
	mStats := sourceStats{
		sourceName:   pm.pkg,
		version:      pm.version,
		reasonCounts: make(map[string]int),
	}

	evalFiles(modTestFiles, info.Dir, pm.pkg, pm.version, rev, writer, &mStats, baseline, regressions)
	return mStats
}

func evalFiles(files []string, baseDir, sourceName, version string, rev *reviewer.AdversarialReviewer, writer *csv.Writer, stats *sourceStats, baseline map[string]string, regressions *[]regressionItem) {
	for _, path := range files {
		relPath, _ := filepath.Rel(baseDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: cannot read %s: %v\n", path, err)
			os.Exit(1)
		}
		stats.totalFiles++

		lines := strings.Split(string(data), "\n")
		var diffBuilder strings.Builder
		diffBuilder.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", relPath, relPath))
		diffBuilder.WriteString("new file mode 100644\n")
		diffBuilder.WriteString("--- /dev/null\n")
		diffBuilder.WriteString(fmt.Sprintf("+++ b/%s\n", relPath))
		diffBuilder.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
		for _, l := range lines {
			diffBuilder.WriteString("+" + l + "\n")
		}

		tempDir, _ := os.MkdirTemp("", "artix_corpus_eval")
		subPath := filepath.Join(tempDir, relPath)
		_ = os.MkdirAll(filepath.Dir(subPath), 0755)
		_ = os.WriteFile(subPath, data, 0644)

		ctx := &reviewer.ReviewContext{
			Ctx:          context.Background(),
			WorkspaceDir: tempDir,
			Diff:         diffBuilder.String(),
			TestResults: []*sandbox.ExecResult{
				{Command: "go test ./...", ExitCode: 0},
			},
		}

		verdict := rev.Evaluate(ctx)
		_ = os.RemoveAll(tempDir)

		verdictStr := string(verdict.Status)
		blocking := ""
		if len(verdict.BlockingIssues) > 0 {
			blocking = verdict.BlockingIssues[0]
		}

		if verdict.Approved {
			stats.passedFiles++
		} else {
			stats.rejectedFiles++
			stats.reasonCounts[blocking]++
		}

		// Regression check against baseline
		key := sourceName + ":" + relPath
		if expectedVerdict, exists := baseline[key]; exists {
			if expectedVerdict == "approved" && verdictStr != "approved" {
				*regressions = append(*regressions, regressionItem{
					source:   sourceName,
					relPath:  relPath,
					expected: expectedVerdict,
					got:      verdictStr,
					reason:   blocking,
				})
			}
		}

		_ = writer.Write([]string{sourceName, version, relPath, verdictStr, blocking})
	}
}
