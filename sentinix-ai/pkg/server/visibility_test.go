package server

import (
	"fmt"
	"net/http"
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
