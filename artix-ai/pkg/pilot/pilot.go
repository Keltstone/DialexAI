package pilot

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"artix/pkg/audit"
)

// TaskRecord represents an independently evaluated task execution record.
type TaskRecord struct {
	TaskID            string    `json:"taskId"`
	Repo              string    `json:"repo"`
	Accepted          bool      `json:"accepted"`
	Rounds            int       `json:"rounds"`
	Tokens            int64     `json:"tokens"`
	USD               float64   `json:"usd"`
	HumanEditDistance int       `json:"humanEditDistance"`
	FalseRejection    bool      `json:"falseRejection"`
	HumanEvaluated    bool      `json:"humanEvaluated"`
	EvaluatedBy       string    `json:"evaluatedBy,omitempty"`
	SpecID            string    `json:"specId"`
	CommitSHA         string    `json:"commitSHA"`
	AuditRecordHash   string    `json:"auditRecordHash"`
	LedgerRef         string    `json:"ledgerRef"`
	Timestamp         time.Time `json:"timestamp"`
}

// Harness manages recording, validating, and streaming raw pilot task metrics.
type Harness struct {
	mu      sync.RWMutex
	records []TaskRecord
	workDir string
}

// NewHarness instantiates a pilot harness.
func NewHarness(workDir ...string) *Harness {
	wd := ""
	if len(workDir) > 0 {
		wd = workDir[0]
	}
	return &Harness{workDir: wd}
}

// VerifyCommitObject resolves the commit SHA using git cat-file -e against the repository.
func VerifyCommitObject(repoDir, commitSHA string) error {
	if commitSHA == "" {
		return errors.New("missing commit SHA")
	}
	cmd := exec.Command("git", "cat-file", "-e", commitSHA+"^{commit}")
	if repoDir != "" {
		cmd.Dir = repoDir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git object resolution failed for commit %s: %w (%s)", commitSHA, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// VerifyAuditRecord resolves and recomputes the audit record hash from the signed audit log.
func VerifyAuditRecord(auditLogPath, expectedHash string) error {
	if auditLogPath == "" {
		return errors.New("missing audit log path")
	}
	data, err := os.ReadFile(auditLogPath)
	if err != nil {
		return fmt.Errorf("failed to read audit log at %s: %w", auditLogPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var evt audit.AuditEvent
		if err := json.Unmarshal([]byte(line), &evt); err == nil {
			computed := audit.ComputeRecordHash(&evt)
			if computed == expectedHash && evt.RecordHash == expectedHash {
				return nil
			}
		}
	}
	return fmt.Errorf("audit record hash %s not found or altered in %s", expectedHash, auditLogPath)
}

// VerifyLedgerEntry verifies that a referenced ledger file exists on disk and is valid JSON.
func VerifyLedgerEntry(ledgerPath string) error {
	if ledgerPath == "" {
		return errors.New("missing ledger path")
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		return fmt.Errorf("ledger file not found at %s: %w", ledgerPath, err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("invalid json in ledger at %s: %w", ledgerPath, err)
	}
	return nil
}

// VerifyLedgerEntryMatch verifies that the ledger file exists, parses its JSON contents,
// and confirms that the entry matching the task ID exists and matches the expected token count and USD cost.
func VerifyLedgerEntryMatch(ledgerPath, taskID string, expectedTokens int64, expectedUSD float64) error {
	if ledgerPath == "" {
		return errors.New("missing ledger path")
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		return fmt.Errorf("failed to read ledger file at %s: %w", ledgerPath, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("invalid json in ledger at %s: %w", ledgerPath, err)
	}

	// Check if task exists in "tasks" or directly in root
	var entry map[string]any
	if tasks, ok := root["tasks"].(map[string]any); ok {
		if t, ok := tasks[taskID].(map[string]any); ok {
			entry = t
		}
	}
	if entry == nil {
		if t, ok := root[taskID].(map[string]any); ok {
			entry = t
		}
	}
	if entry == nil {
		return fmt.Errorf("task %q not found in ledger %s", taskID, ledgerPath)
	}

	// Verify tokens
	tokensVal, hasTokens := entry["tokens"]
	if !hasTokens {
		return fmt.Errorf("task %q in ledger missing 'tokens' field", taskID)
	}
	tokensFloat, ok := tokensVal.(float64)
	if !ok || int64(tokensFloat) != expectedTokens {
		return fmt.Errorf("token count mismatch in ledger for task %q: expected %d, got %v", taskID, expectedTokens, tokensVal)
	}

	// Verify USD
	usdVal, hasUSD := entry["usd"]
	if !hasUSD {
		return fmt.Errorf("task %q in ledger missing 'usd' field", taskID)
	}
	usdFloat, ok := usdVal.(float64)
	diff := usdFloat - expectedUSD
	if diff < 0 {
		diff = -diff
	}
	if !ok || diff > 0.0001 {
		return fmt.Errorf("USD cost mismatch in ledger for task %q: expected %.4f, got %v", taskID, expectedUSD, usdVal)
	}

	return nil
}


// ValidateAndJoinRecord verifies the complete joinability and independence of a task record.
func ValidateAndJoinRecord(r TaskRecord, repoDir ...string) error {
	if r.TaskID == "" {
		return errors.New("pilot task record missing taskId")
	}
	if r.Repo == "" {
		return errors.New("pilot task record missing repo")
	}
	repoLower := strings.ToLower(strings.TrimSpace(r.Repo))
	if repoLower == "artix-ai" || repoLower == "kritix-ai" || repoLower == "socratix-engine" || strings.HasPrefix(repoLower, "vendor/") {
		return fmt.Errorf("vendor repo %q disallowed: pilot tasks must target independent repositories", r.Repo)
	}
	if r.Rounds <= 0 {
		return errors.New("pilot task record rounds must be > 0")
	}
	if r.Tokens <= 0 {
		return errors.New("pilot task record tokens must be > 0")
	}
	if r.USD <= 0.0 {
		return errors.New("pilot task record USD must be > 0")
	}
	if !r.HumanEvaluated {
		return errors.New("pilot task record must be human evaluated (self-grading disallowed)")
	}
	if r.EvaluatedBy == "" || strings.HasPrefix(r.EvaluatedBy, "auditor-sec-") || strings.HasPrefix(r.EvaluatedBy, "user-") {
		return fmt.Errorf("evaluator %q is an unidentifiable alias; verifiable evaluator identity required", r.EvaluatedBy)
	}
	if r.SpecID == "" {
		return errors.New("pilot task record missing specId (cannot join to spec)")
	}
	if r.CommitSHA == "" || len(r.CommitSHA) < 7 {
		return fmt.Errorf("pilot task record missing valid commitSHA: %q (cannot join to commit)", r.CommitSHA)
	}
	if r.AuditRecordHash == "" || len(r.AuditRecordHash) != 64 {
		return fmt.Errorf("pilot task record missing valid auditRecordHash: %q (cannot join to audit log)", r.AuditRecordHash)
	}
	if r.LedgerRef == "" {
		return errors.New("pilot task record missing ledgerRef (cannot join to ledger entry)")
	}

	if len(repoDir) > 0 && repoDir[0] != "" {
		if err := VerifyCommitObject(repoDir[0], r.CommitSHA); err != nil {
			return err
		}
	}
	return nil
}

// RecordTask registers a verified task run into the harness, strictly rejecting unverified/self-attested entries.
func (h *Harness) RecordTask(r TaskRecord) error {
	if err := ValidateAndJoinRecord(r, h.workDir); err != nil {
		return err
	}
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

// LoadRawResults parses newline-delimited JSON raw records from an external reader,
// strictly discarding rows that cannot be joined or fail validation.
func (h *Harness) LoadRawResults(r io.Reader) ([]TaskRecord, error) {
	scanner := bufio.NewScanner(r)
	var records []TaskRecord
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var rec TaskRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("invalid json in raw pilot results: %w", err)
		}
		// Rows that cannot be joined or fail criteria are discarded
		if err := ValidateAndJoinRecord(rec, h.workDir); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// ExportRawResults writes the raw task records as JSON lines to the destination writer.
func (h *Harness) ExportRawResults(w io.Writer) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, rec := range h.records {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
			return err
		}
	}
	return nil
}

