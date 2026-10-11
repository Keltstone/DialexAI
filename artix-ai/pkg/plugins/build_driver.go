package plugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"artix/pkg/policy"
	"artix/pkg/sandbox"
)

// DiagnosticSeverity classifies error levels in compiler or linter output.
type DiagnosticSeverity string

const (
	SeverityError   DiagnosticSeverity = "error"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityInfo    DiagnosticSeverity = "info"
)

// BuildDiagnostic represents a structured compiler or linter message.
type BuildDiagnostic struct {
	File     string             `json:"file"`
	Line     int                `json:"line"`
	Column   int                `json:"column"`
	Severity DiagnosticSeverity `json:"severity"`
	Message  string             `json:"message"`
	Rule     string             `json:"rule,omitempty"`
}

// BuildDriver abstracts language- and build-tool-specific compilation, testing, and linting.
type BuildDriver interface {
	Name() string
	Detect(rootDir string) bool
	Compile(ctx context.Context, rootDir string) *sandbox.ExecResult
	Test(ctx context.Context, rootDir string) *sandbox.ExecResult
	Lint(ctx context.Context, rootDir string) *sandbox.ExecResult
	Warm(ctx context.Context, rootDir string) error
	ParseErrorTrace(output string) []BuildDiagnostic
}

// Registry manages available BuildDriver plugins.
type Registry struct {
	mu      sync.RWMutex
	drivers []BuildDriver
}

// NewRegistry creates a registry populated with default built-in drivers.
func NewRegistry() *Registry {
	r := &Registry{
		drivers: make([]BuildDriver, 0),
	}
	r.Register(&GoDriver{})
	r.Register(&GradleDriver{})
	r.Register(&CargoDriver{})
	r.Register(&NpmDriver{})
	r.Register(&SwiftPMDriver{})
	r.Register(&XcodeDriver{})
	return r
}

// Register adds a build driver to the registry.
func (r *Registry) Register(d BuildDriver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers = append(r.drivers, d)
}

// DetectDriver finds the first registered driver that matches the given repository root.
func (r *Registry) DetectDriver(rootDir string) (BuildDriver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.drivers {
		if d.Detect(rootDir) {
			return d, true
		}
	}
	return nil, false
}

// Go Driver
// GoDriver supports standard Go projects.
type GoDriver struct{}

func (g *GoDriver) Name() string { return "go" }

func (g *GoDriver) Detect(rootDir string) bool {
	_, err := os.Stat(filepath.Join(rootDir, "go.mod"))
	return err == nil
}

func (g *GoDriver) CompileCmd(rootDir string) string {
	return "GOFLAGS=-mod=mod GOPROXY=off go build ./..."
}

func (g *GoDriver) TestCmd(rootDir string) string {
	return "GOFLAGS=-mod=mod GOPROXY=off go test -v ./..."
}

func (g *GoDriver) LintCmd(rootDir string) string {
	return "GOFLAGS=-mod=mod GOPROXY=off go vet ./..."
}

func (g *GoDriver) Warm(ctx context.Context, rootDir string) error {
	cmd := exec.CommandContext(ctx, "go", "mod", "download")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go mod download failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (g *GoDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, g.CompileCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("go", "compile"),
	})
}

func (g *GoDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, g.TestCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("go", "test"),
	})
}

func (g *GoDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, g.LintCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("go", "lint"),
	})
}

var goErrRegex = regexp.MustCompile(`(?m)^([^\s:]+\.go):(\d+):(?:(\d+):)?\s*(.*)$`)

func (g *GoDriver) ParseErrorTrace(output string) []BuildDiagnostic {
	var diags []BuildDiagnostic
	matches := goErrRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		line, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		diags = append(diags, BuildDiagnostic{
			File:     m[1],
			Line:     line,
			Column:   col,
			Severity: SeverityError,
			Message:  strings.TrimSpace(m[4]),
		})
	}
	return diags
}

// Gradle / KMP Driver
// GradleDriver supports Android and Kotlin Multiplatform Gradle projects.
type GradleDriver struct {
	tasksCacheMu sync.RWMutex
	tasksCache   map[string][]string
}

func (gr *GradleDriver) Name() string { return "gradle" }

func (gr *GradleDriver) Detect(rootDir string) bool {
	// Check root build and settings files
	for _, f := range []string{"build.gradle.kts", "build.gradle", "settings.gradle.kts", "settings.gradle"} {
		if _, err := os.Stat(filepath.Join(rootDir, f)); err == nil {
			return true
		}
	}
	// Check immediate subdirectories for build files (nested/multi-module)
	entries, err := os.ReadDir(rootDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				sub := filepath.Join(rootDir, e.Name())
				if _, err := os.Stat(filepath.Join(sub, "build.gradle.kts")); err == nil {
					return true
				}
				if _, err := os.Stat(filepath.Join(sub, "build.gradle")); err == nil {
					return true
				}
			}
		}
	}
	return false
}

func (gr *GradleDriver) IsKMP(rootDir string) bool {
	kmpIndicators := []string{
		`kotlin("multiplatform")`,
		`kotlin(\"multiplatform\")`,
		`org.jetbrains.kotlin.multiplatform`,
		`kotlin-multiplatform`,
		`libs.plugins.kotlinMultiplatform`,
		`alias(libs.plugins.kotlinMultiplatform)`,
	}
	checkFile := func(path string) bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		content := string(data)
		for _, ind := range kmpIndicators {
			if strings.Contains(content, ind) {
				return true
			}
		}
		return false
	}

	for _, f := range []string{"build.gradle.kts", "build.gradle", "settings.gradle.kts", "settings.gradle"} {
		if checkFile(filepath.Join(rootDir, f)) {
			return true
		}
	}

	// Check subdirectories
	entries, err := os.ReadDir(rootDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				sub := filepath.Join(rootDir, e.Name())
				for _, f := range []string{"build.gradle.kts", "build.gradle"} {
					if checkFile(filepath.Join(sub, f)) {
						return true
					}
				}
				if _, err := os.Stat(filepath.Join(sub, "src", "commonMain")); err == nil {
					return true
				}
			}
		}
	}

	if _, err := os.Stat(filepath.Join(rootDir, "src", "commonMain")); err == nil {
		return true
	}
	return false
}

func (gr *GradleDriver) gradlewCmd(rootDir string) string {
	gradlew := "./gradlew"
	if _, err := os.Stat(filepath.Join(rootDir, "gradlew")); err != nil {
		gradlew = "gradle"
	}
	return gradlew
}

func (gr *GradleDriver) CompileCmd(rootDir string) string {
	return gr.gradlewCmd(rootDir) + " assembleDebug --offline"
}

func (gr *GradleDriver) TestCmd(rootDir string) string {
	if gr.IsKMP(rootDir) {
		selected, _, _ := gr.ResolveKMPTasks(context.Background(), rootDir)
		if len(selected) > 0 {
			return gr.gradlewCmd(rootDir) + " " + strings.Join(selected, " ") + " --offline"
		}
	}
	return gr.gradlewCmd(rootDir) + " testDebugUnitTest --offline"
}

func (gr *GradleDriver) LintCmd(rootDir string) string {
	return gr.gradlewCmd(rootDir) + " ktlintCheck --offline"
}

func (gr *GradleDriver) ResolveKMPTasks(ctx context.Context, rootDir string) ([]string, map[string]string, error) {
	allTasks, err := gr.fetchAllTasks(ctx, rootDir)
	if err != nil {
		return nil, nil, err
	}

	host := runtime.GOOS
	skippedTargets := gr.EvaluateKMPTargetsForHost(host, allTasks)

	var selected []string
	taskSet := make(map[string]bool)
	for _, t := range allTasks {
		taskSet[t] = true
	}

	candidates := []string{"jvmTest", "desktopTest", "testDebugUnitTest"}
	for _, c := range candidates {
		if taskSet[c] {
			selected = append(selected, c)
		}
	}

	if host == "darwin" && taskSet["iosSimulatorArm64Test"] {
		selected = append(selected, "iosSimulatorArm64Test")
	}

	if len(selected) == 0 {
		if taskSet["allTests"] && len(skippedTargets) == 0 {
			selected = append(selected, "allTests")
		} else if taskSet["testDebugUnitTest"] {
			selected = append(selected, "testDebugUnitTest")
		} else if taskSet["test"] {
			selected = append(selected, "test")
		}
	}

	return selected, skippedTargets, nil
}

func (gr *GradleDriver) fetchAllTasks(ctx context.Context, rootDir string) ([]string, error) {
	gr.tasksCacheMu.RLock()
	if tasks, ok := gr.tasksCache[rootDir]; ok {
		gr.tasksCacheMu.RUnlock()
		return tasks, nil
	}
	gr.tasksCacheMu.RUnlock()

	gradlew := gr.gradlewCmd(rootDir)
	cmd := exec.CommandContext(ctx, "sh", "-c", gradlew+" tasks --all")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return []string{"testDebugUnitTest"}, nil
	}

	var tasks []string
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			taskName := fields[0]
			if !strings.Contains(taskName, " ") && len(taskName) > 2 {
				tasks = append(tasks, taskName)
			}
		}
	}

	gr.tasksCacheMu.Lock()
	if gr.tasksCache == nil {
		gr.tasksCache = make(map[string][]string)
	}
	gr.tasksCache[rootDir] = tasks
	gr.tasksCacheMu.Unlock()

	return tasks, nil
}

func (gr *GradleDriver) EvaluateKMPTargetsForHost(hostOS string, allTasks []string) map[string]string {
	skipped := make(map[string]string)
	if strings.ToLower(hostOS) != "darwin" {
		for _, t := range allTasks {
			tLower := strings.ToLower(t)
			if strings.HasPrefix(tLower, "ios") || strings.Contains(tLower, "iossimulator") || strings.Contains(tLower, "iosarm") || strings.Contains(tLower, "iostest") {
				skipped[t] = "SKIPPED_HOST_UNSUPPORTED"
			}
		}
	}
	return skipped
}

func (gr *GradleDriver) FormatTargetStatusSummary(passedTasks []string, skippedTargets map[string]string) string {
	if len(skippedTargets) > 0 {
		var skippedParts []string
		for target, status := range skippedTargets {
			skippedParts = append(skippedParts, fmt.Sprintf("%s: %s", target, status))
		}
		return fmt.Sprintf("Passed: [%s]; Skipped: [%s]", strings.Join(passedTasks, ", "), strings.Join(skippedParts, ", "))
	}
	return "All targets green"
}

func (gr *GradleDriver) Warm(ctx context.Context, rootDir string) error {
	gradlew := gr.gradlewCmd(rootDir)
	cmd := exec.CommandContext(ctx, "sh", "-c", gradlew+" --refresh-dependencies dependencies testClasses")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gradle pre-warm failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (gr *GradleDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, gr.CompileCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("gradle", "compile"),
	})
}

func (gr *GradleDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, gr.TestCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("gradle", "test"),
	})
}

func (gr *GradleDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, gr.LintCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("gradle", "lint"),
	})
}

var kotlinErrRegex = regexp.MustCompile(`(?m)^([^\s:]+\.(?:kt|java)):(\d+):(\d+):\s*(?:([we]):\s*)?(.*)$`)

func (gr *GradleDriver) ParseErrorTrace(output string) []BuildDiagnostic {
	var diags []BuildDiagnostic
	matches := kotlinErrRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		sev := SeverityError
		if m[4] == "w" {
			sev = SeverityWarning
		}
		diags = append(diags, BuildDiagnostic{
			File:     m[1],
			Line:     line,
			Column:   col,
			Severity: sev,
			Message:  strings.TrimSpace(m[5]),
		})
	}
	return diags
}

// Cargo / Rust Driver
// CargoDriver supports Rust Cargo projects.
type CargoDriver struct{}

func (c *CargoDriver) Name() string { return "cargo" }

func (c *CargoDriver) Detect(rootDir string) bool {
	_, err := os.Stat(filepath.Join(rootDir, "Cargo.toml"))
	return err == nil
}

func (c *CargoDriver) CompileCmd(rootDir string) string {
	return "cargo check --offline"
}

func (c *CargoDriver) TestCmd(rootDir string) string {
	return "cargo test --offline"
}

func (c *CargoDriver) LintCmd(rootDir string) string {
	return "cargo clippy --offline"
}

func (c *CargoDriver) Warm(ctx context.Context, rootDir string) error {
	cmd := exec.CommandContext(ctx, "cargo", "fetch")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cargo fetch failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *CargoDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, c.CompileCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("cargo", "compile"),
	})
}

func (c *CargoDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, c.TestCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("cargo", "test"),
	})
}

func (c *CargoDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, c.LintCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("cargo", "lint"),
	})
}

var cargoErrRegex = regexp.MustCompile(`(?m)^(error|warning)(?:\[([A-Za-z0-9_]+)\])?:\s+(.*)\n\s+-->\s+([^:]+):(\d+):(\d+)`)

func (c *CargoDriver) ParseErrorTrace(output string) []BuildDiagnostic {
	var diags []BuildDiagnostic
	matches := cargoErrRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		line, _ := strconv.Atoi(m[5])
		col, _ := strconv.Atoi(m[6])
		sev := SeverityError
		if m[1] == "warning" {
			sev = SeverityWarning
		}
		diags = append(diags, BuildDiagnostic{
			File:     m[4],
			Line:     line,
			Column:   col,
			Severity: sev,
			Message:  strings.TrimSpace(m[3]),
			Rule:     m[2],
		})
	}
	return diags
}

// npm / Node Driver
// NpmDriver supports Node.js / TypeScript projects.
type NpmDriver struct{}

func (n *NpmDriver) Name() string { return "npm" }

func (n *NpmDriver) Detect(rootDir string) bool {
	_, err := os.Stat(filepath.Join(rootDir, "package.json"))
	return err == nil
}

func (n *NpmDriver) CompileCmd(rootDir string) string {
	return "npm run build --if-present --offline"
}

func (n *NpmDriver) TestCmd(rootDir string) string {
	return "npm test --offline"
}

func (n *NpmDriver) LintCmd(rootDir string) string {
	return "npm run lint --if-present --offline"
}

func (n *NpmDriver) Warm(ctx context.Context, rootDir string) error {
	cmd := exec.CommandContext(ctx, "npm", "ci")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm ci failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (n *NpmDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, n.CompileCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("npm", "compile"),
	})
}

func (n *NpmDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, n.TestCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("npm", "test"),
	})
}

func (n *NpmDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, n.LintCmd(rootDir), &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: policy.GetTimeout("npm", "lint"),
	})
}

var tsErrRegex = regexp.MustCompile(`(?m)^([^\s:(]+)(?:\((\d+),(\d+)\)|:(\d+):(\d+)):\s*(error|warning)\s*(?:TS\d+:)?\s*(.*)$`)

func (n *NpmDriver) ParseErrorTrace(output string) []BuildDiagnostic {
	var diags []BuildDiagnostic
	matches := tsErrRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		line := 0
		col := 0
		if m[2] != "" {
			line, _ = strconv.Atoi(m[2])
			col, _ = strconv.Atoi(m[3])
		} else if m[4] != "" {
			line, _ = strconv.Atoi(m[4])
			col, _ = strconv.Atoi(m[5])
		}
		sev := SeverityError
		if m[6] == "warning" {
			sev = SeverityWarning
		}
		diags = append(diags, BuildDiagnostic{
			File:     m[1],
			Line:     line,
			Column:   col,
			Severity: sev,
			Message:  strings.TrimSpace(m[7]),
		})
	}
	return diags
}

// DetectOfflineCacheMiss parses build failure output and identifies any missing offline artifact.
func DetectOfflineCacheMiss(output string) (string, bool) {
	// 1. Gradle patterns
	reGradleResolve := regexp.MustCompile(`(?i)(?:Could not resolve|Could not download)\s+([^\s:]+:[^\s:]+:[^\s:]+)[.\s]`)
	if m := reGradleResolve.FindStringSubmatch(output); len(m) > 1 {
		return strings.TrimRight(m[1], "."), true
	}
	reGradleOffline := regexp.MustCompile(`(?i)No cached version of\s+([^\s:]+:[^\s:]+:[^\s:]+)\s+available for offline mode`)
	if m := reGradleOffline.FindStringSubmatch(output); len(m) > 1 {
		return m[1], true
	}

	// 2. Go patterns
	reGoProxy := regexp.MustCompile(`(?:go:\s+)?([^\s:]+@[^\s:]+|[^\s:]+):\s*(?:reading\s+[^\s:]+:)?\s*GOPROXY=off`)
	if m := reGoProxy.FindStringSubmatch(output); len(m) > 1 {
		return m[1], true
	}
	reGoFind := regexp.MustCompile(`cannot find module providing package\s+([^\s:]+)`)
	if m := reGoFind.FindStringSubmatch(output); len(m) > 1 {
		return m[1], true
	}

	// 3. Cargo patterns
	reCargo := regexp.MustCompile(`(?i)failed to select a version for the requirement\s+['"` + "`" + `]?([a-zA-Z0-9_\-]+)`)
	if m := reCargo.FindStringSubmatch(output); len(m) > 1 {
		return m[1], true
	}

	// 4. Npm patterns
	reNpmCache := regexp.MustCompile(`(?i)package\s+([@a-zA-Z0-9_\-\./]+)\s+not in cache`)
	if m := reNpmCache.FindStringSubmatch(output); len(m) > 1 {
		return m[1], true
	}
	reNpmENOTCACHED := regexp.MustCompile(`(?i)ENOTCACHED`)
	if reNpmENOTCACHED.MatchString(output) {
		reNpmPkg := regexp.MustCompile(`(?:npm ERR!\s+)?(?:request to https?://[^\s]+/([^\s/]+)|package\s+([^\s]+))`)
		if m := reNpmPkg.FindStringSubmatch(output); len(m) > 1 {
			pkg := m[1]
			if pkg == "" && len(m) > 2 {
				pkg = m[2]
			}
			if pkg != "" {
				return pkg, true
			}
		}
		return "unknown-npm-package", true
	}

	return "", false
}

// SwiftPMDriver supports Swift Package Manager projects.
type SwiftPMDriver struct{}

func (s *SwiftPMDriver) Name() string { return "swiftpm" }

func (s *SwiftPMDriver) Detect(rootDir string) bool {
	_, err := os.Stat(filepath.Join(rootDir, "Package.swift"))
	return err == nil
}

func (s *SwiftPMDriver) CompileCmd(rootDir string) string {
	return "swift build"
}

func (s *SwiftPMDriver) TestCmd(rootDir string) string {
	return "swift test"
}

func (s *SwiftPMDriver) LintCmd(rootDir string) string {
	return "swiftlint"
}

func (s *SwiftPMDriver) Warm(ctx context.Context, rootDir string) error {
	cmd := exec.CommandContext(ctx, "swift", "package", "resolve")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("swift package resolve failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *SwiftPMDriver) ExecuteOnHost(hostOS string, ctx context.Context, rootDir, phase string) *sandbox.ExecResult {
	if strings.ToLower(hostOS) != "darwin" {
		return &sandbox.ExecResult{
			Command:  "swift " + phase,
			ExitCode: 0,
			Stdout:   "SKIPPED_HOST_UNSUPPORTED: Swift toolchains require macOS host (current: " + hostOS + ")",
			Error:    "SKIPPED_HOST_UNSUPPORTED",
		}
	}
	box := sandbox.NewSandbox(rootDir)
	var cmd string
	var timeout time.Duration
	switch phase {
	case "compile":
		cmd = s.CompileCmd(rootDir)
		timeout = policy.GetTimeout("swiftpm", "compile")
	case "test":
		cmd = s.TestCmd(rootDir)
		timeout = policy.GetTimeout("swiftpm", "test")
	case "lint":
		cmd = s.LintCmd(rootDir)
		timeout = policy.GetTimeout("swiftpm", "lint")
	default:
		cmd = s.TestCmd(rootDir)
		timeout = policy.GetTimeout("swiftpm", "test")
	}
	return box.Run(ctx, cmd, &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: timeout,
	})
}

func (s *SwiftPMDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	return s.ExecuteOnHost(runtime.GOOS, ctx, rootDir, "compile")
}

func (s *SwiftPMDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	return s.ExecuteOnHost(runtime.GOOS, ctx, rootDir, "test")
}

func (s *SwiftPMDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	return s.ExecuteOnHost(runtime.GOOS, ctx, rootDir, "lint")
}

var swiftErrRegex = regexp.MustCompile(`(?m)^([^\s:]+\.swift):(\d+):(?:(\d+):)?\s*(error|warning):\s*(.*)$`)

func (s *SwiftPMDriver) ParseErrorTrace(output string) []BuildDiagnostic {
	var diags []BuildDiagnostic
	matches := swiftErrRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		line, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		sev := SeverityError
		if m[4] == "warning" {
			sev = SeverityWarning
		}
		diags = append(diags, BuildDiagnostic{
			File:     m[1],
			Line:     line,
			Column:   col,
			Severity: sev,
			Message:  strings.TrimSpace(m[5]),
		})
	}
	return diags
}

// XcodeDriver supports native Xcode projects and workspaces.
type XcodeDriver struct {
	Destination string
}

func (x *XcodeDriver) Name() string { return "xcode" }

func (x *XcodeDriver) Detect(rootDir string) bool {
	matchesProj, _ := filepath.Glob(filepath.Join(rootDir, "*.xcodeproj"))
	matchesWs, _ := filepath.Glob(filepath.Join(rootDir, "*.xcworkspace"))
	return len(matchesProj) > 0 || len(matchesWs) > 0
}

func (x *XcodeDriver) DetectScheme(rootDir string) string {
	matchesProj, _ := filepath.Glob(filepath.Join(rootDir, "*.xcodeproj"))
	if len(matchesProj) > 0 {
		base := filepath.Base(matchesProj[0])
		return strings.TrimSuffix(base, ".xcodeproj")
	}
	matchesWs, _ := filepath.Glob(filepath.Join(rootDir, "*.xcworkspace"))
	if len(matchesWs) > 0 {
		base := filepath.Base(matchesWs[0])
		return strings.TrimSuffix(base, ".xcworkspace")
	}
	return "App"
}

func (x *XcodeDriver) getDestination() string {
	if x.Destination != "" {
		return x.Destination
	}
	return "platform=iOS Simulator,name=iPhone 16"
}

func (x *XcodeDriver) derivedDataPath(rootDir string) string {
	return filepath.Join(rootDir, ".derivedData")
}

func (x *XcodeDriver) CompileCmd(rootDir string) string {
	scheme := x.DetectScheme(rootDir)
	return fmt.Sprintf("xcodebuild build -scheme %s -destination '%s' -derivedDataPath %s",
		scheme, x.getDestination(), x.derivedDataPath(rootDir))
}

func (x *XcodeDriver) TestCmd(rootDir string) string {
	scheme := x.DetectScheme(rootDir)
	return fmt.Sprintf("xcodebuild test -scheme %s -destination '%s' -derivedDataPath %s",
		scheme, x.getDestination(), x.derivedDataPath(rootDir))
}

func (x *XcodeDriver) LintCmd(rootDir string) string {
	scheme := x.DetectScheme(rootDir)
	return fmt.Sprintf("xcodebuild analyze -scheme %s -destination '%s' -derivedDataPath %s",
		scheme, x.getDestination(), x.derivedDataPath(rootDir))
}

func (x *XcodeDriver) Warm(ctx context.Context, rootDir string) error {
	cmd := exec.CommandContext(ctx, "xcodebuild", "-resolvePackageDependencies")
	cmd.Dir = rootDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xcode dependency resolve failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (x *XcodeDriver) ExecuteOnHost(hostOS string, ctx context.Context, rootDir, phase string) *sandbox.ExecResult {
	if strings.ToLower(hostOS) != "darwin" {
		return &sandbox.ExecResult{
			Command:  "xcodebuild " + phase,
			ExitCode: 0,
			Stdout:   "SKIPPED_HOST_UNSUPPORTED: Xcode and iOS Simulators require macOS host (current: " + hostOS + ")",
			Error:    "SKIPPED_HOST_UNSUPPORTED",
		}
	}
	box := sandbox.NewSandbox(rootDir)
	var cmd string
	var timeout time.Duration
	switch phase {
	case "compile":
		cmd = x.CompileCmd(rootDir)
		timeout = policy.GetTimeout("xcode", "compile")
	case "test":
		cmd = x.TestCmd(rootDir)
		timeout = policy.GetTimeout("xcode", "test")
	case "lint":
		cmd = x.LintCmd(rootDir)
		timeout = policy.GetTimeout("xcode", "lint")
	default:
		cmd = x.TestCmd(rootDir)
		timeout = policy.GetTimeout("xcode", "test")
	}
	return box.Run(ctx, cmd, &sandbox.ExecOptions{
		Cwd:     rootDir,
		Timeout: timeout,
	})
}

func (x *XcodeDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	return x.ExecuteOnHost(runtime.GOOS, ctx, rootDir, "compile")
}

func (x *XcodeDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	return x.ExecuteOnHost(runtime.GOOS, ctx, rootDir, "test")
}

func (x *XcodeDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	return x.ExecuteOnHost(runtime.GOOS, ctx, rootDir, "lint")
}

func (x *XcodeDriver) ParseErrorTrace(output string) []BuildDiagnostic {
	var diags []BuildDiagnostic
	matches := swiftErrRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		line, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		sev := SeverityError
		if m[4] == "warning" {
			sev = SeverityWarning
		}
		diags = append(diags, BuildDiagnostic{
			File:     m[1],
			Line:     line,
			Column:   col,
			Severity: sev,
			Message:  strings.TrimSpace(m[5]),
		})
	}
	return diags
}
