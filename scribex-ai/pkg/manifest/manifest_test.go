package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := []Mapping{
		{
			Page:    "12345",
			Space:   "ENG",
			Title:   "Auth API",
			Symbols: []string{"pkg/auth/jwt.go#VerifyToken"},
		},
		{
			Page:    "67890",
			Space:   "~personal_123",
			Title:   "Unicode Title 🚀",
			Symbols: []string{"pkg/a/b.go#FuncA", "pkg/c/d.go#FuncB"},
		},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate(valid) failed: %v", err)
	}

	tests := []struct {
		name      string
		ms        []Mapping
		wantEntry string
		wantField string
	}{
		{
			name: "missing page",
			ms: []Mapping{
				{Space: "ENG", Title: "Title", Symbols: []string{"a.go#A"}},
			},
			wantEntry: "entry 1",
			wantField: "field page",
		},
		{
			name: "invalid page non-numeric",
			ms: []Mapping{
				{Page: "../../../pwned", Space: "ENG", Title: "Title", Symbols: []string{"a.go#A"}},
			},
			wantEntry: "entry 1",
			wantField: "field page",
		},
		{
			name: "duplicate page",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: "Title 1", Symbols: []string{"a.go#A"}},
				{Page: "123", Space: "ENG", Title: "Title 2", Symbols: []string{"b.go#B"}},
			},
			wantEntry: "entry 2",
			wantField: "field page",
		},
		{
			name: "invalid space",
			ms: []Mapping{
				{Page: "123", Space: "!INVALID", Title: "Title", Symbols: []string{"a.go#A"}},
			},
			wantEntry: "entry 1",
			wantField: "field space",
		},
		{
			name: "empty title",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: "", Symbols: []string{"a.go#A"}},
			},
			wantEntry: "entry 1",
			wantField: "field title",
		},
		{
			name: "title too long",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: strings.Repeat("A", 256), Symbols: []string{"a.go#A"}},
			},
			wantEntry: "entry 1",
			wantField: "field title",
		},
		{
			name: "title control character",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: "Title\nwith newline", Symbols: []string{"a.go#A"}},
			},
			wantEntry: "entry 1",
			wantField: "field title",
		},
		{
			name: "symbols no hash",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: "Title", Symbols: []string{"pkg/auth/jwt.go"}},
			},
			wantEntry: "entry 1",
			wantField: "field symbols",
		},
		{
			name: "symbols multiple hashes",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: "Title", Symbols: []string{"a.go#B#C"}},
			},
			wantEntry: "entry 1",
			wantField: "field symbols",
		},
		{
			name: "symbols path escape",
			ms: []Mapping{
				{Page: "123", Space: "ENG", Title: "Title", Symbols: []string{"../../etc/passwd#Secret"}},
			},
			wantEntry: "entry 1",
			wantField: "field symbols",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.ms)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantEntry) {
				t.Errorf("error %q should mention %q", msg, tc.wantEntry)
			}
			if !strings.Contains(msg, tc.wantField) {
				t.Errorf("error %q should mention %q", msg, tc.wantField)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")

	// Pwned path in page:
	badYAML := `
mappings:
  - page: "../../../pwned"
    space: ENG
    title: Pwned Page
    symbols: [pkg/a.go#A]
`
	if err := os.WriteFile(path, []byte(badYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load with bad page id should fail")
	}
	if !strings.Contains(err.Error(), "entry 1") || !strings.Contains(err.Error(), "field page") {
		t.Errorf("expected error mentioning entry 1 and field page, got: %v", err)
	}

	// Valid YAML with comments and unknown keys
	goodYAML := `
# Confluence mappings
unknown_top_level: true
mappings:
  - page: "12345"
    unknown_key: "ignored"
    space: ENG
    title: Auth API
    symbols: [pkg/auth/jwt.go#VerifyToken, pkg/auth/jwt.go#SignToken]
`
	if err := os.WriteFile(path, []byte(goodYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed on valid yaml: %v", err)
	}
	if len(ms) != 1 || ms[0].Page != "12345" || len(ms[0].Symbols) != 2 {
		t.Fatalf("unexpected mappings: %+v", ms)
	}
}
