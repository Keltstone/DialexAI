package coder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"artix/pkg/policy"
)

// GenesisTraceHash is the parent hash for the first record in a trace stream.
const GenesisTraceHash = "0000000000000000000000000000000000000000000000000000000000000000"

// RoundTrace records diagnostic details for an iteration round of a convergence job.
type RoundTrace struct {
	TaskID          string    `json:"taskId"`
	Round           int       `json:"round"`
	PromptHash      string    `json:"promptHash"`
	PatchHash       string    `json:"patchHash"`
	Patch           string    `json:"patch,omitempty"`
	SandboxCommand  string    `json:"sandboxCommand"`
	ExitCode        int       `json:"exitCode"`
	ReviewerVerdict string    `json:"reviewerVerdict"`
	BlockingIssues  []string  `json:"blockingIssues,omitempty"`
	Tokens          int       `json:"tokens"`
	Cost            float64   `json:"cost"`
	Phase           string    `json:"phase,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	PrevHash        string    `json:"prevHash"`
	RecordHash      string    `json:"recordHash"`
}

var traceMu sync.Mutex

// HashContent returns the hex SHA256 hash of a string.
func HashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ComputeTraceHash deterministically calculates the SHA-256 hash of the round trace record.
func ComputeTraceHash(t *RoundTrace) string {
	payload := fmt.Sprintf("%s|%s|%d|%s|%s|%s|%d|%s|%d|%.6f|%s|%s",
		t.PrevHash,
		t.TaskID,
		t.Round,
		t.PromptHash,
		t.PatchHash,
		t.SandboxCommand,
		t.ExitCode,
		t.ReviewerVerdict,
		t.Tokens,
		t.Cost,
		t.Phase,
		t.Timestamp.UTC().Format(time.RFC3339Nano),
	)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// AppendRoundTrace writes a RoundTrace entry to trace.jsonl next to the audit log with hash chaining and mode 0600.
func AppendRoundTrace(workspaceDir string, trace *RoundTrace) error {
	traceMu.Lock()
	defer traceMu.Unlock()

	auditLogPath := policy.EffectiveAuditLogPath(workspaceDir)
	tracePath := filepath.Join(filepath.Dir(auditLogPath), "trace.jsonl")
	_ = os.MkdirAll(filepath.Dir(tracePath), 0755)

	if trace.Timestamp.IsZero() {
		trace.Timestamp = time.Now().UTC()
	}

	// Chaining: find previous record's hash
	prevHash := GenesisTraceHash
	if data, err := os.ReadFile(tracePath); err == nil && len(data) > 0 {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) > 0 {
			lastLine := strings.TrimSpace(lines[len(lines)-1])
			if lastLine != "" {
				var lastTr RoundTrace
				if err := json.Unmarshal([]byte(lastLine), &lastTr); err == nil && lastTr.RecordHash != "" {
					prevHash = lastTr.RecordHash
				}
			}
		}
	}

	trace.PrevHash = prevHash
	trace.RecordHash = ComputeTraceHash(trace)

	data, err := json.Marshal(trace)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(tracePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	_ = os.Chmod(tracePath, 0600)
	return nil
}

// VerifyRoundTraces verifies the cryptographic hash chain of trace.jsonl.
func VerifyRoundTraces(workspaceDir string) ([]RoundTrace, error) {
	traceMu.Lock()
	defer traceMu.Unlock()

	auditLogPath := policy.EffectiveAuditLogPath(workspaceDir)
	tracePath := filepath.Join(filepath.Dir(auditLogPath), "trace.jsonl")

	data, err := os.ReadFile(tracePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, nil
	}

	expectedPrevHash := GenesisTraceHash
	var traces []RoundTrace

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tr RoundTrace
		if err := json.Unmarshal([]byte(line), &tr); err != nil {
			return nil, fmt.Errorf("tampering detected in trace.jsonl at record #%d: invalid JSON: %w", i+1, err)
		}

		if tr.PrevHash != expectedPrevHash {
			return nil, fmt.Errorf("tampering detected in trace.jsonl at record #%d: broken hash chain: expected prevHash %s, got %s", i+1, expectedPrevHash, tr.PrevHash)
		}

		computed := ComputeTraceHash(&tr)
		if tr.RecordHash != computed {
			return nil, fmt.Errorf("tampering detected in trace.jsonl at record #%d: record hash altered: expected %s, got %s", i+1, computed, tr.RecordHash)
		}

		expectedPrevHash = tr.RecordHash
		traces = append(traces, tr)
	}

	return traces, nil
}

// ReadRoundTraces reads and verifies all RoundTrace entries for a task from trace.jsonl.
func ReadRoundTraces(workspaceDir string, taskID string) ([]RoundTrace, error) {
	traces, err := VerifyRoundTraces(workspaceDir)
	if err != nil {
		return nil, err
	}

	if taskID == "" {
		return traces, nil
	}

	var filtered []RoundTrace
	for _, tr := range traces {
		if tr.TaskID == taskID {
			filtered = append(filtered, tr)
		}
	}
	return filtered, nil
}
