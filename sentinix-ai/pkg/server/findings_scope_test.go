package server

import (
	"strings"
	"testing"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

// An unfiltered /api/findings must list only granted projects for a developer.
func TestFindingsListScopedToGrants(t *testing.T) {
	e := setup(t)
	pa, _ := e.st.CreateProject(store.Project{Name: "a", Host: "github", Repo: "o/a", Branch: "main"})
	pb, _ := e.st.CreateProject(store.Project{Name: "b", Host: "github", Repo: "o/b", Branch: "main"})
	for p, m := range map[string]string{pa.ID: "msg-in-a", pb.ID: "msg-in-b"} {
		e.st.RecordScan(store.Scan{ProjectID: p, Mode: "branch", Commit: "c"}, []string{"secrets"},
			[]engines.Finding{{Rule: "r", Engine: "secrets", File: "x.go", Line: 1, Severity: engines.High, Message: m}})
	}
	e.st.UpsertUser("d@example.test", "d", auth.Developer)
	e.st.GrantProjectAccess(pa.ID, "d@example.test")
	body := e.call("d@example.test", auth.Developer, "GET", "/api/findings", "").Body.String()
	if !strings.Contains(body, "msg-in-a") || strings.Contains(body, "msg-in-b") {
		t.Fatalf("developer saw wrong set: %s", body)
	}
}

func TestSecurityHeadersEverywhere(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/healthz", "/api/me"} {
		h := e.call("", "", "GET", p, "").Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s missing hardening headers: %v", p, h)
		}
	}
}
