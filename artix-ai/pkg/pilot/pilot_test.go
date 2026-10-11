package pilot

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"artix/pkg/audit"
)

func TestG9_PilotHarnessRejectsIncompleteOrUnverifiedRecords(t *testing.T) {
	harness := NewHarness()

	// Missing TaskID
	err := harness.RecordTask(TaskRecord{
		Repo:              "repo-a",
		Accepted:          true,
		Rounds:            2,
		Tokens:            4500,
		USD:               0.045,
		HumanEditDistance: 12,
		HumanEvaluated:    true,
	})
	if err == nil {
		t.Fatalf("expected error when TaskID is missing")
	}

	// Unverified by human (self-attesting)
	err = harness.RecordTask(TaskRecord{
		TaskID:            "TASK-1",
		Repo:              "repo-a",
		Accepted:          true,
		Rounds:            2,
		Tokens:            4500,
		USD:               0.045,
		HumanEditDistance: 12,
		HumanEvaluated:    false, // self-grading without human reviewer sign-off
	})
	if err == nil {
		t.Fatalf("expected error when task is not human-evaluated")
	}
}

// TestR2_8_Harness_RejectsUnjoinableAndVendorRecords asserts that the harness
// rejects records that cannot be joined to an audit event hash, spec ID, commit SHA,
// and ledger entry, or that target vendor-owned repositories.
func TestR2_8_Harness_RejectsUnjoinableAndVendorRecords(t *testing.T) {
	harness := NewHarness()

	// 1. Missing auditRecordHash
	err := harness.RecordTask(TaskRecord{
		TaskID:            "TASK-001",
		Repo:              "gin-gonic/gin",
		Accepted:          true,
		Rounds:            1,
		Tokens:            5000,
		USD:               0.025,
		HumanEvaluated:    true,
		EvaluatedBy:       "verified-evaluator@domain.org",
		SpecID:            "STORY-101",
		CommitSHA:         "92fc42f01234567890abcdef1234567890abcdef",
		LedgerRef:         "ledger.json:team-qa:2026-10-01",
		AuditRecordHash:   "", // missing
	})
	if err == nil {
		t.Fatalf("expected error when auditRecordHash is missing")
	}

	// 2. Missing commitSHA
	err = harness.RecordTask(TaskRecord{
		TaskID:            "TASK-002",
		Repo:              "gin-gonic/gin",
		Accepted:          true,
		Rounds:            1,
		Tokens:            5000,
		USD:               0.025,
		HumanEvaluated:    true,
		EvaluatedBy:       "verified-evaluator@domain.org",
		SpecID:            "STORY-102",
		CommitSHA:         "", // missing
		LedgerRef:         "ledger.json:team-qa:2026-10-01",
		AuditRecordHash:   "a1b2c3d4e5f60123456789abcdef0123456789abcdef0123456789abcdef0123",
	})
	if err == nil {
		t.Fatalf("expected error when commitSHA is missing")
	}

	// 3. Vendor-owned repository (artix-ai / kritix-ai) must be rejected
	err = harness.RecordTask(TaskRecord{
		TaskID:            "TASK-003",
		Repo:              "artix-ai", // vendor repo
		Accepted:          true,
		Rounds:            1,
		Tokens:            5000,
		USD:               0.025,
		HumanEvaluated:    true,
		EvaluatedBy:       "verified-evaluator@domain.org",
		SpecID:            "STORY-103",
		CommitSHA:         "92fc42f01234567890abcdef1234567890abcdef",
		LedgerRef:         "ledger.json:team-qa:2026-10-01",
		AuditRecordHash:   "a1b2c3d4e5f60123456789abcdef0123456789abcdef0123456789abcdef0123",
	})
	if err == nil {
		t.Fatalf("expected error when repo is vendor-owned (artix-ai)")
	}

	// 4. Unidentifiable evaluator alias must be rejected
	err = harness.RecordTask(TaskRecord{
		TaskID:            "TASK-004",
		Repo:              "spf13/cobra",
		Accepted:          true,
		Rounds:            1,
		Tokens:            5000,
		USD:               0.025,
		HumanEvaluated:    true,
		EvaluatedBy:       "auditor-sec-1", // unidentifiable alias
		SpecID:            "STORY-104",
		CommitSHA:         "92fc42f01234567890abcdef1234567890abcdef",
		LedgerRef:         "ledger.json:team-qa:2026-10-01",
		AuditRecordHash:   "a1b2c3d4e5f60123456789abcdef0123456789abcdef0123456789abcdef0123",
	})
	if err == nil {
		t.Fatalf("expected error when EvaluatedBy is an unidentifiable alias")
	}
}

// TestR4_PilotHarness_ResolvesRealCommitWithGitCatFile verifies that the harness
// executes git cat-file -e to verify that commit SHAs exist in the real git repository.
func TestR4_PilotHarness_ResolvesRealCommitWithGitCatFile(t *testing.T) {
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@domain.org").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Pilot Tester").Run()

	file := filepath.Join(tempDir, "file.txt")
	_ = os.WriteFile(file, []byte("hello\n"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", ".").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "initial commit").Run()

	out, err := exec.Command("git", "-C", tempDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("failed to get head sha: %v", err)
	}
	realSHA := string(out)[:40]

	// 1. Real commit in real repo -> succeeds
	if err := VerifyCommitObject(tempDir, realSHA); err != nil {
		t.Fatalf("expected real commit SHA to resolve via git cat-file: %v", err)
	}

	// 2. Fabricated / non-existent commit SHA -> rejected
	fakeSHA := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	if err := VerifyCommitObject(tempDir, fakeSHA); err == nil {
		t.Fatalf("expected fictitious commit SHA %s to fail git cat-file object resolution, but succeeded", fakeSHA)
	}
}

// TestR4_PilotHarness_ResolvesAuditLogRecordHash verifies that the harness
// verifies the audit record hash from a signed audit log file.
func TestR4_PilotHarness_ResolvesAuditLogRecordHash(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.jsonl")

	evt := audit.AuditEvent{
		EventID:   "evt-100",
		Timestamp: time.Now().UTC(),
		EventType: audit.EventCodeConvergence,
		Status:    "SUCCESS",
		PrevHash:  audit.GenesisHash,
	}
	evt.RecordHash = audit.ComputeRecordHash(&evt)
	evtData, _ := json.Marshal(evt)
	_ = os.WriteFile(logPath, append(evtData, '\n'), 0600)

	// 1. Valid hash in log -> succeeds
	if err := VerifyAuditRecord(logPath, evt.RecordHash); err != nil {
		t.Fatalf("expected valid audit record hash to resolve: %v", err)
	}

	// 2. Fabricated hash (e.g. sha256 of letter 'a') -> rejected
	fabricatedHash := hex.EncodeToString([]byte("0123456789012345678901234567890123456789012345678901234567890123"))
	if err := VerifyAuditRecord(logPath, fabricatedHash); err == nil {
		t.Fatalf("expected fabricated audit record hash to be rejected, but succeeded")
	}
}

// TestR4_PilotHarness_VerifiesLedgerEntry verifies ledger entry file verification.
func TestR4_PilotHarness_VerifiesLedgerEntry(t *testing.T) {
	tempDir := t.TempDir()
	ledgerPath := filepath.Join(tempDir, "ledger.json")
	_ = os.WriteFile(ledgerPath, []byte(`{"teams":{}}`), 0600)

	if err := VerifyLedgerEntry(ledgerPath); err != nil {
		t.Fatalf("expected existing ledger file to resolve: %v", err)
	}

	missingPath := filepath.Join(tempDir, "non_existent_ledger.json")
	if err := VerifyLedgerEntry(missingPath); err == nil {
		t.Fatalf("expected non-existent ledger to fail verification, but succeeded")
	}
}

func TestR4_PilotRubric_DocumentedMethodology(t *testing.T) {
	rubricPath := filepath.Join("..", "..", "docs", "pilot", "RUBRIC.md")
	data, err := os.ReadFile(rubricPath)
	if err != nil {
		t.Fatalf("RUBRIC.md must exist: %v", err)
	}
	content := string(data)
	if len(content) < 200 {
		t.Fatalf("RUBRIC.md must document evaluation methodology")
	}
	if !strings.Contains(content, "Evaluation Methodology") {
		t.Fatalf("RUBRIC.md must contain Evaluation Methodology section")
	}
}

// TestR5_5_PilotHarness_VerifiesLedgerEntryMatchesAuditRecord asserts that the pilot harness
// not only stats the ledger file, but reads the entry and matches tokens/USD to the task record.
func TestR5_5_PilotHarness_VerifiesLedgerEntryMatchesAuditRecord(t *testing.T) {
	tempDir := t.TempDir()
	ledgerPath := filepath.Join(tempDir, "ledger.json")

	ledgerData := map[string]any{
		"tasks": map[string]any{
			"TASK-001": map[string]any{
				"tokens": 4500,
				"usd":    0.045,
			},
		},
	}
	raw, _ := json.Marshal(ledgerData)
	_ = os.WriteFile(ledgerPath, raw, 0600)

	// 1. Matching tokens & USD -> succeeds
	if err := VerifyLedgerEntryMatch(ledgerPath, "TASK-001", 4500, 0.045); err != nil {
		t.Fatalf("expected valid matching ledger entry to succeed: %v", err)
	}

	// 2. Mismatched token count -> rejected
	if err := VerifyLedgerEntryMatch(ledgerPath, "TASK-001", 9999, 0.045); err == nil {
		t.Fatalf("expected token mismatch in ledger entry to fail verification")
	}

	// 3. Mismatched USD cost -> rejected
	if err := VerifyLedgerEntryMatch(ledgerPath, "TASK-001", 4500, 1.50); err == nil {
		t.Fatalf("expected USD cost mismatch in ledger entry to fail verification")
	}

	// 4. Missing task in ledger -> rejected
	if err := VerifyLedgerEntryMatch(ledgerPath, "NON-EXISTENT-TASK", 4500, 0.045); err == nil {
		t.Fatalf("expected missing task in ledger to fail verification")
	}
}


