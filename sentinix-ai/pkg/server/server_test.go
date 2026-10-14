package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sentinix/pkg/advisories"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/store"
)

type env struct {
	t   *testing.T
	app *App
	h   http.Handler
	st  *store.Store
}

func setup(t *testing.T) *env {
	st, err := store.Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st, Sess: auth.Sessions{Key: []byte("test")}}
	return &env{t, a, a.Handler(), st}
}

// call runs a request as role (cookie session) or with a bearer token when role starts with "sx_".
func (e *env) call(who, role, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	switch {
	case strings.HasPrefix(who, "sx_"):
		r.Header.Set("Authorization", "Bearer "+who)
	case who != "":
		if _, err := e.st.UpsertUser(who, who, role); err != nil {
			e.t.Fatal(err)
		}
		w := httptest.NewRecorder()
		e.app.Sess.Issue(w, who, time.Hour)
		r.AddCookie(w.Result().Cookies()[0])
		if method != "GET" {
			r.Header.Set("X-Requested-With", "sentinix")
		}
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *env) ok(w *httptest.ResponseRecorder, want int) {
	e.t.Helper()
	if w.Code != want {
		e.t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body)
	}
}

func TestAuthAndApprovalFlow(t *testing.T) {
	e := setup(t)
	e.ok(e.call("", "", "GET", "/api/projects", ""), 401)
	e.ok(e.call("dev@example.test", auth.Developer, "GET", "/api/projects", ""), 200)

	p, _ := e.st.CreateProject(store.Project{Name: "p", Host: "gh", Repo: "o/r"})
	tok, _ := e.st.CreateToken(p.ID, "ci")
	body := `{"commit":"c1","ran":["secrets"],"findings":[{"rule":"r","engine":"secrets","file":"a.kt","line":1,"severity":3,"evidence":"x"}]}`
	e.ok(e.call(tok, "", "POST", "/api/projects/"+p.ID+"/ingest", body), 200)
	e.ok(e.call(tok, "", "POST", "/api/projects/other/ingest", body), 403)
	e.ok(e.call(tok, "", "GET", "/api/projects", ""), 403) // tokens cannot list projects

	var fs []store.Finding
	w := e.call("dev@example.test", auth.Developer, "GET", "/api/findings", "")
	json.Unmarshal(w.Body.Bytes(), &fs)
	if len(fs) != 1 {
		t.Fatalf("findings: %s", w.Body)
	}
	url := "/api/findings/" + fs[0].ID + "/triage"
	until := time.Now().Add(48 * time.Hour).Unix()
	acc := `{"status":"accepted","reason":"legacy","until":` + itoa(until) + `}`

	e.ok(e.call("dev@example.test", auth.Developer, "POST", url, acc), 403)
	// CSRF: a cookie POST without the header is refused
	e.st.UpsertUser("rev1@example.test", "rev1", auth.Reviewer)
	r := httptest.NewRequest("POST", url, strings.NewReader(acc))
	c := httptest.NewRecorder()
	e.app.Sess.Issue(c, "rev1@example.test", time.Hour)
	r.AddCookie(c.Result().Cookies()[0])
	rw := httptest.NewRecorder()
	e.h.ServeHTTP(rw, r)
	e.ok(rw, 403)

	w = e.call("rev1@example.test", auth.Reviewer, "POST", url, acc)
	e.ok(w, 202)
	var ap map[string]string
	json.Unmarshal(w.Body.Bytes(), &ap)
	e.ok(e.call("rev1@example.test", auth.Reviewer, "POST", "/api/approvals/"+ap["approval"]+"/decide", `{"approve":true}`), 409)
	e.ok(e.call("rev2@example.test", auth.Reviewer, "POST", "/api/approvals/"+ap["approval"]+"/decide", `{"approve":true}`), 200)
	got, _ := e.st.Finding(fs[0].ID)
	if got.Status != "accepted" {
		t.Fatalf("status %s", got.Status)
	}

	// disabling a rule also needs a second person and lands in the audit log
	w = e.call("rev1@example.test", auth.Reviewer, "POST", "/api/approvals", `{"kind":"disable_rule","payload":{"rule":"android/debuggable"}}`)
	e.ok(w, 202)
	json.Unmarshal(w.Body.Bytes(), &ap)
	e.ok(e.call("rev2@example.test", auth.Reviewer, "POST", "/api/approvals/"+ap["approval"]+"/decide", `{"approve":true}`), 200)
	if dis, _, _ := e.st.RuleOverrides(); !dis["android/debuggable"] {
		t.Fatal("override not applied")
	}
	e.ok(e.call("dev@example.test", auth.Developer, "GET", "/api/audit", ""), 403)
	e.ok(e.call("aud@example.test", auth.Auditor, "GET", "/api/audit", ""), 200)
	e.ok(e.call("rev1@example.test", auth.Reviewer, "GET", "/api/posture", ""), 200)
}

func TestOnlyOwnerGrantsOwner(t *testing.T) {
	e := setup(t)
	e.st.UpsertUser("bob@example.test", "bob", auth.Developer)
	set := `{"email":"bob@example.test","role":"owner"}`
	e.ok(e.call("adm@example.test", auth.Admin, "PUT", "/api/users/role", set), 403)
	e.ok(e.call("own@example.test", auth.Owner, "PUT", "/api/users/role", set), 204)
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestAdvisoryRescanOpensThenFixes(t *testing.T) {
	vulnerable := true
	osv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/querybatch" {
			if vulnerable {
				w.Write([]byte(`{"results":[{"vulns":[{"id":"GHSA-9"}]}]}`))
			} else {
				w.Write([]byte(`{"results":[{}]}`))
			}
			return
		}
		w.Write([]byte(`{"id":"GHSA-9","summary":"s","database_specific":{"severity":"HIGH"}}`))
	}))
	defer osv.Close()
	e := setup(t)
	e.app.OSV = &advisories.Client{URL: osv.URL}
	p, _ := e.st.CreateProject(store.Project{Name: "p", Host: "gh", Repo: "o/r"})
	e.st.ReplaceComponents(p.ID, []advisories.Component{{Ecosystem: "PyPI", Name: "flask", Version: "2.0.0", File: "requirements.txt"}})
	if err := e.app.RescanAdvisories(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fs, _ := e.st.Findings(store.Filter{Project: p.ID, Status: "open"}); len(fs) != 1 || fs[0].Rule != "advisory/GHSA-9" {
		t.Fatalf("%+v", fs)
	}
	vulnerable = false
	e.app.RescanAdvisories(context.Background())
	if fs, _ := e.st.Findings(store.Filter{Project: p.ID, Status: "fixed"}); len(fs) != 1 {
		t.Fatalf("not fixed: %+v", fs)
	}
}
