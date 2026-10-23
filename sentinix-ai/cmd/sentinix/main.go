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
	"strings"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/bench"
	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
	"sentinix/pkg/packs"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

var version = "1.0.0"

const usage = "usage: sentinix scan [flags] [path] | sign [flags] [file] | surface [base-ref] | packs keygen|sign|verify | bench <dir> | audit verify [-db <dsn>] | version"

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
	case "packs":
		packCmd(args)
	case "bench":
		benchCmd(args)
	case "audit":
		auditCmd(args)
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
	sarif := fl.String("sarif", "", "write SARIF to this file")
	packDir := fl.String("packs", "", "directory of signed extra rule packs")
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
	if *sarif != "" {
		f, err := os.Create(*sarif)
		die(err)
		die(report.SARIF(f, all))
		f.Close()
	}
	report.Text(os.Stdout, all, ran)
	if code == 0 && !*reportOnly && policy.Fails(all, min) {
		code = 1
	}
	os.Exit(code)
}

func packCmd(a []string) {
	if len(a) < 2 {
		die(fmt.Errorf("usage: packs keygen <prefix> | sign <body.json> <keyfile> | verify <signed.json> <pubfile>"))
	}
	b64 := base64.StdEncoding
	switch a[0] {
	case "keygen":
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

