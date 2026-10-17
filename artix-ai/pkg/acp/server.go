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

	"artix/pkg/audit"
	"artix/pkg/coder"
	"artix/pkg/gate"
	"artix/pkg/git"
	"artix/pkg/persona"
	"artix/pkg/policy"
	"artix/pkg/repo"
	"artix/pkg/reviewer"
	"artix/pkg/spec"
)

var sessionCounter uint64

// Server implements the Agent Communication Protocol (ACP) standard JSON-RPC 2.0 server.
type Server struct {
	rootDir         string
	allowedRoots    []string
	budget          *coder.TokenBudget
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
	maxTokens int
	maxUSD    float64
	createdAt time.Time
}

// SetAllowedRoots configures allowed workspace directory roots.
func (s *Server) SetAllowedRoots(roots []string) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.allowedRoots = roots
}

// SetBudget configures the token budget for planning and reviews.
func (s *Server) SetBudget(b *coder.TokenBudget) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.budget = b
}

func (s *Server) validateRootDir(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		trimmed = s.rootDir
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "file:") {
		return "", fmt.Errorf("invalid rootDir: file:// scheme prohibited")
	}
	clean := filepath.Clean(trimmed)
	evalClean, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("invalid rootDir %q: %w", clean, err)
	}
	clean = filepath.Clean(evalClean)

	s.sessionsMu.RLock()
	roots := s.allowedRoots
	s.sessionsMu.RUnlock()
	if len(roots) == 0 {
		return "", fmt.Errorf("prohibited rootDir %q: outside allowed roots", clean)
	}
	allowed := false
	for _, ar := range roots {
		evalAR, err := filepath.EvalSymlinks(ar)
		if err != nil {
			evalAR = filepath.Clean(ar)
		} else {
			evalAR = filepath.Clean(evalAR)
		}
		rel, err := filepath.Rel(evalAR, clean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("prohibited rootDir %q: outside allowed roots", clean)
	}
	return clean, nil
}

// NewServer initializes an ACP server for standard I/O communication.
func NewServer(rootDir string, reg *persona.Registry, in io.Reader, out io.Writer) *Server {
	if reg == nil {
		reg = persona.NewRegistry(rootDir)
	}
	return &Server{
		rootDir:         rootDir,
		allowedRoots:    []string{rootDir},
		budget:          coder.LoadBudgetFromEnv(),
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

const MaxACPMessageBytes = 8 * 1024 * 1024 // 8 MiB

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
		if len(line) > MaxACPMessageBytes {
			return nil, fmt.Errorf("line frame exceeds maximum allowed limit of %d bytes", MaxACPMessageBytes)
		}
		return bytes.TrimSpace(line), nil
	}

	// Content-Length header based framing
	contentLength := 0
	headerBytes := 0
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		headerBytes += len(line)
		if headerBytes > 64*1024 {
			return nil, fmt.Errorf("header size exceeds maximum limit")
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

	if contentLength <= 0 || contentLength > MaxACPMessageBytes {
		return nil, fmt.Errorf("invalid Content-Length: %d (must be between 1 and %d bytes)", contentLength, MaxACPMessageBytes)
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

		root := params.RootDir
		if root == "" {
			root = s.rootDir
		}
		validatedRoot, err := s.validateRootDir(root)
		if err != nil {
			s.sendError(req.ID, -32602, err.Error(), nil)
			return
		}

		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = fmt.Sprintf("acp-sess-%d-%d", time.Now().Unix(), atomic.AddUint64(&sessionCounter, 1))
		}

		domain := params.Domain
		if domain == "" {
			domain = "backend_engineer"
		}

		s.sessionsMu.Lock()
		s.sessions[sessionID] = &sessionState{
			id:        sessionID,
			rootDir:   validatedRoot,
			domain:    domain,
			maxTokens: params.MaxTokens,
			maxUSD:    params.MaxUSD,
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

		validatedRoot, err := s.validateRootDir(sess.rootDir)
		if err != nil {
			s.sendError(req.ID, -32602, err.Error(), nil)
			return
		}
		sess.rootDir = validatedRoot

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

func (s *Server) emitAudit(rootDir string, ev audit.AuditEvent) error {
	err := audit.Default(rootDir).Emit(ev)
	if err != nil && policy.IsEnterprise() {
		return fmt.Errorf("enterprise audit emission failed: %w", err)
	}
	return nil
}

func (s *Server) resolveBudget(sess *sessionState, params SessionPromptParams) *coder.TokenBudget {
	s.sessionsMu.RLock()
	orig := s.budget
	s.sessionsMu.RUnlock()
	if orig == nil {
		orig = coder.LoadBudgetFromEnv()
	}

	var b *coder.TokenBudget
	if orig != nil {
		bCopy := *orig
		b = &bCopy
	}

	maxTokens := params.MaxTokens
	if maxTokens <= 0 {
		maxTokens = sess.maxTokens
	}
	maxUSD := params.MaxUSD
	if maxUSD <= 0 {
		maxUSD = sess.maxUSD
	}

	if b == nil {
		if maxTokens > 0 || maxUSD > 0 {
			b = &coder.TokenBudget{
				MaxStoryTokens: maxTokens,
				MaxStoryCost:   maxUSD,
			}
		}
	} else {
		// Client values may ONLY LOWER the cap; never elevate or mutate shared budget
		if maxTokens > 0 {
			if b.MaxStoryTokens <= 0 || maxTokens < b.MaxStoryTokens {
				b.MaxStoryTokens = maxTokens
			}
		}
		if maxUSD > 0 {
			if b.MaxStoryCost <= 0 || maxUSD < b.MaxStoryCost {
				b.MaxStoryCost = maxUSD
			}
		}
	}
	return b
}

func (s *Server) handlePlanPrompt(ctx context.Context, reqID any, sess *sessionState, params SessionPromptParams) {
	if _, err := gate.Pass(ctx, gate.Request{
		EntryPoint: gate.EntryPointACPSession,
		Actor:      "acp-client",
		ProjectDir: sess.rootDir,
		Prompt:     params.Prompt,
	}); err != nil {
		s.sendError(reqID, -32000, fmt.Sprintf("gate rejected session: %v", err), nil)
		return
	}

	s.sendNotification("session/progress", SessionProgressNotification{
		SessionID: sess.id,
		Phase:     "PLANNING",
		Status:    "in_progress",
		Message:   fmt.Sprintf("Starting deliberation on story: %s", params.Prompt),
		Timestamp: time.Now().UTC(),
	})

	b := s.resolveBudget(sess, params)
	if b != nil {
		if exhausted, reason := b.CheckExhaustion(); exhausted {
			if err := s.emitAudit(sess.rootDir, audit.AuditEvent{
				EventType: audit.EventBudgetExhausted,
				Status:    "FAILED",
				Details: map[string]any{
					"reason":  reason,
					"context": "acp.plan",
				},
			}); err != nil {
				s.sendError(reqID, -32000, "Enterprise audit failure", err.Error())
				return
			}
			s.sendError(reqID, -32000, fmt.Sprintf("budget exhausted: %s", reason), nil)
			return
		}
	}

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
		_ = s.emitAudit(sess.rootDir, audit.AuditEvent{
			EventType: audit.EventSpecDeliberation,
			Status:    "FAILED",
			Details: map[string]any{
				"error":  err.Error(),
				"prompt": params.Prompt,
			},
		})
		s.sendError(reqID, -32000, "Planning failed", err.Error())
		return
	}

	usedTokens := coder.EstimateTokens(params.Prompt) + coder.EstimateTokens(fmt.Sprintf("%+v", storySpec))
	var costUSD float64
	if b != nil {
		price, _ := b.GetModelPrice(params.Model)
		costUSD = (float64(usedTokens) / 1000.0) * price
		b.RecordRoundUsage(usedTokens, costUSD)
		if exhausted, reason := b.CheckExhaustion(); exhausted {
			_ = s.emitAudit(sess.rootDir, audit.AuditEvent{
				EventType: audit.EventBudgetExhausted,
				Status:    "FAILED",
				Details: map[string]any{
					"reason":  reason,
					"context": "acp.plan",
				},
			})
			s.sendError(reqID, -32000, fmt.Sprintf("budget exhausted: %s", reason), nil)
			return
		}
	}

	if err := s.emitAudit(sess.rootDir, audit.AuditEvent{
		EventType: audit.EventSpecDeliberation,
		Status:    "SUCCESS",
		Cost:      costUSD,
		Details: map[string]any{
			"tokens":    usedTokens,
			"specId":    storySpec.ID,
			"title":     storySpec.Title,
			"scenarios": len(storySpec.AcceptanceCriteria),
		},
	}); err != nil {
		s.sendError(reqID, -32000, "Enterprise audit failure", err.Error())
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

	b := s.resolveBudget(sess, params)
	if b != nil {
		if exhausted, reason := b.CheckExhaustion(); exhausted {
			if err := s.emitAudit(sess.rootDir, audit.AuditEvent{
				EventType: audit.EventBudgetExhausted,
				Status:    "FAILED",
				Details: map[string]any{
					"reason":  reason,
					"context": "acp.review",
				},
			}); err != nil {
				s.sendError(reqID, -32000, "Enterprise audit failure", err.Error())
				return
			}
			s.sendError(reqID, -32000, fmt.Sprintf("budget exhausted: %s", reason), nil)
			return
		}
	}

	// Read diff from git
	gDriver := git.NewDriver(sess.rootDir)
	diff, _ := gDriver.Diff(false)
	if strings.TrimSpace(diff) == "" {
		stagedDiff, _ := gDriver.Diff(true)
		diff = stagedDiff
	}

	// Empty-diff reviews are not approved!
	if strings.TrimSpace(diff) == "" {
		verdictSummary := "empty diff: no changes found to review"
		if err := s.emitAudit(sess.rootDir, audit.AuditEvent{
			EventType: audit.EventReviewerVerdict,
			Status:    "REJECTED",
			Details: map[string]any{
				"summary":        verdictSummary,
				"blockingIssues": []string{"Workspace diff is empty; no code changes were produced."},
				"approved":       false,
			},
		}); err != nil {
			s.sendError(reqID, -32000, "Enterprise audit failure", err.Error())
			return
		}
		s.sendResponse(reqID, SessionPromptResult{
			SessionID: sess.id,
			Status:    "REJECTED",
			Summary:   verdictSummary,
			Approved:  false,
		})
		return
	}

	advReviewer := reviewer.NewAdversarialReviewer(s.registry)
	testCount := reviewer.CountTestsInWorkspace(sess.rootDir)
	rCtx := &reviewer.ReviewContext{
		WorkspaceDir:    sess.rootDir,
		Diff:            diff,
		TestCountBefore: testCount,
		TestCountAfter:  testCount,
	}

	verdict := advReviewer.Evaluate(rCtx)

	reviewTokens := coder.EstimateTokens(diff)
	var costUSD float64
	if b != nil {
		price, _ := b.GetModelPrice(params.Model)
		costUSD = (float64(reviewTokens) / 1000.0) * price
		b.RecordRoundUsage(reviewTokens, costUSD)
		if exhausted, reason := b.CheckExhaustion(); exhausted {
			_ = s.emitAudit(sess.rootDir, audit.AuditEvent{
				EventType: audit.EventBudgetExhausted,
				Status:    "FAILED",
				Details: map[string]any{
					"reason":  reason,
					"context": "acp.review",
				},
			})
			s.sendError(reqID, -32000, fmt.Sprintf("budget exhausted: %s", reason), nil)
			return
		}
	}

	status := "SUCCESS"
	if !verdict.Approved {
		status = "REJECTED"
	}
	if err := s.emitAudit(sess.rootDir, audit.AuditEvent{
		EventType: audit.EventReviewerVerdict,
		Status:    status,
		Cost:      costUSD,
		Details: map[string]any{
			"tokens":         reviewTokens,
			"summary":        verdict.Summary,
			"blockingIssues": verdict.BlockingIssues,
			"warnings":       verdict.Warnings,
			"approved":       verdict.Approved,
		},
	}); err != nil {
		s.sendError(reqID, -32000, "Enterprise audit failure", err.Error())
		return
	}

	s.sendResponse(reqID, SessionPromptResult{
		SessionID: sess.id,
		Status:    string(verdict.Status),
		Summary:   verdict.Summary,
		Approved:  verdict.Approved,
	})
}
