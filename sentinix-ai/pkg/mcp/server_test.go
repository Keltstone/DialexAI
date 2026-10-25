package mcp

import (
	"bytes"
	"strings"
	"testing"
)

func TestServeTools(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sentinix_entrypoints"}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}
`)
	var out bytes.Buffer
	if err := Serve(in, &out, "../../bench/hunt-android"); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{`"name":"sentinix"`, "sentinix_scan", "MapActivity", `"isError":true`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %s", want, s)
		}
	}
	if strings.Count(s, "\n") != 4 {
		t.Fatalf("notification must get no reply: %s", s)
	}
}
