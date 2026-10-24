package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"scribex/pkg/config"
	"scribex/pkg/docs"
	"scribex/pkg/wiki"
)

var obj = func(props map[string]any, req ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": req}
}
var str = map[string]any{"type": "string"}

var tools = []map[string]any{
	{"name": "scribex_check", "description": "Report stale or dead doc anchors in the repo.", "inputSchema": obj(map[string]any{})},
	{"name": "scribex_wiki_lint", "description": "Broken wiki links, orphan pages, duplicate names.", "inputSchema": obj(map[string]any{})},
	{"name": "scribex_wiki_search", "description": "Search wiki pages (title and body).", "inputSchema": obj(map[string]any{"query": str}, "query")},
	{"name": "scribex_wiki_page", "description": "Read one wiki page by name.", "inputSchema": obj(map[string]any{"name": str}, "name")},
	{"name": "scribex_wiki_backlinks", "description": "Pages that link to the given page.", "inputSchema": obj(map[string]any{"name": str}, "name")},
}

// Serve speaks newline-delimited JSON-RPC (MCP stdio). Tools are read-only.
func Serve(in io.Reader, out io.Writer, root string) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string         `json:"protocolVersion"`
				Name            string         `json:"name"`
				Arguments       map[string]any `json:"arguments"`
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
			resp["result"] = map[string]any{"protocolVersion": v, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "scribex", "version": "1.0.0"}}
		case "ping":
			resp["result"] = map[string]any{}
		case "tools/list":
			resp["result"] = map[string]any{"tools": tools}
		case "tools/call":
			text, err := call(root, req.Params.Name, req.Params.Arguments)
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

func call(root, name string, args map[string]any) (string, error) {
	arg := func(k string) string { s, _ := args[k].(string); return s }
	if name == "scribex_check" {
		r, err := docs.Check(root, false, nil)
		return r.Markdown(), err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "", err
	}
	w, err := wiki.Load(filepath.Join(root, cfg.Wiki.Dir))
	if err != nil {
		return "", err
	}
	switch name {
	case "scribex_wiki_lint":
		if p := w.Lint(cfg.Wiki.Home); len(p) > 0 {
			return strings.Join(p, "\n"), nil
		}
		return "wiki clean", nil
	case "scribex_wiki_search":
		return strings.Join(w.Search(arg("query")), "\n"), nil
	case "scribex_wiki_page":
		p := w.Pages[wiki.Key(arg("name"))]
		if p == nil {
			return "", fmt.Errorf("no such page")
		}
		return p.Body, nil
	case "scribex_wiki_backlinks":
		bl := w.Backlinks()[wiki.Key(arg("name"))]
		sort.Strings(bl)
		return strings.Join(bl, "\n"), nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}
