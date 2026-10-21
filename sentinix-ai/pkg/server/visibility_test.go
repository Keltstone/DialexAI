package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

func TestVisibility_DeveloperWithoutGrantGets404(t *testing.T) {
	e := setup(t)

	// Create two projects
	projA, err := e.st.CreateProject(store.Project{Name: "project-a", Host: "github", Repo: "org/a", Branch: "main"})
	if err != nil {
		t.Fatalf("create projA: %v", err)
	}
	projB, err := e.st.CreateProject(store.Project{Name: "project-b", Host: "github", Repo: "org/b", Branch: "main"})
	if err != nil {
		t.Fatalf("create projB: %v", err)
	}

	// Add findings, scan, sbom component, and surface to projB
	fB := engines.Finding{
		Rule:     "sec/test-rule",
		Engine:   "secrets",
		File:     "pkg/b.go",
		Line:     10,
		Severity: engines.High,
		Message:  "sensitive data in project B",
	}
	scB, err := e.st.RecordScan(store.Scan{ProjectID: projB.ID, Mode: "branch", Commit: "bbb111"}, []string{"secrets"}, []engines.Finding{fB})
	if err != nil {
		t.Fatalf("record scan projB: %v", err)
	}
	fsB, err := e.st.Findings(store.Filter{Project: projB.ID})
	if err != nil || len(fsB) == 0 {
		t.Fatalf("expected findings in projB, got %v (err=%v)", fsB, err)
	}
	findingBID := fsB[0].ID

	// Add component for SBOM
	_, _ = e.st.DB().Exec("insert into components(project_id, ecosystem, name, version, file) values($1, 'Go', 'b-lib', '1.0.0', 'go.mod')", projB.ID)

	// Developer has grant only for project A
	devEmail := "dev-alice@example.test"
	e.st.UpsertUser(devEmail, "Alice", auth.Developer)
	if err := e.st.GrantProjectAccess(projA.ID, devEmail); err != nil {
		t.Fatalf("grant project A access: %v", err)
	}

	// Table-driven test: developer without a project grant receives 404 on all read endpoints for project B
	endpoints := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "findings",
			method: "GET",
			path:   fmt.Sprintf("/api/findings?project=%s", projB.ID),
		},
		{
			name:   "finding detail",
			method: "GET",
			path:   fmt.Sprintf("/api/findings/%s", findingBID),
		},
		{
			name:   "scans",
			method: "GET",
			path:   fmt.Sprintf("/api/projects/%s/scans", projB.ID),
		},
		{
			name:   "SBOM",
			method: "GET",
			path:   fmt.Sprintf("/api/projects/%s/sbom", projB.ID),
		},
		{
			name:   "surface",
			method: "GET",
			path:   fmt.Sprintf("/api/projects/%s/surface", projB.ID),
		},
		{
			name:   "reports (html)",
			method: "GET",
			path:   fmt.Sprintf("/api/projects/%s/report/html", projB.ID),
		},
		{
			name:   "reports (json)",
			method: "GET",
			path:   fmt.Sprintf("/api/projects/%s/report/json", projB.ID),
		},
	}

	for _, tc := range endpoints {
		t.Run(tc.name+"_refused_404_without_grant", func(t *testing.T) {
			w := e.call(devEmail, auth.Developer, tc.method, tc.path, "")
			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404 for %s %s without grant, got status %d (body: %s)", tc.method, tc.path, w.Code, w.Body.String())
			}
		})
	}

	// When developer is granted access to project B, endpoints succeed (not 404)
	if err := e.st.GrantProjectAccess(projB.ID, devEmail); err != nil {
		t.Fatalf("grant project B access: %v", err)
	}
	for _, tc := range endpoints {
		t.Run(tc.name+"_allowed_with_grant", func(t *testing.T) {
			w := e.call(devEmail, auth.Developer, tc.method, tc.path, "")
			if w.Code == http.StatusNotFound {
				t.Fatalf("expected non-404 for %s %s with grant, got 404", tc.method, tc.path)
			}
		})
	}

	_ = scB
}

func TestDeveloperCannotListAllFindingsAcrossProjects(t *testing.T) {
	e := setup(t)

	// Create Project A and Project B
	projA, err := e.st.CreateProject(store.Project{Name: "project-a", Host: "github", Repo: "corp/a", Branch: "main"})
	if err != nil {
		t.Fatalf("create projA: %v", err)
	}
	projB, err := e.st.CreateProject(store.Project{Name: "project-b", Host: "github", Repo: "corp/b", Branch: "main"})
	if err != nil {
		t.Fatalf("create projB: %v", err)
	}

	// Add findings to Project A and Project B
	fA := engines.Finding{
		Rule:     "aws-access-token",
		Engine:   "secrets",
		File:     "auth/aws.go",
		Line:     12,
		Severity: engines.Critical,
		Message:  "AWS secret key leaked in project A",
	}
	if _, err := e.st.RecordScan(store.Scan{ProjectID: projA.ID, Mode: "branch", Commit: "aaa111"}, []string{"secrets"}, []engines.Finding{fA}); err != nil {
		t.Fatalf("record scan projA: %v", err)
	}

	fB := engines.Finding{
		Rule:     "stripe-access-token",
		Engine:   "secrets",
		File:     "billing/stripe.go",
		Line:     45,
		Severity: engines.Critical,
		Message:  "sk_live_123456789012345678901234",
	}
	if _, err := e.st.RecordScan(store.Scan{ProjectID: projB.ID, Mode: "branch", Commit: "bbb222"}, []string{"secrets"}, []engines.Finding{fB}); err != nil {
		t.Fatalf("record scan projB: %v", err)
	}

	// Grant dev@corp.test access only to Project A
	devEmail := "dev@corp.test"
	e.st.UpsertUser(devEmail, "Developer", auth.Developer)
	if err := e.st.GrantProjectAccess(projA.ID, devEmail); err != nil {
		t.Fatalf("grant project A: %v", err)
	}

	// Make a GET /api/findings request (without ?project=) as dev@corp.test
	// Assert that the response is NOT 200 OK (expect 400 or 403)
	wUnscoped := e.call(devEmail, auth.Developer, "GET", "/api/findings", "")
	if wUnscoped.Code == http.StatusOK {
		t.Fatalf("expected non-200 (400 Bad Request or 403 Forbidden) without project param, got 200 OK: %s", wUnscoped.Body.String())
	}
	if wUnscoped.Code != http.StatusBadRequest && wUnscoped.Code != http.StatusForbidden {
		t.Fatalf("expected 400 or 403 when project param is missing for developer, got %d", wUnscoped.Code)
	}
	t.Logf("Developer GET /api/findings without project param response: status=%d %s (body=%q)", wUnscoped.Code, http.StatusText(wUnscoped.Code), strings.TrimSpace(wUnscoped.Body.String()))

	// Make a GET /api/findings?project=<Project_B_ID> as dev@corp.test
	// Assert that the response is 404 Not Found or 403 Forbidden
	wB := e.call(devEmail, auth.Developer, "GET", "/api/findings?project="+projB.ID, "")
	if wB.Code != http.StatusNotFound && wB.Code != http.StatusForbidden {
		t.Fatalf("expected 404 Not Found or 403 Forbidden for Project B, got %d (body: %s)", wB.Code, wB.Body.String())
	}
	t.Logf("Developer GET /api/findings?project=%s response: status=%d %s", projB.ID, wB.Code, http.StatusText(wB.Code))

	// Make a GET /api/findings?project=<Project_A_ID> as dev@corp.test
	// Assert that the response is 200 OK and contains only Project A findings
	wA := e.call(devEmail, auth.Developer, "GET", "/api/findings?project="+projA.ID, "")
	if wA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Project A, got %d (body: %s)", wA.Code, wA.Body.String())
	}
	bodyA := wA.Body.String()
	if !strings.Contains(bodyA, "aws-access-token") {
		t.Fatalf("expected Project A findings in response, got: %s", bodyA)
	}
	if strings.Contains(bodyA, "stripe-access-token") || strings.Contains(bodyA, projB.ID) {
		t.Fatalf("leaked Project B findings to developer: %s", bodyA)
	}
	t.Logf("Developer GET /api/findings?project=%s response: status=%d OK (contains only Project A findings)", projA.ID, wA.Code)
}
