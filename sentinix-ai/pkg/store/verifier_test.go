package store

import "testing"

func TestAuditVerifierFlagsTamper(t *testing.T) {
	st, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	st.AddAudit(Audit{Actor: "a", Action: "x"})
	st.StartAuditVerifier(1<<62, t.Logf)
	if !st.AuditChainOK() {
		t.Fatal("clean chain flagged")
	}
	st.db.Exec("update audit set actor='evil'")
	st.StartAuditVerifier(1<<62, t.Logf)
	if st.AuditChainOK() {
		t.Fatal("tamper not flagged")
	}
}
