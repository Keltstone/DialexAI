package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"artix/pkg/pilot"
)

func runPilot(cwd string, args []string, human io.Writer, sendJSON func(any), isJSON bool, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: artix pilot report [--since <RFC3339>] [--json]\n")
		return 1
	}

	subcmd := args[0]
	if subcmd != "report" {
		fmt.Fprintf(stderr, "Unknown pilot subcommand: %s. Expected 'report'\n", subcmd)
		return 1
	}

	fs := flag.NewFlagSet("pilot report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var sinceStr string
	fs.StringVar(&sinceStr, "since", "", "Filter audit log records since timestamp (RFC3339)")

	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	var since time.Time
	if sinceStr != "" {
		var parseErr error
		since, parseErr = time.Parse(time.RFC3339, sinceStr)
		if parseErr != nil {
			since, parseErr = time.Parse("2006-01-02", sinceStr)
		}
		if parseErr != nil {
			fmt.Fprintf(stderr, "Invalid --since format: %v (expected RFC3339)\n", parseErr)
			return 1
		}
	}

	metrics, err := pilot.GeneratePilotReport(cwd, since)
	if err != nil {
		fmt.Fprintf(stderr, "Error generating pilot report: %v\n", err)
		return 1
	}

	if isJSON {
		sendJSON(map[string]any{
			"ok":      true,
			"metrics": metrics,
		})
		return 0
	}

	fmt.Fprint(human, metrics.FormatPlainText())
	return 0
}
