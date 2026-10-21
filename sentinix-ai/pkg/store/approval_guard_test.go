package store

import "testing"

func TestApprovalExpiryAndLastOwner(t *testing.T) {
	st, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	st.UpsertUser("o@corp.test", "o", "owner")
	st.UpsertUser("r@corp.test", "r", "reviewer")
	id, _ := st.Propose("role_change", map[string]any{"email": "o@corp.test", "role": "developer"}, "admin@corp.test")
	if _, err := st.Decide(id, "r@corp.test", true); err == nil {
		t.Fatal("last owner demoted")
	}
	old, _ := st.Propose("fail_on", map[string]any{"project": "x", "fail_on": "high"}, "admin@corp.test")
	st.db.Exec("update approvals set ts=1 where id=$1", old)
	if _, err := st.Decide(old, "r@corp.test", true); err == nil {
		t.Fatal("expired approval decided")
	}
}

func TestReviewerCannotApproveOwnerRoleChanges(t *testing.T) {
	st, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	st.UpsertUser("o1@corp.test", "o1", "owner")
	st.UpsertUser("o2@corp.test", "o2", "owner")
	st.UpsertUser("r@corp.test", "r", "reviewer")
	st.UpsertUser("dev@corp.test", "dev", "developer")

	// Reviewer cannot approve demoting an Owner (even when multiple owners exist)
	id1, _ := st.Propose("role_change", map[string]any{"email": "o1@corp.test", "role": "developer"}, "admin@corp.test")
	if _, err := st.Decide(id1, "r@corp.test", true); err == nil {
		t.Fatal("reviewer should not be able to approve demoting an owner")
	}

	// Another owner CAN approve demoting an owner
	if _, err := st.Decide(id1, "o2@corp.test", true); err != nil {
		t.Fatalf("second owner should be able to approve owner demotion: %v", err)
	}

	// Reviewer cannot approve promoting a user to Owner
	id2, _ := st.Propose("role_change", map[string]any{"email": "dev@corp.test", "role": "owner"}, "admin@corp.test")
	if _, err := st.Decide(id2, "r@corp.test", true); err == nil {
		t.Fatal("reviewer should not be able to approve promoting to owner")
	}
}

