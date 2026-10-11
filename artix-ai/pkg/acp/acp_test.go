package acp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

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
