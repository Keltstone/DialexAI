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
	ds := []docs.Delta{{"a.go", "Login", "changed"}, {"a.go", "Extra", "added"}, {"a.go", "S.Gone", "removed"}}
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
	if !strings.Contains(b.String(), "PROJ-1,5,a.go,Login,changed,README.md,9") {
		t.Fatal(b.String())
	}
}

func TestGithubUpsertUpdatesInPlace(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
		}
		if r.Method == "GET" {
			json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "body": "<!-- scribex-report -->\nold"}})
		} else {
			w.Write([]byte("{}"))
		}
	}))
	defer srv.Close()
	if err := github.Upsert(srv.URL, "tok", "o/r", 3, "hi"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "GET,PATCH" {
		t.Fatal(methods)
	}
}

func TestConfluenceDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["status"] != "draft" {
			w.WriteHeader(400)
		}
		w.Write([]byte(`{"id":"42"}`))
	}))
	defer srv.Close()
	id, err := confluence.Client{Base: srv.URL, User: "u", Token: "t"}.CreateDraft("ENG", "T", confluence.Body(map[string]string{"a#B": "func B() <x>"}))
	if err != nil || id != "42" {
		t.Fatal(id, err)
	}
}

func TestDashboardRBAC(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc P() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "R.md"), []byte("<!-- @scribex:anchor(a.go#P) -->\n<!-- @scribex:end -->\n<!-- @scribex:anchor(a.go#Dead) -->\n<!-- @scribex:end -->\n"), 0o644)
	h := dashboard.Handler(
		[]dashboard.Repo{{"r1", dir, "core"}, {"r2", dir, "web"}},
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
