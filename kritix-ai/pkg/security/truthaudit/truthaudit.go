package truthaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Violation represents a single detected integrity violation.
type Violation struct {
	Gate        string `json:"gate"`
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
}

// AuditResult aggregates all structural checks.
type AuditResult struct {
	Passed     bool        `json:"passed"`
	Violations []Violation `json:"violations"`
}

// Auditor executes structural truth and integrity verification across the repository.
type Auditor struct {
	RepoRoot  string
	KritixDir string
}

// NewAuditor constructs an Auditor with resolved paths.
func NewAuditor(repoRoot string) *Auditor {
	kritixDir := repoRoot
	if _, err := os.Stat(filepath.Join(repoRoot, "kritix-ai")); err == nil {
		kritixDir = filepath.Join(repoRoot, "kritix-ai")
	}
	return &Auditor{
		RepoRoot:  repoRoot,
		KritixDir: kritixDir,
	}
}

// RunAll executes all 12 structural truth checks.
func (a *Auditor) RunAll() (*AuditResult, error) {
	var violations []Violation

	// 1. Provenance Gate (benchmark.json & raw log digest)
	v, _ := a.CheckProvenance()
	violations = append(violations, v...)

	// 2. AST Provenance (No hardcoded numeric literals in optimizer/cmd structs)
	v, _ = a.CheckASTProvenance()
	violations = append(violations, v...)

	// 3. Label Honesty
	v, _ = a.CheckLabelHonesty()
	violations = append(violations, v...)

	// 4. Event Integrity (JSONL orphan events check)
	v, _ = a.CheckEventIntegrity()
	violations = append(violations, v...)

	// 5. Self-Certification Gate
	v, _ = a.CheckSelfCertification()
	violations = append(violations, v...)

	// 6. Defaults & Secrets Gate (AST scan)
	v, _ = a.CheckDefaultsAndSecrets()
	violations = append(violations, v...)

	// 7. Stats Gate (No t.Logf threshold bypasses)
	v, _ = a.CheckStatsGate()
	violations = append(violations, v...)

	// 8. Paths Gate (No /Users/ or file:// in tracked files)
	v, _ = a.CheckPaths()
	violations = append(violations, v...)

	// 9. Prerequisite Honesty Gate (harness scripts fail closed when tools are missing)
	v, _ = a.CheckPrerequisiteHonesty()
	violations = append(violations, v...)

	// 10. Independence Gate (AST scan ensuring study tests do not artificially loop cases)
	v, _ = a.CheckIndependenceGate()
	violations = append(violations, v...)

	// 11. Synthetic Headline Gate (classifier figures must not headline without label)
	v, _ = a.CheckSyntheticHeadline()
	violations = append(violations, v...)

	// 12. Hygiene Gate (no TestSample, debug, or scratch tests)
	v, _ = a.CheckHygiene()
	violations = append(violations, v...)

	return &AuditResult{
		Passed:     len(violations) == 0,
		Violations: violations,
	}, nil
}

// CheckProvenance validates benchmark.json against git HEAD and raw log SHA256 digest.
func (a *Auditor) CheckProvenance() ([]Violation, error) {
	var violations []Violation
	benchPath := filepath.Join(a.KritixDir, "benchmark.json")
	data, err := os.ReadFile(benchPath)
	if err != nil {
		violations = append(violations, Violation{
			Gate:        "Provenance",
			File:        "benchmark.json",
			Description: "benchmark.json missing or unreadable",
		})
		return violations, nil
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		violations = append(violations, Violation{
			Gate:        "Provenance",
			File:        "benchmark.json",
			Description: fmt.Sprintf("invalid benchmark.json format: %v", err),
		})
		return violations, nil
	}

	// Must have harness_run_id
	harnessID, _ := m["harness_run_id"].(string)
	if harnessID == "" {
		violations = append(violations, Violation{
			Gate:        "Provenance",
			File:        "benchmark.json",
			Description: "missing required harness_run_id in benchmark.json",
		})
	}

	// Must have tree_clean flag set to true
	treeClean, hasTreeClean := m["tree_clean"].(bool)
	if !hasTreeClean || !treeClean {
		violations = append(violations, Violation{
			Gate:        "Provenance",
			File:        "benchmark.json",
			Description: "missing required 'tree_clean: true' flag in benchmark.json",
		})
	}

	// Classifier metrics must NOT sit unlabelled at top level; must be nested under synthetic_corpus
	if _, ok := m["raw_tokens_avg"]; ok {
		violations = append(violations, Violation{
			Gate:        "Label Honesty",
			File:        "benchmark.json",
			Description: "raw_tokens_avg must not sit at top-level; must be nested under synthetic_corpus",
		})
	}
	if _, ok := m["optimized_tokens_avg"]; ok {
		violations = append(violations, Violation{
			Gate:        "Label Honesty",
			File:        "benchmark.json",
			Description: "optimized_tokens_avg must not sit at top-level; must be nested under synthetic_corpus",
		})
	}
	if _, ok := m["token_savings_percent"]; ok {
		violations = append(violations, Violation{
			Gate:        "Label Honesty",
			File:        "benchmark.json",
			Description: "token_savings_percent must not sit at top-level; must be nested under synthetic_corpus",
		})
	}

	synthCorpus, hasSynth := m["synthetic_corpus"].(map[string]interface{})
	if !hasSynth || synthCorpus == nil {
		violations = append(violations, Violation{
			Gate:        "Label Honesty",
			File:        "benchmark.json",
			Description: "missing required synthetic_corpus object containing classifier metrics",
		})
	}

	// Exact commit rule + evidence-only-diff rule (No HEAD~1 / parent allowance)
	commit, _ := m["commit"].(string)
	headCommit := getGitHead(a.KritixDir)
	if headCommit != "" && commit != "" {
		matchesExact := strings.HasPrefix(headCommit, commit) || strings.HasPrefix(commit, headCommit)
		if !matchesExact {
			// 1. Verify that recorded commit is an ancestor of HEAD
			ancestorCmd := exec.Command("git", "-C", a.KritixDir, "merge-base", "--is-ancestor", commit, "HEAD")
			if err := ancestorCmd.Run(); err != nil {
				violations = append(violations, Violation{
					Gate:        "Provenance",
					File:        "benchmark.json",
					Description: fmt.Sprintf("stale git commit in benchmark.json: recorded %q is not an ancestor of HEAD %q", commit, headCommit),
				})
			} else {
				// 2. Scope the diff to kritix-ai/** (path-limited diff)
				var diffCmd *exec.Cmd
				if a.RepoRoot != "" && a.RepoRoot != a.KritixDir {
					diffCmd = exec.Command("git", "-C", a.RepoRoot, "diff", "--name-only", commit+"..HEAD", "--", "kritix-ai/")
				} else {
					diffCmd = exec.Command("git", "-C", a.KritixDir, "diff", "--name-only", commit+"..HEAD", "--", ".")
				}

				diffOut, err := diffCmd.Output()
				if err != nil {
					violations = append(violations, Violation{
						Gate:        "Provenance",
						File:        "benchmark.json",
						Description: fmt.Sprintf("stale git commit in benchmark.json: recorded %q vs actual HEAD %q (git diff error: %v)", commit, headCommit, err),
					})
				} else {
					diffFiles := strings.Split(strings.TrimSpace(string(diffOut)), "\n")
					onlyEvidenceDiff := true
					var nonEvidenceFiles []string
					for _, df := range diffFiles {
						df = strings.TrimSpace(df)
						if df == "" {
							continue
						}
						isEvidence := strings.HasSuffix(df, "benchmark.json") ||
							strings.Contains(df, "evidence/") ||
							strings.Contains(df, ".dev/") ||
							strings.Contains(df, "capture_log")
						if !isEvidence {
							onlyEvidenceDiff = false
							nonEvidenceFiles = append(nonEvidenceFiles, df)
						}
					}
					if !onlyEvidenceDiff {
						violations = append(violations, Violation{
							Gate:        "Provenance",
							File:        "benchmark.json",
							Description: fmt.Sprintf("stale git commit in benchmark.json: recorded %q vs HEAD %q; non-evidence files modified: %v", commit, headCommit, nonEvidenceFiles),
						})
					}
				}
			}
		}
	}

	// Must have raw_log_path and valid sha256 matching the file on disk
	rawLogPath, _ := m["raw_log_path"].(string)
	rawLogSHA, _ := m["raw_log_sha256"].(string)

	if rawLogPath == "" || rawLogPath == "none" {
		violations = append(violations, Violation{
			Gate:        "Provenance",
			File:        "benchmark.json",
			Description: "missing required raw_log_path in benchmark.json",
		})
	} else {
		resolvedPath := rawLogPath
		if !filepath.IsAbs(resolvedPath) {
			// Try relative to KritixDir and RepoRoot
			if _, err := os.Stat(filepath.Join(a.KritixDir, resolvedPath)); err == nil {
				resolvedPath = filepath.Join(a.KritixDir, resolvedPath)
			} else if _, err := os.Stat(filepath.Join(a.RepoRoot, resolvedPath)); err == nil {
				resolvedPath = filepath.Join(a.RepoRoot, resolvedPath)
			}
		}

		rawBytes, err := os.ReadFile(resolvedPath)
		if err != nil {
			violations = append(violations, Violation{
				Gate:        "Provenance",
				File:        "benchmark.json",
				Description: fmt.Sprintf("raw_log_path %q does not exist on disk", rawLogPath),
			})
		} else {
			h := sha256.Sum256(rawBytes)
			computedSHA := hex.EncodeToString(h[:])
			if computedSHA != rawLogSHA {
				violations = append(violations, Violation{
					Gate:        "Provenance",
					File:        "benchmark.json",
					Description: fmt.Sprintf("raw_log_sha256 mismatch: recorded %q vs computed %q", rawLogSHA, computedSHA),
				})
			}
		}
	}

	return violations, nil
}

// CheckASTProvenance scans Go files under pkg/optimizer and cmd for hardcoded fallback metrics.
func (a *Auditor) CheckASTProvenance() ([]Violation, error) {
	var violations []Violation
	fset := token.NewFileSet()

	dirs := []string{
		filepath.Join(a.KritixDir, "pkg", "optimizer"),
		filepath.Join(a.KritixDir, "cmd"),
	}

	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return nil
			}

			ast.Inspect(node, func(n ast.Node) bool {
				// Check for hardcoded Medusa / Saleor 245000 / 185000 constants in structs
				if lit, ok := n.(*ast.BasicLit); ok {
					if lit.Kind == token.STRING && (strings.Contains(lit.Value, "Medusa / Saleor") || strings.Contains(lit.Value, "Medusa Storefront (Next.js")) {
						violations = append(violations, Violation{
							Gate:        "AST Provenance",
							File:        relPath(a.KritixDir, path),
							Line:        fset.Position(lit.Pos()).Line,
							Description: fmt.Sprintf("hardcoded fabricated app name in AST: %s", lit.Value),
						})
					}
					if lit.Kind == token.INT && (lit.Value == "245000" || lit.Value == "185000") {
						violations = append(violations, Violation{
							Gate:        "AST Provenance",
							File:        relPath(a.KritixDir, path),
							Line:        fset.Position(lit.Pos()).Line,
							Description: fmt.Sprintf("hardcoded fabricated LOC in AST: %s", lit.Value),
						})
					}
				}
				return true
			})
			return nil
		})
	}

	return violations, nil
}

// CheckLabelHonesty ensures run names/app labels accurately reflect executed targets.
func (a *Auditor) CheckLabelHonesty() ([]Violation, error) {
	var violations []Violation
	benchPath := filepath.Join(a.KritixDir, "benchmark.json")
	data, err := os.ReadFile(benchPath)
	if err != nil {
		return violations, nil
	}

	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)

	targetApp, _ := m["target_app_name"].(string)
	executedTarget, _ := m["executed_target"].(string)

	if strings.Contains(targetApp, "Medusa") || strings.Contains(targetApp, "Saleor") {
		// App name requires docker digest
		digests, _ := m["docker_image_digests"].([]interface{})
		if len(digests) == 0 {
			violations = append(violations, Violation{
				Gate:        "Label Honesty",
				File:        "benchmark.json",
				Description: fmt.Sprintf("app name %q claimed without docker_image_digests", targetApp),
			})
		}
	}

	if executedTarget == "corpus-classifier" && targetApp != "corpus-classifier" && !strings.Contains(targetApp, "corpus") {
		violations = append(violations, Violation{
			Gate:        "Label Honesty",
			File:        "benchmark.json",
			Description: fmt.Sprintf("target_app_name %q does not match executed_target %q", targetApp, executedTarget),
		})
	}

	return violations, nil
}

// CheckEventIntegrity checks for orphan events in JSONL evidence files.
func (a *Auditor) CheckEventIntegrity() ([]Violation, error) {
	var violations []Violation
	evidenceDir := filepath.Join(a.RepoRoot, ".dev", "kritix-ai", "evidence")
	if _, err := os.Stat(evidenceDir); err != nil {
		return violations, nil
	}

	_ = filepath.Walk(evidenceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		startedEvents := make(map[string]int)
		completedEvents := make(map[string]int)

		for lineIdx, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var ev map[string]interface{}
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			eventName, _ := ev["event"].(string)
			if strings.HasSuffix(eventName, "_started") {
				startedEvents[eventName] = lineIdx + 1
			}
			if strings.HasSuffix(eventName, "_completed") || eventName == "case_evaluated" {
				base := strings.TrimSuffix(eventName, "_completed")
				completedEvents[base+"_started"] = lineIdx + 1
			}
		}

		for startedEv, lineNum := range startedEvents {
			if _, ok := completedEvents[startedEv]; !ok && startedEv == "docker_harness_started" {
				violations = append(violations, Violation{
					Gate:        "Event Integrity",
					File:        relPath(a.RepoRoot, path),
					Line:        lineNum,
					Description: fmt.Sprintf("orphan started event %q with no completion or container records", startedEv),
				})
			}
		}
		return nil
	})

	return violations, nil
}

// CheckSelfCertification prevents tracked or uncaptured ADOPT/VERDICT files.
func (a *Auditor) CheckSelfCertification() ([]Violation, error) {
	var violations []Violation

	// 1. Check tracked files via git ls-files
	cmd := exec.Command("git", "-C", a.RepoRoot, "ls-files", "*ADOPT*", "*VERDICT*", "*COUNCIL*")
	out, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if f != "" {
				violations = append(violations, Violation{
					Gate:        "Self Certification",
					File:        f,
					Description: "tracked *ADOPT*/*VERDICT*/*COUNCIL* file found in git tracking",
				})
			}
		}
	}

	// 2. Check untracked files in .dev/
	checkDevPath := func(dir string) {
		captureLogPath := filepath.Join(dir, "capture_log.jsonl")
		validHashes := make(map[string]bool)
		if logData, err := os.ReadFile(captureLogPath); err == nil {
			for _, line := range strings.Split(string(logData), "\n") {
				var rec map[string]interface{}
				if json.Unmarshal([]byte(line), &rec) == nil {
					if h, ok := rec["sha256"].(string); ok {
						validHashes[h] = true
					}
				}
			}
		}

		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			base := filepath.Base(path)
			inCouncilDir := strings.Contains(path, "COUNCIL_ROUND_") || strings.Contains(path, "COUNCIL_")
			if strings.Contains(base, "ADOPT") || strings.Contains(base, "VERDICT") || inCouncilDir {
				fileBytes, err := os.ReadFile(path)
				if err == nil {
					h := sha256.Sum256(fileBytes)
					hashStr := hex.EncodeToString(h[:])
					if !validHashes[hashStr] {
						violations = append(violations, Violation{
							Gate:        "Self Certification",
							File:        relPath(a.RepoRoot, path),
							Description: fmt.Sprintf("orphan council/verdict file %q hash %q absent from capture log", base, hashStr),
						})
					}
				}
			}
			return nil
		})
	}

	checkDevPath(filepath.Join(a.RepoRoot, ".dev", "kritix-ai"))
	if a.KritixDir != a.RepoRoot {
		checkDevPath(filepath.Join(a.KritixDir, ".dev", "kritix-ai"))
	}

	return violations, nil
}

// CheckDefaultsAndSecrets performs AST analysis for insecure defaults and tokens.
func (a *Auditor) CheckDefaultsAndSecrets() ([]Violation, error) {
	var violations []Violation
	fset := token.NewFileSet()

	pkgDir := filepath.Join(a.KritixDir, "pkg")
	_ = filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			// Check for InsecureSkipVerify: true
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "InsecureSkipVerify" {
					if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
						violations = append(violations, Violation{
							Gate:        "Defaults and Secrets",
							File:        relPath(a.KritixDir, path),
							Line:        fset.Position(kv.Pos()).Line,
							Description: "InsecureSkipVerify: true detected in production code",
						})
					}
				}
			}
			return true
		})
		return nil
	})

	return violations, nil
}

// CheckStatsGate verifies that all test rate assertions use t.Fatalf/t.Errorf, not t.Logf notes.
func (a *Auditor) CheckStatsGate() ([]Violation, error) {
	var violations []Violation
	fset := token.NewFileSet()

	pkgDir := filepath.Join(a.KritixDir, "pkg")
	_ = filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "truthaudit_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Check for forbidden loose logging of CI bounds
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if strings.Contains(line, `t.Logf("Note on CI`) || strings.Contains(line, `t.Logf("CI bound note`) {
				violations = append(violations, Violation{
					Gate:        "Stats Gate",
					File:        relPath(a.KritixDir, path),
					Line:        i + 1,
					Description: "t.Logf-only statistical rate or CI bound note (must assert with t.Fatalf)",
				})
			}
		}

		node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
		if err != nil {
			return nil
		}
		_ = node
		return nil
	})

	return violations, nil
}

// CheckPaths verifies that no /Users/ or file:// paths exist in tracked files under kritix-ai.
func (a *Auditor) CheckPaths() ([]Violation, error) {
	var violations []Violation

	cmd := exec.Command("git", "-C", a.KritixDir, "grep", "-n", "-E", "(/Users/|file://)", "--", ".", ":!scripts/truth-audit.sh", ":!pkg/security/truthaudit/truthaudit.go", ":!pkg/security/truthaudit/truthaudit_test.go")
	out, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				violations = append(violations, Violation{
					Gate:        "Path Exposure",
					File:        line,
					Description: "absolute /Users/ or file:// URL detected in tracked file",
				})
			}
		}
	}

	return violations, nil
}

// CheckPrerequisiteHonesty verifies that harness scripts fail closed when tools are missing from PATH
// and write no result files under evidence/.
func (a *Auditor) CheckPrerequisiteHonesty() ([]Violation, error) {
	var violations []Violation
	harnessScript := filepath.Join(a.KritixDir, "scripts", "harness.sh")
	if abs, err := filepath.Abs(harnessScript); err == nil {
		harnessScript = abs
	}
	if _, err := os.Stat(harnessScript); err != nil {
		return violations, nil
	}

	cmd := exec.Command("bash", harnessScript, "audit_probe")
	cmd.Dir = a.KritixDir
	cmd.Env = []string{
		"PATH=/usr/bin:/bin", // Minimal path without docker, chrome, k6, vault, psql
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		violations = append(violations, Violation{
			Gate:        "Prerequisite Honesty",
			File:        "scripts/harness.sh",
			Description: "harness exited 0 when prerequisites were missing (must fail closed with non-zero exit)",
		})
	}

	if !strings.Contains(stderr.String(), "PREREQUISITE_MISSING") {
		violations = append(violations, Violation{
			Gate:        "Prerequisite Honesty",
			File:        "scripts/harness.sh",
			Description: "harness stderr did not contain PREREQUISITE_MISSING on stripped PATH",
		})
	}

	// Verify no results file written
	probeEvidence := filepath.Join(a.RepoRoot, ".dev", "kritix-ai", "evidence", "audit_probe")
	if _, err := os.Stat(probeEvidence); err == nil {
		_ = os.RemoveAll(probeEvidence)
		violations = append(violations, Violation{
			Gate:        "Prerequisite Honesty",
			File:        "scripts/harness.sh",
			Description: "harness wrote evidence directory when prerequisites were missing",
		})
	}

	return violations, nil
}

// CheckIndependenceGate inspects AST of study tests to ensure case lists are not looped or repeated.
func (a *Auditor) CheckIndependenceGate() ([]Violation, error) {
	var violations []Violation
	fset := token.NewFileSet()

	pkgDir := filepath.Join(a.KritixDir, "pkg")
	_ = filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Test") {
				return true
			}

			// Scan for loops with repeat or multiplier variables inside study or semantic tests
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				if forStmt, ok := inner.(*ast.ForStmt); ok {
					if init, ok := forStmt.Init.(*ast.AssignStmt); ok {
						for _, lhs := range init.Lhs {
							if ident, ok := lhs.(*ast.Ident); ok {
								if ident.Name == "repeat" || ident.Name == "multiplier" {
									violations = append(violations, Violation{
										Gate:        "Independence Gate",
										File:        relPath(a.KritixDir, path),
										Line:        fset.Position(forStmt.Pos()).Line,
										Description: fmt.Sprintf("artificial case-multiplication loop %q detected in %s", ident.Name, fn.Name.Name),
									})
								}
							}
						}
					}
				}
				return true
			})

			return true
		})
		return nil
	})

	return violations, nil
}

// CheckSyntheticHeadline ensures classifier-derived metrics do not appear as unlabelled headlines in README or docs.
func (a *Auditor) CheckSyntheticHeadline() ([]Violation, error) {
	var violations []Violation
	readmePath := filepath.Join(a.KritixDir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			lower := strings.ToLower(line)
			if (strings.HasPrefix(line, "#") || strings.HasPrefix(line, "- **")) && i < 30 {
				if strings.Contains(lower, "90-95% token") || strings.Contains(lower, "99.22%") || strings.Contains(lower, "971→8") {
					if !strings.Contains(lower, "synthetic") {
						violations = append(violations, Violation{
							Gate:        "Synthetic Headline",
							File:        "README.md",
							Line:        i + 1,
							Description: "synthetic token compression figure in headline without synthetic-corpus label",
						})
					}
				}
			}
		}
	}
	return violations, nil
}

// CheckHygiene ensures no stray TestSample or scratch debug tests exist in the codebase.
func (a *Auditor) CheckHygiene() ([]Violation, error) {
	var violations []Violation
	fset := token.NewFileSet()

	pkgDir := filepath.Join(a.KritixDir, "pkg")
	_ = filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Name.Name == "TestSample" || strings.HasPrefix(fn.Name.Name, "TestDebug") || strings.HasPrefix(fn.Name.Name, "TestScratch") {
					violations = append(violations, Violation{
						Gate:        "Hygiene",
						File:        relPath(a.KritixDir, path),
						Line:        fset.Position(fn.Pos()).Line,
						Description: fmt.Sprintf("stray/scratch test function %q found in codebase", fn.Name.Name),
					})
				}
			}
			return true
		})
		return nil
	})

	return violations, nil
}

func getGitHead(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func getGitParent(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD~1")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func relPath(base, target string) string {
	r, err := filepath.Rel(base, target)
	if err == nil {
		return r
	}
	return target
}
