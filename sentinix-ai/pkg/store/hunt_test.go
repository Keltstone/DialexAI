package store

import (
	"context"
	"testing"
)

func TestHuntStoreOperations(t *testing.T) {
	st, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()

	// 1. Create a HuntRun
	run := HuntRun{
		ID:        "run-123",
		ProjectID: "proj-abc",
		Mode:      "full",
		Status:    "running",
	}
	if err := st.CreateHuntRun(ctx, run); err != nil {
		t.Fatalf("CreateHuntRun failed: %v", err)
	}

	// 2. Fetch it back
	r, err := st.GetHuntRun(ctx, "run-123")
	if err != nil || r == nil || r.Status != "running" {
		t.Fatalf("unexpected fetched run: %+v, err: %v", r, err)
	}

	// 3. Save a task
	task := HuntTaskRecord{
		ID:         "task-1",
		RunID:      "run-123",
		TaskKey:    "hash1|MainActivity.kt|confused-deputy|strict",
		Status:     "success",
		CodeHash:   "hash1",
		File:       "MainActivity.kt",
		Class:      "confused-deputy",
		Pass:       "strict",
		Model:      "claude-3-7-sonnet",
		ResultJSON: `{"vulnerable":true}`,
	}
	if err := st.SaveHuntTask(ctx, task); err != nil {
		t.Fatalf("SaveHuntTask failed: %v", err)
	}

	// 4. Look up cached task by task key
	cached, ok, err := st.GetCachedHuntTask(ctx, "hash1|MainActivity.kt|confused-deputy|strict")
	if err != nil || !ok || cached.ID != "task-1" {
		t.Fatalf("unexpected cached task: %+v, ok: %v, err: %v", cached, ok, err)
	}

	// 5. Record a tool call
	tc := HuntToolCallRecord{
		ID:            "tool-1",
		RunID:         "run-123",
		TaskID:        "task-1",
		Tool:          "read_file",
		ArgsHash:      "args_hash_1",
		BytesReturned: 512,
	}
	if err := st.RecordHuntToolCall(ctx, tc); err != nil {
		t.Fatalf("RecordHuntToolCall failed: %v", err)
	}

	// 6. Fetch tool calls
	tcs, err := st.GetHuntRunToolCalls(ctx, "run-123")
	if err != nil || len(tcs) != 1 || tcs[0].Tool != "read_file" {
		t.Fatalf("unexpected tool calls: %+v, err: %v", tcs, err)
	}

	// 7. Update run to completed with accounting
	r.Status = "completed"
	r.Calls = 5
	r.Tokens = 12000
	r.CostUSD = 0.04
	r.FindingsCount = 1
	if err := st.UpdateHuntRun(ctx, *r); err != nil {
		t.Fatalf("UpdateHuntRun failed: %v", err)
	}

	// 8. List runs
	runs, err := st.ListHuntRuns(ctx, "proj-abc", 10)
	if err != nil || len(runs) != 1 || runs[0].Status != "completed" {
		t.Fatalf("unexpected runs list: %+v, err: %v", runs, err)
	}
}
