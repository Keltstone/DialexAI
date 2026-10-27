package store

import (
	"testing"
)

func TestHuntConfigApprovalWorkflow(t *testing.T) {
	st, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p, err := st.CreateProject(Project{Name: "p1", Host: "gh", Repo: "o/r", AI: true})
	if err != nil {
		t.Fatal(err)
	}

	st.UpsertUser("proposer@corp.test", "Proposer", "developer")
	st.UpsertUser("approver@corp.test", "Approver", "reviewer")

	// Propose a loosening hunt config change
	cfgJSON := `{"version":1,"gate":"confirmed-only","max_calls":500}`
	appID, err := st.Propose("hunt_config", map[string]any{
		"project": p.ID,
		"config":  cfgJSON,
	}, "proposer@corp.test")
	if err != nil {
		t.Fatalf("propose failed: %v", err)
	}

	// Proposer cannot approve their own change
	if _, err := st.Decide(appID, "proposer@corp.test", true); err == nil {
		t.Fatal("proposer should not be able to self-approve")
	}

	// Approver approves
	app, err := st.Decide(appID, "approver@corp.test", true)
	if err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	if app.Status != "approved" {
		t.Fatalf("expected approved status, got %s", app.Status)
	}

	// Verify project_hunt_configs table has the new config
	savedCfg, err := st.GetProjectHuntConfig(p.ID)
	if err != nil || savedCfg != cfgJSON {
		t.Fatalf("unexpected saved config: %s (err: %v)", savedCfg, err)
	}

	// Verify audit chain has recorded the approval
	audits, err := st.AuditLog(10)
	if err != nil || len(audits) == 0 {
		t.Fatalf("expected audit log entries, got %v (err: %v)", audits, err)
	}
	found := false
	for _, a := range audits {
		if a.Action == "approved:hunt_config" && a.Approver == "approver@corp.test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("audit log missing approved:hunt_config entry: %+v", audits)
	}
}
