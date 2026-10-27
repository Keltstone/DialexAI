package hunt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sentinix/pkg/store"
)

func TestToolsComprehensive(t *testing.T) {
	tmp := t.TempDir()

	// 1. Setup sample files in scan root
	subDir := filepath.Join(tmp, "src")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `<manifest><application><activity android:name=".MainActivity" android:exported="true"/></application></manifest>`
	if err := os.WriteFile(filepath.Join(tmp, "AndroidManifest.xml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	goCode := `package main

type UserService struct {}

func (s *UserService) Authenticate(token string) bool {
    return token == "secret"
}

func main() {
    srv := &UserService{}
    srv.Authenticate("test")
}
`
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte(goCode), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Setup tools instance
	tl := &tools{
		root:   tmp,
		policy: "local-only",
		eps: []Entry{
			{Kinds: []string{"route"}, Name: "Authenticate", File: "src/main.go"},
		},
		cfg: &HuntConfig{
			Tools: ToolsConfig{
				Enabled:      true,
				StepCap:      5,
				SizeCapBytes: 5000,
			},
		},
	}

	ctx := context.Background()

	// Test read_file
	rf := tl.run(ctx, toolCall{Tool: "read_file", Path: "src/main.go", Start: 1, End: 5})
	if !strings.Contains(rf, "package main") || !strings.Contains(rf, "UserService struct") {
		t.Errorf("read_file failed, got: %s", rf)
	}

	// Test read_file path traversal prevention (must fail or error)
	rfEsc := tl.run(ctx, toolCall{Tool: "read_file", Path: "../../etc/passwd", Start: 1, End: 10})
	if !strings.Contains(rfEsc, "error") {
		t.Errorf("expected error escaping scan root, got: %s", rfEsc)
	}

	// Test list_dir
	ld := tl.run(ctx, toolCall{Tool: "list_dir", Path: "."})
	if !strings.Contains(ld, "[file] AndroidManifest.xml") || !strings.Contains(ld, "[dir]  src") {
		t.Errorf("list_dir failed, got: %s", ld)
	}

	// Test grep
	gp := tl.run(ctx, toolCall{Tool: "grep", Pattern: "Authenticate"})
	if !strings.Contains(gp, "src/main.go") || !strings.Contains(gp, "Authenticate") {
		t.Errorf("grep failed, got: %s", gp)
	}

	// Test read_symbol
	rs := tl.run(ctx, toolCall{Tool: "read_symbol", Symbol: "Authenticate", Path: "src/main.go"})
	if !strings.Contains(rs, "Authenticate(token string)") {
		t.Errorf("read_symbol failed, got: %s", rs)
	}

	// Test find_references
	fr := tl.run(ctx, toolCall{Tool: "find_references", Symbol: "UserService"})
	if !strings.Contains(fr, "src/main.go:3") || !strings.Contains(fr, "src/main.go:10") {
		t.Errorf("find_references failed, got: %s", fr)
	}

	// Test callers
	cl := tl.run(ctx, toolCall{Tool: "callers", Symbol: "Authenticate"})
	if !strings.Contains(cl, "src/main.go:11") {
		t.Errorf("callers failed, got: %s", cl)
	}

	// Test manifest_query
	mq := tl.run(ctx, toolCall{Tool: "manifest_query", Tag: "activity", Attr: "exported"})
	if !strings.Contains(mq, "AndroidManifest.xml") || !strings.Contains(mq, "MainActivity") {
		t.Errorf("manifest_query failed, got: %s", mq)
	}

	// Test semgrep_file
	sf := tl.run(ctx, toolCall{Tool: "semgrep_file", Path: "src/main.go", Rule: `token == "secret"`})
	if !strings.Contains(sf, "src/main.go:6") {
		t.Errorf("semgrep_file failed, got: %s", sf)
	}

	// Test codeql_query when DB not present
	cq := tl.run(ctx, toolCall{Tool: "codeql_query", Query: "from Function f select f"})
	if !strings.Contains(cq, "codeql database not found") {
		t.Errorf("expected missing codeql database message, got: %s", cq)
	}

	// Test entrypoints
	ep := tl.run(ctx, toolCall{Tool: "entrypoints"})
	if !strings.Contains(ep, "Authenticate") {
		t.Errorf("entrypoints failed, got: %s", ep)
	}
}

func TestToolsAllowlistAndSizeCap(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "long.txt")
	longContent := strings.Repeat("A very long line of text that exceeds small caps.\n", 20)
	if err := os.WriteFile(filePath, []byte(longContent), 0644); err != nil {
		t.Fatal(err)
	}

	tl := &tools{
		root: tmp,
		cfg: &HuntConfig{
			Tools: ToolsConfig{
				Enabled:      true,
				Allowlist:    []string{"read_file"},
				SizeCapBytes: 50,
			},
		},
	}

	ctx := context.Background()

	// Allowed tool with size cap
	rf := tl.run(ctx, toolCall{Tool: "read_file", Path: "long.txt", Start: 1, End: 10})
	if len(rf) > 50 {
		t.Errorf("expected size capped at 50, got %d bytes", len(rf))
	}

	// Disallowed tool
	ld := tl.run(ctx, toolCall{Tool: "list_dir", Path: "."})
	if !strings.Contains(ld, "not allowed by policy") {
		t.Errorf("expected allowlist rejection, got: %s", ld)
	}
}

func TestToolsAuditLogging(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	runID := "test-run-1"
	if err := st.CreateHuntRun(ctx, store.HuntRun{ID: runID, Status: "running"}); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "sample.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	tl := &tools{
		root:   tmp,
		store:  st,
		runID:  runID,
		taskID: "task-001",
	}

	_ = tl.run(ctx, toolCall{Tool: "read_file", Path: "sample.txt", Start: 1, End: 5})

	calls, err := st.GetHuntRunToolCalls(ctx, runID)
	if err != nil {
		t.Fatalf("failed to query tool calls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 logged tool call, got %d", len(calls))
	}
	if calls[0].Tool != "read_file" || calls[0].TaskID != "task-001" {
		t.Errorf("unexpected logged tool call: %+v", calls[0])
	}
}
