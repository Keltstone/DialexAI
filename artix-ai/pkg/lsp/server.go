package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"artix/pkg/gate"
	"artix/pkg/steering"
)

// Server implements a Language Server Protocol 3.17 backend for Artix AI.
type Server struct {
	rootDir         string
	in              *bufio.Reader
	out             io.Writer
	docs            map[string]string
	docsMu          sync.RWMutex
	running         bool
	runningMu       sync.Mutex
	inFlightCancels map[string]context.CancelFunc
	inFlightMu      sync.Mutex
}

// NewServer creates a new LSP server instance.
func NewServer(rootDir string, in io.Reader, out io.Writer) *Server {
	return &Server{
		rootDir:         rootDir,
		in:              bufio.NewReader(in),
		out:             out,
		docs:            make(map[string]string),
		inFlightCancels: make(map[string]context.CancelFunc),
	}
}

// Serve runs the request read loop until EOF or exit.
func (s *Server) Serve() error {
	s.runningMu.Lock()
	s.running = true
	s.runningMu.Unlock()

	for {
		s.runningMu.Lock()
		alive := s.running
		s.runningMu.Unlock()
		if !alive {
			break
		}

		payload, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req LSPRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			continue
		}

		s.handleRequest(&req)
	}

	return nil
}

func (s *Server) readMessage() ([]byte, error) {
	var contentLength int

	// Read headers
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // Blank line indicates end of headers
		}

		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				val := strings.TrimSpace(parts[1])
				contentLength, _ = strconv.Atoi(val)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing or zero content-length")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.in, body); err != nil {
		return nil, err
	}

	return body, nil
}

func (s *Server) sendNotification(method string, params interface{}) {
	rawParams, _ := json.Marshal(params)
	notif := LSPRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}

	payload, _ := json.Marshal(notif)
	s.writeMessage(payload)
}

func (s *Server) sendResponse(id interface{}, result interface{}, errObj *LSPError) {
	resp := LSPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   errObj,
	}

	if result != nil {
		rawRes, _ := json.Marshal(result)
		resp.Result = rawRes
	} else {
		resp.Result = []byte("null")
	}

	payload, _ := json.Marshal(resp)
	s.writeMessage(payload)
}

func (s *Server) writeMessage(payload []byte) {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	_, _ = s.out.Write([]byte(header))
	_, _ = s.out.Write(payload)
}

func (s *Server) handleRequest(req *LSPRequest) {
	if req.Method == "$/cancelRequest" {
		var p CancelParams
		if err := json.Unmarshal(req.Params, &p); err == nil && p.ID != nil {
			idStr := fmt.Sprintf("%v", p.ID)
			s.inFlightMu.Lock()
			if cancel, exists := s.inFlightCancels[idStr]; exists {
				cancel()
				delete(s.inFlightCancels, idStr)
			}
			s.inFlightMu.Unlock()
		}
		return
	}

	var ctx context.Context
	var cancel context.CancelFunc
	var idStr string
	if req.ID != nil {
		idStr = fmt.Sprintf("%v", req.ID)
		ctx, cancel = context.WithCancel(context.Background())
		s.inFlightMu.Lock()
		s.inFlightCancels[idStr] = cancel
		s.inFlightMu.Unlock()
		defer func() {
			s.inFlightMu.Lock()
			delete(s.inFlightCancels, idStr)
			s.inFlightMu.Unlock()
			cancel()
		}()
	}

	switch req.Method {
	case "initialize":
		result := map[string]interface{}{
			"capabilities": map[string]interface{}{
				"textDocumentSync":   1, // Full sync
				"codeActionProvider": true,
				"codeLensProvider": map[string]interface{}{
					"resolveProvider": false,
				},
				"executeCommandProvider": map[string]interface{}{
					"commands": []string{"artix.fixTaboo", "artix.deliberate", "artix.plan"},
				},
			},
			"serverInfo": map[string]string{
				"name":    "artix-lsp",
				"version": "1.0.0",
			},
		}
		s.sendResponse(req.ID, result, nil)

	case "initialized":
		// No-op

	case "shutdown":
		s.sendResponse(req.ID, nil, nil)

	case "exit":
		s.runningMu.Lock()
		s.running = false
		s.runningMu.Unlock()

	case "textDocument/didOpen":
		var p DidOpenTextDocumentParams
		if err := json.Unmarshal(req.Params, &p); err == nil {
			s.docsMu.Lock()
			s.docs[p.TextDocument.URI] = p.TextDocument.Text
			s.docsMu.Unlock()
			s.validateDocument(p.TextDocument.URI, p.TextDocument.Text)
		}

	case "textDocument/didChange":
		var p DidChangeTextDocumentParams
		if err := json.Unmarshal(req.Params, &p); err == nil && len(p.ContentChanges) > 0 {
			text := p.ContentChanges[0].Text
			s.docsMu.Lock()
			s.docs[p.TextDocument.URI] = text
			s.docsMu.Unlock()
			s.validateDocument(p.TextDocument.URI, text)
		}

	case "textDocument/codeAction":
		var p CodeActionParams
		_ = json.Unmarshal(req.Params, &p)
		actions := []CodeAction{
			{
				Title: "Artix: Fix Taboo Space Violation",
				Kind:  "quickfix",
				Command: &Command{
					Title:   "Fix Violation",
					Command: "artix.fixTaboo",
				},
			},
			{
				Title: "Artix: Deliberate with Stakeholder Council",
				Kind:  "refactor",
				Command: &Command{
					Title:   "Deliberate",
					Command: "artix.deliberate",
				},
			},
		}
		s.sendResponse(req.ID, actions, nil)

	case "textDocument/codeLens":
		if ctx != nil && ctx.Err() != nil {
			s.sendResponse(req.ID, nil, &LSPError{Code: CodeRequestCancelled, Message: "Request cancelled"})
			return
		}
		var p CodeLensParams
		_ = json.Unmarshal(req.Params, &p)
		lenses := []CodeLens{
			{
				Range: Range{
					Start: Position{Line: 0, Character: 0},
					End:   Position{Line: 0, Character: 10},
				},
				Command: &Command{
					Title:   "⚡ Artix: Deliberate Story Spec",
					Command: "artix.plan",
				},
			},
		}
		s.sendResponse(req.ID, lenses, nil)

	case "workspace/executeCommand":
		s.handleExecuteCommand(req)

	default:
		if req.ID != nil {
			s.sendResponse(req.ID, nil, &LSPError{Code: -32601, Message: "Method not found"})
		}
	}
}

func (s *Server) validateDocument(uri, text string) {
	var diags []Diagnostic
	lines := strings.Split(text, "\n")

	// Ingest taboo rules
	agg := steering.NewAggregator(s.rootDir)
	rules, _ := agg.CollectLocalRules()
	binder := steering.NewBinder(steering.SteeringConfig{}, rules)
	steerCtx := binder.CompilePersonaSteering("android_engineer")

	for lineIdx, line := range lines {
		// Check Taboo rule: raw SQLite
		if strings.Contains(line, "android.database.sqlite") {
			diags = append(diags, Diagnostic{
				Range: Range{
					Start: Position{Line: lineIdx, Character: 0},
					End:   Position{Line: lineIdx, Character: len(line)},
				},
				Severity: SeverityError,
				Code:     "TABOO-01",
				Source:   "Artix Steering",
				Message:  "Violates Taboo Space: Direct SQLite usage prohibited in UI layer.",
			})
		}

		// Check Taboo rule: main thread blocking
		if strings.Contains(line, "Thread.sleep") {
			diags = append(diags, Diagnostic{
				Range: Range{
					Start: Position{Line: lineIdx, Character: 0},
					End:   Position{Line: lineIdx, Character: len(line)},
				},
				Severity: SeverityWarning,
				Code:     "TABOO-02",
				Source:   "Artix Steering",
				Message:  "Violates Taboo Space: Blocking main thread with Thread.sleep.",
			})
		}

		_ = steerCtx
	}

	s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

func (s *Server) handleExecuteCommand(req *LSPRequest) {
	var p struct {
		Command   string `json:"command"`
		Arguments []any  `json:"arguments"`
	}
	_ = json.Unmarshal(req.Params, &p)

	if _, err := gate.Pass(context.Background(), gate.Request{
		EntryPoint: gate.EntryPointLSPCommand,
		Actor:      "lsp-client",
		ProjectDir: s.rootDir,
		Prompt:     p.Command,
	}); err != nil {
		s.sendResponse(req.ID, nil, &LSPError{
			Code:    CodeRequestCancelled,
			Message: fmt.Sprintf("Gate rejected LSP command %s: %v", p.Command, err),
		})
		return
	}

	switch p.Command {
	case "artix.fixTaboo", "artix.deliberate", "artix.plan":
		s.sendResponse(req.ID, map[string]any{"ok": true, "command": p.Command, "governed": true}, nil)
	default:
		s.sendResponse(req.ID, nil, &LSPError{Code: -32601, Message: fmt.Sprintf("Unknown command: %s", p.Command)})
	}
}
