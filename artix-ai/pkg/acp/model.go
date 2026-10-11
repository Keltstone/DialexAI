package acp

import (
	"encoding/json"
	"time"
)

// JSON-RPC 2.0 and ACP Protocol Models

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// ACP Protocol Handshake & Capabilities

type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
	Capabilities    map[string]any         `json:"capabilities,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	ServerInfo      ServerInfo        `json:"serverInfo"`
	Capabilities    AgentCapabilities `json:"capabilities"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type AgentCapabilities struct {
	Planning          bool     `json:"planning"`
	Coding            bool     `json:"coding"`
	Review            bool     `json:"review"`
	WorktreeIsolation bool     `json:"worktreeIsolation"`
	SupportedPersonas []string `json:"supportedPersonas"`
	StreamEvents      bool     `json:"streamEvents"`
}

// Session Models

type SessionNewParams struct {
	SessionID string `json:"sessionId,omitempty"`
	RootDir   string `json:"rootDir,omitempty"`
	Domain    string `json:"domain,omitempty"`
}

type SessionNewResult struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
}

type SessionPromptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    string `json:"prompt"`
	Mode      string `json:"mode,omitempty"` // "plan", "code", "review", or "auto"
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
}

type SessionPromptResult struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	SpecID    string `json:"specId,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Output    string `json:"output,omitempty"`
	Approved  bool   `json:"approved,omitempty"`
}

type SessionProgressNotification struct {
	SessionID string    `json:"sessionId"`
	Phase     string    `json:"phase"`
	Round     int       `json:"round,omitempty"`
	MaxRounds int       `json:"maxRounds,omitempty"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload,omitempty"`
}
