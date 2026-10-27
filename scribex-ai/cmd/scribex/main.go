// scribex: keep docs in sync with code.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"scribex/pkg/anchors"
	"scribex/pkg/confluence"
	"scribex/pkg/dashboard"
	"scribex/pkg/diagram"
	"scribex/pkg/docs"
	"scribex/pkg/fsx"
	"scribex/pkg/github"
	"scribex/pkg/gitx"
	"scribex/pkg/httpx"
	"scribex/pkg/jira"
	"scribex/pkg/manifest"
	"scribex/pkg/mcp"
	"scribex/pkg/safepath"
	"scribex/pkg/secrets"
	"scribex/pkg/trace"
)

var version = "1.0.0"

var force *bool
var preset, mcpURL, mcpExe, mcpArgs, mcpEnv, mcpHdr *string

const usage = `usage: scribex <cmd> [flags]
  init | check [-base ref] [-report f.md] | sync [-commit] | diagram | scavenge
  comment -repo o/r -pr N [-base ref] [-body-file path] [-author LOGIN] (env GITHUB_TOKEN, GITHUB_API_URL)
  confluence push|pull [page]              (env ATLASSIAN_URL, ATLASSIAN_USER, ATLASSIAN_TOKEN)
  trace -base ref [-key K] [-pr N] [-out f.csv] [-verify-jira] [-require-jira] [-allow-no-key]
  serve -repos file -rbac file -allowed-root dir [-addr 127.0.0.1:8080] [-cache-ttl 30s] [-tls-cert c -tls-key k] [-behind-proxy] [-trusted-proxy cidr]
  sync -ai                                 (LLM prose update for changed signatures; see config)
  config init|show|set <k> <v>|validate|test
  secrets set <NAME> (value on stdin) | list | check
  ci init github|gitlab [-force]
  mcp add <name> [-preset p | -url u | -cmd c -args a,b] [-env K=VAR] [-header K=Bearer ${VAR}] | list | remove <name> | check | export | serve
  wiki new <title> | lint | search <q> | backlinks <page> | render -out dir
  hash-token <token> | version`

func main() {
	httpx.Version = version
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := fs.String("C", ".", "repo root")
	base := fs.String("base", "", "git ref to diff against")
	report := fs.String("report", "", "check: write markdown report to file")
	bodyFile := fs.String("body-file", "", "comment: post pre-rendered report file")
	author := fs.String("author", "", "comment: override comment author login")
	commit := fs.Bool("commit", false, "sync: companion commit of changed docs")
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	pr := fs.Int("pr", 0, "pull request number")
	key := fs.String("key", "", "Jira key (default: from branch name)")
	post := fs.Bool("post", false, "jira: also comment on the ticket")
	out := fs.String("out", "", "output file (default stdout)")
	verifyJira := fs.Bool("verify-jira", false, "trace: reality-check ticket against Jira API")
	requireJira := fs.Bool("require-jira", false, "trace: fail if Jira credentials or key are missing")
	allowNoKey := fs.Bool("allow-no-key", false, "trace: allow empty Jira key in trace evidence")
	addr := fs.String("addr", "127.0.0.1:8080", "serve address")
	repos := fs.String("repos", "", "serve: repos file")
	rbac := fs.String("rbac", "", "serve: rbac file")
	allowedRoot := fs.String("allowed-root", "", "serve: root directory containing all repos")
	cacheTTL := fs.Duration("cache-ttl", 30*time.Second, "serve: cache TTL for doc measurements")
	tlsCert := fs.String("tls-cert", "", "serve: TLS certificate file")
	tlsKey := fs.String("tls-key", "", "serve: TLS private key file")
	behindProxy := fs.Bool("behind-proxy", false, "serve: running behind TLS-terminating reverse proxy")
	trustedProxy := fs.String("trusted-proxy", "", "serve: comma-separated trusted proxy CIDRs")
	ai := fs.Bool("ai", false, "sync: let the configured LLM update prose for changed signatures")
	force = fs.Bool("force", false, "overwrite generated files")
	preset = fs.String("preset", "", "mcp add: "+strings.Join(mcp.PresetNames(), "|"))
	mcpURL = fs.String("url", "", "mcp add: remote server URL")
	mcpExe = fs.String("cmd", "", "mcp add: stdio server command")
	mcpArgs = fs.String("args", "", "mcp add: comma-separated args")
	mcpEnv = fs.String("env", "", "mcp add: KEY=ENV_VAR,... (stored as ${ENV_VAR})")
	mcpHdr = fs.String("header", "", "mcp add: Name=value (value must use ${ENV_VAR} for credentials)")
	var a []string // flags may appear before or after positionals
	for rest := os.Args[2:]; ; {
		fs.Parse(rest)
		if fs.NArg() == 0 {
			break
		}
		a = append(a, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	secrets.Load(*root)

	var err error
	switch os.Args[1] {
	case "version", "--version":
		fmt.Println("Scribex AI version", version)
	case "init":
		err = initCmd(*root)
	case "check":
		err = check(*root, *base, *report)
	case "sync":
		var preDirty map[string]bool
		if *commit {
			if _, lookErr := exec.LookPath("git"); lookErr != nil {
				err = errors.New("git not found")
				break
			}
			var errDirty error
			preDirty, errDirty = parsePreDirty(*root)
			if errDirty != nil {
				err = errDirty
				break
			}
		}
		var prose anchors.Prose
		if *ai {
			if prose, err = proseFn(*root); err != nil {
				break
			}
		}
		var r docs.Report
		r, err = docs.Check(*root, true, prose)
		if err != nil {
			break
		}
		aiFailed := 0
		for _, i := range r.Issues {
			fmt.Println(i)
			if i.Code == anchors.CodeAIRejected || i.Code == anchors.CodeAIFailed {
				aiFailed++
			}
		}
		if aiFailed > 0 {
			err = fmt.Errorf("%d ai rewrite(s) not applied (anchors left stale)", aiFailed)
			break
		}
		if len(r.Issues) > 0 {
			err = fmt.Errorf("%d anchor issue(s) (see above)", len(r.Issues))
			break
		}
		if *commit {
			_, err = companion(*root, r.Changed, preDirty)
		}
	case "diagram":
		err = diagramCmd(*root)
	case "scavenge":
		err = scavenge(*root)
	case "comment":
		err = comment(*root, *base, *repo, *pr, *bodyFile, *author)
	case "confluence":
		err = confluenceCmd(*root, a)
	case "jira":
		err = jiraCmd(*root, *base, *key, *post)
	case "trace":
		err = traceCmd(*root, *base, *key, *pr, *out, *verifyJira, *requireJira, *allowNoKey)
	case "serve":
		err = serve(*root, *repos, *rbac, *addr, *allowedRoot, *cacheTTL, *tlsCert, *tlsKey, *behindProxy, *trustedProxy)
	case "config":
		err = configCmd(*root, a)
	case "secrets":
		err = secretsCmd(*root, a)
	case "ci":
		err = ciCmd(*root, a)
	case "mcp":
		err = mcpCmd(*root, a)
	case "wiki":
		err = wikiCmd(*root, a, *out)
	case "hash-token":
		if len(a) != 1 {
			err = fmt.Errorf("hash-token <token>")
		} else {
			fmt.Println(dashboard.HashToken(a[0]))
		}
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "scribex:", err)
		os.Exit(1)
	}
}

func initCmd(root string) error {
	dir, err := safepath.ScribexDir(root, true)
	if err != nil {
		return err
	}
	m := filepath.Join(dir, "manifest.yaml")
	if _, err := os.Stat(m); err == nil {
		return nil
	}
	return fsx.WriteFileIn(root, filepath.Join(safepath.DirName, "manifest.yaml"), []byte("# Confluence page-to-symbol mappings\nmappings:\n#  - page: \"12345\"\n#    space: ENG\n#    title: Auth API\n#    symbols: [pkg/auth/jwt.go#VerifyToken]\n"), 0o644)
}

// gate prints the impact-gate verdict; returns false when no public contract changed (0 tokens).
func gate(root, base string) (bool, error) {
	res, err := docs.Deltas(root, base)
	if err != nil {
		return false, err
	}
	for _, u := range res.Unknown {
		fmt.Printf("impact gate: cannot prove no change in %s: %s\n", u.File, u.Reason)
	}
	if len(res.Deltas) == 0 && len(res.Unknown) == 0 {
		fmt.Println("impact gate: no public contract change; 0 tokens, nothing to do")
		return false, nil
	}
	if len(res.Deltas) > 0 {
		fmt.Printf("impact gate: %d contract delta(s)\n", len(res.Deltas))
	}
	return true, nil
}

func writeReport(path string, data []byte) error {
	dir := filepath.Dir(path)
	dfi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("parent directory does not exist: %s", dir)
		}
		return err
	}
	if !dfi.IsDir() {
		return fmt.Errorf("parent is not a directory: %s", dir)
	}

	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write report: %s is a symlink", path)
		}
		if fi.IsDir() {
			return fmt.Errorf("refusing to write report: %s is a directory", path)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing to write report: %s is not a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return fsx.WriteAtomic(path, data, 0o644)
}

func check(root, base, report string) error {
	var gateRun bool = true
	if base != "" {
		run, err := gate(root, base)
		if err != nil {
			return err
		}
		gateRun = run
	}

	r, checkErr := docs.Check(root, false, nil)
	for _, s := range r.Skipped {
		fmt.Fprintf(os.Stderr, "skipped symlink: %s\n", s)
	}
	for _, i := range r.Issues {
		fmt.Println(i)
	}

	if report != "" {
		if err := writeReport(report, []byte(r.Markdown())); err != nil {
			fmt.Fprintf(os.Stderr, "error writing report: %v\n", err)
			return err
		}
	}

	if !gateRun {
		return nil
	}
	if checkErr != nil {
		return checkErr
	}
	if len(r.Issues) > 0 {
		return fmt.Errorf("%d doc issue(s)", len(r.Issues))
	}
	return nil
}

func scavenge(root string) error {
	r, err := docs.Check(root, false, nil)
	if err != nil {
		return err
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(os.Stderr, "skipped symlink: %s\n", s)
	}
	all := append(r.Issues, docs.Links(root)...)
	for _, i := range all {
		fmt.Println(i)
	}
	if len(all) > 0 {
		return fmt.Errorf("%d stale doc item(s)", len(all))
	}
	return nil
}

func cleanAPIURL(raw, name string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: invalid URL", name)
	}
	if u.User != nil {
		return "", fmt.Errorf("%s: userinfo not allowed", name)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%s: query and fragment not allowed", name)
	}
	if u.Scheme != "https" {
		if u.Scheme == "http" && os.Getenv("SCRIBEX_ALLOW_INSECURE_LOCAL") == "1" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1") {
			// allowed loopback
		} else {
			return "", fmt.Errorf("%s: scheme must be https", name)
		}
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
}

func pagesPath(root, id string) (string, error) {
	if !safepath.ValidPageID(id) {
		return "", errors.New("invalid page id")
	}
	dir, err := safepath.ScribexDir(root, true)
	if err != nil {
		return "", err
	}
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(pagesDir, id+".html"), nil
}

func readBodyFile(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to read symlinked body-file: %s", path)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to read non-regular body-file: %s", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	lr := io.LimitReader(f, int64(github.MaxBodyLen+1))
	b, err := io.ReadAll(lr)
	if err != nil {
		return "", err
	}
	if len(b) > github.MaxBodyLen {
		return "", errors.New("body-file too large")
	}

	if !utf8.Valid(b) {
		return "", errors.New("invalid UTF-8 in body-file")
	}
	if bytes.ContainsRune(b, '\x00') {
		return "", errors.New("body-file contains NUL byte")
	}

	return docs.SanitizeReport(string(b))
}

func comment(root, base, repo string, pr int, bodyFile, author string) error {
	if repo == "" || !github.ValidRepo(repo) {
		return fmt.Errorf("invalid -repo: %s", repo)
	}
	if pr <= 0 || pr > 2147483647 {
		return fmt.Errorf("invalid -pr: %d", pr)
	}

	var body string
	if bodyFile != "" {
		sanitized, err := readBodyFile(bodyFile)
		if err != nil {
			msg := fmt.Sprintf("report failed validation: %v", err)
			fmt.Fprintln(os.Stderr, msg)
			if summaryFile := os.Getenv("GITHUB_STEP_SUMMARY"); summaryFile != "" {
				if sfi, lerr := os.Lstat(summaryFile); lerr == nil {
					if sfi.Mode()&os.ModeSymlink != 0 {
						return errors.New(msg)
					}
				}
				if sf, oerr := os.OpenFile(summaryFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); oerr == nil {
					_, _ = sf.WriteString(msg + "\n")
					_ = sf.Close()
				}
			}
			return errors.New(msg)
		}
		body = sanitized
	} else {
		var unanalysed []docs.Unknown
		if base != "" { // impact gate: stay silent when nothing public changed
			res, err := docs.Deltas(root, base)
			if err != nil {
				return err
			}
			for _, u := range res.Unknown {
				fmt.Printf("impact gate: cannot prove no change in %s: %s\n", u.File, u.Reason)
			}
			if len(res.Deltas) == 0 && len(res.Unknown) == 0 {
				fmt.Println("impact gate: no public contract change; 0 tokens, nothing to do")
				return nil
			}
			if len(res.Deltas) > 0 {
				fmt.Printf("impact gate: %d contract delta(s)\n", len(res.Deltas))
			}
			unanalysed = res.Unknown
		}
		r, err := docs.Check(root, false, nil)
		if err != nil {
			return err
		}
		r.Unknown = unanalysed
		body = r.Markdown()
	}

	degradeOnFork := func() error {
		fmt.Println("scribex: cannot post PR comment (read-only token); report follows")
		fmt.Println(body)
		if summaryFile := os.Getenv("GITHUB_STEP_SUMMARY"); summaryFile != "" {
			if fi, err := os.Lstat(summaryFile); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to write to symlinked GITHUB_STEP_SUMMARY: %s", summaryFile)
			}
			f, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
			if err != nil {
				return fmt.Errorf("open GITHUB_STEP_SUMMARY: %w", err)
			}
			defer f.Close()
			if _, err := f.WriteString(body + "\n"); err != nil {
				return fmt.Errorf("write GITHUB_STEP_SUMMARY: %w", err)
			}
		}
		return nil
	}

	tok := os.Getenv("GITHUB_TOKEN")
	if tok == "" {
		if gl := os.Getenv("GITLAB_TOKEN"); gl != "" {
			tok = gl
		}
	}
	if tok == "" {
		return degradeOnFork()
	}

	api := os.Getenv("GITHUB_API_URL")
	if api == "" {
		if gl := os.Getenv("CI_API_V4_URL"); gl != "" {
			api = gl
		} else if gl := os.Getenv("CI_SERVER_URL"); gl != "" {
			api = strings.TrimRight(gl, "/") + "/api/v4"
		} else {
			api = "https://api.github.com"
		}
	}
	cleanAPI, err := cleanAPIURL(api, "GITHUB_API_URL")
	if err != nil {
		return err
	}

	var opts []github.Option
	if author != "" {
		opts = append(opts, github.WithAuthor(author))
	}

	res, err := github.Upsert(cleanAPI, tok, repo, pr, body, opts...)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "Resource not accessible") && (strings.Contains(errStr, "403") || strings.Contains(errStr, "404")) {
			return degradeOnFork()
		}
		return err
	}

	fmt.Printf("scribex comment: %s (id %d)\n", res.Action, res.ID)
	return nil
}

func atlassian() (url, user, tok string, err error) {
	rawURL, user, tok := os.Getenv("ATLASSIAN_URL"), os.Getenv("ATLASSIAN_USER"), os.Getenv("ATLASSIAN_TOKEN")
	if rawURL == "" || tok == "" {
		return "", "", "", fmt.Errorf("set ATLASSIAN_URL, ATLASSIAN_USER, ATLASSIAN_TOKEN")
	}
	cleaned, err := cleanAPIURL(rawURL, "ATLASSIAN_URL")
	if err != nil {
		return "", "", "", err
	}
	return cleaned, user, tok, nil
}

func confluenceCmd(root string, a []string) error {
	u, user, tok, err := atlassian()
	if err != nil {
		return err
	}
	c := confluence.Client{Base: u, User: user, Token: tok}
	if len(a) == 0 {
		return fmt.Errorf("confluence push|pull [page]")
	}
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		return err
	}
	ms, err := manifest.Load(filepath.Join(dir, "manifest.yaml"))
	if err != nil {
		return err
	}
	statePath := filepath.Join(dir, "state.json")
	state := map[string]string{} // page -> hash of last pushed signatures
	if b, err := os.ReadFile(statePath); err == nil {
		if err := json.Unmarshal(b, &state); err != nil {
			return fmt.Errorf("corrupt state.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, m := range ms {
		if len(a) > 1 && a[1] != m.Page {
			continue
		}
		switch a[0] {
		case "pull":
			body, err := c.Pull(m.Page)
			if err != nil {
				return err
			}
			if _, err := pagesPath(root, m.Page); err != nil {
				return err
			}
			relPage := filepath.Join(safepath.DirName, "pages", m.Page+".html")
			if err := fsx.WriteFileIn(root, relPage, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Println("pulled", m.Page)
		case "push":
			sigs := map[string]string{}
			for _, s := range m.Symbols {
				f, sym, _ := strings.Cut(s, "#")
				symMap, err := docs.Symbols(root, f)
				if err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
				if sig, ok := symMap[sym]; ok {
					sigs[s] = sig
				}
			}
			body, err := confluence.Body(sigs)
			if err != nil {
				return err
			}
			h := sha256.Sum256([]byte(body))
			hash := hex.EncodeToString(h[:8])
			if state[m.Page] == hash {
				continue
			}
			id, err := c.CreateDraft(m.Space, m.Title+" (Scribex draft)", body)
			if err != nil {
				return err
			}
			state[m.Page] = hash
			fmt.Printf("draft %s opened for page %s\n", id, m.Page)
		default:
			return fmt.Errorf("confluence push|pull [page]")
		}
	}
	if a[0] == "push" {
		b, _ := json.MarshalIndent(state, "", "  ")
		return fsx.WriteFileIn(root, filepath.Join(safepath.DirName, "state.json"), append(b, '\n'), 0o644)
	}
	return nil
}

func branch(root string) string {
	b, _ := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	return strings.TrimSpace(string(b))
}

func incompleteError(unknowns []docs.Unknown) error {
	files := make([]string, len(unknowns))
	for i, u := range unknowns {
		files[i] = u.File
	}
	return fmt.Errorf("incomplete: cannot analyse %d file(s): %s", len(unknowns), strings.Join(files, ", "))
}

func jiraCmd(root, base, key string, post bool) error {
	if base == "" {
		return fmt.Errorf("jira needs -base")
	}
	if key != "" && !jira.ValidKey(key) {
		return fmt.Errorf("invalid Jira key")
	}
	if key == "" {
		key = jira.Key(branch(root))
	}
	if key == "" {
		return fmt.Errorf("no Jira key in branch name; pass -key")
	}
	if !jira.ValidKey(key) {
		return fmt.Errorf("invalid Jira key")
	}
	u, user, tok, err := atlassian()
	if err != nil {
		return err
	}
	res, err := docs.Deltas(root, base)
	if err != nil {
		return err
	}
	if len(res.Unknown) > 0 {
		return incompleteError(res.Unknown)
	}
	ds := res.Deltas
	c := jira.Client{Base: u, User: user, Token: tok}
	text, err := c.Text(key)
	if err != nil {
		return err
	}
	flags := jira.Check(text, ds)
	if len(flags) == 0 {
		fmt.Println(key + ": PR matches ticket")
		return nil
	}
	body := "Scribex reality-check for " + key + ":\n* " + strings.Join(flags, "\n* ")
	fmt.Println(body)
	if post {
		if _, err := c.PostRealityCheck(key, body); err != nil {
			return err
		}
	}
	return fmt.Errorf("%d mismatch(es)", len(flags))
}

func traceCmd(root, base, key string, pr int, out string, verifyJira, requireJira, allowNoKey bool) error {
	if base == "" {
		return fmt.Errorf("trace needs -base")
	}

	if out != "" {
		for _, p := range []string{out, out + ".json"} {
			if fi, err := os.Lstat(p); err == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("refusing to write to symlink %s", p)
				}
				if fi.IsDir() {
					return fmt.Errorf("refusing to write to directory %s", p)
				}
			}
		}
	}

	baseSHA, err := gitx.ResolveCommit(root, base)
	if err != nil {
		return err
	}
	headSHA, err := gitx.ResolveCommit(root, "HEAD")
	if err != nil {
		return err
	}

	headTimeRaw, err := gitx.Run(root, "show", "-s", "--format=%ct", "HEAD")
	if err != nil {
		return err
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(headTimeRaw)), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse commit timestamp: %w", err)
	}
	headCommitTimeUTC := time.Unix(sec, 0).UTC().Format(time.RFC3339)

	if key == "" {
		key = jira.Key(branch(root))
	}

	if key == "" {
		if !allowNoKey {
			return fmt.Errorf("missing Jira key; pass -key or -allow-no-key")
		}
	} else {
		if !jira.StrictKey(key) {
			return fmt.Errorf("invalid Jira key format: %q (must match ^[A-Z][A-Z0-9_]{1,10}-[0-9]{1,9}$)", key)
		}
	}

	jiraVerified := false
	if key != "" {
		u, user, tok, err := atlassian()
		hasCreds := (err == nil && u != "" && tok != "")
		if !hasCreds {
			if requireJira {
				fmt.Fprintln(os.Stderr, "error: -require-jira specified but Jira credentials not configured")
				os.Exit(2)
			}
			fmt.Fprintln(os.Stderr, "warning: Jira credentials not configured; jira_verified=false")
		} else {
			cl := jira.Client{Base: u, User: user, Token: tok}
			ok, err := cl.Issue(key)
			if err != nil {
				return fmt.Errorf("jira verification failed for %s: %w", key, err)
			}
			jiraVerified = ok
		}
	}

	res, err := docs.Deltas(root, base)
	if err != nil {
		return err
	}
	if len(res.Unknown) > 0 {
		return incompleteError(res.Unknown)
	}
	ds := res.Deltas
	var ms []manifest.Mapping
	if dir, err := safepath.ScribexDir(root, false); err == nil {
		ms, _ = manifest.Load(filepath.Join(dir, "manifest.yaml"))
	}

	prs := ""
	if pr > 0 {
		prs = fmt.Sprint(pr)
	}

	opts := trace.Options{
		Version:           version,
		BaseSHA:           baseSHA,
		HeadSHA:           headSHA,
		HeadCommitTimeUTC: headCommitTimeUTC,
		Jira:              key,
		JiraVerified:      jiraVerified,
		PR:                prs,
		Deltas:            ds,
		Refs:              docs.Refs(root),
		Mappings:          ms,
	}

	csvBytes, rows, err := trace.Generate(opts)
	if err != nil {
		return err
	}

	if out == "" {
		_, err := os.Stdout.Write(csvBytes)
		return err
	}

	h := sha256.Sum256(csvBytes)
	csvSHA := hex.EncodeToString(h[:])

	meta := trace.Meta{
		Schema:         "scribex-trace-meta/1",
		Status:         "complete",
		Rows:           rows,
		CSVSHA256:      csvSHA,
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		ScribexVersion: version,
		BaseSHA:        baseSHA,
		HeadSHA:        headSHA,
		JiraVerified:   jiraVerified,
		UnknownFiles:   []string{},
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	metaBytes = append(metaBytes, '\n')

	if err := trace.WriteFiles(out, csvBytes, metaBytes); err != nil {
		return err
	}
	fmt.Println(csvSHA)
	return nil
}

func serve(root, reposF, rbacF, addr, allowedRoot string, cacheTTL time.Duration, tlsCert, tlsKey string, behindProxy bool, trustedProxy string) error {
	if err := dashboard.ValidateServeConfig(addr, allowedRoot, tlsCert, tlsKey, behindProxy); err != nil {
		return err
	}

	rs, err := dashboard.LoadRepos(reposF, allowedRoot)
	if err != nil {
		return err
	}
	ps, err := dashboard.LoadRBAC(rbacF)
	if err != nil {
		return err
	}

	var trustedPrefixes []netip.Prefix
	if trustedProxy != "" {
		for _, part := range strings.Split(trustedProxy, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			prefix, err := netip.ParsePrefix(part)
			if err != nil {
				addr, addrErr := netip.ParseAddr(part)
				if addrErr != nil {
					return fmt.Errorf("invalid trusted-proxy CIDR/IP %q: %w", part, err)
				}
				bits := 32
				if addr.Is6() {
					bits = 128
				}
				prefix = netip.PrefixFrom(addr, bits)
			}
			trustedPrefixes = append(trustedPrefixes, prefix)
		}
	}

	dashServer, err := dashboard.NewServer(dashboard.Config{
		AllowedRoot:    allowedRoot,
		CacheTTL:       cacheTTL,
		BehindProxy:    behindProxy,
		TrustedProxies: trustedPrefixes,
		Now:            time.Now,
		AccessLog:      os.Stderr,
	}, rs, ps, root)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           dashServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	if tlsCert != "" && tlsKey != "" {
		httpSrv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
	}()

	fmt.Println("scribex dashboard on", addr)
	if tlsCert != "" && tlsKey != "" {
		return httpSrv.ListenAndServeTLS(tlsCert, tlsKey)
	}
	return httpSrv.ListenAndServe()
}

func parsePreDirty(root string) (map[string]bool, error) {
	out, err := gitx.Run(root, "status", "--porcelain", "-z", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	preDirty := map[string]bool{}
	parts := bytes.Split(out, []byte{0})
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		code := string(p[:2])
		path := string(p[3:])
		preDirty[path] = true
		if strings.HasPrefix(code, "R") || strings.HasPrefix(code, "C") {
			if i+1 < len(parts) && len(parts[i+1]) > 0 {
				i++
				preDirty[string(parts[i])] = true
			}
		}
	}
	return preDirty, nil
}

// companion commits only .md files touched by sync; no-op when clean.
func companion(root string, changed []string, preDirty map[string]bool) (string, error) {
	if len(changed) == 0 {
		fmt.Println("nothing to commit")
		return "", nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return "", errors.New("git not found")
	}

	if _, err := gitx.Run(root, "symbolic-ref", "-q", "HEAD"); err != nil {
		return "", errors.New("refusing to commit on detached HEAD")
	}

	for _, f := range changed {
		if preDirty[f] {
			return "", fmt.Errorf("refusing to commit: %s had uncommitted changes before sync", f)
		}
	}

	nameEnv := os.Getenv("SCRIBEX_GIT_NAME")
	emailEnv := os.Getenv("SCRIBEX_GIT_EMAIL")

	var nameCfg, emailCfg string
	if out, err := gitx.Run(root, "config", "user.name"); err == nil {
		nameCfg = strings.TrimSpace(string(out))
	}
	if out, err := gitx.Run(root, "config", "user.email"); err == nil {
		emailCfg = strings.TrimSpace(string(out))
	}

	effectiveName := nameCfg
	if nameEnv != "" {
		effectiveName = nameEnv
	}
	effectiveEmail := emailCfg
	if emailEnv != "" {
		effectiveEmail = emailEnv
	}

	if effectiveName == "" || effectiveEmail == "" {
		return "", errors.New("git identity not configured (set user.name/user.email or SCRIBEX_GIT_NAME/SCRIBEX_GIT_EMAIL)")
	}

	addArgs := append([]string{"--literal-pathspecs", "add", "--"}, changed...)
	if _, err := gitx.Run(root, addArgs...); err != nil {
		return "", err
	}

	commitArgs := []string{}
	if nameEnv != "" {
		commitArgs = append(commitArgs, "-c", "user.name="+nameEnv)
	}
	if emailEnv != "" {
		commitArgs = append(commitArgs, "-c", "user.email="+emailEnv)
	}
	commitArgs = append(commitArgs, "--literal-pathspecs", "commit", "-m", "docs: scribex sync", "-m", "Scribex-Docs: true", "--only", "--")
	commitArgs = append(commitArgs, changed...)

	if _, err := gitx.Run(root, commitArgs...); err != nil {
		return "", err
	}

	revOut, err := gitx.Run(root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(revOut))
	fmt.Println(sha)
	return sha, nil
}

func diagramCmd(root string) error {
	d, err := diagram.Mermaid(root)
	if err != nil {
		return err
	}
	n := 0
	for _, f := range docs.Files(root) {
		b, _ := os.ReadFile(f)
		if o, ok := diagram.Replace(string(b), d); ok {
			rel, err := filepath.Rel(root, f)
			if err != nil {
				return err
			}
			if err := fsx.WriteFileIn(root, rel, []byte(o), 0o644); err != nil {
				return err
			}
			fmt.Println("updated", f)
			n++
		}
	}
	if n == 0 {
		fmt.Println(d)
	}
	return nil
}
