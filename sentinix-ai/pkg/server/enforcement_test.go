package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

type fakeHost struct {
	state   githost.State
	desc    string
	comment string
}

func (f *fakeHost) Name() string                                        { return "fake" }
func (f *fakeHost) Parse(*http.Request, []byte) (*githost.Event, error) { return nil, nil }
func (f *fakeHost) CloneAuth() (string, string)                         { return "", "" }
func (f *fakeHost) RepoURL(p string) string                             { return p }
func (f *fakeHost) ConfiguredHost() string                              { return "fake.example" }
func (f *fakeHost) Status(_ context.Context, _ *githost.Event, s githost.State, desc string) error {
	f.state = s
	f.desc = desc
	return nil
}
func (f *fakeHost) Comment(_ context.Context, _ *githost.Event, b string) error {
	f.comment = b
	return nil
}

func review(t *testing.T, a *App, sev engines.Severity) *fakeHost {
	h := &fakeHost{state: -1}
	res := &scheduler.Result{New: []engines.Finding{{Rule: "r", File: "a.kt", Line: 1, Severity: sev}}, Ran: []string{"secrets"}}
	a.finishReview(context.Background(), h, &githost.Event{RepoPath: "o/r", PR: "1"}, res, time.Second)
	return h
}

func TestBlockOrReportPerProject(t *testing.T) {
	e := setup(t)
	e.app.FailOn = engines.High
	e.app.Enforcement = "block"

	// unregistered repo: server defaults apply
	if h := review(t, e.app, engines.High); h.state != githost.Failure || !strings.Contains(h.comment, "blocked") {
		t.Fatalf("default block: %v %s", h.state, h.comment)
	}
	if h := review(t, e.app, engines.Medium); h.state != githost.Success {
		t.Fatal("below threshold must pass")
	}

	p, _ := e.st.CreateProject(store.Project{Name: "p", Host: "fake", Repo: "o/r"})
	if p.Enforcement != "block" {
		t.Fatalf("new projects block by default, got %q", p.Enforcement)
	}
	// per-project report mode: findings are posted but never block
	p.Enforcement = "report"
	if err := e.st.UpdateProject(p); err != nil {
		t.Fatal(err)
	}
	if h := review(t, e.app, engines.Critical); h.state != githost.Success || !strings.Contains(h.comment, "do not block") {
		t.Fatalf("report mode must not fail the status: %v %s", h.state, h.comment)
	}
	// per-project threshold
	p.Enforcement = "block"
	p.FailOn = "medium"
	if err := e.st.UpdateProject(p); err != nil {
		t.Fatal(err)
	}
	if h := review(t, e.app, engines.Medium); h.state != githost.Failure {
		t.Fatal("project threshold ignored")
	}
	e.ok(e.call("adm@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID, `{"enforcement":"maybe"}`), 400)
	e.ok(e.call("adm@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID, `{"fail_on":"huge"}`), 400)
	e.ok(e.call("rev@example.test", auth.Reviewer, "PATCH", "/api/projects/"+p.ID, `{"enforcement":"report"}`), 403)
}

func TestCleanPRInBlockModeYieldsSuccessWithRequiredEngines(t *testing.T) {
	e := setup(t)
	required := []string{"packs", "secrets", "osv-scanner", "semgrep", "gitleaks"}
	e.app.RequiredEngines = required
	e.app.Enforcement = "block"
	e.app.FailOn = engines.High

	h := &fakeHost{state: -1}
	res := &scheduler.Result{
		New:     nil, // clean PR
		Ran:     required,
		Skipped: nil,
		Commit:  "1111111111111111111111111111111111111111",
	}

	e.app.finishReview(context.Background(), h, &githost.Event{RepoPath: "o/r", PR: "10"}, res, time.Second)

	if h.state != githost.Success {
		t.Fatalf("expected Success status in block mode for clean PR, got: %v (desc: %s)", h.state, h.desc)
	}
	t.Logf("Clean PR in block mode status: %v (desc: %q)", h.state, h.desc)
	t.Logf("Ran lists all required engines: %v", res.Ran)
}
