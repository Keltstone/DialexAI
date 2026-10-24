// scribex: keep docs in sync with code.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"scribex/pkg/anchors"
	"scribex/pkg/confluence"
	"scribex/pkg/dashboard"
	"scribex/pkg/diagram"
	"scribex/pkg/docs"
	"scribex/pkg/github"
	"scribex/pkg/jira"
	"scribex/pkg/manifest"
	"scribex/pkg/mcp"
	"scribex/pkg/secrets"
	"scribex/pkg/trace"
)

var version = "1.0.0"

var force *bool
var preset, mcpURL, mcpExe, mcpArgs, mcpEnv, mcpHdr *string

const usage = `usage: scribex <cmd> [flags]
  init | check [-base ref] | sync [-commit] | diagram | scavenge
  comment -repo o/r -pr N [-base ref]      (env GITHUB_TOKEN, GITHUB_API_URL)
  confluence push|pull [page]              (env ATLASSIAN_URL, ATLASSIAN_USER, ATLASSIAN_TOKEN)
  jira -base ref [-key K] [-post]          (same env)
  trace -base ref [-key K] [-pr N] [-out f.csv]
  serve -repos file -rbac file [-addr :8080]
  sync -ai                                 (LLM prose update for changed signatures; see config)
  config init|show|set <k> <v>|validate|test
  secrets set <NAME> (value on stdin) | list | check
  ci init github|gitlab [-force]
  mcp add <name> [-preset p | -url u | -cmd c -args a,b] [-env K=VAR] [-header K=Bearer ${VAR}] | list | remove <name> | check | export | serve
  wiki new <title> | lint | search <q> | backlinks <page> | render -out dir
  hash-token <token> | version`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := fs.String("C", ".", "repo root")
	base := fs.String("base", "", "git ref to diff against")
	commit := fs.Bool("commit", false, "sync: companion commit of changed docs")
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	pr := fs.Int("pr", 0, "pull request number")
	key := fs.String("key", "", "Jira key (default: from branch name)")
	post := fs.Bool("post", false, "jira: also comment on the ticket")
	out := fs.String("out", "", "output file (default stdout)")
	addr := fs.String("addr", ":8080", "serve address")
	repos := fs.String("repos", "", "serve: repos file")
	rbac := fs.String("rbac", "", "serve: rbac file")
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
		err = check(*root, *base)
	case "sync":
		var prose anchors.Prose
		if *ai {
			if prose, err = proseFn(*root); err != nil {
				break
			}
		}
		var r docs.Report
		if r, err = docs.Check(*root, true, prose); err == nil {
			for _, i := range r.Issues {
				fmt.Println(i)
			}
			if *commit {
				err = companion(*root)
			}
		}
	case "diagram":
		err = diagramCmd(*root)
	case "scavenge":
		err = scavenge(*root)
	case "comment":
		err = comment(*root, *base, *repo, *pr)
	case "confluence":
		err = confluenceCmd(*root, a)
	case "jira":
		err = jiraCmd(*root, *base, *key, *post)
	case "trace":
		err = traceCmd(*root, *base, *key, *pr, *out)
	case "serve":
		err = serve(*root, *repos, *rbac, *addr)
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
	dir := filepath.Join(root, ".scribex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m := filepath.Join(dir, "manifest.yaml")
	if _, err := os.Stat(m); err == nil {
		return nil
	}
	return os.WriteFile(m, []byte("# Confluence page-to-symbol mappings\nmappings:\n#  - page: \"12345\"\n#    space: ENG\n#    title: Auth API\n#    symbols: [pkg/auth/jwt.go#VerifyToken]\n"), 0o644)
}

// gate prints the impact-gate verdict; returns false when no public contract changed (0 tokens).
func gate(root, base string) (bool, error) {
	ds, err := docs.Deltas(root, base)
	if err != nil {
		return false, err
	}
	if len(ds) == 0 {
		fmt.Println("impact gate: no public contract change; 0 tokens, nothing to do")
		return false, nil
	}
	fmt.Printf("impact gate: %d contract delta(s)\n", len(ds))
	return true, nil
}

func check(root, base string) error {
	if base != "" {
		if run, err := gate(root, base); err != nil || !run {
			return err
		}
	}
	r, err := docs.Check(root, false, nil)
	for _, i := range r.Issues {
		fmt.Println(i)
	}
	if err == nil && len(r.Issues) > 0 {
		err = fmt.Errorf("%d doc issue(s)", len(r.Issues))
	}
	return err
}

func scavenge(root string) error {
	r, err := docs.Check(root, false, nil)
	if err != nil {
		return err
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

func comment(root, base, repo string, pr int) error {
	tok := os.Getenv("GITHUB_TOKEN")
	if repo == "" || pr == 0 || tok == "" {
		return fmt.Errorf("need -repo, -pr and GITHUB_TOKEN")
	}
	if base != "" { // impact gate: stay silent when nothing public changed
		if run, err := gate(root, base); err != nil || !run {
			return err
		}
	}
	r, err := docs.Check(root, false, nil)
	if err != nil {
		return err
	}
	api := os.Getenv("GITHUB_API_URL")
	if api == "" {
		api = "https://api.github.com"
	}
	return github.Upsert(api, tok, repo, pr, r.Markdown())
}

func atlassian() (url, user, tok string, err error) {
	url, user, tok = os.Getenv("ATLASSIAN_URL"), os.Getenv("ATLASSIAN_USER"), os.Getenv("ATLASSIAN_TOKEN")
	if url == "" || tok == "" {
		err = fmt.Errorf("set ATLASSIAN_URL, ATLASSIAN_USER, ATLASSIAN_TOKEN")
	}
	return
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
	ms, err := manifest.Load(filepath.Join(root, ".scribex", "manifest.yaml"))
	if err != nil {
		return err
	}
	statePath := filepath.Join(root, ".scribex", "state.json")
	state := map[string]string{} // page -> hash of last pushed signatures
	if b, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(b, &state)
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
			p := filepath.Join(root, ".scribex", "pages", m.Page+".html")
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Println("pulled", m.Page)
		case "push":
			sigs := map[string]string{}
			for _, s := range m.Symbols {
				f, sym, _ := strings.Cut(s, "#")
				if sig, ok := docs.Symbols(root, f)[sym]; ok {
					sigs[s] = sig
				}
			}
			body := confluence.Body(sigs)
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
		return os.WriteFile(statePath, b, 0o644)
	}
	return nil
}

func branch(root string) string {
	b, _ := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	return strings.TrimSpace(string(b))
}

func jiraCmd(root, base, key string, post bool) error {
	if base == "" {
		return fmt.Errorf("jira needs -base")
	}
	if key == "" {
		key = jira.Key(branch(root))
	}
	if key == "" {
		return fmt.Errorf("no Jira key in branch name; pass -key")
	}
	u, user, tok, err := atlassian()
	if err != nil {
		return err
	}
	ds, err := docs.Deltas(root, base)
	if err != nil {
		return err
	}
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
		if err := c.Comment(key, body); err != nil {
			return err
		}
	}
	return fmt.Errorf("%d mismatch(es)", len(flags))
}

func traceCmd(root, base, key string, pr int, out string) error {
	if base == "" {
		return fmt.Errorf("trace needs -base")
	}
	if key == "" {
		key = jira.Key(branch(root))
	}
	ds, err := docs.Deltas(root, base)
	if err != nil {
		return err
	}
	ms, _ := manifest.Load(filepath.Join(root, ".scribex", "manifest.yaml"))
	w := os.Stdout
	if out != "" {
		if w, err = os.Create(out); err != nil {
			return err
		}
		defer w.Close()
	}
	prs := ""
	if pr > 0 {
		prs = fmt.Sprint(pr)
	}
	return trace.Write(w, key, prs, ds, docs.Refs(root), ms)
}

func serve(root, reposF, rbacF, addr string) error {
	rs, err := dashboard.LoadRepos(reposF)
	if err != nil {
		return err
	}
	ps, err := dashboard.LoadRBAC(rbacF)
	if err != nil {
		return err
	}
	fmt.Println("scribex dashboard on", addr)
	return http.ListenAndServe(addr, dashboard.Handler(rs, ps, root))
}

// companion commits only .md files touched by sync; no-op when clean.
func companion(root string) error {
	if out, _ := exec.Command("git", "-C", root, "status", "--porcelain", "--", "*.md").Output(); len(out) == 0 {
		return nil
	}
	if err := exec.Command("git", "-C", root, "add", "--", "*.md").Run(); err != nil {
		return err
	}
	return exec.Command("git", "-C", root, "commit", "-m", "docs: scribex sync", "--", "*.md").Run()
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
			if err := os.WriteFile(f, []byte(o), 0o644); err != nil {
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
