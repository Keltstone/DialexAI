package mcp

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPConfig(t *testing.T) {
	root := t.TempDir()

	p := Path(root)
	if !strings.HasSuffix(p, filepath.Join(".scribex", "mcp.json")) {
		t.Fatalf("unexpected path: %s", p)
	}

	// Load empty
	f, err := Load(root)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(f.Servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(f.Servers))
	}

	// Add server and save
	f.Servers["test"] = Server{
		Command: "echo",
		Args:    []string{"hello"},
		Env: map[string]string{
			"API_KEY": "${MY_KEY}",
		},
	}
	if err := Save(root, f); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Reload
	f2, err := Load(root)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(f2.Servers) != 1 || f2.Servers["test"].Command != "echo" {
		t.Fatalf("unexpected loaded servers: %+v", f2.Servers)
	}

	// Presets
	names := PresetNames()
	if len(names) == 0 {
		t.Fatal("expected presets")
	}

	// Validation
	sGood := Server{Command: "cmd"}
	if err := sGood.Validate(); err != nil {
		t.Fatalf("expected valid: %v", err)
	}

	sBadCmdURL := Server{Command: "cmd", URL: "http://example.com"}
	if err := sBadCmdURL.Validate(); err == nil {
		t.Fatal("expected error for both command and url")
	}

	sBadLiteral := Server{
		Command: "cmd",
		Env: map[string]string{
			"API_TOKEN": "literal_secret",
		},
	}
	if err := sBadLiteral.Validate(); err == nil {
		t.Fatal("expected error for literal secret")
	}
	if bad := sBadLiteral.Literals(); len(bad) != 1 || bad[0] != "API_TOKEN" {
		t.Fatalf("expected bad literals [API_TOKEN], got: %v", bad)
	}

	// Missing
	sMissing := Server{
		Command: "cmd",
		Env: map[string]string{
			"FOO": "${UNSET_ENV_VAR_12345}",
		},
	}
	miss := sMissing.Missing()
	if len(miss) != 1 || miss[0] != "UNSET_ENV_VAR_12345" {
		t.Fatalf("expected missing [UNSET_ENV_VAR_12345], got: %v", miss)
	}
}

func TestMCPServe(t *testing.T) {
	root := t.TempDir()

	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"scribex_check"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"nonexistent_method"}`,
	}
	in := bytes.NewBufferString(strings.Join(requests, "\n") + "\n")
	var out bytes.Buffer

	if err := Serve(in, &out, root); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d: %s", len(lines), out.String())
	}

	for _, line := range lines {
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("invalid json response: %v, line: %s", err, line)
		}
	}
}
