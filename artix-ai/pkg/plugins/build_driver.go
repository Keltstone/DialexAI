package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"artix/pkg/metrics"
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
	if policy.IsEnterprise() && !sandbox.IsConfinementAvailable() {
		return fmt.Errorf("warm refused in enterprise mode: confinement is unavailable")
	}
	box := sandbox.NewSandbox(rootDir)
	res := box.RunWithNetwork(ctx, "go mod download", &sandbox.ExecOptions{
		Cwd:       rootDir,
		Timeout:   policy.GetTimeout("go", "warm"),
		Toolchain: "go",
	})
	if !res.Success() {
		return fmt.Errorf("go mod download failed: %s (%s)", res.Error, res.Stderr)
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

	if err := gr.ValidateWrapper(rootDir); err != nil {
		return nil, err
	}

	gradlew := gr.gradlewCmd(rootDir)
	box := sandbox.NewSandbox(rootDir)
	res := box.Run(ctx, gradlew+" tasks --all", &sandbox.ExecOptions{
		Cwd:           rootDir,
		Toolchain:     "gradle",
		AllowLoopback: true,
	})
	if !res.Success() {
		return []string{"testDebugUnitTest"}, nil
	}

	var tasks []string
	lines := strings.Split(res.Stdout, "\n")
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

func parseJavaProperties(data []byte) map[string]string {
	props := make(map[string]string)
	rawLines := strings.Split(string(data), "\n")
	var combinedLines []string

	var current strings.Builder
	for _, l := range rawLines {
		l = strings.TrimRight(l, "\r")
		trimmed := strings.TrimSpace(l)
		if current.Len() == 0 && (strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!")) {
			continue
		}
		// Count trailing backslashes
		bsCount := 0
		for i := len(l) - 1; i >= 0; i-- {
			if l[i] == '\\' {
				bsCount++
			} else {
				break
			}
		}
		if bsCount%2 == 1 {
			// Line continuation: strip trailing backslash and append
			current.WriteString(l[:len(l)-1])
		} else {
			current.WriteString(l)
			combinedLines = append(combinedLines, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		combinedLines = append(combinedLines, current.String())
	}

	for _, line := range combinedLines {
		line = strings.TrimLeft(line, " \t\f\r\n")
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		var keyBuilder strings.Builder
		i := 0
		n := len(line)
		escaped := false
		for i < n {
			c := line[i]
			if escaped {
				keyBuilder.WriteByte(c)
				escaped = false
				i++
				continue
			}
			if c == '\\' {
				escaped = true
				i++
				continue
			}
			if c == '=' || c == ':' || c == ' ' || c == '\t' || c == '\f' {
				break
			}
			keyBuilder.WriteByte(c)
			i++
		}
		key := keyBuilder.String()

		for i < n && (line[i] == ' ' || line[i] == '\t' || line[i] == '\f') {
			i++
		}
		if i < n && (line[i] == '=' || line[i] == ':') {
			i++
			for i < n && (line[i] == ' ' || line[i] == '\t' || line[i] == '\f') {
				i++
			}
		}
		val := line[i:]
		props[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return props
}

func (gr *GradleDriver) ValidateWrapper(rootDir string) error {
	propsPath := filepath.Join(rootDir, "gradle", "wrapper", "gradle-wrapper.properties")
	data, err := os.ReadFile(propsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	props := parseJavaProperties(data)
	for k, rawURL := range props {
		normKey := strings.ToLower(strings.TrimSpace(k))
		if normKey == "distributionurl" || strings.Contains(normKey, "distributionurl") {
			rawURL = strings.ReplaceAll(rawURL, `\:`, ":")
			rawURL = strings.ReplaceAll(rawURL, `\/`, "/")
			u, err := url.Parse(rawURL)
			if err != nil || u.Scheme != "https" {
				return fmt.Errorf("untrusted gradle wrapper distributionUrl: %s", rawURL)
			}
			host := strings.ToLower(u.Hostname())
			if host != "services.gradle.org" && host != "downloads.gradle-dn.com" &&
				!strings.HasSuffix(host, ".services.gradle.org") &&
				!strings.HasSuffix(host, ".downloads.gradle-dn.com") {
				return fmt.Errorf("untrusted gradle wrapper distributionUrl: %s", rawURL)
			}
		}
	}
	return nil
}

func (gr *GradleDriver) ResolveTasksToWarm(ctx context.Context, rootDir string) []string {
	var tasks []string
	allTasks, err := gr.fetchAllTasks(ctx, rootDir)
	taskMap := make(map[string]bool)
	if err == nil {
		for _, t := range allTasks {
			taskMap[t] = true
		}
	}

	if taskMap["downloadKotlinNativeDistribution"] {
		tasks = append(tasks, "downloadKotlinNativeDistribution")
	}

	if gr.IsKMP(rootDir) {
		selected, _, _ := gr.ResolveKMPTasks(ctx, rootDir)
		for _, t := range selected {
			tasks = append(tasks, t)
			tLower := strings.ToLower(t)
			if strings.HasPrefix(tLower, "ios") || strings.Contains(tLower, "native") {
				if taskMap["compileTestKotlinIosSimulatorArm64"] {
					tasks = append(tasks, "compileTestKotlinIosSimulatorArm64")
				}
				if taskMap["linkDebugTestIosSimulatorArm64"] {
					tasks = append(tasks, "linkDebugTestIosSimulatorArm64")
				}
			}
		}
		if len(tasks) == 0 || (len(tasks) == 1 && taskMap["downloadKotlinNativeDistribution"]) {
			tasks = append(tasks, "testClasses")
		}
	} else {
		tasks = append(tasks, "testClasses")
	}

	seen := make(map[string]bool)
	var deduped []string
	for _, t := range tasks {
		if !seen[t] && t != "" {
			seen[t] = true
			deduped = append(deduped, t)
		}
	}
	return deduped
}

func (gr *GradleDriver) Warm(ctx context.Context, rootDir string) error {
	if policy.IsEnterprise() && !sandbox.IsConfinementAvailable() {
		return fmt.Errorf("warm refused in enterprise mode: confinement is unavailable")
	}
	if err := gr.ValidateWrapper(rootDir); err != nil {
		return err
	}
	gradlew := gr.gradlewCmd(rootDir)
	warmTasks := gr.ResolveTasksToWarm(ctx, rootDir)
	cmdStr := gradlew + " --refresh-dependencies dependencies " + strings.Join(warmTasks, " ")

	box := sandbox.NewSandbox(rootDir)
	res := box.RunWithNetwork(ctx, cmdStr, &sandbox.ExecOptions{
		Cwd:           rootDir,
		Timeout:       policy.GetTimeout("gradle", "warm"),
		Toolchain:     "gradle",
		AllowLoopback: true,
	})
	if !res.Success() {
		combinedErr := res.Stderr + "\n" + res.Stdout
		if artifact, ok := DetectOfflineCacheMiss(combinedErr); ok {
			metrics.RecordOfflineCacheMiss()
			policy.BlockAutonomy(fmt.Sprintf("OFFLINE_CACHE_MISS: %s", artifact))
			return fmt.Errorf("OFFLINE_CACHE_MISS: %s", artifact)
		}
		return fmt.Errorf("gradle pre-warm failed: %s (%s)", res.Error, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func (gr *GradleDriver) Compile(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, gr.CompileCmd(rootDir), &sandbox.ExecOptions{
		Cwd:           rootDir,
		Timeout:       policy.GetTimeout("gradle", "compile"),
		Toolchain:     "gradle",
		AllowLoopback: true,
	})
}

func (gr *GradleDriver) Test(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, gr.TestCmd(rootDir), &sandbox.ExecOptions{
		Cwd:           rootDir,
		Timeout:       policy.GetTimeout("gradle", "test"),
		Toolchain:     "gradle",
		AllowLoopback: true,
	})
}

func (gr *GradleDriver) Lint(ctx context.Context, rootDir string) *sandbox.ExecResult {
	box := sandbox.NewSandbox(rootDir)
	return box.Run(ctx, gr.LintCmd(rootDir), &sandbox.ExecOptions{
		Cwd:           rootDir,
		Timeout:       policy.GetTimeout("gradle", "lint"),
		Toolchain:     "gradle",
		AllowLoopback: true,
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
	if policy.IsEnterprise() && !sandbox.IsConfinementAvailable() {
		return fmt.Errorf("warm refused in enterprise mode: confinement is unavailable")
	}
	box := sandbox.NewSandbox(rootDir)
	res := box.RunWithNetwork(ctx, "cargo fetch", &sandbox.ExecOptions{
		Cwd:       rootDir,
		Timeout:   policy.GetTimeout("cargo", "warm"),
		Toolchain: "cargo",
	})
	if !res.Success() {
		return fmt.Errorf("cargo fetch failed: %s (%s)", res.Error, strings.TrimSpace(res.Stderr))
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
	if policy.IsEnterprise() && !sandbox.IsConfinementAvailable() {
		return fmt.Errorf("warm refused in enterprise mode: confinement is unavailable")
	}
	box := sandbox.NewSandbox(rootDir)
	res := box.RunWithNetwork(ctx, "npm ci --ignore-scripts", &sandbox.ExecOptions{
		Cwd:       rootDir,
		Timeout:   policy.GetTimeout("npm", "warm"),
		Toolchain: "npm",
	})
	if !res.Success() {
		return fmt.Errorf("npm ci failed: %s (%s)", res.Error, strings.TrimSpace(res.Stderr))
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
	return "swift test --disable-sandbox"
}

func (s *SwiftPMDriver) LintCmd(rootDir string) string {
	return "swiftlint"
}

func (s *SwiftPMDriver) Warm(ctx context.Context, rootDir string) error {
	if policy.IsEnterprise() && !sandbox.IsConfinementAvailable() {
		return fmt.Errorf("warm refused in enterprise mode: confinement is unavailable")
	}
	box := sandbox.NewSandbox(rootDir)
	res := box.RunWithNetwork(ctx, "swift package resolve", &sandbox.ExecOptions{
		Cwd:       rootDir,
		Timeout:   policy.GetTimeout("swiftpm", "warm"),
		Toolchain: "swiftpm",
	})
	if !res.Success() {
		return fmt.Errorf("swift package resolve failed: %s (%s)", res.Error, strings.TrimSpace(res.Stderr))
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
		Cwd:       rootDir,
		Timeout:   timeout,
		Toolchain: "swiftpm",
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

var (
	cachedSimulatorName string
	cachedSimulatorMu   sync.Mutex
)

func detectAvailableSimulator() string {
	cachedSimulatorMu.Lock()
	defer cachedSimulatorMu.Unlock()
	if cachedSimulatorName != "" {
		return cachedSimulatorName
	}

	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "xcrun", "simctl", "list", "devices", "available", "-j")
		out, err := cmd.Output()
		if err == nil {
			var simData struct {
				Devices map[string][]struct {
					Name        string `json:"name"`
					IsAvailable bool   `json:"isAvailable"`
					State       string `json:"state"`
				} `json:"devices"`
			}
			if json.Unmarshal(out, &simData) == nil {
				for _, devList := range simData.Devices {
					for _, d := range devList {
						if d.IsAvailable && strings.HasPrefix(d.Name, "iPhone") {
							cachedSimulatorName = d.Name
							return cachedSimulatorName
						}
					}
				}
			}
		}
	}

	cachedSimulatorName = "iPhone 17"
	return cachedSimulatorName
}

func (x *XcodeDriver) DetectScheme(rootDir string) string {
	// 1. Check Package.swift
	pkgSwift := filepath.Join(rootDir, "Package.swift")
	if data, err := os.ReadFile(pkgSwift); err == nil {
		re := regexp.MustCompile(`name:\s*"([^"]+)"`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			return m[1]
		}
	}

	// 2. Query xcodebuild -list -json if available on darwin
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "xcodebuild", "-list", "-json")
		cmd.Dir = rootDir
		if out, err := cmd.Output(); err == nil {
			var listData struct {
				Project struct {
					Schemes []string `json:"schemes"`
				} `json:"project"`
				Workspace struct {
					Schemes []string `json:"schemes"`
				} `json:"workspace"`
			}
			if json.Unmarshal(out, &listData) == nil {
				if len(listData.Workspace.Schemes) > 0 {
					return listData.Workspace.Schemes[0]
				}
				if len(listData.Project.Schemes) > 0 {
					return listData.Project.Schemes[0]
				}
			}
		}
	}

	// 3. Fallback to globbing project/workspace files
	matchesWs, _ := filepath.Glob(filepath.Join(rootDir, "*.xcworkspace"))
	if len(matchesWs) > 0 {
		base := filepath.Base(matchesWs[0])
		return strings.TrimSuffix(base, ".xcworkspace")
	}
	matchesProj, _ := filepath.Glob(filepath.Join(rootDir, "*.xcodeproj"))
	if len(matchesProj) > 0 {
		base := filepath.Base(matchesProj[0])
		return strings.TrimSuffix(base, ".xcodeproj")
	}

	return "App"
}

func (x *XcodeDriver) getDestination() string {
	if x.Destination != "" {
		return x.Destination
	}
	sim := detectAvailableSimulator()
	return fmt.Sprintf("platform=iOS Simulator,name=%s", sim)
}

func (x *XcodeDriver) derivedDataPath(rootDir string) string {
	return filepath.Join(rootDir, ".derivedData")
}

func (x *XcodeDriver) EnsureDerivedDataExcluded(rootDir string) {
	cmd := exec.Command("git", "rev-parse", "--git-path", "info/exclude")
	cmd.Dir = rootDir
	out, err := cmd.Output()
	var excludePath string
	if err == nil {
		excludePath = strings.TrimSpace(string(out))
		if !filepath.IsAbs(excludePath) {
			excludePath = filepath.Join(rootDir, excludePath)
		}
	} else {
		gitDir := filepath.Join(rootDir, ".git")
		if fi, sErr := os.Stat(gitDir); sErr == nil && fi.IsDir() {
			excludePath = filepath.Join(gitDir, "info", "exclude")
		} else {
			return
		}
	}
	_ = os.MkdirAll(filepath.Dir(excludePath), 0755)
	data, _ := os.ReadFile(excludePath)
	if !strings.Contains(string(data), ".derivedData") {
		f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			defer f.Close()
			if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
				_, _ = f.WriteString("\n")
			}
			_, _ = f.WriteString(".derivedData\n.derivedData/**\n")
		}
	}
}

func (x *XcodeDriver) CompileCmd(rootDir string) string {
	x.EnsureDerivedDataExcluded(rootDir)
	scheme := x.DetectScheme(rootDir)
	return fmt.Sprintf("xcodebuild build -scheme %s -destination '%s' -derivedDataPath %s -IDEPackageSupportDisableManifestSandbox=1",
		scheme, x.getDestination(), x.derivedDataPath(rootDir))
}

func (x *XcodeDriver) TestCmd(rootDir string) string {
	x.EnsureDerivedDataExcluded(rootDir)
	scheme := x.DetectScheme(rootDir)
	return fmt.Sprintf("xcodebuild test -scheme %s -destination '%s' -derivedDataPath %s -IDEPackageSupportDisableManifestSandbox=1",
		scheme, x.getDestination(), x.derivedDataPath(rootDir))
}

func (x *XcodeDriver) LintCmd(rootDir string) string {
	x.EnsureDerivedDataExcluded(rootDir)
	scheme := x.DetectScheme(rootDir)
	return fmt.Sprintf("xcodebuild analyze -scheme %s -destination '%s' -derivedDataPath %s -IDEPackageSupportDisableManifestSandbox=1",
		scheme, x.getDestination(), x.derivedDataPath(rootDir))
}

func (x *XcodeDriver) Warm(ctx context.Context, rootDir string) error {
	if policy.IsEnterprise() && !sandbox.IsConfinementAvailable() {
		return fmt.Errorf("warm refused in enterprise mode: confinement is unavailable")
	}
	x.EnsureDerivedDataExcluded(rootDir)
	box := sandbox.NewSandbox(rootDir)
	res := box.RunWithNetwork(ctx, "xcodebuild -resolvePackageDependencies", &sandbox.ExecOptions{
		Cwd:       rootDir,
		Timeout:   policy.GetTimeout("xcode", "warm"),
		Toolchain: "xcode",
	})
	if !res.Success() {
		return fmt.Errorf("xcode dependency resolve failed: %s (%s)", res.Error, strings.TrimSpace(res.Stderr))
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
	x.EnsureDerivedDataExcluded(rootDir)
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
		Cwd:       rootDir,
		Timeout:   timeout,
		Toolchain: "xcode",
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
