// Package mcp is a read-only MCP stdio server (newline-delimited JSON-RPC) so coding agents can ask
// Sentinix for findings and entry points. Tools are deterministic: no model, no network, root fixed at start.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sentinix/pkg/engines"
	"sentinix/pkg/hunt"
	"sentinix/pkg/packs"
)

var empty = map[string]any{"type": "object", "properties": map[string]any{}}

var tools = []map[string]any{
	{"name": "sentinix_scan", "description": "Scan the project with built-in rule packs and secrets engine; one finding per line.", "inputSchema": empty},
	{"name": "sentinix_entrypoints", "description": "List exported components, deeplink handlers and HTTP routes (audit targets).", "inputSchema": empty},
	{"name": "sentinix_hunt_plan", "description": "Preview planned AI audit entry points, vulnerability classes, task count, and cost estimate without running models.", "inputSchema": empty},
	{"name": "sentinix_hunt_results", "description": "List AI audit findings and exploitability status from recent runs.", "inputSchema": empty},
}

func Serve(in io.Reader, out io.Writer, root string) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
				Name            string `json:"name"`
			} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue // malformed or notification
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "initialize":
			v := req.Params.ProtocolVersion
			if v == "" {
				v = "2024-11-05"
			}
			resp["result"] = map[string]any{"protocolVersion": v, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "sentinix", "version": "1.0.0"}}
		case "ping":
			resp["result"] = map[string]any{}
		case "tools/list":
			resp["result"] = map[string]any{"tools": tools}
		case "tools/call":
			text, err := call(root, req.Params.Name)
			if err != nil {
				text = err.Error()
			}
			resp["result"] = map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": err != nil}
		default:
			resp["error"] = map[string]any{"code": -32601, "message": "method not found"}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func call(root, name string) (string, error) {
	switch name {
	case "sentinix_scan":
		ps, err := packs.Builtin()
		if err != nil {
			return "", err
		}
		var lines []string
		var ran []string
		for _, e := range engines.Bundled(packs.Engine{Packs: ps}) {
			fs, err := e.Scan(context.Background(), root)
			if err != nil { // engine unavailable is a coverage gap, say so rather than "clean"
				lines = append(lines, "not covered: "+err.Error())
				continue
			}
			ran = append(ran, e.Name())
			for _, f := range fs {
				lines = append(lines, fmt.Sprintf("[%s] %s %s:%d %s", f.Severity, f.Rule, f.File, f.Line, f.Message))
			}
		}
		return strings.Join(append(lines, "engines run: "+strings.Join(ran, ", ")+". A clean result is not proof of security."), "\n"), nil
	case "sentinix_entrypoints":
		eps, err := hunt.Entrypoints(root)
		if err != nil {
			return "", err
		}
		var lines []string
		for _, e := range eps {
			lines = append(lines, fmt.Sprintf("%s %s %s", strings.Join(e.Kinds, "+"), e.Name, e.File))
		}
		return strings.Join(lines, "\n"), nil
	case "sentinix_hunt_plan":
		cfg, _ := hunt.ResolveConfig(root, nil, hunt.CLIFlags{})
		eps, err := hunt.EntrypointsWithConfig(root, &cfg)
		if err != nil {
			return "", err
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("Planned hunt audit for %s:", root))
		lines = append(lines, fmt.Sprintf("- Taskflow: %s", cfg.Taskflow))
		lines = append(lines, fmt.Sprintf("- Entry points discovered: %d", len(eps)))
		lib, _ := hunt.NewClassLibrary(cfg.Classes, nil)
		totalPasses := 0
		for _, ep := range eps {
			cls := lib.ClassesFor(ep, cfg.Platform, nil)
			totalPasses += len(cls)
		}
		lines = append(lines, fmt.Sprintf("- Total task passes: %d", totalPasses))
		lines = append(lines, fmt.Sprintf("- Policy: %s", cfg.Policy))
		lines = append(lines, fmt.Sprintf("- Budget cap: %d calls, $%.2f USD", cfg.MaxCalls, cfg.MaxCostUSD))
		return strings.Join(lines, "\n"), nil
	case "sentinix_hunt_results":
		return "Use 'sentinix hunt runs' or check dashboard at http://localhost:4150 for full historical findings.", nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}
