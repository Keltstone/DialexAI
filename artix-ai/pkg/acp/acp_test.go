package acp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"artix/pkg/audit"
	"artix/pkg/coder"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestACPServer_InitializeAndSessionFlow(t *testing.T) {
	tmpDir := t.TempDir()

	inReader, inWriter := io.Pipe()
	outBuf := &bytes.Buffer{}

	server := NewServer(tmpDir, nil, inReader, outBuf)

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Serve()
	}()

	sendReq := func(method string, id int, params any) {
		pBytes, _ := json.Marshal(params)
		req := Request{
			JSONRPC: "2.0",
			ID:      id,
			Method:  method,
			Params:  pBytes,
		}
		data, _ := json.Marshal(req)
		_, _ = inWriter.Write(append(data, '\n'))
	}

	// 1. Initialize
	sendReq("initialize", 1, InitializeParams{
		ProtocolVersion: "2026-01-01",
		ClientInfo:      ClientInfo{Name: "TestClient", Version: "1.0"},
	})

	time.Sleep(50 * time.Millisecond)

	// 2. New Session
	sendReq("session/new", 2, SessionNewParams{
		Domain: "backend_engineer",
	})

	// 3. Pre-seed session before prompt
	server.sessionsMu.Lock()
	server.sessions["acp-test-session"] = &sessionState{
		id:        "acp-test-session",
		rootDir:   tmpDir,
		domain:    "backend_engineer",
		createdAt: time.Now(),
	}
	server.sessionsMu.Unlock()

	// Prompt
	sendReq("session/prompt", 3, SessionPromptParams{
		SessionID: "acp-test-session",
		Prompt:    "plan Add user registration with JWT tokens",
		Mode:      "plan",
	})

	time.Sleep(100 * time.Millisecond)

	// Shutdown
	sendReq("shutdown", 4, nil)
	_ = inWriter.Close()

	<-serverDone

	output := outBuf.String()
	if !strings.Contains(output, "artix-acp") {
		t.Errorf("expected initialize response with artix-acp, got: %s", output)
	}
	if !strings.Contains(output, "session/progress") {
		t.Errorf("expected session/progress stream notification in output, got: %s", output)
	}
}

func TestACPServer_MethodNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	inReader, inWriter := io.Pipe()
	outBuf := &bytes.Buffer{}

	server := NewServer(tmpDir, nil, inReader, outBuf)
	go func() {
		_ = server.Serve()
	}()

	req := Request{
		JSONRPC: "2.0",
		ID:      99,
		Method:  "unknown/method",
	}
	data, _ := json.Marshal(req)
	_, _ = inWriter.Write(append(data, '\n'))

	time.Sleep(50 * time.Millisecond)
	_ = inWriter.Close()
	server.Close()

	if !strings.Contains(outBuf.String(), "-32601") {
		t.Errorf("expected method not found error code -32601, got: %s", outBuf.String())
	}
}

func TestT4_ACP_RootDirValidation(t *testing.T) {
	tmpDir := t.TempDir()
	inReader, inWriter := io.Pipe()
	outBuf := &safeBuffer{}

	server := NewServer(tmpDir, nil, inReader, outBuf)
	server.SetAllowedRoots([]string{tmpDir})

	go func() {
		_ = server.Serve()
	}()
	defer server.Close()
	defer inWriter.Close()

	sendReq := func(id int, rootDir string) string {
		outBuf.Reset()
		pBytes, _ := json.Marshal(SessionNewParams{
			RootDir: rootDir,
		})
		req := Request{
			JSONRPC: "2.0",
			ID:      id,
			Method:  "session/new",
			Params:  pBytes,
		}
		data, _ := json.Marshal(req)
		_, _ = inWriter.Write(append(data, '\n'))
		time.Sleep(50 * time.Millisecond)
		return outBuf.String()
	}

	// 1. Prohibited /etc
	res1 := sendReq(1, "/etc")
	if !strings.Contains(res1, "-32602") || !strings.Contains(res1, "prohibited") {
		t.Errorf("expected error -32602 for /etc rootDir, got: %s", res1)
	}

	// 2. Prohibited file://
	res2 := sendReq(2, "file:///tmp")
	if !strings.Contains(res2, "-32602") || !strings.Contains(res2, "file://") {
		t.Errorf("expected error -32602 for file:// rootDir, got: %s", res2)
	}

	// 3. Prohibited root outside AllowedRoots
	otherDir := t.TempDir()
	res3 := sendReq(3, otherDir)
	if !strings.Contains(res3, "-32602") || !strings.Contains(res3, "outside allowed roots") {
		t.Errorf("expected error -32602 for rootDir outside allowed roots, got: %s", res3)
	}

	// 4. Allowed root succeeds
	res4 := sendReq(4, tmpDir)
	if strings.Contains(res4, "-32602") || !strings.Contains(res4, "ready") {
		t.Errorf("expected success for valid allowed rootDir, got: %s", res4)
	}
}

func TestT4_ACP_PlanAndReview_BudgetAndAuditEmitters(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repository
	_ = exec.Command("git", "init", tmpDir).Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.email", "test@artix.ai").Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.name", "Artix Test").Run()
	mainGo := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0644)
	_ = exec.Command("git", "-C", tmpDir, "add", "main.go").Run()
	_ = exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run()

	inReader, inWriter := io.Pipe()
	outBuf := &safeBuffer{}

	server := NewServer(tmpDir, nil, inReader, outBuf)
	go func() {
		_ = server.Serve()
	}()
	defer server.Close()
	defer inWriter.Close()

	sendReq := func(method string, id int, params any) string {
		outBuf.Reset()
		pBytes, _ := json.Marshal(params)
		req := Request{
			JSONRPC: "2.0",
			ID:      id,
			Method:  method,
			Params:  pBytes,
		}
		data, _ := json.Marshal(req)
		_, _ = inWriter.Write(append(data, '\n'))
		time.Sleep(150 * time.Millisecond)
		return outBuf.String()
	}

	// Session initialization
	sendReq("session/new", 1, SessionNewParams{
		SessionID: "sess-t4-01",
		RootDir:   tmpDir,
	})

	// 1. Plan prompt generates spec and emits audit event
	planRes := sendReq("session/prompt", 2, SessionPromptParams{
		SessionID: "sess-t4-01",
		Prompt:    "plan Implement a secure webhook listener for github",
		Mode:      "plan",
	})
	if !strings.Contains(planRes, "completed") {
		t.Errorf("expected plan to complete, got: %s", planRes)
	}

	// Check audit log for plan event
	events, err := audit.Default(tmpDir).ReadEvents()
	if err != nil {
		t.Fatalf("failed to read audit events: %v", err)
	}
	foundPlanEvent := false
	for _, e := range events {
		if e.EventType == audit.EventSpecDeliberation && e.Status == "SUCCESS" {
			foundPlanEvent = true
			if e.Details == nil || e.Details["specId"] == nil {
				t.Errorf("audit event missing specId detail: %+v", e)
			}
			break
		}
	}
	if !foundPlanEvent {
		t.Errorf("expected EventSpecDeliberation audit event in audit log, but not found")
	}

	// 2. Review on empty diff is NOT approved
	revEmptyRes := sendReq("session/prompt", 3, SessionPromptParams{
		SessionID: "sess-t4-01",
		Prompt:    "review",
		Mode:      "review",
	})
	if !strings.Contains(revEmptyRes, "REJECTED") || strings.Contains(revEmptyRes, `"approved":true`) {
		t.Errorf("expected empty-diff review to be REJECTED and approved=false, got: %s", revEmptyRes)
	}

	// Check audit log for empty-diff rejection
	events, _ = audit.Default(tmpDir).ReadEvents()
	foundEmptyDiffReview := false
	for _, e := range events {
		if e.EventType == audit.EventReviewerVerdict && e.Status == "REJECTED" {
			foundEmptyDiffReview = true
			break
		}
	}
	if !foundEmptyDiffReview {
		t.Errorf("expected EventReviewerVerdict with REJECTED status in audit log for empty diff")
	}

	// 3. Review on non-empty diff evaluates correctly
	_ = os.WriteFile(mainGo, []byte("package main\n\n// Added function\nfunc Helper() int { return 42 }\nfunc main() {}\n"), 0644)
	revNonEmptyRes := sendReq("session/prompt", 4, SessionPromptParams{
		SessionID: "sess-t4-01",
		Prompt:    "review current changes",
		Mode:      "review",
	})
	if strings.Contains(revNonEmptyRes, "empty diff") {
		t.Errorf("expected non-empty diff review not to report empty diff, got: %s", revNonEmptyRes)
	}

	// 4. Honor budget MaxTokens and MaxUSD exhaustion
	budgetServer := NewServer(tmpDir, nil, inReader, outBuf)
	budgetServer.SetBudget(&coder.TokenBudget{
		MaxStoryTokens:  1, // Exhausted cap
		UsedStoryTokens: 10,
	})
	server.SetBudget(&coder.TokenBudget{
		MaxStoryTokens:  1,
		UsedStoryTokens: 10,
	})

	planBudgetExhausted := sendReq("session/prompt", 5, SessionPromptParams{
		SessionID: "sess-t4-01",
		Prompt:    "plan Something requiring tokens",
		Mode:      "plan",
	})
	if !strings.Contains(planBudgetExhausted, "budget exhausted") {
		t.Errorf("expected plan to fail with budget exhausted, got: %s", planBudgetExhausted)
	}

	revBudgetExhausted := sendReq("session/prompt", 6, SessionPromptParams{
		SessionID: "sess-t4-01",
		Prompt:    "review",
		Mode:      "review",
	})
	if !strings.Contains(revBudgetExhausted, "budget exhausted") {
		t.Errorf("expected review to fail with budget exhausted, got: %s", revBudgetExhausted)
	}

	// Verify budget.exhausted audit events emitted
	events, _ = audit.Default(tmpDir).ReadEvents()
	foundBudgetExhausted := false
	for _, e := range events {
		if e.EventType == audit.EventBudgetExhausted {
			foundBudgetExhausted = true
			break
		}
	}
	if !foundBudgetExhausted {
		t.Errorf("expected EventBudgetExhausted audit event in audit log")
	}
}
