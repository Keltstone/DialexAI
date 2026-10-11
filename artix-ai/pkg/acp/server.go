package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"artix/pkg/persona"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/spec"
)

var sessionCounter uint64

// Server implements the Agent Communication Protocol (ACP) standard JSON-RPC 2.0 server.
type Server struct {
	rootDir         string
	in              *bufio.Reader
	out             io.Writer
	outMu           sync.Mutex
	registry        *persona.Registry
	running         bool
	runningMu       sync.Mutex
	sessions        map[string]*sessionState
	sessionsMu      sync.RWMutex
	inFlightCancels map[string]context.CancelFunc
	inFlightMu      sync.Mutex
}

type sessionState struct {
	id        string
	rootDir   string
	domain    string
	createdAt time.Time
}

// NewServer initializes an ACP server for standard I/O communication.
func NewServer(rootDir string, reg *persona.Registry, in io.Reader, out io.Writer) *Server {
	if reg == nil {
		reg = persona.NewRegistry(rootDir)
	}
	return &Server{
		rootDir:         rootDir,
		in:              bufio.NewReader(in),
		out:             out,
		registry:        reg,
		sessions:        make(map[string]*sessionState),
		inFlightCancels: make(map[string]context.CancelFunc),
	}
}

// Serve runs the request processing loop until EOF or shutdown.
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

		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}

	return nil
}

// Close terminates the server loop.
func (s *Server) Close() {
	s.runningMu.Lock()
	s.running = false
	s.runningMu.Unlock()

	s.inFlightMu.Lock()
	for _, cancel := range s.inFlightCancels {
		cancel()
	}
	s.inFlightCancels = make(map[string]context.CancelFunc)
	s.inFlightMu.Unlock()
}

func (s *Server) readMessage() ([]byte, error) {
	// Peek to determine framing: Content-Length header or line-delimited JSON
	peekBytes, err := s.in.Peek(1)
	if err != nil {
		return nil, err
	}

	if peekBytes[0] == '{' || peekBytes[0] == '[' {
		// Line-delimited JSON
		line, err := s.in.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			return nil, err
		}
		return bytes.TrimSpace(line), nil
	}

	// Content-Length header based framing
	contentLength := 0
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				contentLength, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}

	if contentLength <= 0 {
		return nil, fmt.Errorf("invalid or missing Content-Length")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.in, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Server) sendResponse(id any, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)

	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func (s *Server) sendError(id any, code int, msg string, data any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	}
	bytes, _ := json.Marshal(resp)

	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(bytes), bytes)
}

func (s *Server) sendNotification(method string, params any) {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(notif)

	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func (s *Server) handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		var params InitializeParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}

		supported := make([]string, 0)
		for _, p := range s.registry.List() {
			supported = append(supported, p.ID)
		}

		res := InitializeResult{
			ProtocolVersion: "2026-01-01",
			ServerInfo: ServerInfo{
				Name:    "artix-acp",
				Version: "1.0.0",
			},
			Capabilities: AgentCapabilities{
				Planning:          true,
				Coding:            true,
				Review:            true,
				WorktreeIsolation: true,
				SupportedPersonas: supported,
				StreamEvents:      true,
			},
		}
		s.sendResponse(req.ID, res)

	case "initialized":
		// No response required for notification

	case "session/new":
		var params SessionNewParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}

		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = fmt.Sprintf("acp-sess-%d-%d", time.Now().Unix(), atomic.AddUint64(&sessionCounter, 1))
		}

		root := params.RootDir
		if root == "" {
			root = s.rootDir
		}

		domain := params.Domain
		if domain == "" {
			domain = "backend_engineer"
		}

		s.sessionsMu.Lock()
		s.sessions[sessionID] = &sessionState{
			id:        sessionID,
			rootDir:   root,
			domain:    domain,
			createdAt: time.Now(),
		}
		s.sessionsMu.Unlock()

		s.sendResponse(req.ID, SessionNewResult{
			SessionID: sessionID,
			Status:    "ready",
		})

	case "session/prompt":
		var params SessionPromptParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}

		if params.SessionID == "" {
			s.sendError(req.ID, -32602, "Missing session ID", nil)
			return
		}

		s.sessionsMu.RLock()
		sess, ok := s.sessions[params.SessionID]
		s.sessionsMu.RUnlock()
		if !ok {
			s.sendError(req.ID, -32602, fmt.Sprintf("Session %q not found", params.SessionID), nil)
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		s.inFlightMu.Lock()
		s.inFlightCancels[params.SessionID] = cancel
		s.inFlightMu.Unlock()
		defer func() {
			s.inFlightMu.Lock()
			delete(s.inFlightCancels, params.SessionID)
			s.inFlightMu.Unlock()
		}()

		mode := strings.ToLower(params.Mode)
		if mode == "" || mode == "auto" {
			if strings.HasPrefix(strings.ToLower(params.Prompt), "plan ") || strings.Contains(strings.ToLower(params.Prompt), "story:") {
				mode = "plan"
			} else if strings.HasPrefix(strings.ToLower(params.Prompt), "review") {
				mode = "review"
			} else {
				mode = "plan"
			}
		}

		switch mode {
		case "plan":
			s.handlePlanPrompt(ctx, req.ID, sess, params)
		case "review":
			s.handleReviewPrompt(ctx, req.ID, sess, params)
		default:
			s.handlePlanPrompt(ctx, req.ID, sess, params)
		}

	case "session/cancel":
		var params struct {
			SessionID string `json:"sessionId"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}

		s.inFlightMu.Lock()
		if cancel, ok := s.inFlightCancels[params.SessionID]; ok {
			cancel()
			delete(s.inFlightCancels, params.SessionID)
		}
		s.inFlightMu.Unlock()

		s.sendResponse(req.ID, map[string]any{"cancelled": true})

	case "shutdown":
		s.Close()
		s.sendResponse(req.ID, map[string]any{"ok": true})

	case "exit":
		s.Close()

	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("Method %q not found", req.Method), nil)
	}
}

func (s *Server) handlePlanPrompt(ctx context.Context, reqID any, sess *sessionState, params SessionPromptParams) {
	s.sendNotification("session/progress", SessionProgressNotification{
		SessionID: sess.id,
		Phase:     "PLANNING",
		Status:    "in_progress",
		Message:   fmt.Sprintf("Starting deliberation on story: %s", params.Prompt),
		Timestamp: time.Now().UTC(),
	})

	council := spec.NewCouncil(s.registry)

	council.SetStreamHandler(func(ev spec.CouncilStreamEvent) {
		s.sendNotification("session/progress", SessionProgressNotification{
			SessionID: sess.id,
			Phase:     "PLANNING",
			Round:     ev.Round,
			MaxRounds: ev.TotalRounds,
			Status:    ev.Status,
			Message:   ev.Message,
			Timestamp: ev.Timestamp,
			Payload:   ev.Payload,
		})
	})

	repoCtx, _ := repo.DetectContext(sess.rootDir)
	pCtx := &spec.PlanningContext{
		StoryPrompt: params.Prompt,
		RepoContext: repoCtx,
		Style:       spec.StyleStandard,
	}

	storySpec, err := council.Plan(ctx, pCtx)
	if err != nil {
		s.sendError(reqID, -32000, "Planning failed", err.Error())
		return
	}

	specsDir := filepath.Join(sess.rootDir, "docs", "specs")
	specPath, _, _ := spec.WriteSpecWithProvenance(specsDir, storySpec, nil)

	s.sendResponse(reqID, SessionPromptResult{
		SessionID: sess.id,
		Status:    "completed",
		SpecID:    storySpec.ID,
		Summary:   fmt.Sprintf("Spec %s: %s (%d scenarios)", storySpec.ID, storySpec.Title, len(storySpec.AcceptanceCriteria)),
		Output:    specPath,
		Approved:  true,
	})
}

func (s *Server) handleReviewPrompt(ctx context.Context, reqID any, sess *sessionState, params SessionPromptParams) {
	s.sendNotification("session/progress", SessionProgressNotification{
		SessionID: sess.id,
		Phase:     "REVIEW",
		Status:    "in_progress",
		Message:   "Reviewing working tree against taboo rules and acceptance criteria",
		Timestamp: time.Now().UTC(),
	})

	advReviewer := reviewer.NewAdversarialReviewer(s.registry)
	testCount := reviewer.CountTestsInWorkspace(sess.rootDir)
	rCtx := &reviewer.ReviewContext{
		WorkspaceDir:    sess.rootDir,
		TestCountBefore: testCount,
		TestCountAfter:  testCount,
	}

	verdict := advReviewer.Evaluate(rCtx)

	s.sendResponse(reqID, SessionPromptResult{
		SessionID: sess.id,
		Status:    string(verdict.Status),
		Summary:   verdict.Summary,
		Approved:  verdict.Approved,
	})
}
