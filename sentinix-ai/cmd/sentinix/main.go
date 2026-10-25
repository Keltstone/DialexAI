// sentinix: scan a repo, apply baseline + gate, map a diff's attack surface, manage rule packs.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/ai"
	"sentinix/pkg/bench"
	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
	"sentinix/pkg/hunt"
	"sentinix/pkg/mcp"
	"sentinix/pkg/packs"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
	"sentinix/pkg/sandbox"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

var version = "1.0.0"

const usage = "usage: sentinix scan [flags] [path] | sign [flags] [file] | surface [base-ref] | packs keygen|sign|verify|new|test | rules new|test|sign | policy init|show | hook install|run|uninstall | bench <dir> | audit verify [-db <dsn>] | hunt -ai models.json [path] | mcp serve [dir] | version"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("Sentinix AI version %s\n", version)
	case "scan":
		scan(args)
	case "sign":
		signCmd(args)
	case "surface":
		surface(args)
	case "packs", "rules":
		packCmd(args)
	case "policy":
		policyCmd(args)
	case "hook":
		hookCmd(args)
	case "bench":
		benchCmd(args)
	case "audit":
		auditCmd(args)
	case "hunt":
		huntCmd(args)
	case "mcp":
		if len(args) < 1 || args[0] != "serve" {
			fmt.Fprintln(os.Stderr, "usage: sentinix mcp serve [dir]")
			os.Exit(2)
		}
		root := "."
		if len(args) > 1 {
			root = args[1]
		}
		die(mcp.Serve(os.Stdin, os.Stdout, root))
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}

func scan(args []string) {
	fl := flag.NewFlagSet("scan", flag.ExitOnError)
	baselinePath := fl.String("baseline", "", "baseline JSON; only findings not in it are reported")
	update := fl.Bool("update-baseline", false, "write current findings to -baseline and exit 0")
	reportOnly := fl.Bool("report-only", false, "print findings but never fail on them (exit 0 unless an engine failed)")
	gate := fl.String("fail-on", "high", "fail at or above: info|low|medium|high|critical")
	policyPath := fl.String("policy", "", "path to project policy JSON (defaults to .sentinix/policy.json if present)")
	sarif := fl.String("sarif", "", "write SARIF to this file")
	packDir := fl.String("packs", "", "directory of signed extra rule packs")
	customRules := fl.String("rules", "", "path to custom rule pack JSON (loads without signature for local dev)")
	trust := fl.String("trust", "", "file of trusted pack public keys (needed with -packs)")
	server := fl.String("server", "", "Sentinix server URL; upload findings for this project (CI)")
	project := fl.String("project", "", "project id on the server (with -server)")
	tokenEnv := fl.String("token-env", "SENTINIX_TOKEN", "environment variable holding the project API token")
	osv := fl.String("osv", "", "OSV API URL for dependency advisories (empty: off, nothing leaves the machine)")
	maxFetchBytes := fl.Int64("max-fetch-bytes", 500<<20, "maximum fetch and working-tree size in bytes")
	fl.Parse(args)
	scheduler.MaxFetchBytes = *maxFetchBytes
	root := "."
	if fl.NArg() > 0 {
		root = fl.Arg(0)
	}
	min, err := engines.ParseSeverity(*gate)
	die(err)
	pe, err := packs.NewEngine(*packDir, *trust)
	die(err)
	if *customRules != "" {
		rb, err := os.ReadFile(*customRules)
		die(err)
		cp, err := packs.Parse(rb)
		die(err)
		pe.Packs = append(pe.Packs, cp)
	} else if *packDir == "" {
		localRulesPath := filepath.Join(root, ".sentinix", "rules.json")
		if _, err := os.Stat(localRulesPath); err == nil {
			if rb, err := os.ReadFile(localRulesPath); err == nil {
				if cp, err := packs.Parse(rb); err == nil {
					pe.Packs = append(pe.Packs, cp)
				}
			}
		}
	}

	var all []engines.Finding
	var ran []string
	code := 0
	extra := []engines.Engine{pe}
	if *osv != "" {
		extra = append(extra, advisories.Engine{Client: advisories.Client{URL: *osv}})
	}
	for _, e := range engines.Bundled(extra...) {
		fs, err := e.Scan(context.Background(), root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "engine error:", err) // a broken engine must not look like a clean scan
			code = 3
			continue
		}
		ran = append(ran, e.Name())
		all = append(all, fs...)
	}

	if *server != "" {
		if code == 0 { // never upload a partial scan: the server would mark the missing engine's findings fixed
			die(upload(*server, *project, os.Getenv(*tokenEnv), ran, all))
		}
	}
	if *update {
		if *baselinePath == "" {
			die(fmt.Errorf("-update-baseline needs -baseline"))
		}
		die(policy.SaveBaseline(*baselinePath, all))
		return
	}
	if *baselinePath != "" {
		b, err := policy.LoadBaseline(*baselinePath)
		die(err)
		all = b.New(all)
	}
	var projPolicy *policy.ProjectPolicy
	if *policyPath != "" {
		p, err := policy.LoadProjectPolicy(*policyPath)
		die(err)
		projPolicy = p
	} else {
		defaultPolicyPath := filepath.Join(root, ".sentinix", "policy.json")
		if _, err := os.Stat(defaultPolicyPath); err == nil {
			if p, err := policy.LoadProjectPolicy(defaultPolicyPath); err == nil {
				projPolicy = p
			}
		}
	}
	var ev policy.EvaluatedFindings
	if projPolicy != nil {
		ev = policy.EvaluateFindings(all, projPolicy)
		all = ev.Active()
	}

	if *sarif != "" {
		f, err := os.Create(*sarif)
		die(err)
		die(report.SARIF(f, all))
		f.Close()
	}
	report.Text(os.Stdout, all, ran)
	if code == 0 && !*reportOnly {
		if projPolicy != nil {
			if policy.PolicyFails(ev) {
				code = 1
			}
		} else if policy.Fails(all, min) {
			code = 1
		}
	}
	os.Exit(code)
}

func policyCmd(a []string) {
	if len(a) < 1 {
		die(fmt.Errorf("usage: sentinix policy init [path] | show [path]"))
	}
	switch a[0] {
	case "init":
		target := ".sentinix/policy.json"
		if len(a) > 1 {
			target = a[1]
		}
		dir := filepath.Dir(target)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				die(err)
			}
		}
		pol := policy.ProjectPolicy{
			Version:      "1.0.0",
			DefaultLevel: policy.LevelWarn,
			Rules: map[string]policy.RuleEnforcementLevel{
				"android/exported-component":      policy.LevelBlock,
				"go/insecure-tls-skip-verify":     policy.LevelBlock,
				"python/insecure-deserialization": policy.LevelBlock,
				"node/eval-injection":             policy.LevelBlock,
				"dotnet/insecure-deserialization": policy.LevelBlock,
				"php/insecure-unserialize":        policy.LevelBlock,
				"rust/transmute":                  policy.LevelBlock,
			},
		}
		b, err := json.MarshalIndent(pol, "", "  ")
		die(err)
		if err := os.WriteFile(target, append(b, '\n'), 0o644); err != nil {
			die(err)
		}
		fmt.Printf("Initialized policy at %s\n", target)
	case "show":
		target := ".sentinix/policy.json"
		if len(a) > 1 {
			target = a[1]
		}
		p, err := policy.LoadProjectPolicy(target)
		die(err)
		b, _ := json.MarshalIndent(p, "", "  ")
		fmt.Println(string(b))
	default:
		die(fmt.Errorf("unknown policy subcommand: %s", a[0]))
	}
}

func packCmd(a []string) {
	if len(a) < 1 {
		die(fmt.Errorf("usage: packs keygen <prefix> | sign <body.json> <keyfile> | verify <signed.json> <pubfile> | new [name] [file] | test <file>"))
	}
	b64 := base64.StdEncoding
	switch a[0] {
	case "new":
		name := "custom-rules"
		outFile := "rules.json"
		if len(a) > 1 {
			name = a[1]
			if strings.HasSuffix(a[1], ".json") {
				outFile = a[1]
				name = strings.TrimSuffix(filepath.Base(a[1]), ".json")
			} else {
				outFile = a[1] + ".json"
			}
		}
		if len(a) > 2 {
			outFile = a[2]
		}
		tmpl := map[string]any{
			"name":     name,
			"version":  "1.0.0",
			"category": "security",
			"stacks":   []string{},
			"rules": []map[string]any{
				{
					"id":          name + "/no-hardcoded-secret",
					"severity":    "high",
					"cwe":         "CWE-798",
					"message":     "Hardcoded credential or token detected. Use environment variables or secret vault.",
					"remediation": "Replace hardcoded value with System.getenv() or VaultConfig.getSecret()",
					"files":       []string{"*.kt", "*.java", "*.go", "*.ts", "*.js", "*.py"},
					"pattern":     `(?i)(api[_-]?key|auth[_-]?token|secret[_-]?key)\s*[:=]\s*["'][a-zA-Z0-9_\-]{16,}["']`,
					"unless":      "EXAMPLE|TEST|MOCK|DUMMY",
				},
			},
			"tests": []map[string]any{
				{
					"rule": name + "/no-hardcoded-secret",
					"file": "AuthService.kt",
					"bad":  `val apiKey = "live_sk_abcdef1234567890"`,
					"good": `val apiKey = VaultConfig.getSecret("api_key")`,
				},
			},
		}
		data, err := json.MarshalIndent(tmpl, "", "  ")
		die(err)
		if dir := filepath.Dir(outFile); dir != "" && dir != "." {
			os.MkdirAll(dir, 0o755)
		}
		die(os.WriteFile(outFile, append(data, '\n'), 0o644))
		fmt.Printf("✓ Created custom security rule pack template in %s\n", outFile)
		fmt.Printf("  • Edit patterns, messages, and test fixtures in %s\n", outFile)
		fmt.Printf("  • Verify anytime: sentinix rules test %s\n", outFile)
		fmt.Printf("  • Run locally:    sentinix scan -rules %s .\n", outFile)
	case "test":
		if len(a) < 2 {
			die(fmt.Errorf("test needs <rules.json>"))
		}
		body, err := os.ReadFile(a[1])
		die(err)
		var signed packs.Signed
		if json.Unmarshal(body, &signed) == nil && len(signed.Body) > 0 {
			body = signed.Body
		}
		p, err := packs.Parse(body)
		die(err)
		fmt.Printf("✓ Pack %q v%s: %d rule(s) compiled successfully.\n", p.Name, p.Version, len(p.Rules))
		for _, s := range p.Samples {
			fmt.Printf("  • Rule %q verified on sample %q (good and bad fixtures passed)\n", s.Rule, s.File)
		}
		fmt.Println("🎉 All custom rules and regression tests verified!")
	case "keygen":
		if len(a) < 2 {
			die(fmt.Errorf("keygen needs <prefix>"))
		}
		pub, priv, err := ed25519.GenerateKey(nil)
		die(err)
		die(os.WriteFile(a[1]+".key", []byte(b64.EncodeToString(priv)), 0o600))
		die(os.WriteFile(a[1]+".pub", []byte(b64.EncodeToString(pub)+"\n"), 0o644))
	case "sign":
		if len(a) < 3 {
			die(fmt.Errorf("sign needs <body.json> <keyfile>"))
		}
		body, err := os.ReadFile(a[1])
		die(err)
		_, err = packs.Parse(body) // refuse to sign a pack whose own tests fail
		die(err)
		kb, err := os.ReadFile(a[2])
		die(err)
		priv, err := b64.DecodeString(strings.TrimSpace(string(kb)))
		die(err)
		out, err := packs.Sign(body, ed25519.PrivateKey(priv))
		die(err)
		os.Stdout.Write(out)
	case "verify":
		if len(a) < 3 {
			die(fmt.Errorf("verify needs <signed.json> <pubfile>"))
		}
		data, err := os.ReadFile(a[1])
		die(err)
		keys, err := packs.ReadKeys(a[2])
		die(err)
		p, err := packs.Load(data, keys)
		die(err)
		fmt.Printf("ok: %s %s, %d rules\n", p.Name, p.Version, len(p.Rules))
	default:
		die(fmt.Errorf("unknown packs command %q", a[0]))
	}
}

func hookCmd(a []string) {
	if len(a) < 1 {
		die(fmt.Errorf("usage: hook install | run | uninstall"))
	}
	switch a[0] {
	case "install":
		if _, err := os.Stat(".git"); os.IsNotExist(err) {
			die(fmt.Errorf(".git directory not found; run this command from the root of a git repository"))
		}
		hooksDir := filepath.Join(".git", "hooks")
		os.MkdirAll(hooksDir, 0o755)
		hookFile := filepath.Join(hooksDir, "pre-commit")
		hookScript := `#!/usr/bin/env sh
# Sentinix Developer Pre-Commit Hook
# Prevents committing secrets and high-severity policy violations.

if ! command -v sentinix >/dev/null 2>&1; then
    if [ -x "./dist/bin/sentinix" ]; then
        SENTINIX_BIN="./dist/bin/sentinix"
    else
        exit 0
    fi
else
    SENTINIX_BIN="sentinix"
fi

$SENTINIX_BIN hook run
`
		err := os.WriteFile(hookFile, []byte(hookScript), 0o755)
		die(err)
		fmt.Printf("✓ Sentinix pre-commit hook installed to %s\n", hookFile)
	case "uninstall":
		hookFile := filepath.Join(".git", "hooks", "pre-commit")
		if _, err := os.Stat(hookFile); err == nil {
			os.Remove(hookFile)
			fmt.Printf("✓ Sentinix pre-commit hook removed from %s\n", hookFile)
		} else {
			fmt.Println("Pre-commit hook was not installed.")
		}
	case "run":
		out, err := exec.Command("git", "diff", "--cached", "--name-only").Output()
		if err != nil {
			os.Exit(0)
		}
		files := strings.Fields(strings.TrimSpace(string(out)))
		if len(files) == 0 {
			os.Exit(0)
		}
		cmd := exec.Command(os.Args[0], "scan", "-fail-on", "high", ".")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "\n🛑 Sentinix Pre-Commit Check Failed! High-severity issues detected.")
			fmt.Fprintln(os.Stderr, "Resolve the issues above before committing, or bypass with: git commit --no-verify")
			os.Exit(1)
		}
		fmt.Println("✓ Sentinix pre-commit check: clean.")
	default:
		die(fmt.Errorf("unknown hook command %q", a[0]))
	}
}

func surface(args []string) {
	base := "origin/main"
	if len(args) > 0 {
		base = args[0]
	}
	out, err := exec.Command("git", "diff", "-U0", base+"...HEAD").Output()
	die(err)
	items, err := diffmap.Parse(strings.NewReader(string(out)))
	die(err)
	for _, i := range items {
		fmt.Printf("%-10s %-7s %s: %s\n", i.Type, i.Change, i.File, i.Detail)
	}
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func upload(server, project, token string, ran []string, fs []engines.Finding) error {
	if project == "" || token == "" {
		return fmt.Errorf("-server needs -project and the token environment variable")
	}
	commit, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	wireFs := make([]engines.Finding, len(fs))
	for i, f := range fs {
		wireFs[i] = f
		if f.Match != "" {
			h := sha256.Sum256([]byte(f.Match))
			wireFs[i].Match = hex.EncodeToString(h[:])
		}
	}
	body, _ := json.Marshal(map[string]any{"commit": strings.TrimSpace(string(commit)), "findings": wireFs})
	req, err := http.NewRequest("POST", strings.TrimRight(server, "/")+"/api/projects/"+project+"/ingest", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	if signingKey := os.Getenv("SENTINIX_SIGNING_KEY"); signingKey != "" {
		if kb, err := os.ReadFile(signingKey); err == nil {
			signingKey = strings.TrimSpace(string(kb))
		}
		if b, err := base64.StdEncoding.DecodeString(signingKey); err == nil {
			var privKey ed25519.PrivateKey
			if len(b) == ed25519.PrivateKeySize {
				privKey = ed25519.PrivateKey(b)
			} else if len(b) == ed25519.SeedSize {
				privKey = ed25519.NewKeyFromSeed(b)
			}
			if len(privKey) > 0 {
				ts := fmt.Sprintf("%d", time.Now().Unix())
				h := sha256.Sum256(body)
				commitSHA := strings.TrimSpace(string(commit))
				msg := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%x", project, commitSHA, "", "secrets", "findings", ts, h)
				sig := base64.StdEncoding.EncodeToString(ed25519.Sign(privKey, []byte(msg)))
				req.Header.Set("X-Signature", sig)
				req.Header.Set("X-Signature-Timestamp", ts)
			}
		}
	}

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("upload failed: %s", resp.Status)
	}
	return nil
}

func signCmd(args []string) {
	fl := flag.NewFlagSet("sign", flag.ExitOnError)
	keyArg := fl.String("key", "", "ed25519 private key (base64 string, or path to key file)")
	project := fl.String("project", "", "project ID")
	commit := fl.String("commit", "", "commit SHA (defaults to git rev-parse HEAD)")
	pr := fl.String("pr", "", "pull request number or identifier")
	engine := fl.String("engine", "detekt", "engine name (e.g. detekt, ktlint, konsist)")
	format := fl.String("format", "sarif", "report format (sarif, junit, findings)")
	timestamp := fl.Int64("timestamp", 0, "unix timestamp in seconds (defaults to current time)")
	raw := fl.Bool("raw", false, "output only the base64 signature string")
	fl.Parse(args)

	if *project == "" {
		die(fmt.Errorf("sign requires -project"))
	}
	keyStr := *keyArg
	if keyStr == "" {
		keyStr = os.Getenv("SENTINIX_SIGNING_KEY")
	}
	if keyStr == "" {
		die(fmt.Errorf("sign requires -key or SENTINIX_SIGNING_KEY"))
	}

	var privKey ed25519.PrivateKey
	if kb, err := os.ReadFile(keyStr); err == nil {
		keyStr = strings.TrimSpace(string(kb))
	}
	if b, err := base64.StdEncoding.DecodeString(keyStr); err == nil {
		if len(b) == ed25519.PrivateKeySize {
			privKey = ed25519.PrivateKey(b)
		} else if len(b) == ed25519.SeedSize {
			privKey = ed25519.NewKeyFromSeed(b)
		}
	} else if len(keyStr) == ed25519.PrivateKeySize {
		privKey = ed25519.PrivateKey([]byte(keyStr))
	}
	if len(privKey) == 0 {
		die(fmt.Errorf("invalid ed25519 private key (must be base64-encoded 64-byte key or 32-byte seed)"))
	}

	commitSHA := *commit
	if commitSHA == "" {
		if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
			commitSHA = strings.TrimSpace(string(out))
		}
	}
	if commitSHA == "" {
		die(fmt.Errorf("sign requires -commit or a git repository"))
	}

	ts := *timestamp
	if ts == 0 {
		ts = time.Now().Unix()
	}

	var body []byte
	var err error
	if fl.NArg() > 0 {
		body, err = os.ReadFile(fl.Arg(0))
		die(err)
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			body, err = io.ReadAll(os.Stdin)
			die(err)
		} else {
			die(fmt.Errorf("sign requires a report file argument or stdin"))
		}
	}

	bodyHash := sha256.Sum256(body)
	msg := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%x", *project, commitSHA, *pr, *engine, *format, ts, bodyHash)
	sig := ed25519.Sign(privKey, []byte(msg))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	if *raw {
		fmt.Println(sigB64)
	} else {
		fmt.Printf("X-Signature: %s\n", sigB64)
		fmt.Printf("X-Signature-Timestamp: %d\n", ts)
	}
}

// benchCmd scores the built-in packs on a labelled sample project against the spec targets.
func benchCmd(a []string) {
	if len(a) < 1 {
		die(fmt.Errorf("usage: bench <dir>"))
	}
	ps, err := packs.Builtin()
	die(err)
	m, err := bench.Run(a[0], []engines.Engine{packs.Engine{Packs: ps}})
	die(err)
	fmt.Printf("planted %d, reported %d, precision %.0f%%, recall %.0f%%\n", m.Planted, m.Reported, m.Precision*100, m.Recall*100)
	for _, x := range m.Missed {
		fmt.Printf("missed: %s in %s\n", x.Rule, x.File)
	}
	for _, f := range m.FalsePositives {
		fmt.Printf("false positive: %s %s:%d\n", f.Rule, f.File, f.Line)
	}
	if !m.Meets(0.80, 0.70) {
		os.Exit(1)
	}
}

func auditCmd(args []string) {
	if len(args) < 1 || args[0] != "verify" {
		fmt.Fprintln(os.Stderr, "usage: sentinix audit verify [-db <dsn>]")
		os.Exit(2)
	}
	fl := flag.NewFlagSet("audit verify", flag.ExitOnError)
	dsn := fl.String("db", "sqlite:sentinix.db", "database DSN (sqlite:<path> or postgres://…)")
	fl.Parse(args[1:])

	st, err := store.Open(*dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening store: %v\n", err)
		os.Exit(1)
	}
	if err := st.VerifyAuditChain(); err != nil {
		fmt.Fprintf(os.Stderr, "AUDIT CHAIN VERIFICATION FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("AUDIT CHAIN VERIFIED: all rows valid, hash chain intact.")
}


func huntCmd(args []string) {
	fl := flag.NewFlagSet("hunt", flag.ExitOnError)
	models := fl.String("ai", "", "models JSON (required)")
	policy := fl.String("policy", "local-only", "model policy: local-only|cloud-redacted|cloud")
	maxCalls := fl.Int("max-calls", 200, "hard cap on model calls")
	sbImage := fl.String("sandbox-image", "", "container image; with -poc-cmd, model-written PoC tests run in it (no network)")
	pocCmd := fl.String("poc-cmd", "", "test command for PoC tests, e.g. \"./gradlew test\" (needs -sandbox-image)")
	huntPacks := fl.String("hunt-packs", "", "directory of signed hunt packs (extra vulnerability classes)")
	trust := fl.String("trust", "", "file of trusted pack public keys (needed with -hunt-packs)")
	variant := fl.String("variant", "", "file with a known vulnerability description; also hunt for variants of it")
	useTools := fl.Bool("tools", false, "let the model read files and grep inside the path before answering (read-only)")
	toolSteps := fl.Int("tool-steps", 3, "max tool calls per task")
	platform := fl.String("platform", "", "audit only mobile or backend entry points (default: both)")
	resume := fl.String("resume", "", "state file; finished tasks are skipped on re-run")
	only := fl.String("only", "", "run only tasks whose file|class contains this text")
	ghsa := fl.String("variant-ghsa", "", "public GitHub advisory id (GHSA-...); fetched and used like -variant")
	fl.Parse(args)
	root := "."
	if fl.NArg() > 0 {
		root = fl.Arg(0)
	}
	if *models == "" {
		die(fmt.Errorf("usage: hunt -ai models.json [-policy p] [-max-calls n] [path]"))
	}
	ms, err := ai.LoadModels(*models)
	die(err)
	e := &hunt.Engine{Models: ms, Policy: *policy, MaxCalls: *maxCalls}
	e.Tools, e.ToolSteps, e.Platform, e.Only = *useTools, *toolSteps, *platform, *only
	if *resume != "" {
		e.State, err = hunt.LoadState(*resume)
		die(err)
	}
	if *variant != "" {
		b, err := os.ReadFile(*variant)
		die(err)
		e.Variant = string(b)
	}
	if *ghsa != "" {
		e.Variant, err = hunt.FetchGHSA(context.Background(), *ghsa, "")
		die(err)
	}
	if *huntPacks != "" {
		keys, err := packs.ReadKeys(*trust)
		die(err)
		files, _ := filepath.Glob(filepath.Join(*huntPacks, "*.json"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			die(err)
			p, err := hunt.LoadPack(b, keys)
			if err != nil {
				die(fmt.Errorf("%s: %w", f, err))
			}
			e.Packs = append(e.Packs, p)
		}
	}
	if *pocCmd != "" {
		if *sbImage == "" {
			die(fmt.Errorf("-poc-cmd runs model-written code: -sandbox-image is required"))
		}
		e.Run, e.PoCCmd = sandbox.Docker{Image: *sbImage}.Run, strings.Fields(*pocCmd)
	}
	fs, err := e.Scan(context.Background(), root)
	die(err)
	for _, f := range fs {
		fmt.Printf("[%s] %s %s (AI-suggested, unverified) %s %s\n", f.Severity, f.Rule, f.File, f.Message, f.Evidence)
	}
	fmt.Printf("%d finding(s). Engines run: llm-audit (advisory).", len(fs))
	if e.Truncated {
		fmt.Printf(" Not covered: stopped at -max-calls %d.", *maxCalls)
	}
	fmt.Println()
}
