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
