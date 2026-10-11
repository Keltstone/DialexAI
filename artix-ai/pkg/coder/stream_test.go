package coder

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStreamEmitter_Emit(t *testing.T) {
	buf := &bytes.Buffer{}
	emitter := NewStreamEmitter(buf)

	emitter.Emit(StreamEvent{
		Type:    EventPatchGenStart,
		TaskID:  "task-123",
		Phase:   "PATCH_GENERATION",
		Round:   1,
		Message: "Generating initial patch",
		Payload: map[string]any{"model": "claude-3-7-sonnet"},
	})

	emitter.Emit(StreamEvent{
		Type:    EventSandboxRunDone,
		TaskID:  "task-123",
		Phase:   "SANDBOX_EXECUTION",
		Round:   1,
		Status:  "passed",
		Message: "Tests passed: 10/10",
		Payload: map[string]any{"passed": 10, "failed": 0},
	})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var ev1 StreamEvent
	if err := json.Unmarshal([]byte(lines[0]), &ev1); err != nil {
		t.Fatalf("failed to unmarshal ev1: %v", err)
	}
	if ev1.Type != EventPatchGenStart || ev1.TaskID != "task-123" || ev1.Round != 1 {
		t.Errorf("unexpected ev1: %+v", ev1)
	}
	if ev1.Timestamp.IsZero() {
		t.Errorf("expected non-zero timestamp")
	}

	var ev2 StreamEvent
	if err := json.Unmarshal([]byte(lines[1]), &ev2); err != nil {
		t.Fatalf("failed to unmarshal ev2: %v", err)
	}
	if ev2.Type != EventSandboxRunDone || ev2.Status != "passed" {
		t.Errorf("unexpected ev2: %+v", ev2)
	}
}

func TestStreamEmitter_NilSafe(t *testing.T) {
	var nilEmitter *StreamEmitter
	// Should not panic
	nilEmitter.Emit(StreamEvent{Type: EventError})

	emptyEmitter := NewStreamEmitter(nil)
	emptyEmitter.Emit(StreamEvent{Type: EventError})
}
