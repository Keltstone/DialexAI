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
}

// BuildBwrapArgs builds the bubblewrap execution arguments for Linux environments.
func BuildBwrapArgs(writable []string, net bool, cmdStr string) []string {
	args := []string{"--unshare-user-try", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--die-with-parent"}

	for _, w := range writable {
		if _, err := os.Stat(w); err == nil {
			args = append(args, "--bind", w, w)
			gitDir := filepath.Join(w, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				args = append(args, "--ro-bind", gitDir, gitDir)
			}
		}
	}

	// Hide sensitive credential directories from reads via empty tmpfs mounts and deny writes via remount-ro
	// Note: Placed after writable binds so tmpfs shadows any parent directory binds (e.g. in /tmp)
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, denyRel := range SensitiveReadDenyPaths {
			denyPath := filepath.Join(home, denyRel)
			if _, err := os.Stat(denyPath); err == nil {
				args = append(args, "--tmpfs", denyPath, "--remount-ro", denyPath)
			}
		}
	}

	if !net {
		args = append(args, "--unshare-net")
	}
	args = append(args, "sh", "-c", cmdStr)
	return args
}

// Confinement: writes are limited to the workspace, temp dirs and common tool caches; network
// is denied unless ARTIX_SANDBOX_NETWORK=1. Reads are restricted to exclude sensitive user credentials.
//
// In Enterprise Mode (ARTIX_ENTERPRISE=1):
// 1. ARTIX_SANDBOX=off is strictly disallowed and ignored.
// 2. Fail-Closed: If no confinement tool exists (sandbox-exec or bwrap), execution is REFUSED.
func confine(cwd, cmdStr string) (*exec.Cmd, string) {
	plain := func() (*exec.Cmd, string) { return exec.Command("sh", "-c", cmdStr), IsolationProcessGroup }

	isEnterprise := policy.IsEnterprise()
	sandboxOff := os.Getenv("ARTIX_SANDBOX") == "off" || os.Getenv("KRITIX_SANDBOX") == "off"

	if sandboxOff && !isEnterprise {
		return plain()
	}

	net := os.Getenv("ARTIX_SANDBOX_NETWORK") == "1" || os.Getenv("KRITIX_SANDBOX_NETWORK") == "1"
	home, _ := os.UserHomeDir()
	real := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	writable := []string{real(cwd), real(os.TempDir()), "/tmp", "/private/tmp", "/private/var/folders"}
	for _, c := range []string{"Library/Caches", ".cache", "go/pkg", ".gradle", ".m2", ".npm", ".cargo"} {
		writable = append(writable, filepath.Join(home, c))
	}

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
		if !net {
			p.WriteString("(deny network*)")
		}

		// Read deny list for credentials & tokens
		if home != "" {
			p.WriteString("(deny file-read*")
			for _, denyRel := range SensitiveReadDenyPaths {
				denyPath := filepath.Join(home, denyRel)
				fmt.Fprintf(&p, " (subpath %q)", denyPath)
				if resolved := real(denyPath); resolved != denyPath {
					fmt.Fprintf(&p, " (subpath %q)", resolved)
				}
			}
			p.WriteString(")")
		}

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
		args := BuildBwrapArgs(writable, net, cmdStr)
		return exec.Command(bwrap, args...), IsolationOSSandbox

	default:
		if isEnterprise {
			return nil, IsolationRefused
		}
		return plain()
	}
}
