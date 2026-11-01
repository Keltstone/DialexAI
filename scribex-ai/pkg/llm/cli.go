package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"scribex/pkg/config"
	"scribex/pkg/prose"
)

// Completer is the one thing the rest of scribex needs from a model backend.
type Completer interface {
	Complete(system, user string) (string, error)
}

// CLISpec describes how to drive one vendor CLI non-interactively. The table is code, never config:
// a repo-controlled .scribex/config.yaml may only choose a Name from it.
type CLISpec struct {
	Name      string
	Bin       string
	Args      []string // fixed headless, tools-off/read-only arguments
	PromptArg string   // if set, the prompt is sent as one argv element PromptArg+prompt; otherwise via stdin
	ModelFlag string   // "" = CLI has no model flag we use
	Agentic   bool     // CLI cannot be run without tool/file-edit ability; refused unless explicitly allowed
	Reviewer  bool     // reviewer-only entry (never selectable as ai.cli_command)
	Suggested []string // advisory model IDs; never used to reject a model
	Checked   string   // YYYY-MM-DD the Args and Suggested were last verified
	Verified  string   // CLI version the Args were verified against; "" = not verified on any build
}

var cliTable = []CLISpec{
	{
		Name: "claude", Bin: "claude",
		Args:      []string{"-p", "--tools", "", "--no-session-persistence", "--output-format", "text"},
		ModelFlag: "--model",
		Suggested: []string{"claude-fable-5-1", "claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"},
		Checked:   "2026-11-01", Verified: "claude 2.1.283",
	},
	{
		Name: "codex", Bin: "codex",
		Args:      []string{"exec", "--sandbox", "read-only", "-"},
		ModelFlag: "--model",
		Checked:   "2026-11-01", // flags from vendor docs only; not installed here to verify
	},
	{
		Name: "gemini", Bin: "gemini",
		Args:      []string{"--approval-mode", "plan"},
		ModelFlag: "--model",
		Suggested: []string{"auto", "pro", "flash", "flash-lite"},
		Checked:   "2026-11-01", // flags from vendor docs only; not installed here to verify
	},
	{
		Name: "agy", Bin: "agy",
		Args:      []string{"--mode", "plan", "--output-format", "text", "--print-timeout", "170s"},
		PromptArg: "-p=",
		ModelFlag: "--model",
		Suggested: []string{"gemini-3.8-flash-high", "gemini-3.8-flash-medium", "claude-sonnet-4-6", "claude-opus-4-6-thinking", "gpt-oss-120b-medium"},
		Checked:   "2026-11-01", Verified: "agy (print mode, 2026-11-01)",
	},
}

// CLISpecFor returns the table entry for name.
func CLISpecFor(name string) (CLISpec, bool) {
	for _, s := range cliTable {
		if s.Name == name {
			return s, true
		}
	}
	return CLISpec{}, false
}

// CLISpecs returns a copy of the table (for `config models` and the dashboard).
func CLISpecs() []CLISpec { return append([]CLISpec(nil), cliTable...) }

var (
	cliTimeout      = 180 * time.Second
	cliMaxOut       = 2 << 20
	cliMaxPrompt    = 512 << 10
	cliMaxArgPrompt = 200 << 10
)

type limitBuf struct {
	b   bytes.Buffer
	max int
	hit bool
}

func (l *limitBuf) Write(p []byte) (int, error) {
	if l.b.Len()+len(p) > l.max {
		l.hit = true
		return 0, errors.New("output limit exceeded")
	}
	return l.b.Write(p)
}

type cliClient struct {
	spec  CLISpec
	bin   string
	model string
	extra []string
}

func newCLI(a config.AI) (Completer, error) {
	spec, ok := CLISpecFor(a.CLICommand)
	if !ok || spec.Reviewer {
		return nil, fmt.Errorf("ai.cli_command must be one of %s", strings.Join(config.CLINames, ", "))
	}
	if a.Model != "" && !config.ValidModel(a.Model) {
		return nil, errors.New("ai.model has invalid characters")
	}
	if spec.Verified == "" && os.Getenv("SCRIBEX_ALLOW_UNVERIFIED_CLI") != "1" {
		return nil, fmt.Errorf("%s: headless invocation is not verified on any build yet; check `%s --help`, then set SCRIBEX_ALLOW_UNVERIFIED_CLI=1 to try it", spec.Name, spec.Bin)
	}
	if spec.Agentic && os.Getenv("SCRIBEX_ALLOW_AGENTIC_CLI") != "1" {
		return nil, fmt.Errorf("%s cannot be run without tool access; set SCRIBEX_ALLOW_AGENTIC_CLI=1 to allow it", spec.Name)
	}
	bin, err := exec.LookPath(spec.Bin)
	if err != nil {
		return nil, fmt.Errorf("%s not found on PATH (install it, or choose another ai.provider)", spec.Bin)
	}
	c := &cliClient{spec: spec, bin: bin, model: a.Model}
	// Names (not values) of extra env vars to pass through, from the user's own environment only.
	for _, n := range strings.Split(os.Getenv("SCRIBEX_CLI_PASS_ENV"), ",") {
		if n = strings.TrimSpace(n); config.EnvName.MatchString(n) {
			c.extra = append(c.extra, n)
		}
	}
	return c, nil
}

var baseEnv = []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "TERM", "SHELL", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "SSL_CERT_FILE", "SSL_CERT_DIR"}

func scrubbedEnv(tmp string, extra []string) []string {
	var env []string
	for _, k := range append(append([]string(nil), baseEnv...), extra...) {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "TMPDIR="+tmp)
}

var stderrWS = regexp.MustCompile(`\s+`)

func cleanStderr(s string) string {
	s = strings.TrimSpace(stderrWS.ReplaceAllString(s, " "))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	if prose.SecretShape(s) {
		return "[redacted]"
	}
	return s
}

func (c *cliClient) Complete(system, user string) (string, error) {
	prompt := "<instructions>\n" + system + "\n</instructions>\n\n" + user
	if len(prompt) > cliMaxPrompt {
		return "", fmt.Errorf("prompt too large for %s (%d bytes)", c.spec.Name, len(prompt))
	}
	args := append([]string(nil), c.spec.Args...)
	if c.model != "" && c.spec.ModelFlag != "" {
		args = append(args, c.spec.ModelFlag, c.model)
	}
	if c.spec.PromptArg != "" {
		if len(prompt) > cliMaxArgPrompt {
			return "", fmt.Errorf("prompt too large for %s argv (%d bytes)", c.spec.Name, len(prompt))
		}
		args = append(args, c.spec.PromptArg+prompt)
	}
	tmp, err := os.MkdirTemp("", "scribex-cli-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = tmp
	cmd.Env = scrubbedEnv(tmp, c.extra)
	cmd.WaitDelay = 2 * time.Second
	if c.spec.PromptArg == "" {
		cmd.Stdin = strings.NewReader(prompt)
	}
	out := &limitBuf{max: cliMaxOut}
	errb := &limitBuf{max: 8 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	runErr := cmd.Run()
	if out.hit {
		return "", fmt.Errorf("%s output exceeded %d bytes", c.spec.Name, cliMaxOut)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s timed out after %s", c.spec.Name, cliTimeout)
	}
	if runErr != nil {
		return "", fmt.Errorf("%s failed: %v: %s", c.spec.Name, runErr, cleanStderr(errb.b.String()))
	}
	text := strings.TrimSpace(out.b.String())
	if text == "" {
		return "", fmt.Errorf("%s returned empty output", c.spec.Name)
	}
	return text, nil
}
