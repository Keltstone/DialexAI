package coder

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// StreamEventType defines the category of real-time lifecycle event.
type StreamEventType string

const (
	EventCouncilStart        StreamEventType = "council_start"
	EventCouncilRound        StreamEventType = "council_round"
	EventCouncilDone         StreamEventType = "council_done"
	EventWorktreeProvisioned StreamEventType = "worktree_provisioned"
	EventPatchGenStart       StreamEventType = "patch_gen_start"
	EventPatchGenDone        StreamEventType = "patch_gen_done"
	EventSandboxRunStart     StreamEventType = "sandbox_run_start"
	EventSandboxRunChunk     StreamEventType = "sandbox_run_chunk"
	EventSandboxRunDone      StreamEventType = "sandbox_run_done"
	EventReviewStart         StreamEventType = "review_start"
	EventReviewVerdict       StreamEventType = "review_verdict"
	EventAwaitingApproval    StreamEventType = "awaiting_approval"
	EventConverged           StreamEventType = "converged"
	EventError               StreamEventType = "error"
)

// StreamEvent represents a structured, real-time lifecycle event.
type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	TaskID    string          `json:"taskId,omitempty"`
	Phase     string          `json:"phase,omitempty"`
	Round     int             `json:"round,omitempty"`
	MaxRounds int             `json:"maxRounds,omitempty"`
	Role      string          `json:"role,omitempty"`
	Status    string          `json:"status,omitempty"`
	Message   string          `json:"message,omitempty"`
	Payload   map[string]any  `json:"payload,omitempty"`
}

// StreamEmitter is a thread-safe NDJSON event writer.
type StreamEmitter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewStreamEmitter initializes a new StreamEmitter writing to the specified writer.
func NewStreamEmitter(w io.Writer) *StreamEmitter {
	return &StreamEmitter{w: w}
}

// Emit writes a single JSON-encoded StreamEvent followed by a newline.
func (e *StreamEmitter) Emit(event StreamEvent) {
	if e == nil || e.w == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	data, _ := json.Marshal(event)
	_, _ = e.w.Write(append(data, '\n'))
}
