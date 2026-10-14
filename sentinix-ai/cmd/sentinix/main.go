// sentinix: scan a repo, apply baseline + gate, map a diff's attack surface, manage rule packs.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"sentinix/pkg/advisories"
	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
	"sentinix/pkg/packs"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
)

const usage = "usage: sentinix scan [flags] [path] | surface [base-ref] | packs keygen|sign|verify"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "scan":
		scan(args)
	case "surface":
		surface(args)
	case "packs":
		packCmd(args)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}

func scan(args []string) {
	fl := flag.NewFlagSet("scan", flag.ExitOnError)
	baselinePath := fl.String("baseline", "", "baseline JSON; only findings not in it are reported")
	update := fl.Bool("update-baseline", false, "write current findings to -baseline and exit 0")
	gate := fl.String("fail-on", "high", "fail at or above: info|low|medium|high|critical")
	sarif := fl.String("sarif", "", "write SARIF to this file")
	packDir := fl.String("packs", "", "directory of signed extra rule packs")
	trust := fl.String("trust", "", "file of trusted pack public keys (needed with -packs)")
	osv := fl.String("osv", "", "OSV API URL for dependency advisories (empty: off, nothing leaves the machine)")
	fl.Parse(args)
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
		fs, err := e.Scan(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "engine error:", err) // a broken engine must not look like a clean scan
			code = 3
			continue
		}
		ran = append(ran, e.Name())
		all = append(all, fs...)
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
	if code == 0 && policy.Fails(all, min) {
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
