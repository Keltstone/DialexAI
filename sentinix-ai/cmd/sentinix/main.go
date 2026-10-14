// sentinix: scan a repo, apply baseline + gate, optionally map a diff's attack surface.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "scan" && os.Args[1] != "surface" {
		fmt.Fprintln(os.Stderr, "usage: sentinix scan [flags] [path] | sentinix surface [base-ref]")
		os.Exit(2)
	}
	if os.Args[1] == "surface" {
		surface(os.Args[2:])
		return
	}
	fl := flag.NewFlagSet("scan", flag.ExitOnError)
	baselinePath := fl.String("baseline", "", "baseline JSON; only findings not in it are reported")
	update := fl.Bool("update-baseline", false, "write current findings to -baseline and exit 0")
	gate := fl.String("fail-on", "high", "fail at or above: info|low|medium|high|critical")
	sarif := fl.String("sarif", "", "write SARIF to this file")
	fl.Parse(os.Args[2:])
	root := "."
	if fl.NArg() > 0 {
		root = fl.Arg(0)
	}
	min, err := engines.ParseSeverity(*gate)
	die(err)

	var all []engines.Finding
	var ran []string
	for _, e := range engines.Bundled() {
		fs, err := e.Scan(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "engine error:", err) // a broken engine must not look like a clean scan
			defer os.Exit(3)
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
	if policy.Fails(all, min) {
		defer os.Exit(1)
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
