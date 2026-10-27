package server

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sentinix/pkg/advisories"
	"sentinix/pkg/engines"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/policy"
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
	e.st.GrantProjectAccess(p.ID, "dev@example.test") // developers only see granted projects
	body := `{"commit":"c1","ran":["secrets"],"findings":[{"rule":"r","engine":"secrets","file":"a.kt","line":1,"severity":3,"evidence":"x"}]}`
	e.ok(e.call(tok, "", "POST", "/api/projects/"+p.ID+"/ingest", body), 200)
	e.ok(e.call(tok, "", "POST", "/api/projects/other/ingest", body), 403)
	e.ok(e.call(tok, "", "GET", "/api/projects", ""), 403) // tokens cannot list projects

	var fs []store.Finding
	w := e.call("dev@example.test", auth.Developer, "GET", "/api/findings?project="+p.ID, "")
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
	w = e.call("rev1@example.test", auth.Reviewer, "POST", "/api/approvals", `{"kind":"disable_rule","payload":{"rule":"android/debuggable","scope":"*","until":"2029-01-01T00:00:00Z"}}`)
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
	e.st.UpsertUser("own2@example.test", "own2", auth.Owner)
	set := `{"email":"bob@example.test","role":"owner"}`
	e.ok(e.call("adm@example.test", auth.Admin, "PUT", "/api/users/role", set), 403)

	// Owner PUT /api/users/role returns 202 approval_required
	w := e.call("own@example.test", auth.Owner, "PUT", "/api/users/role", set)
	e.ok(w, 202)
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "approval_required" || resp["approval_id"] == "" {
		t.Fatalf("expected approval_required with approval_id, got %v", resp)
	}

	// Proposer cannot approve
	e.ok(e.call("own@example.test", auth.Owner, "POST", "/api/approvals/"+resp["approval_id"]+"/decide", `{"approve":true}`), 409)
	// Target user cannot approve
	e.ok(e.call("bob@example.test", auth.Developer, "POST", "/api/approvals/"+resp["approval_id"]+"/decide", `{"approve":true}`), 403)

	// Second owner approves
	e.ok(e.call("own2@example.test", auth.Owner, "POST", "/api/approvals/"+resp["approval_id"]+"/decide", `{"approve":true}`), 200)
}

func TestOwnerAndAliasCannotSelfApproveDisableRule(t *testing.T) {
	e := setup(t)
	e.st.UpsertUser("dev@example.test", "Developer", auth.Developer)
	e.st.UpsertUser("owner@corp.test", "Owner", auth.Owner)
	e.st.UpsertUser("owner+alias@corp.test", "Owner Alias", auth.Owner)
	e.st.UpsertUser("independent-owner@corp.test", "Independent Owner", auth.Owner)

	// 1. Owner PUT /api/users/role returns approval_required
	roleReq := `{"email":"dev@example.test","role":"owner"}`
	wRole := e.call("owner@corp.test", auth.Owner, "PUT", "/api/users/role", roleReq)
	e.ok(wRole, 202)
	t.Logf("Owner PUT /api/users/role response: status=%d body=%s", wRole.Code, wRole.Body.String())

	// 2. Owner proposes disable_rule
	ruleReq := `{"kind":"disable_rule","payload":{"rule":"sec/dangerous-api","scope":"*","until":"2029-01-01T00:00:00Z"}}`
	wProp := e.call("owner@corp.test", auth.Owner, "POST", "/api/approvals", ruleReq)
	e.ok(wProp, 202)
	var ap map[string]string
	json.Unmarshal(wProp.Body.Bytes(), &ap)
	appID := ap["approval"]

	// 3. Proposing Owner attempts to self-approve -> rejected
	wSelf := e.call("owner@corp.test", auth.Owner, "POST", "/api/approvals/"+appID+"/decide", `{"approve":true}`)
	e.ok(wSelf, 409)
	t.Logf("Proposer self-approval attempt response: status=%d body=%s", wSelf.Code, wSelf.Body.String())

	// 4. Owner alias attempts to approve -> rejected
	wAlias := e.call("owner+alias@corp.test", auth.Owner, "POST", "/api/approvals/"+appID+"/decide", `{"approve":true}`)
	e.ok(wAlias, 409)
	t.Logf("Alias self-approval attempt response: status=%d body=%s", wAlias.Code, wAlias.Body.String())

	// 5. Independent second owner approves -> succeeds
	wSecond := e.call("independent-owner@corp.test", auth.Owner, "POST", "/api/approvals/"+appID+"/decide", `{"approve":true}`)
	e.ok(wSecond, 200)
	t.Logf("Independent second owner approval response: status=%d body=%s", wSecond.Code, wSecond.Body.String())
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

func TestReportFormats(t *testing.T) {
	e := setup(t)
	p, _ := e.st.CreateProject(store.Project{Name: "p", Host: "gh", Repo: "o/r"})
	for _, f := range []string{"csv", "json", "sarif", "sbom", "html"} {
		e.ok(e.call("aud@example.test", auth.Auditor, "GET", "/api/projects/"+p.ID+"/report/"+f, ""), 200)
	}
	e.ok(e.call("aud@example.test", auth.Auditor, "GET", "/api/projects/"+p.ID+"/report/exe", ""), 404)
}

func TestAIRefusedUnlessProjectOptedIn(t *testing.T) {
	e := setup(t)
	p, _ := e.st.CreateProject(store.Project{Name: "p", Host: "gh", Repo: "o/r"})
	e.st.RecordScan(store.Scan{ProjectID: p.ID}, []string{"secrets"}, nil)
	e.st.RecordScan(store.Scan{ProjectID: p.ID, Mode: "ci"}, []string{"secrets"}, []engines.Finding{{Rule: "r", Engine: "secrets", File: "a.kt", Line: 1, Severity: engines.High, Evidence: "x"}})
	fs, _ := e.st.Findings(store.Filter{Project: p.ID})
	w := e.call("rev@example.test", auth.Reviewer, "POST", "/api/findings/"+fs[0].ID+"/ai/triage", "")
	e.ok(w, 409) // no models configured / project not opted in
}

func TestIngestTrust_UnsignedCappedAndRefusesReopen(t *testing.T) {
	e := setup(t)
	p, _ := e.st.CreateProject(store.Project{Name: "p", Host: "gh", Repo: "o/r"})
	tok, _ := e.st.CreateToken(p.ID, "ci")

	// 1. Unsigned ingest with Critical severity
	unsignedBody := `{"commit":"c1","findings":[{"rule":"vuln","engine":"secrets","file":"main.go","line":10,"severity":4,"message":"crit","evidence":"token"}]}`
	req := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest", strings.NewReader(unsignedBody))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("ingest failed: status=%d body=%s", w.Code, w.Body)
	}

	fs, _ := e.st.Findings(store.Filter{Project: p.ID})
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	f := fs[0]
	if f.Severity != engines.Medium {
		t.Errorf("expected unsigned finding severity to be capped at Medium (2), got %v", f.Severity)
	}
	if f.Source != "ci-unattested" {
		t.Errorf("expected source to be ci-unattested, got %q", f.Source)
	}

	// Unattested findings must not fail the gate
	if policy.Fails([]engines.Finding{f.Finding}, engines.Medium) {
		t.Errorf("expected unattested finding to be excluded from gate, but policy.Fails returned true")
	}

	// Unattested findings must be excluded from AI context
	wAI := e.call("rev@example.test", auth.Reviewer, "POST", "/api/findings/"+f.ID+"/ai/triage", "")
	if wAI.Code != 400 {
		t.Errorf("expected AI triage on unattested finding to fail with 400, got %d", wAI.Code)
	}

	// 2. Mark finding as fixed
	if _, err := e.st.DB().Exec("update findings set status='fixed' where id=$1", f.ID); err != nil {
		t.Fatal(err)
	}

	// 3. Post unsigned ingest again - must refuse to reopen fixed finding
	req = httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest", strings.NewReader(unsignedBody))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("ingest failed: status=%d body=%s", w.Code, w.Body)
	}

	fsAfter, _ := e.st.Findings(store.Filter{Project: p.ID})
	if len(fsAfter) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fsAfter))
	}
	if fsAfter[0].Status != "fixed" {
		t.Errorf("expected unattested finding to remain fixed, but got status %q", fsAfter[0].Status)
	}

	// 4. Correctly signed ingest with ed25519 signature keeps Critical severity and reopens
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	e.app.TrustedKeys = []ed25519.PublicKey{pub}

	signedBody := `{"commit":"c2","findings":[{"rule":"vuln","engine":"secrets","file":"main.go","line":10,"severity":4,"message":"crit","evidence":"token"}]}`
	ts := fmt.Sprintf("%d", time.Now().Unix())
	bodyHash := sha256.Sum256([]byte(signedBody))
	msg := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%x", p.ID, "c2", "", "secrets", "findings", ts, bodyHash)
	sig := ed25519.Sign(priv, []byte(msg))
	sigHeader := base64.StdEncoding.EncodeToString(sig)

	req = httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest", strings.NewReader(signedBody))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Signature", sigHeader)
	req.Header.Set("X-Signature-Timestamp", ts)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("signed ingest failed: status=%d body=%s", w.Code, w.Body)
	}

	fsSigned, _ := e.st.Findings(store.Filter{Project: p.ID})
	if len(fsSigned) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fsSigned))
	}
	if fsSigned[0].Severity != engines.Critical {
		t.Errorf("expected signed finding to retain Critical severity, got %v", fsSigned[0].Severity)
	}
	if fsSigned[0].Status != "open" {
		t.Errorf("expected signed finding to reopen, got status %q", fsSigned[0].Status)
	}
}

func TestHuntServerAPIEndpoints(t *testing.T) {
	e := setup(t)
	p, err := e.st.CreateProject(store.Project{Name: "hunt-project", Repo: "keltstone/hunt"})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Trigger hunt run
	w := e.call("reviewer@example.test", auth.Reviewer, "POST", "/api/projects/"+p.ID+"/hunt/run", "")
	e.ok(w, 200)

	var run store.HuntRun
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatalf("failed to decode hunt run: %v", err)
	}
	if run.ProjectID != p.ID || run.Status != "running" {
		t.Errorf("unexpected run created: %+v", run)
	}

	// 2. List hunt runs
	wList := e.call("admin@example.test", auth.Admin, "GET", "/api/projects/"+p.ID+"/hunt/runs", "")
	e.ok(wList, 200)
	var runs []store.HuntRun
	if err := json.Unmarshal(wList.Body.Bytes(), &runs); err != nil {
		t.Fatalf("failed to decode hunt runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Errorf("expected 1 run in list, got %+v", runs)
	}

	// 3. Get hunt run details
	wDetail := e.call("admin@example.test", auth.Admin, "GET", "/api/projects/"+p.ID+"/hunt/runs/"+run.ID, "")
	e.ok(wDetail, 200)
	var detail map[string]any
	if err := json.Unmarshal(wDetail.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode run detail: %v", err)
	}
	if detail["run"] == nil {
		t.Errorf("missing run in detail response: %+v", detail)
	}
}


