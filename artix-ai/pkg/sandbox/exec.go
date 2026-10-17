package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultMaxOutputBytes limits output capture to 1MB to prevent memory exhaustion
	DefaultMaxOutputBytes = 1024 * 1024
	// DefaultTimeout is the fallback command execution timeout
	DefaultTimeout = 5 * time.Minute
)

// ExecOptions specifies execution parameters for a sandboxed command.
type ExecOptions struct {
	Cwd            string            `json:"cwd"`
	Timeout        time.Duration     `json:"timeout"`
	MaxOutputBytes int               `json:"maxOutputBytes"`
	Env            map[string]string `json:"env,omitempty"`
	AllowNetwork   bool              `json:"allowNetwork,omitempty"`
	AllowLoopback  bool              `json:"allowLoopback,omitempty"`
	Toolchain      string            `json:"toolchain,omitempty"`
}

// ExecResult contains the deterministic result of running a command.
type ExecResult struct {
	Command    string `json:"command"`
	Isolation  string `json:"isolation"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMs int64  `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
	Truncated  bool   `json:"truncated"`
	Error      string `json:"error,omitempty"`
}

// Success returns true if the command exited with code 0 and did not time out.
func (r *ExecResult) Success() bool {
	return r.ExitCode == 0 && !r.TimedOut && r.Error == ""
}

// cappedBuffer keeps at most max bytes and discards the rest, so a runaway command cannot
// exhaust memory while it runs.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p) // report everything consumed, or io.Copy treats it as a short write and stops draining
	if room := c.max - c.buf.Len(); len(p) > room {
		c.truncated = true
		p = p[:max(room, 0)]
	}
	c.buf.Write(p)
	return n, nil
}

// BuildSanitizedChildEnv constructs a clean child process environment from an allowlist
// (PATH, HOME=temp, LANG, GOPATH, GOCACHE, etc.) and strictly strips all provider, forge,
// audit, and secret variables.
func BuildSanitizedChildEnv(cwd string, extraEnv map[string]string) []string {
	allowlistKeys := map[string]bool{
		"PATH":                true,
		"LANG":                true,
		"LC_ALL":              true,
		"LC_CTYPE":            true,
		"TERM":                true,
		"TZ":                  true,
		"USER":                true,
		"LOGNAME":             true,
		"SHELL":               true,
		"GOPATH":              true,
		"GOROOT":              true,
		"GOCACHE":             true,
		"GOPROXY":             true,
		"GONOSUMDB":           true,
		"GOPRIVATE":           true,
		"GOFLAGS":             true,
		"CGO_ENABLED":         true,
		"GO111MODULE":         true,
		"GOMODCACHE":          true,
		"GOTMPDIR":            true,
		"TMPDIR":              true,
		"TEMP":                true,
		"TMP":                 true,
		"NODE_PATH":           true,
		"PYTHONPATH":          true,
		"CARGO_HOME":          true,
		"RUSTUP_HOME":         true,
		"GRADLE_USER_HOME":    true,
		"M2_HOME":             true,
		"DEVELOPER_DIR":       true,
		"JAVA_HOME":           true,
		"ANDROID_HOME":        true,
		"ANDROID_SDK_ROOT":    true,
		"KONAN_DATA_DIR":      true,
		"GRADLE_RO_DEP_CACHE": true,
	}

	isDeniedKey := func(key string) bool {
		kUpper := strings.ToUpper(key)
		if strings.Contains(kUpper, "TOKEN") ||
			strings.Contains(kUpper, "KEY") ||
			strings.Contains(kUpper, "SECRET") ||
			strings.Contains(kUpper, "PASSWORD") ||
			strings.Contains(kUpper, "AUTH") ||
			strings.Contains(kUpper, "CREDENTIAL") ||
			strings.Contains(kUpper, "FORGE") ||
			strings.Contains(kUpper, "AUDIT") ||
			strings.Contains(kUpper, "SIGN") ||
			strings.Contains(kUpper, "ENTERPRISE") ||
			strings.Contains(kUpper, "OPENAI") ||
			strings.Contains(kUpper, "ANTHROPIC") ||
			strings.Contains(kUpper, "GEMINI") ||
			strings.Contains(kUpper, "GITHUB") ||
			strings.Contains(kUpper, "GITLAB") ||
			strings.Contains(kUpper, "BITBUCKET") ||
			strings.Contains(kUpper, "AWS") ||
			strings.Contains(kUpper, "SSH") ||
			strings.Contains(kUpper, "PRIVATE") ||
			strings.Contains(kUpper, "BEARER") ||
			strings.HasPrefix(kUpper, "KRITIX_") {
			return true
		}
		if strings.HasPrefix(kUpper, "ARTIX_") && !strings.HasPrefix(kUpper, "ARTIX_ARG_") {
			return true
		}
		return false
	}

	envMap := make(map[string]string)
	for _, envStr := range os.Environ() {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k, v := parts[0], parts[1]
		if isDeniedKey(k) {
			continue
		}
		if allowlistKeys[strings.ToUpper(k)] {
			envMap[k] = v
		}
	}

	// Per P0-3, P0-5, and N1: Redirect mutable toolchain homes & caches to per-job tmp
	jobCacheDir := filepath.Join(os.TempDir(), "artix-job-caches")
	jobHome := filepath.Join(jobCacheDir, "home")
	jobGradle := filepath.Join(jobCacheDir, "gradle")
	jobSwiftPM := filepath.Join(jobCacheDir, "swiftpm")
	jobKonan := filepath.Join(jobCacheDir, "konan")
	jobCargo := filepath.Join(jobCacheDir, "cargo")
	jobNpm := filepath.Join(jobCacheDir, "npm")
	_ = os.MkdirAll(jobHome, 0700)
	_ = os.MkdirAll(jobGradle, 0700)
	_ = os.MkdirAll(jobSwiftPM, 0700)
	_ = os.MkdirAll(jobKonan, 0700)
	_ = os.MkdirAll(jobCargo, 0700)
	_ = os.MkdirAll(jobNpm, 0700)

	envMap["HOME"] = jobHome
	envMap["GRADLE_USER_HOME"] = jobGradle
	envMap["SWIFTPM_CACHE"] = jobSwiftPM
	envMap["KONAN_DATA_DIR"] = jobKonan
	envMap["CARGO_HOME"] = jobCargo
	envMap["npm_config_cache"] = jobNpm
	envMap["JAVA_TOOL_OPTIONS"] = "-Duser.home=" + jobHome
	envMap["_JAVA_OPTIONS"] = "-Duser.home=" + jobHome

	// On macOS, fallback to installed Xcode if DEVELOPER_DIR was not explicitly passed
	if runtime.GOOS == "darwin" && envMap["DEVELOPER_DIR"] == "" {
		if _, err := os.Stat("/Applications/Xcode.app/Contents/Developer"); err == nil {
			envMap["DEVELOPER_DIR"] = "/Applications/Xcode.app/Contents/Developer"
		}
	}

	// Merge extraEnv (filtering out denied keys)
	if extraEnv != nil {
		for k, v := range extraEnv {
			if !isDeniedKey(k) {
				envMap[k] = v
			}
		}
	}

	var childEnv []string
	for k, v := range envMap {
		childEnv = append(childEnv, fmt.Sprintf("%s=%s", k, v))
	}
	return childEnv
}

// Sandbox provides safe, isolated command execution.
type Sandbox struct {
	defaultCwd string
}

// NewSandbox creates a Sandbox bound to a workspace directory.
func NewSandbox(workspaceDir string) *Sandbox {
	return &Sandbox{
		defaultCwd: workspaceDir,
	}
}

// RunWithNetwork executes a command with network access allowed (for dedicated fetch/warm operations).
func (s *Sandbox) RunWithNetwork(ctx context.Context, cmdStr string, opts *ExecOptions) *ExecResult {
	if opts == nil {
		opts = &ExecOptions{}
	}
	optsCopy := *opts
	optsCopy.AllowNetwork = true
	return s.Run(ctx, cmdStr, &optsCopy)
}

// Run executes a shell command line string with safety boundaries.
func (s *Sandbox) Run(ctx context.Context, cmdStr string, opts *ExecOptions) *ExecResult {
	start := time.Now()

	cwd := s.defaultCwd
	timeout := DefaultTimeout
	maxBytes := DefaultMaxOutputBytes

	if opts != nil {
		if opts.Cwd != "" {
			cwd = opts.Cwd
		}
		if opts.Timeout > 0 {
			timeout = opts.Timeout
		}
		if opts.MaxOutputBytes > 0 {
			maxBytes = opts.MaxOutputBytes
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	net := false
	if opts != nil {
		net = opts.AllowNetwork || (opts.Env != nil && opts.Env["ARTIX_SANDBOX_NETWORK"] == "1")
	}
	if !net {
		net = os.Getenv("ARTIX_SANDBOX_NETWORK") == "1" || os.Getenv("KRITIX_SANDBOX_NETWORK") == "1"
	}
	loopback := false
	if opts != nil {
		loopback = opts.AllowLoopback || strings.EqualFold(opts.Toolchain, "gradle") || strings.EqualFold(opts.Toolchain, "kmp")
	}

	cmd, isolation := confine(cwd, cmdStr, net, loopback)
	if cmd == nil || isolation == IsolationRefused {
		return &ExecResult{
			Command:    cmdStr,
			Isolation:  IsolationRefused,
			ExitCode:   126,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      "sandbox confinement failure: enterprise mode requires kernel-level sandbox isolation (sandbox-exec or bwrap); unconfined execution refused",
		}
	}
	cmd = exec.CommandContext(execCtx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = cwd

	configureProcessGroup(cmd)

	stdoutBuf := &cappedBuffer{max: maxBytes}
	stderrBuf := &cappedBuffer{max: maxBytes}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	var extraEnv map[string]string
	if opts != nil {
		extraEnv = opts.Env
	}
	cmd.Env = BuildSanitizedChildEnv(cwd, extraEnv)

	runErr := cmd.Run()
	duration := time.Since(start)

	result := &ExecResult{
		Command:    cmdStr,
		Isolation:  isolation,
		DurationMs: duration.Milliseconds(),
		ExitCode:   0,
	}

	// Check timeout
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.ExitCode = -1
		result.Error = fmt.Sprintf("command timed out after %v", timeout)
	} else if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Error = runErr.Error()
		}
	}

	// Capture and truncate output
	result.Truncated = stdoutBuf.truncated || stderrBuf.truncated
	result.Stdout = strings.TrimSpace(stdoutBuf.buf.String())
	result.Stderr = strings.TrimSpace(stderrBuf.buf.String())

	return result
}
