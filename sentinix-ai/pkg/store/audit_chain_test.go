package store

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

func TestAuditHashChain_VerifiesAndDetectsTampering(t *testing.T) {
	st, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Add multiple audit entries
	entries := []Audit{
		{Actor: "alice@corp.test", Action: "scan:full", Target: "p1", Reason: "manual trigger"},
		{Actor: "bob@corp.test", Action: "triage:accepted", Target: "f1", Before: "open", After: "accepted"},
		{Actor: "charlie@corp.test", Action: "role:propose:reviewer", Target: "dave@corp.test", Before: "developer", After: "reviewer"},
	}
	for _, a := range entries {
		if err := st.AddAudit(a); err != nil {
			t.Fatalf("AddAudit failed: %v", err)
		}
	}

	// 2. Verify audit chain passes when intact
	if err := st.VerifyAuditChain(); err != nil {
		t.Fatalf("expected audit chain to verify, got: %v", err)
	}

	// 3. Query rows to inspect
	audits, err := st.AuditLog(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 3 {
		t.Fatalf("expected 3 audits, got %d", len(audits))
	}
	t.Logf("Audit row 0 (latest): id=%s action=%s hash=%s prev_hash=%s", audits[0].ID, audits[0].Action, audits[0].Hash, audits[0].PrevHash)
	t.Logf("Audit row 1: id=%s action=%s hash=%s prev_hash=%s", audits[1].ID, audits[1].Action, audits[1].Hash, audits[1].PrevHash)
	t.Logf("Audit row 2 (first): id=%s action=%s hash=%s prev_hash=%s", audits[2].ID, audits[2].Action, audits[2].Hash, audits[2].PrevHash)

	// 4. Tamper with one row (edit actor)
	middleID := audits[1].ID
	_, err = st.db.Exec("UPDATE audit SET actor='attacker@evil.test' WHERE id=$1", middleID)
	if err != nil {
		t.Fatalf("failed to tamper with audit row: %v", err)
	}

	// 5. Verification must now FAIL
	err = st.VerifyAuditChain()
	if err == nil {
		t.Fatal("expected audit chain verification to fail after row tampering, but it passed!")
	}
	t.Logf("Audit verification failure output (as expected after tamper): %v", err)
}

func TestAuditChain_PostgresReal(t *testing.T) {
	pgDSN := "postgres://localhost:5432/sentinix_test?sslmode=disable"
	st, err := Open(pgDSN)
	if err != nil {
		t.Skipf("skipping Postgres test: %v", err)
	}
	defer st.db.Close()

	// Clean audit table
	_, _ = st.db.Exec("TRUNCATE TABLE audit CASCADE")
	_, _ = st.db.Exec("TRUNCATE TABLE approvals CASCADE")
	_, _ = st.db.Exec("TRUNCATE TABLE users CASCADE")

	// 1. Add audit entries
	if err := st.AddAudit(Audit{Actor: "alice@test.corp", Action: "propose:rule", Target: "r1"}); err != nil {
		t.Fatalf("AddAudit 1 failed: %v", err)
	}
	if err := st.AddAudit(Audit{Actor: "bob@test.corp", Action: "approve:rule", Target: "r1"}); err != nil {
		t.Fatalf("AddAudit 2 failed: %v", err)
	}

	// 2. Query audit log
	logs, err := st.AuditLog(10)
	if err != nil {
		t.Fatalf("AuditLog failed on Postgres: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit rows, got %d", len(logs))
	}

	// 3. Verify audit chain
	if err := st.VerifyAuditChain(); err != nil {
		t.Fatalf("VerifyAuditChain failed on Postgres: %v", err)
	}
}

func TestAuditConcurrent(t *testing.T) {
	pgDSN := os.Getenv("PGDSN")
	if pgDSN == "" {
		pgDSN = "postgres://localhost:5432/sentinix_test?sslmode=disable"
	}
	st, err := Open(pgDSN)
	if err != nil {
		t.Skipf("skipping Postgres test: %v", err)
	}
	defer st.db.Close()

	// Clean audit table
	_, _ = st.db.Exec("TRUNCATE TABLE audit CASCADE")
	_, _ = st.db.Exec("TRUNCATE TABLE approvals CASCADE")
	_, _ = st.db.Exec("TRUNCATE TABLE users CASCADE")

	const goroutines = 50
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := st.AddAudit(Audit{
				Actor:  fmt.Sprintf("worker-%d@test.corp", idx),
				Action: "scan:ci",
				Target: fmt.Sprintf("target-%d", idx),
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent AddAudit failed: %v", err)
	}

	// 1. Verify 0 duplicate seq
	var dupCount int
	err = st.db.QueryRow("SELECT count(seq) - count(distinct seq) FROM audit").Scan(&dupCount)
	if err != nil {
		t.Fatalf("check duplicate seq failed: %v", err)
	}
	if dupCount != 0 {
		t.Fatalf("expected 0 duplicate seq, got %d", dupCount)
	}
	t.Logf("0 duplicate seq (dupCount = %d)", dupCount)

	// 2. Verify audit chain
	chainErr := st.VerifyAuditChain()
	if chainErr != nil {
		t.Fatalf("VerifyAuditChain failed: %v", chainErr)
	}
	t.Logf("VerifyAuditChain returning: %v", chainErr)

	// 3. Upgrade from rows with seq=0
	// Insert rows with seq=0 to simulate pre-upgrade state
	_, err = st.db.Exec("INSERT INTO audit(id,seq,ts,actor,action,target,commit_sha,approver,source,before_val,after_val,engines,pack_versions,skipped_files,mode,outcome,reason,prev_hash,hash) VALUES('legacy-1', 0, 1000, 'legacy@corp', 'legacy:action', 't1', '', '', '', '', '', '', '', '', '', '', '', '', 'legacyhash1')")
	if err != nil {
		t.Fatalf("failed inserting legacy row: %v", err)
	}
	if err := st.UpgradeAuditSeqZero(); err != nil {
		t.Fatalf("UpgradeAuditSeqZero failed: %v", err)
	}

	var zeroCount int
	if err := st.db.QueryRow("SELECT count(*) FROM audit WHERE seq=0").Scan(&zeroCount); err != nil || zeroCount != 0 {
		t.Fatalf("expected 0 rows with seq=0 after upgrade, got %d (err=%v)", zeroCount, err)
	}
	if err := st.VerifyAuditChain(); err != nil {
		t.Fatalf("VerifyAuditChain failed after upgrade: %v", err)
	}
}

