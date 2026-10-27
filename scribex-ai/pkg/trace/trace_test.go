package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scribex/pkg/docs"
	"scribex/pkg/jira"
	"scribex/pkg/manifest"
)

func TestCellFormulaNeutralisation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"=1+1", "'=1+1"},
		{"+cmd|' /C calc'!A0", "'+cmd|' /C calc'!A0"},
		{"-2+3", "'-2+3"},
		{"@SUM(1)", "'@SUM(1)"},
		{"\tx", "'\tx"},
		{`=HYPERLINK("http://e","x")`, `'=HYPERLINK("http://e","x")`},
		{" =1+1", "' =1+1"},
		{"  +cmd", "'  +cmd"},
		{"   -2+3", "'   -2+3"},
		{"    @SUM", "'    @SUM"},
		{"\r=test", "'\r=test"},
		{"hello\nworld", "hello world"},
		{"hello\r\nworld", "hello world"},
		{"normal", "normal"},
		{"PROJ-123", "PROJ-123"},
		{"", ""},
	}

	for _, tt := range tests {
		got := Cell(tt.input)
		if got != tt.want {
			t.Errorf("Cell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTraceDeterminismAndSorting(t *testing.T) {
	opts := Options{
		Version:           "1.0.0",
		BaseSHA:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:           "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		HeadCommitTimeUTC: "2026-10-27T12:00:00Z",
		Jira:              "PROJ-456",
		JiraVerified:      true,
		PR:                "42",
		Deltas: []docs.Delta{
			{File: "z.go", Sym: "FuncZ", Kind: "added"},
			{File: "a.go", Sym: "Beta", Kind: "modified"},
			{File: "a.go", Sym: "Alpha", Kind: "deleted"},
			{File: "a.go", Sym: "Alpha", Kind: "added"},
		},
		Refs: map[string][]string{
			"a.go#Alpha": {"doc2.md", "doc1.md", "doc2.md"}, // unsorted + duplicates
			"a.go#Beta":  {"docB.md"},
			"z.go#FuncZ": {"docZ.md"},
		},
		Mappings: []manifest.Mapping{
			{Page: "102", Symbols: []string{"a.go#Alpha"}},
			{Page: "101", Symbols: []string{"a.go#Alpha"}},
			{Page: "102", Symbols: []string{"a.go#Alpha"}}, // duplicate
			{Page: "200", Symbols: []string{"z.go#FuncZ"}},
		},
	}

	csvBytes1, rows1, err1 := Generate(opts)
	if err1 != nil {
		t.Fatalf("Generate failed: %v", err1)
	}
	if rows1 != 4 {
		t.Fatalf("expected 4 rows, got %d", rows1)
	}

	csvBytes2, rows2, err2 := Generate(opts)
	if err2 != nil {
		t.Fatalf("Generate 2 failed: %v", err2)
	}
	if rows2 != rows1 {
		t.Fatalf("row count mismatch: %d vs %d", rows2, rows1)
	}

	if !bytes.Equal(csvBytes1, csvBytes2) {
		t.Fatal("Generate is not deterministic across runs")
	}

	lines := strings.Split(strings.TrimSpace(string(csvBytes1)), "\n")
	if len(lines) != 5 { // 1 header + 4 data rows
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Verify header
	expectedHeader := "schema,scribex_version,base_sha,head_sha,head_commit_time_utc,jira,jira_verified,pr,file,symbol,change,repo_docs,confluence_pages"
	if lines[0] != expectedHeader {
		t.Errorf("header = %q, want %q", lines[0], expectedHeader)
	}

	// Verify row sorting by (file, symbol, change):
	// Row 1: a.go, Alpha, added
	// Row 2: a.go, Alpha, deleted
	// Row 3: a.go, Beta, modified
	// Row 4: z.go, FuncZ, added
	if !strings.Contains(lines[1], "a.go,Alpha,added,doc1.md;doc2.md,101;102") {
		t.Errorf("row 1 incorrect: %s", lines[1])
	}
	if !strings.Contains(lines[2], "a.go,Alpha,deleted,doc1.md;doc2.md,101;102") {
		t.Errorf("row 2 incorrect: %s", lines[2])
	}
	if !strings.Contains(lines[3], "a.go,Beta,modified,docB.md,") {
		t.Errorf("row 3 incorrect: %s", lines[3])
	}
	if !strings.Contains(lines[4], "z.go,FuncZ,added,docZ.md,200") {
		t.Errorf("row 4 incorrect: %s", lines[4])
	}
}

func TestZeroDeltasRow(t *testing.T) {
	opts := Options{
		Version:           "1.0.0",
		BaseSHA:           "1111111111111111111111111111111111111111",
		HeadSHA:           "2222222222222222222222222222222222222222",
		HeadCommitTimeUTC: "2026-10-27T10:00:00Z",
		Jira:              "PROJ-999",
		JiraVerified:      false,
		PR:                "10",
		Deltas:            nil, // zero deltas
	}

	csvBytes, rows, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row for zero deltas, got %d", rows)
	}

	lines := strings.Split(strings.TrimSpace(string(csvBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 row), got %d", len(lines))
	}

	wantRow := "scribex-trace/1,1.0.0,1111111111111111111111111111111111111111,2222222222222222222222222222222222222222,2026-10-27T10:00:00Z,PROJ-999,false,10,,,none,,"
	if lines[1] != wantRow {
		t.Errorf("zero-delta row = %q, want %q", lines[1], wantRow)
	}
}

func TestFormulaInEveryColumn(t *testing.T) {
	corpus := []string{
		"=1+1",
		"+cmd|' /C calc'!A0",
		"-2+3",
		"@SUM(1)",
		"\tx",
		`=HYPERLINK("http://e","x")`,
		" =1+1",
	}

	for _, hostile := range corpus {
		opts := Options{
			Version:           hostile,
			BaseSHA:           hostile,
			HeadSHA:           hostile,
			HeadCommitTimeUTC: hostile,
			Jira:              hostile,
			JiraVerified:      true,
			PR:                hostile,
			Deltas: []docs.Delta{
				{File: hostile, Sym: hostile, Kind: hostile},
			},
			Refs: map[string][]string{
				hostile + "#" + hostile: {hostile},
			},
			Mappings: []manifest.Mapping{
				{Page: hostile, Symbols: []string{hostile + "#" + hostile}},
			},
		}

		csvBytes, _, err := Generate(opts)
		if err != nil {
			t.Fatalf("Generate failed on hostile input %q: %v", hostile, err)
		}

		lines := strings.Split(strings.TrimSpace(string(csvBytes)), "\n")
		dataRow := lines[1]
		// In CSV format, verify that neither '=' nor '+' nor '@' nor '-' nor '\t' appear unquoted at the start of any cell
		cells := strings.Split(dataRow, ",")
		for i, c := range cells {
			trimmed := strings.Trim(c, `"`)
			if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "\t") {
				t.Fatalf("cell %d (%q) starts with hostile character in row: %s", i, c, dataRow)
			}
		}
	}
}

func TestJiraVerificationStubs(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")

	// 1. 200 OK -> Issue returns true, nil
	t.Run("Status200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"key":"PROJ-123"}`))
		}))
		defer srv.Close()

		cl := jira.Client{Base: srv.URL, Token: "token123"}
		ok, err := cl.Issue("PROJ-123")
		if err != nil || !ok {
			t.Fatalf("expected ok=true, got ok=%v, err=%v", ok, err)
		}
	})

	// 2. 404 Not Found -> error
	t.Run("Status404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"errorMessages":["Issue Does Not Exist"]}`))
		}))
		defer srv.Close()

		cl := jira.Client{Base: srv.URL, Token: "token123"}
		ok, err := cl.Issue("PROJ-123")
		if err == nil || ok {
			t.Fatalf("expected error on 404, got ok=%v, err=%v", ok, err)
		}
	})

	// 3. 500 Internal Error -> error
	t.Run("Status500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`internal server error`))
		}))
		defer srv.Close()

		cl := jira.Client{Base: srv.URL, Token: "token123"}
		ok, err := cl.Issue("PROJ-123")
		if err == nil || ok {
			t.Fatalf("expected error on 500, got ok=%v, err=%v", ok, err)
		}
	})

	// 4. Key regex strictness
	t.Run("StrictKeyRegex", func(t *testing.T) {
		valid := []string{"PROJ-1", "ABC_XYZ-12345", "A1-999999999"}
		invalid := []string{"", "proj-1", "123-456", "TOOLONGPROJECTNAME-1", "A-", "A-0", "-123", "=PROJ-1", "P-01"}

		for _, k := range valid {
			if !jira.StrictKey(k) {
				t.Errorf("StrictKey(%q) = false, want true", k)
			}
		}
		for _, k := range invalid {
			if jira.StrictKey(k) {
				t.Errorf("StrictKey(%q) = true, want false", k)
			}
		}
	})
}

func TestSidecarMetadataAndAtomicPreservation(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "evidence.csv")
	jsonPath := filepath.Join(dir, "evidence.csv.json")

	// Pre-populate with existing content
	oldCSV := []byte("old,csv,data\n")
	oldJSON := []byte(`{"status":"old"}` + "\n")
	if err := os.WriteFile(csvPath, oldCSV, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, oldJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		Version:           "1.2.3",
		BaseSHA:           "abcdef0123456789abcdef0123456789abcdef01",
		HeadSHA:           "123456789abcdef0123456789abcdef012345678",
		HeadCommitTimeUTC: "2026-10-27T14:30:00Z",
		Jira:              "SEC-42",
		JiraVerified:      true,
		PR:                "99",
		Deltas: []docs.Delta{
			{File: "main.go", Sym: "Run", Kind: "modified"},
		},
	}

	csvBytes, rows, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	meta := Meta{
		Schema:         "scribex-trace-meta/1",
		Status:         "complete",
		Rows:           rows,
		CSVSHA256:      hex.EncodeToString(func() []byte { h := sha256.Sum256(csvBytes); return h[:] }()),
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		ScribexVersion: opts.Version,
		BaseSHA:        opts.BaseSHA,
		HeadSHA:        opts.HeadSHA,
		JiraVerified:   opts.JiraVerified,
		UnknownFiles:   []string{},
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	metaBytes = append(metaBytes, '\n')

	if err := WriteFiles(csvPath, csvBytes, metaBytes); err != nil {
		t.Fatalf("WriteFiles failed: %v", err)
	}

	// Verify written CSV
	readCSV, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readCSV, csvBytes) {
		t.Error("CSV bytes do not match")
	}

	// Verify written sidecar JSON
	readJSON, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var readMeta Meta
	if err := json.Unmarshal(readJSON, &readMeta); err != nil {
		t.Fatal(err)
	}
	if readMeta.Schema != "scribex-trace-meta/1" || readMeta.Status != "complete" || readMeta.Rows != 1 || !readMeta.JiraVerified {
		t.Errorf("unexpected meta content: %+v", readMeta)
	}
	if readMeta.CSVSHA256 != meta.CSVSHA256 {
		t.Errorf("sha mismatch: %s vs %s", readMeta.CSVSHA256, meta.CSVSHA256)
	}
}
