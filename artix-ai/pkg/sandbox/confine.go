package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"artix/pkg/policy"
)

// Isolation names how a command was confined. Reported in ExecResult so the UI and docs never
// claim more than actually happened.
const (
	IsolationOSSandbox    = "os-sandbox"    // macOS sandbox-exec, or Linux bubblewrap
	IsolationProcessGroup = "process-group" // no filesystem/network restriction
	IsolationRefused      = "refused"       // execution refused due to fail-closed policy
)

// SensitiveReadDenyPaths lists sensitive credential and token paths blocked from sandbox reads.
var SensitiveReadDenyPaths = []string{
	".ssh",
	".aws",
	".kube",
	".gnupg",
	".config/gcloud",
	".azure",
	".netrc",
	".artix",
	".kritix",
	".config/gh",
	".git-credentials",
	".docker",
	".npmrc",
	".pypirc",
	".config/git",
	"Library/Keychains",
	".mozilla",
	".config/google-chrome",
	"Library/Application Support/Google/Chrome",
	"Library/Application Support/Firefox",
	"Library/Application Support/BraveSoftware",
	".zsh_history",
	".bash_history",
	".gitconfig",
	".config/op",
	".slack",
}

// IsConfinementAvailable returns whether kernel-level sandbox tooling is available on this host.
func IsConfinementAvailable() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("sandbox-exec")
		return err == nil
	case "linux":
		_, err := exec.LookPath("bwrap")
		return err == nil
	default:
		return false
	}
}

// BuildBwrapArgs builds the bubblewrap execution arguments for Linux environments.
func BuildBwrapArgs(writable []string, net bool, cmdStr string, env ...[]string) []string {
	args := []string{
		"--unshare-user-try",
		"--unshare-pid",
		"--unshare-ipc",
		"--new-session",
		"--clearenv",
		"--cap-drop", "ALL",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--die-with-parent",
	}

	// 1. Bind /tmp first so specific worktree/repo binds within /tmp are layered on top
	hasTmp := false
	for _, w := range writable {
		if w == "/tmp" || w == "/var/tmp" {
			hasTmp = true
			break
		}
	}
	if hasTmp {
		args = append(args, "--bind", "/tmp", "/tmp")
	}

	// 2. Bind specific writable directories and their .git as read-only on top
	for _, w := range writable {
		if w == "/tmp" || w == "/var/tmp" {
			continue
		}
		if _, err := os.Stat(w); err == nil {
			args = append(args, "--bind", w, w)
			gitDir := filepath.Join(w, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				args = append(args, "--ro-bind", gitDir, gitDir)
			}
		}
	}

	// 3. Hide sensitive credential directories from reads via empty tmpfs mounts and deny writes via remount-ro
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, denyRel := range SensitiveReadDenyPaths {
			denyPath := filepath.Join(home, denyRel)
			if _, err := os.Stat(denyPath); err == nil {
				args = append(args, "--tmpfs", denyPath, "--remount-ro", denyPath)
			}
		}
	}

	// 4. Set environment variables after --clearenv
	var childEnv []string
	if len(env) > 0 {
		childEnv = env[0]
	} else {
		childEnv = BuildSanitizedChildEnv("", nil)
	}
	for _, kv := range childEnv {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			args = append(args, "--setenv", parts[0], parts[1])
		}
	}

	if !net {
		args = append(args, "--unshare-net")
	}
	args = append(args, "sh", "-c", cmdStr)
	return args
}

// Confinement: writes are limited to the workspace, temp dirs and isolated per-job caches; network
// is denied (except localhost/loopback) unless net is true. Reads are restricted to exclude sensitive user credentials.
//
// In Enterprise Mode (ARTIX_ENTERPRISE=1):
// 1. ARTIX_SANDBOX=off is strictly disallowed and ignored.
// 2. Fail-Closed: If no confinement tool exists (sandbox-exec or bwrap), execution is REFUSED.
func confine(cwd, cmdStr string, net bool, loopback bool) (*exec.Cmd, string) {
	plain := func() (*exec.Cmd, string) { return exec.Command("sh", "-c", cmdStr), IsolationProcessGroup }

	isEnterprise := policy.IsEnterprise()
	sandboxOff := os.Getenv("ARTIX_SANDBOX") == "off" || os.Getenv("KRITIX_SANDBOX") == "off"

	if sandboxOff && !isEnterprise {
		return plain()
	}

	home, _ := os.UserHomeDir()
	real := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}

	jobCacheDir := filepath.Join(os.TempDir(), "artix-job-caches")
	_ = os.MkdirAll(jobCacheDir, 0700)
	writable := []string{real(cwd), real(os.TempDir()), "/tmp", "/private/tmp", "/private/var/folders", jobCacheDir}

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err != nil {
			if isEnterprise {
				return nil, IsolationRefused
			}
			return plain()
		}
		var p strings.Builder
		p.WriteString("(version 1)(allow default)")
		if net {
			p.WriteString("(allow network*)")
		} else if loopback {
			// Per P0-5: loopback allowed for Gradle FileLockContentionHandler, outbound denied
			p.WriteString("(deny network*)")
			p.WriteString("(allow network-bind (local ip \"localhost:*\"))")
			p.WriteString("(allow network-inbound (local ip \"localhost:*\"))")
			p.WriteString("(allow network-outbound (to ip \"localhost:*\"))")
			p.WriteString("(allow network-bind (local unix-socket))")
			p.WriteString("(allow network-inbound (local unix-socket))")
			p.WriteString("(allow network-outbound (literal \"/private/var/run/mDNSResponder\"))")
			p.WriteString("(allow network-outbound (literal \"/private/var/run/syslog\"))")
		} else {
			p.WriteString("(deny network*)")
		}

		// Read deny list for credentials & tokens
		p.WriteString("(deny file-read*")
		if home != "" {
			for _, denyRel := range SensitiveReadDenyPaths {
				denyPath := filepath.Join(home, denyRel)
				fmt.Fprintf(&p, " (subpath %q)", denyPath)
				if resolved := real(denyPath); resolved != denyPath {
					fmt.Fprintf(&p, " (subpath %q)", resolved)
				}
			}
		}
		if runtime.GOOS == "darwin" {
			if _, err := os.Stat("/Users/Shared"); err == nil {
				fmt.Fprintf(&p, " (subpath %q)", "/Users/Shared")
			}
		}
		p.WriteString(")")

		// Write deny list
		p.WriteString("(deny file-write*)(allow file-write* (literal \"/dev/null\") (literal \"/dev/tty\") (regex #\"^/dev/fd/\")")
		for _, w := range writable {
			fmt.Fprintf(&p, " (subpath %q)", w)
		}
		p.WriteString(")")

		// Deny write explicitly for protected governance and credential paths
		if home != "" {
			p.WriteString("(deny file-write*")
			for _, denyRel := range []string{".artix", ".kritix", ".ssh", ".aws", ".kube"} {
				denyPath := filepath.Join(home, denyRel)
				fmt.Fprintf(&p, " (subpath %q)", denyPath)
				if resolved := real(denyPath); resolved != denyPath {
					fmt.Fprintf(&p, " (subpath %q)", resolved)
				}
			}
			// Per P0-3 b: Host caches are read-only; deny writing to host persistence directories
			for _, cacheRel := range []string{".gradle", ".cargo", ".npm", ".m2", ".cache", "go/pkg", ".konan", "Library/Caches"} {
				cachePath := filepath.Join(home, cacheRel)
				fmt.Fprintf(&p, " (subpath %q)", cachePath)
				if resolved := real(cachePath); resolved != cachePath {
					fmt.Fprintf(&p, " (subpath %q)", resolved)
				}
			}
			p.WriteString(")")
		}

		// Deny write explicitly for .git repository metadata and hooks inside workspace
		p.WriteString("(deny file-write*")
		gitPath := filepath.Join(cwd, ".git")
		fmt.Fprintf(&p, " (subpath %q)", gitPath)
		fmt.Fprintf(&p, " (literal %q)", gitPath)
		if resolved := real(gitPath); resolved != gitPath {
			fmt.Fprintf(&p, " (subpath %q)", resolved)
			fmt.Fprintf(&p, " (literal %q)", resolved)
		}
		p.WriteString(" (regex #\"(^|/)\\.git(/|$)\")")
		p.WriteString(")")

		return exec.Command("sandbox-exec", "-p", p.String(), "sh", "-c", cmdStr), IsolationOSSandbox

	case "linux":
		bwrap, err := exec.LookPath("bwrap")
		if err != nil {
			if isEnterprise {
				return nil, IsolationRefused
			}
			return plain()
		}
		args := BuildBwrapArgs(writable, net, cmdStr, BuildSanitizedChildEnv(cwd, nil))
		return exec.Command(bwrap, args...), IsolationOSSandbox

	default:
		if isEnterprise {
			return nil, IsolationRefused
		}
		return plain()
	}
}
