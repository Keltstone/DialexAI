package forge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"artix/pkg/audit"
	"artix/pkg/knowledge"
	"artix/pkg/pilot"
	"artix/pkg/policy"
	"artix/pkg/steering"
)

// WebhookServerConfig defines configuration for the webhook daemon.
type WebhookServerConfig struct {
	ListenAddr          string
	GitHubSecret        string
	GitLabToken         string
	RequireWebhookAuth  bool
	AllowedCloneHosts   []string
	JobsAuthToken       string
	TriggerLabel        string
	AllowedUsers        []string
	DefaultDomain       string
	Worker              *RemoteWorker
	MaxConcurrentJobs   int
	MaxQueuedJobs       int
	MaxBodyLength       int
	JobTimeout          time.Duration
	StoragePath         string
	OfflineCacheDir     string
	WorkspaceDir        string
	PilotStopRuleConfig pilot.PilotStopRuleConfig
}

// JobStatus tracks the state of an asynchronous worker run.
type JobStatus struct {
	ID        string              `json:"id"`
	Source    string              `json:"source"`
	Tenant    string              `json:"tenant"`
	Status    string              `json:"status"` // "queued", "running", "completed", "failed"
	CreatedAt time.Time           `json:"createdAt"`
	Result    *RemoteWorkerResult `json:"result,omitempty"`
	Error     string              `json:"error,omitempty"`
}

type daemonPersistentState struct {
	Jobs        map[string]*JobStatus `json:"jobs"`
	Deliveries  map[string]time.Time  `json:"deliveries"`
}

// WebhookServer handles incoming VCS webhook events and executes remote jobs.
type WebhookServer struct {
	cfg                 WebhookServerConfig
	jobs                map[string]*JobStatus
	jobsMu              sync.RWMutex
	sem                 chan struct{}
	processedDeliveries map[string]time.Time
	deliveriesMu        sync.Mutex
	branchLocks         map[string]*sync.Mutex
	branchLocksMu       sync.Mutex
	storageMu           sync.Mutex
}

// NewWebhookServer creates a new webhook server.
func NewWebhookServer(cfg WebhookServerConfig) *WebhookServer {
	if cfg.DefaultDomain == "" {
		cfg.DefaultDomain = "backend_engineer"
	}
	if cfg.JobTimeout <= 0 {
		if envTimeout := os.Getenv("ARTIX_JOB_TIMEOUT"); envTimeout != "" {
			if d, err := time.ParseDuration(envTimeout); err == nil && d > 0 {
				cfg.JobTimeout = d
			}
		}
		if cfg.JobTimeout <= 0 {
			cfg.JobTimeout = 10 * time.Minute
		}
	}
	if cfg.StoragePath == "" {
		if envStorage := os.Getenv("ARTIX_DAEMON_STORAGE"); envStorage != "" {
			cfg.StoragePath = envStorage
		}
	}
	if cfg.MaxConcurrentJobs <= 0 {
		if envVal := os.Getenv("ARTIX_MAX_CONCURRENT_JOBS"); envVal != "" {
			if n, err := strconv.Atoi(envVal); err == nil && n > 0 {
				cfg.MaxConcurrentJobs = n
			}
		}
		if cfg.MaxConcurrentJobs <= 0 {
			cfg.MaxConcurrentJobs = 4
		}
	}
	if cfg.OfflineCacheDir == "" {
		if envCache := os.Getenv("ARTIX_OFFLINE_CACHE_DIR"); envCache != "" {
			cfg.OfflineCacheDir = envCache
		}
	}
	if cfg.MaxQueuedJobs <= 0 {
		if envVal := os.Getenv("ARTIX_MAX_QUEUED_JOBS"); envVal != "" {
			if n, err := strconv.Atoi(envVal); err == nil && n > 0 {
				cfg.MaxQueuedJobs = n
			}
		}
		if cfg.MaxQueuedJobs <= 0 {
			cfg.MaxQueuedJobs = 16
		}
	}

	s := &WebhookServer{
		cfg:                 cfg,
		jobs:                make(map[string]*JobStatus),
		sem:                 make(chan struct{}, cfg.MaxConcurrentJobs),
		processedDeliveries: make(map[string]time.Time),
		branchLocks:         make(map[string]*sync.Mutex),
	}

	// Load persisted state if configured
	s.loadState()

	return s
}

// Handler returns the http.Handler for routing webhook requests.
func (s *WebhookServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/webhook/github", s.handleGitHubWebhook)
	mux.HandleFunc("/webhook/gitlab", s.handleGitLabWebhook)
	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/tasks", s.handleTasks)
	return mux
}

func (s *WebhookServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *WebhookServer) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Validate secret if configured or in enterprise/authenticated mode
	if s.cfg.RequireWebhookAuth || policy.IsEnterprise() {
		if s.cfg.GitHubSecret == "" {
			http.Error(w, "webhook secret configuration is required in enterprise / authenticated mode", http.StatusForbidden)
			return
		}
	}
	if s.cfg.GitHubSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(s.cfg.GitHubSecret, sig, body) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Delivery ID deduplication
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID != "" && s.isDuplicateDelivery(deliveryID) {
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusConflict
		if s.cfg.StoragePath != "" {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ignored",
			"error":  "duplicate delivery ID",
			"msg":    "duplicate delivery ID",
		})
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "ping" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"pong"}`))
		return
	}

	maxBodyLen := s.cfg.MaxBodyLength
	if maxBodyLen <= 0 {
		maxBodyLen = 65536
	}

	if eventType == "pull_request_review_comment" || eventType == "pull_request_review" {
		var prReviewEvent struct {
			Action      string `json:"action"`
			PullRequest struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
				URL    string `json:"html_url"`
			} `json:"pull_request"`
			Comment struct {
				Body              string `json:"body"`
				AuthorAssociation string `json:"author_association"`
				HTMLURL           string `json:"html_url"`
				User              struct {
					Login string `json:"login"`
				} `json:"user"`
			} `json:"comment"`
			Review struct {
				Body              string `json:"body"`
				AuthorAssociation string `json:"author_association"`
				HTMLURL           string `json:"html_url"`
				User              struct {
					Login string `json:"login"`
				} `json:"user"`
			} `json:"review"`
			Sender struct {
				Login             string `json:"login"`
				AuthorAssociation string `json:"author_association"`
			} `json:"sender"`
			Repository struct {
				CloneURL string `json:"clone_url"`
				Name     string `json:"name"`
				Owner    struct {
					Login string `json:"login"`
				} `json:"owner"`
				DefaultBranch string `json:"default_branch"`
			} `json:"repository"`
		}

		if err := json.Unmarshal(body, &prReviewEvent); err != nil {
			http.Error(w, "invalid json payload", http.StatusBadRequest)
			return
		}

		if prReviewEvent.Action != "created" && prReviewEvent.Action != "submitted" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"msg":"ignored pr review action"}`))
			return
		}

		commentBody := strings.TrimSpace(prReviewEvent.Comment.Body)
		author := prReviewEvent.Comment.User.Login
		commentURL := prReviewEvent.Comment.HTMLURL

		if commentBody == "" && prReviewEvent.Review.Body != "" {
			commentBody = strings.TrimSpace(prReviewEvent.Review.Body)
			author = prReviewEvent.Review.User.Login
			commentURL = prReviewEvent.Review.HTMLURL
		}

		if author == "" {
			author = prReviewEvent.Sender.Login
		}

		if commentBody == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"msg":"empty review comment body"}`))
			return
		}

		if len(commentBody) > maxBodyLen {
			http.Error(w, fmt.Sprintf("rejected: comment body length %d exceeds maximum cap of %d bytes", len(commentBody), maxBodyLen), http.StatusRequestEntityTooLarge)
			return
		}

		// Check prompt injection in review comment
		if detections := steering.ScanPromptInjection(commentBody); len(detections) > 0 {
			http.Error(w, fmt.Sprintf("rejected: untrusted comment text contains prompt injection or policy/steering instructions: %s", strings.Join(detections, "; ")), http.StatusBadRequest)
			return
		}

		// Synthesize rule from PR comment
		synth := knowledge.NewRuleSynthesizer()
		prov := knowledge.Provenance{
			Author:       author,
			PRCommentURL: commentURL,
			SessionID:    fmt.Sprintf("pr-%d", prReviewEvent.PullRequest.Number),
		}
		rule, err := synth.SynthesizeFromCommentWithProvenance(commentBody, prov)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to synthesize rule: %v", err), http.StatusBadRequest)
			return
		}

		// Save synthesized rule in knowledge store as proposed
		workDir := s.cfg.StoragePath
		if workDir == "" && s.cfg.Worker != nil {
			workDir = s.cfg.Worker.WorkRoot()
		}
		if workDir == "" {
			workDir = filepath.Join(os.TempDir(), "artix-worker")
		}

		kStore := knowledge.NewStore(workDir)
		ki := &knowledge.KnowledgeItem{
			ID:           rule.RuleID,
			Title:        rule.Name,
			Category:     knowledge.CategoryArchitecture,
			Breakthrough: rule.RuleText,
			Context:      fmt.Sprintf("Synthesized from PR review comment by %s on PR #%d", author, prReviewEvent.PullRequest.Number),
			Author:       author,
			SourcePR:     fmt.Sprintf("%d", prReviewEvent.PullRequest.Number),
			PRCommentURL: commentURL,
			Status:       "proposed", // Proposed: requires artix knowledge ratify <id>
			TTL:          rule.TTL,
			CreatedAt:    rule.CreatedAt,
			ExpiresAt:    rule.ExpiresAt,
		}
		if err := kStore.Save(ki); err != nil {
			http.Error(w, fmt.Sprintf("failed to persist synthesized rule: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "proposed",
			"ruleId": rule.RuleID,
			"rule":   rule,
			"msg":    "PR review comment synthesized into proposed rule; requires 'artix knowledge ratify <ruleId>' to activate",
		})
		return
	}

	if eventType == "issue_comment" {
		var commentEvent struct {
			Action string `json:"action"`
			Issue  struct {
				Title             string `json:"title"`
				Body              string `json:"body"`
				AuthorAssociation string `json:"author_association"`
				User              struct {
					Login string `json:"login"`
				} `json:"user"`
			} `json:"issue"`
			Comment struct {
				Body              string `json:"body"`
				AuthorAssociation string `json:"author_association"`
				User              struct {
					Login string `json:"login"`
				} `json:"user"`
			} `json:"comment"`
			Sender struct {
				Login             string `json:"login"`
				AuthorAssociation string `json:"author_association"`
			} `json:"sender"`
			Repository struct {
				CloneURL string `json:"clone_url"`
				Name     string `json:"name"`
				Owner    struct {
					Login string `json:"login"`
				} `json:"owner"`
				DefaultBranch string `json:"default_branch"`
			} `json:"repository"`
		}

		if err := json.Unmarshal(body, &commentEvent); err != nil {
			http.Error(w, "invalid json payload", http.StatusBadRequest)
			return
		}

		if commentEvent.Action != "created" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"msg":"ignored comment action"}`))
			return
		}

		if !strings.Contains(commentEvent.Comment.Body, "/artix") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"msg":"ignored comment: missing /artix command"}`))
			return
		}

		if len(commentEvent.Comment.Body) > maxBodyLen || len(commentEvent.Issue.Title) > 1024 {
			http.Error(w, fmt.Sprintf("rejected: comment body length %d exceeds maximum cap of %d bytes", len(commentEvent.Comment.Body), maxBodyLen), http.StatusRequestEntityTooLarge)
			return
		}

		// Authorization checks: issue author, comment author, and trigger sender must all be authorized
		if !s.isUserAuthorized(commentEvent.Issue.User.Login, commentEvent.Issue.AuthorAssociation) {
			http.Error(w, "unauthorized issue author: parent issue author is not authorized", http.StatusForbidden)
			return
		}
		if !s.isUserAuthorized(commentEvent.Comment.User.Login, commentEvent.Comment.AuthorAssociation) {
			http.Error(w, "unauthorized comment author: /artix command author is not authorized", http.StatusForbidden)
			return
		}
		senderAssoc := commentEvent.Sender.AuthorAssociation
		if senderAssoc == "" {
			senderAssoc = commentEvent.Comment.AuthorAssociation
		}
		if !s.isUserAuthorized(commentEvent.Sender.Login, senderAssoc) {
			http.Error(w, "unauthorized sender: trigger sender is not authorized", http.StatusForbidden)
			return
		}

		untrustedContent := commentEvent.Comment.Body + "\n" + commentEvent.Issue.Body
		if detections := steering.ScanPromptInjection(untrustedContent); len(detections) > 0 {
			http.Error(w, fmt.Sprintf("rejected: untrusted comment text contains prompt injection or policy/steering instructions: %s", strings.Join(detections, "; ")), http.StatusBadRequest)
			return
		}

		if !s.canAcceptJob() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":  "concurrency cap or queue limit exceeded",
				"status": "rate_limited",
			})
			return
		}

		jobID := fmt.Sprintf("gh-comment-%d", time.Now().UnixNano())
		target := RemoteRepoTarget{
			CloneURL: commentEvent.Repository.CloneURL,
			Owner:    commentEvent.Repository.Owner.Login,
			Repo:     commentEvent.Repository.Name,
			Branch:   commentEvent.Repository.DefaultBranch,
		}
		task := &RemoteWorkerTask{
			Target: target,
			Auth:   ForgeAuth{Type: ForgeGitHub},
			Prompt: fmt.Sprintf("%s\n\n%s\n\nCommand: %s", commentEvent.Issue.Title, commentEvent.Issue.Body, commentEvent.Comment.Body),
			Domain: s.cfg.DefaultDomain,
		}
		s.startJob(jobID, "github", target.Owner, task)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "queued",
			"jobId":  jobID,
		})
		return
	}

	// Parse issues event
	var event struct {
		Action string `json:"action"`
		Issue  struct {
			Title             string `json:"title"`
			Body              string `json:"body"`
			AuthorAssociation string `json:"author_association"`
			User              struct {
				Login string `json:"login"`
			} `json:"user"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"issue"`
		Sender struct {
			Login             string `json:"login"`
			AuthorAssociation string `json:"author_association"`
		} `json:"sender"`
		Repository struct {
			CloneURL string `json:"clone_url"`
			Name     string `json:"name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	if event.Action != "opened" && event.Action != "labeled" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"ignored event action"}`))
		return
	}

	if len(event.Issue.Body) > maxBodyLen || len(event.Issue.Title) > 1024 {
		http.Error(w, fmt.Sprintf("rejected: issue body length %d exceeds maximum cap of %d bytes", len(event.Issue.Body), maxBodyLen), http.StatusRequestEntityTooLarge)
		return
	}

	// Author & Sender authorization check:
	// 1. Issue author MUST be authorized (the author created the prompt/instructions).
	// 2. Sender MUST also be authorized (the sender triggered/labeled the event).
	issueAuthor := event.Issue.User.Login
	if !s.isUserAuthorized(issueAuthor, event.Issue.AuthorAssociation) {
		http.Error(w, "unauthorized issue author: prompt author is not an authorized member/owner or in allowedUsers", http.StatusForbidden)
		return
	}

	senderLogin := event.Sender.Login
	if senderLogin == "" {
		senderLogin = issueAuthor
	}
	senderAssoc := event.Sender.AuthorAssociation
	if senderAssoc == "" {
		if senderLogin == issueAuthor || event.Issue.User.Login == "" {
			senderAssoc = event.Issue.AuthorAssociation
		}
	}
	if !s.isUserAuthorized(senderLogin, senderAssoc) {
		http.Error(w, "unauthorized sender: event trigger sender is not authorized", http.StatusForbidden)
		return
	}

	// Treat issue text as untrusted: run sanitizer and reject any policy/steering injection
	untrustedContent := event.Issue.Title + "\n" + event.Issue.Body
	if detections := steering.ScanPromptInjection(untrustedContent); len(detections) > 0 {
		http.Error(w, fmt.Sprintf("rejected: untrusted issue text contains prompt injection or policy/steering instructions: %s", strings.Join(detections, "; ")), http.StatusBadRequest)
		return
	}

	// Trigger requirement: require explicit label (artix, artix:run, or s.cfg.TriggerLabel) or command (/artix)
	triggerLabel := s.cfg.TriggerLabel
	if triggerLabel == "" {
		triggerLabel = "artix"
	}
	hasLabel := false
	for _, l := range event.Issue.Labels {
		if strings.EqualFold(l.Name, triggerLabel) || strings.EqualFold(l.Name, "artix:run") || strings.EqualFold(l.Name, "artix") || strings.EqualFold(l.Name, "kritix") {
			hasLabel = true
			break
		}
	}
	hasCommand := strings.Contains(event.Issue.Title, "/artix") || strings.Contains(event.Issue.Body, "/artix")

	if !hasLabel && !hasCommand {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"ignored event: issue requires trigger label or /artix command"}`))
		return
	}

	if !s.canAcceptJob() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":  "concurrency cap or queue limit exceeded",
			"status": "rate_limited",
		})
		return
	}

	jobID := fmt.Sprintf("gh-%d", time.Now().UnixNano())
	target := RemoteRepoTarget{
		CloneURL: event.Repository.CloneURL,
		Owner:    event.Repository.Owner.Login,
		Repo:     event.Repository.Name,
		Branch:   event.Repository.DefaultBranch,
	}

	task := &RemoteWorkerTask{
		Target: target,
		Auth: ForgeAuth{
			Type: ForgeGitHub,
		},
		Prompt: fmt.Sprintf("%s\n\n%s", event.Issue.Title, event.Issue.Body),
		Domain: s.cfg.DefaultDomain,
	}

	s.startJob(jobID, "github", target.Owner, task)

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"jobId":  jobID,
		"status": "queued",
	})
}

func (s *WebhookServer) handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.RequireWebhookAuth || policy.IsEnterprise() {
		if s.cfg.GitLabToken == "" {
			http.Error(w, "gitlab webhook token configuration is required in enterprise / authenticated mode", http.StatusForbidden)
			return
		}
	}

	if s.cfg.GitLabToken != "" {
		token := r.Header.Get("X-Gitlab-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.GitLabToken)) != 1 {
			http.Error(w, "invalid gitlab token", http.StatusUnauthorized)
			return
		}
	}

	// Delivery ID deduplication for GitLab
	deliveryID := r.Header.Get("X-Gitlab-Event-UUID")
	if deliveryID != "" && s.isDuplicateDelivery(deliveryID) {
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusConflict
		if s.cfg.StoragePath != "" {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ignored",
			"error":  "duplicate delivery ID",
			"msg":    "duplicate delivery ID",
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var event struct {
		ObjectKind string `json:"object_kind"`
		Project    struct {
			GitHTTPURL        string `json:"git_http_url"`
			PathWithNamespace string `json:"path_with_namespace"`
			DefaultBranch     string `json:"default_branch"`
		} `json:"project"`
		ObjectAttributes struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Action      string `json:"action"`
			Labels      []struct {
				Title string `json:"title"`
			} `json:"labels"`
		} `json:"object_attributes"`
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	if event.ObjectKind != "issue" || (event.ObjectAttributes.Action != "open" && event.ObjectAttributes.Action != "reopen") {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"ignored event"}`))
		return
	}

	// Cap issue description and title length
	maxBodyLen := s.cfg.MaxBodyLength
	if maxBodyLen <= 0 {
		maxBodyLen = 65536
	}
	if len(event.ObjectAttributes.Description) > maxBodyLen || len(event.ObjectAttributes.Title) > 1024 {
		http.Error(w, fmt.Sprintf("rejected: issue description length %d exceeds maximum cap of %d bytes", len(event.ObjectAttributes.Description), maxBodyLen), http.StatusRequestEntityTooLarge)
		return
	}

	// User authorization check if AllowedUsers is configured or in enterprise mode
	if len(s.cfg.AllowedUsers) > 0 {
		allowed := false
		for _, u := range s.cfg.AllowedUsers {
			if strings.EqualFold(u, event.User.Username) {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "unauthorized sender: user not in allowedUsers", http.StatusForbidden)
			return
		}
	} else if policy.IsEnterprise() {
		http.Error(w, "unauthorized sender: unverifiable sender in enterprise mode", http.StatusForbidden)
		return
	}

	// Treat issue text as untrusted: run sanitizer and reject any policy/steering injection
	untrustedContent := event.ObjectAttributes.Title + "\n" + event.ObjectAttributes.Description
	if detections := steering.ScanPromptInjection(untrustedContent); len(detections) > 0 {
		http.Error(w, fmt.Sprintf("rejected: untrusted issue text contains prompt injection or policy/steering instructions: %s", strings.Join(detections, "; ")), http.StatusBadRequest)
		return
	}

	// Trigger requirement: label or command
	hasLabel := false
	triggerLabel := s.cfg.TriggerLabel
	if triggerLabel == "" {
		triggerLabel = "artix"
	}
	for _, l := range event.ObjectAttributes.Labels {
		if strings.EqualFold(l.Title, triggerLabel) || strings.EqualFold(l.Title, "artix:run") || strings.EqualFold(l.Title, "artix") || strings.EqualFold(l.Title, "kritix") {
			hasLabel = true
			break
		}
	}
	hasCommand := strings.Contains(event.ObjectAttributes.Title, "/artix") || strings.Contains(event.ObjectAttributes.Description, "/artix")

	if !hasLabel && !hasCommand {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"msg":"ignored event: issue requires trigger label or /artix command"}`))
		return
	}

	if !s.canAcceptJob() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":  "concurrency cap or queue limit exceeded",
			"status": "rate_limited",
		})
		return
	}

	parts := strings.Split(event.Project.PathWithNamespace, "/")
	owner := ""
	repo := ""
	if len(parts) >= 2 {
		owner = parts[0]
		repo = parts[1]
	}

	jobID := fmt.Sprintf("gl-%d", time.Now().UnixNano())
	target := RemoteRepoTarget{
		CloneURL: event.Project.GitHTTPURL,
		Owner:    owner,
		Repo:     repo,
		Branch:   event.Project.DefaultBranch,
	}

	task := &RemoteWorkerTask{
		Target: target,
		Auth: ForgeAuth{
			Type: ForgeGitLab,
		},
		Prompt: fmt.Sprintf("%s\n\n%s", event.ObjectAttributes.Title, event.ObjectAttributes.Description),
		Domain: s.cfg.DefaultDomain,
	}

	s.startJob(jobID, "gitlab", owner, task)

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"jobId":  jobID,
		"status": "queued",
	})
}

func (s *WebhookServer) canAcceptJob() bool {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	activeOrQueued := 0
	for _, j := range s.jobs {
		if j.Status == "queued" || j.Status == "running" {
			activeOrQueued++
		}
	}
	limit := s.cfg.MaxConcurrentJobs + s.cfg.MaxQueuedJobs
	return activeOrQueued < limit
}

func (s *WebhookServer) isAuthorized(r *http.Request) bool {
	if s.cfg.JobsAuthToken != "" || policy.IsEnterprise() {
		if s.cfg.JobsAuthToken == "" {
			return false // fail closed in enterprise mode when no token is set
		}
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.JobsAuthToken)) != 1 {
			return false
		}
	}
	return true
}

func (s *WebhookServer) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.checkJobStoreWritable(); err != nil {
		http.Error(w, fmt.Sprintf("service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	if err := s.checkEnterpriseAuditSink(); err != nil {
		http.Error(w, fmt.Sprintf("service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	if err := s.checkOfflineCache(); err != nil {
		http.Error(w, fmt.Sprintf("service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func (s *WebhookServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(RenderPrometheusMetrics()))
}

func (s *WebhookServer) checkJobStoreWritable() error {
	storePath := s.cfg.StoragePath
	if storePath == "" {
		if s.cfg.Worker != nil && s.cfg.Worker.WorkRoot() != "" {
			storePath = filepath.Join(s.cfg.Worker.WorkRoot(), "jobs.json")
		} else {
			storePath = filepath.Join(os.TempDir(), "artix-jobs.json")
		}
	}
	dir := filepath.Dir(storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("job store directory %s unwritable: %w", dir, err)
	}
	testFile := filepath.Join(dir, fmt.Sprintf(".readyz_test_%d", time.Now().UnixNano()))
	if err := os.WriteFile(testFile, []byte("ready"), 0600); err != nil {
		return fmt.Errorf("job store unwritable: %w", err)
	}
	_ = os.Remove(testFile)
	return nil
}

func (s *WebhookServer) checkEnterpriseAuditSink() error {
	if !policy.IsEnterprise() {
		return nil
	}
	logger := audit.Default(s.cfg.StoragePath)
	var sinks []policy.RemoteSinkConfig
	if logger != nil {
		sinks = logger.RemoteSinks()
	}
	if len(sinks) == 0 {
		pol := policy.Active()
		sinks = pol.AuditRemoteSinks
	}
	if len(sinks) == 0 {
		if ep := os.Getenv("ARTIX_AUDIT_REMOTE_SINK"); ep != "" {
			sinks = append(sinks, policy.RemoteSinkConfig{Type: "http", Endpoint: ep})
		} else if ep := os.Getenv("ARTIX_AUDIT_HTTP_ENDPOINT"); ep != "" {
			sinks = append(sinks, policy.RemoteSinkConfig{Type: "http", Endpoint: ep})
		}
	}
	if len(sinks) == 0 {
		return fmt.Errorf("no enterprise audit sinks configured")
	}
	for _, sink := range sinks {
		if !isSinkReachable(sink) {
			return fmt.Errorf("enterprise audit sink unreachable: %s (%s)", sink.Endpoint, sink.Type)
		}
	}
	return nil
}

func (s *WebhookServer) checkOfflineCache() error {
	cacheDir := s.cfg.OfflineCacheDir
	if cacheDir == "" {
		cacheDir = os.Getenv("ARTIX_OFFLINE_CACHE_DIR")
	}
	if cacheDir != "" {
		if _, err := os.Stat(cacheDir); err != nil {
			return fmt.Errorf("offline cache is missing: %w", err)
		}
		return nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "Library/Caches"),
		filepath.Join(home, ".cache"),
		filepath.Join(os.TempDir(), "artix-cache"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return nil
		}
	}
	return fmt.Errorf("offline cache is missing: no default cache directory found")
}

func isSinkReachable(sink policy.RemoteSinkConfig) bool {
	endpoint := sink.Endpoint
	if endpoint == "" {
		return false
	}
	switch strings.ToLower(sink.Type) {
	case "http", "https":
		client := &http.Client{
			Timeout: 1 * time.Second,
		}
		resp, err := client.Get(endpoint)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	case "syslog", "tcp", "udp":
		proto := strings.ToLower(sink.Type)
		if proto == "syslog" {
			proto = "udp"
		}
		conn, err := net.DialTimeout(proto, endpoint, 1*time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	default:
		return true
	}
}

func (s *WebhookServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	// Authentication gate for /jobs endpoint
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized: valid jobs authorization token required", http.StatusUnauthorized)
		return
	}

	tenantQuery := r.URL.Query().Get("tenant")
	if tenantHeader := r.Header.Get("X-Artix-Tenant"); tenantHeader != "" && tenantQuery == "" {
		tenantQuery = tenantHeader
	}

	// Server-side tenant binding: enforce explicit tenant query in enterprise/authenticated mode
	if (s.cfg.JobsAuthToken != "" || policy.IsEnterprise()) && tenantQuery == "" {
		http.Error(w, "tenant parameter or X-Artix-Tenant header is required", http.StatusBadRequest)
		return
	}

	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()

	filtered := make(map[string]*JobStatus)
	for id, job := range s.jobs {
		if tenantQuery == "" || job.Tenant == tenantQuery {
			filtered[id] = job
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(filtered)
}

func (s *WebhookServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	// Authentication gate for /tasks endpoint
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized: valid authorization token required", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodGet {
		s.handleJobs(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if !s.canAcceptJob() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":  "concurrency cap or queue limit exceeded",
				"status": "rate_limited",
			})
			return
		}

		var sub struct {
			Prompt   string `json:"prompt"`
			Domain   string `json:"domain,omitempty"`
			RootDir  string `json:"rootDir,omitempty"`
			Tenant   string `json:"tenant,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
			http.Error(w, "invalid json payload", http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(sub.Prompt) == "" {
			http.Error(w, "prompt is required", http.StatusBadRequest)
			return
		}

		domain := sub.Domain
		if domain == "" {
			domain = s.cfg.DefaultDomain
		}

		tenant := sub.Tenant
		if tenant == "" {
			tenant = "local"
		}

		taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
		task := &RemoteWorkerTask{
			Target: RemoteRepoTarget{
				CloneURL: sub.RootDir,
				Branch:   "HEAD",
				Owner:    tenant,
				Repo:     "workspace",
			},
			Prompt: sub.Prompt,
			Domain: domain,
		}

		stopCfg := s.cfg.PilotStopRuleConfig
		if polCfg := policy.Active(); polCfg != nil {
			if polCfg.Pilot.MaxAbortRate > 0 && stopCfg.MaxAbortRate <= 0 {
				stopCfg.MaxAbortRate = polCfg.Pilot.MaxAbortRate
			}
			if polCfg.Pilot.MaxCostPerStory > 0 && stopCfg.MaxCostPerStory <= 0 {
				stopCfg.MaxCostPerStory = polCfg.Pilot.MaxCostPerStory
			}
		}
		if envAbort := os.Getenv("ARTIX_PILOT_MAX_ABORT_RATE"); envAbort != "" && stopCfg.MaxAbortRate <= 0 {
			if v, err := strconv.ParseFloat(envAbort, 64); err == nil && v > 0 {
				stopCfg.MaxAbortRate = v
			}
		}
		if envCost := os.Getenv("ARTIX_PILOT_MAX_COST_PER_STORY"); envCost != "" && stopCfg.MaxCostPerStory <= 0 {
			if v, err := strconv.ParseFloat(envCost, 64); err == nil && v > 0 {
				stopCfg.MaxCostPerStory = v
			}
		}
		if stopCfg.MaxAbortRate > 0 || stopCfg.MaxCostPerStory > 0 {
			wsDir := s.cfg.WorkspaceDir
			if wsDir == "" {
				wsDir = sub.RootDir
			}
			if wsDir == "" {
				wsDir = "."
			}
			if err := pilot.CheckPilotStopRules(wsDir, stopCfg); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":  err.Error(),
					"status": "refused",
				})
				return
			}
		}

		s.startJob(taskID, "task_api", tenant, task)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"taskId": taskID,
			"status": "queued",
		})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *WebhookServer) startJob(id, source, tenant string, task *RemoteWorkerTask) {
	status := &JobStatus{
		ID:        id,
		Source:    source,
		Tenant:    tenant,
		Status:    "queued",
		CreatedAt: time.Now(),
	}

	s.jobsMu.Lock()
	s.jobs[id] = status
	s.jobsMu.Unlock()
	s.saveState()

	if s.cfg.Worker == nil {
		return
	}

	go func() {
		// Acquire concurrency slot FIRST to avoid lock-hoarding goroutine buildup
		s.sem <- struct{}{}
		defer func() { <-s.sem }()

		// Acquire per-branch lock to prevent concurrent jobs racing on the same branch
		branchKey := fmt.Sprintf("%s/%s:%s", task.Target.Owner, task.Target.Repo, task.Target.Branch)
		branchLock := s.getBranchLock(branchKey)
		branchLock.Lock()
		defer branchLock.Unlock()

		s.jobsMu.Lock()
		status.Status = "running"
		s.jobsMu.Unlock()
		s.saveState()

		timeout := s.cfg.JobTimeout
		if timeout <= 0 {
			timeout = 10 * time.Minute
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		res := s.cfg.Worker.Execute(ctx, task)

		s.jobsMu.Lock()
		status.Result = res
		if res != nil && res.Success {
			status.Status = "completed"
		} else {
			status.Status = "failed"
			if res != nil {
				status.Error = res.Error
			}
		}
		s.jobsMu.Unlock()
		s.saveState()
	}()
}

func (s *WebhookServer) isDuplicateDelivery(deliveryID string) bool {
	s.deliveriesMu.Lock()
	if _, exists := s.processedDeliveries[deliveryID]; exists {
		s.deliveriesMu.Unlock()
		return true
	}
	s.processedDeliveries[deliveryID] = time.Now()
	s.deliveriesMu.Unlock()
	s.saveState()
	return false
}

func (s *WebhookServer) saveState() {
	if s.cfg.StoragePath == "" {
		return
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()

	s.jobsMu.RLock()
	jobsCopy := make(map[string]*JobStatus, len(s.jobs))
	for k, v := range s.jobs {
		jobsCopy[k] = v
	}
	s.jobsMu.RUnlock()

	s.deliveriesMu.Lock()
	delivCopy := make(map[string]time.Time, len(s.processedDeliveries))
	for k, v := range s.processedDeliveries {
		delivCopy[k] = v
	}
	s.deliveriesMu.Unlock()

	state := daemonPersistentState{
		Jobs:       jobsCopy,
		Deliveries: delivCopy,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(s.cfg.StoragePath, data, 0600)
	}
}

func (s *WebhookServer) loadState() {
	if s.cfg.StoragePath == "" {
		return
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()

	data, err := os.ReadFile(s.cfg.StoragePath)
	if err != nil || len(data) == 0 {
		return
	}

	var state daemonPersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	s.jobsMu.Lock()
	if state.Jobs != nil {
		for k, v := range state.Jobs {
			s.jobs[k] = v
		}
	}
	s.jobsMu.Unlock()

	s.deliveriesMu.Lock()
	if state.Deliveries != nil {
		for k, v := range state.Deliveries {
			s.processedDeliveries[k] = v
		}
	}
	s.deliveriesMu.Unlock()
}

func (s *WebhookServer) getBranchLock(key string) *sync.Mutex {
	s.branchLocksMu.Lock()
	defer s.branchLocksMu.Unlock()
	lock, ok := s.branchLocks[key]
	if !ok {
		lock = &sync.Mutex{}
		s.branchLocks[key] = lock
	}
	return lock
}

// GetJob returns status of a specific job.
func (s *WebhookServer) GetJob(id string) (*JobStatus, bool) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

func (s *WebhookServer) isUserAuthorized(username, authorAssociation string) bool {
	if len(s.cfg.AllowedUsers) > 0 {
		for _, u := range s.cfg.AllowedUsers {
			if strings.EqualFold(u, username) {
				return true
			}
		}
		return false
	}
	if authorAssociation != "" {
		assoc := strings.ToUpper(strings.TrimSpace(authorAssociation))
		return assoc == "OWNER" || assoc == "MEMBER" || assoc == "COLLABORATOR"
	}
	// Default-deny: empty or unverified association is strictly rejected
	return false
}

func verifyGitHubSignature(secret, signatureHeader string, body []byte) bool {
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	expectedMAC := strings.TrimPrefix(signatureHeader, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actualMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedMAC), []byte(actualMAC))
}

