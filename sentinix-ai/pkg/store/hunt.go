package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// HuntRun represents a single execution of the hunt engine.
type HuntRun struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"project_id"`
	Mode          string  `json:"mode"`
	Status        string  `json:"status"` // "running", "completed", "truncated", "failed"
	Started       int64   `json:"started"`
	DurationMs    int64   `json:"duration_ms"`
	Calls         int     `json:"calls"`
	Tokens        int     `json:"tokens"`
	CostUSD       float64 `json:"cost_usd"`
	FindingsCount int     `json:"findings_count"`
	Gaps          string  `json:"gaps"`
}

// HuntTaskRecord represents one fresh-context task within a run.
type HuntTaskRecord struct {
	ID         string `json:"id"`
	RunID      string `json:"run_id"`
	TaskKey    string `json:"task_key"`
	Status     string `json:"status"` // "success", "skipped", "error"
	CodeHash   string `json:"code_hash"`
	File       string `json:"file"`
	Class      string `json:"class"`
	Pass       string `json:"pass"`
	Model      string `json:"model"`
	ResultJSON string `json:"result_json"`
	Started    int64  `json:"started"`
	DurationMs int64  `json:"duration_ms"`
}

// HuntToolCallRecord stores an audited tool execution.
type HuntToolCallRecord struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	TaskID        string `json:"task_id"`
	Tool          string `json:"tool"`
	ArgsHash      string `json:"args_hash"`
	BytesReturned int64  `json:"bytes_returned"`
	TS            int64  `json:"ts"`
}

// CreateHuntRun inserts a new hunt run record.
func (s *Store) CreateHuntRun(ctx context.Context, r HuntRun) error {
	if r.ID == "" {
		r.ID = id()
	}
	if r.Started == 0 {
		r.Started = now()
	}
	_, err := s.db.ExecContext(ctx,
		"insert into hunt_runs(id, project_id, mode, status, started, duration_ms, calls, tokens, cost_usd, findings_count, gaps) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
		r.ID, r.ProjectID, r.Mode, r.Status, r.Started, r.DurationMs, r.Calls, r.Tokens, r.CostUSD, r.FindingsCount, r.Gaps)
	return err
}

// UpdateHuntRun updates an existing hunt run.
func (s *Store) UpdateHuntRun(ctx context.Context, r HuntRun) error {
	_, err := s.db.ExecContext(ctx,
		"update hunt_runs set status=$2, duration_ms=$3, calls=$4, tokens=$5, cost_usd=$6, findings_count=$7, gaps=$8 where id=$1",
		r.ID, r.Status, r.DurationMs, r.Calls, r.Tokens, r.CostUSD, r.FindingsCount, r.Gaps)
	return err
}

// GetHuntRun retrieves a run by its ID.
func (s *Store) GetHuntRun(ctx context.Context, runID string) (*HuntRun, error) {
	row := s.db.QueryRowContext(ctx,
		"select id, project_id, mode, status, started, duration_ms, calls, tokens, cost_usd, findings_count, gaps from hunt_runs where id=$1",
		runID)
	var r HuntRun
	err := row.Scan(&r.ID, &r.ProjectID, &r.Mode, &r.Status, &r.Started, &r.DurationMs, &r.Calls, &r.Tokens, &r.CostUSD, &r.FindingsCount, &r.Gaps)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListHuntRuns returns recent runs for a project ordered by started descending.
func (s *Store) ListHuntRuns(ctx context.Context, projectID string, limit int) ([]HuntRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		"select id, project_id, mode, status, started, duration_ms, calls, tokens, cost_usd, findings_count, gaps from hunt_runs where project_id=$1 or $1='' order by started desc limit $2",
		projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []HuntRun
	for rows.Next() {
		var r HuntRun
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Mode, &r.Status, &r.Started, &r.DurationMs, &r.Calls, &r.Tokens, &r.CostUSD, &r.FindingsCount, &r.Gaps); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// SaveHuntTask records a completed task in the store.
func (s *Store) SaveHuntTask(ctx context.Context, t HuntTaskRecord) error {
	if t.ID == "" {
		t.ID = id()
	}
	if t.Started == 0 {
		t.Started = now()
	}
	_, err := s.db.ExecContext(ctx,
		"insert into hunt_tasks(id, run_id, task_key, status, code_hash, file, class, pass, model, result_json, started, duration_ms) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
		t.ID, t.RunID, t.TaskKey, t.Status, t.CodeHash, t.File, t.Class, t.Pass, t.Model, t.ResultJSON, t.Started, t.DurationMs)
	return err
}

// GetCachedHuntTask looks up a previously cached task result by task_key.
func (s *Store) GetCachedHuntTask(ctx context.Context, taskKey string) (*HuntTaskRecord, bool, error) {
	row := s.db.QueryRowContext(ctx,
		"select id, run_id, task_key, status, code_hash, file, class, pass, model, result_json, started, duration_ms from hunt_tasks where task_key=$1 order by started desc limit 1",
		taskKey)
	var t HuntTaskRecord
	err := row.Scan(&t.ID, &t.RunID, &t.TaskKey, &t.Status, &t.CodeHash, &t.File, &t.Class, &t.Pass, &t.Model, &t.ResultJSON, &t.Started, &t.DurationMs)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &t, true, nil
}

// RecordHuntToolCall logs an individual tool invocation to the audit log table.
func (s *Store) RecordHuntToolCall(ctx context.Context, c HuntToolCallRecord) error {
	if c.ID == "" {
		c.ID = id()
	}
	if c.TS == 0 {
		c.TS = now()
	}
	_, err := s.db.ExecContext(ctx,
		"insert into hunt_tool_calls(id, run_id, task_id, tool, args_hash, bytes_returned, ts) values($1, $2, $3, $4, $5, $6, $7)",
		c.ID, c.RunID, c.TaskID, c.Tool, c.ArgsHash, c.BytesReturned, c.TS)
	return err
}

// GetHuntRunToolCalls returns tool calls associated with a run.
func (s *Store) GetHuntRunToolCalls(ctx context.Context, runID string) ([]HuntToolCallRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		"select id, run_id, task_id, tool, args_hash, bytes_returned, ts from hunt_tool_calls where run_id=$1 order by ts asc",
		runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []HuntToolCallRecord
	for rows.Next() {
		var c HuntToolCallRecord
		if err := rows.Scan(&c.ID, &c.RunID, &c.TaskID, &c.Tool, &c.ArgsHash, &c.BytesReturned, &c.TS); err != nil {
			return nil, err
		}
		calls = append(calls, c)
	}
	return calls, rows.Err()
}

// FormatHuntRunSummary returns a formatted string of run statistics.
func (r *HuntRun) Summary() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}
