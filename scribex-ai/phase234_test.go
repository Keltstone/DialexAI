package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scribex/pkg/confluence"
	"scribex/pkg/dashboard"
	"scribex/pkg/docs"
	"scribex/pkg/github"
	"scribex/pkg/jira"
	"scribex/pkg/manifest"
	"scribex/pkg/trace"
)

func TestJiraAndTrace(t *testing.T) {
	ds := []docs.Delta{
		{File: "a.go", Sym: "Login", Kind: "changed"},
		{File: "a.go", Sym: "Extra", Kind: "added"},
		{File: "a.go", Sym: "S.Gone", Kind: "removed"},
	}
	f := jira.Check("Change Login to take ctx", ds)
	if len(f) != 2 || !strings.Contains(f[0], "scope creep") || !strings.Contains(f[1], "breaking") {
		t.Fatalf("%v", f)
	}
	if jira.Key("feature/PROJ-123-x") != "PROJ-123" {
		t.Fatal("key")
	}
	ms := manifest.Parse("mappings:\n  - page: \"9\"\n    space: ENG\n    symbols: [a.go#Login, b.go#X]\n")
	var b strings.Builder
	trace.Write(&b, "PROJ-1", "5", ds[:1], map[string][]string{"a.go#Login": {"README.md"}}, ms)
	if !strings.Contains(b.String(), "PROJ-1,false,5,a.go,Login,modified,README.md,9") {
		t.Fatal(b.String())
	}
}

func TestGithubUpsertUpdatesInPlace(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "bot")
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
		}
		if r.Method == "GET" {
			json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "body": "<!-- scribex-report -->\nold", "user": map[string]any{"login": "bot", "type": "Bot"}}})
		} else {
			w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()
	if _, err := github.Upsert(srv.URL, "tok", "o/r", 3, "hi"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "GET,PATCH" {
		t.Fatal(methods)
	}
}

func TestConfluenceDraft(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["status"] != "draft" {
			w.WriteHeader(400)
		}
		w.Write([]byte(`{"id":"42"}`))
	}))
	defer srv.Close()
	body, err := confluence.Body(map[string]string{"a#B": "func B() <x>"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := confluence.Client{Base: srv.URL, User: "u", Token: "t"}.CreateDraft("ENG", "T", body)
	if err != nil || id != "42" {
		t.Fatal(id, err)
	}
}

func TestDashboardRBAC(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc P() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "R.md"), []byte("<!-- @scribex:anchor(a.go#P) -->\n<!-- @scribex:end -->\n<!-- @scribex:anchor(a.go#Dead) -->\n<!-- @scribex:end -->\n"), 0o644)
	h := dashboard.Handler(
		[]dashboard.Repo{
			{Name: "r1", Path: dir, Team: "core"},
			{Name: "r2", Path: dir, Team: "web"},
		},
		[]dashboard.Principal{{Hash: dashboard.HashToken("adm"), Admin: true}, {Hash: dashboard.HashToken("v"), Teams: map[string]bool{"core": true}}}, dir)
	get := func(tok string) (int, []dashboard.Health) {
		r := httptest.NewRequest("GET", "/api/health", nil)
		if tok != "" {
			r.SetBasicAuth("x", tok)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var hs []dashboard.Health
		json.NewDecoder(w.Body).Decode(&hs)
		return w.Code, hs
	}
	if c, _ := get(""); c != 401 {
		t.Fatal("anon", c)
	}
	if _, hs := get("adm"); len(hs) != 2 || hs[0].Stale != 1 || hs[0].Dead != 1 || hs[0].Drift != 1 {
		t.Fatalf("%+v", hs)
	}
	if _, hs := get("v"); len(hs) != 1 || hs[0].Team != "core" {
		t.Fatalf("%+v", hs)
	}
}
