package audit

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"artix/pkg/policy"
	"artix/pkg/sandbox"
)

var (
	bearerRegex    = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9_\-\.]{16,}`)
	githubPatRegex = regexp.MustCompile(`\b(gh[pous]_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{40,})\b`)
	kvSecretRegex  = regexp.MustCompile(`(?i)\b(api[_-]?key|secret|password|access[_-]?token|auth[_-]?token)(\s*[:=]\s*["']?)[A-Za-z0-9_\-\.]{8,}(["']?)`)
	privKeyRegex   = regexp.MustCompile(`-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`)
)

// RedactSecrets replaces sensitive tokens, API keys, passwords, and private keys with redaction markers.
func RedactSecrets(input string) string {
	if input == "" {
		return ""
	}
	out := privKeyRegex.ReplaceAllString(input, "[REDACTED PRIVATE KEY]")
	out = bearerRegex.ReplaceAllString(out, "$1[REDACTED]")
	out = githubPatRegex.ReplaceAllString(out, "[REDACTED TOKEN]")
	out = kvSecretRegex.ReplaceAllString(out, "$1$2[REDACTED]$3")
	return out
}

func redactValue(val any) any {
	switch v := val.(type) {
	case string:
		return RedactSecrets(v)
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, item := range v {
			lowerK := strings.ToLower(k)
			if strings.Contains(lowerK, "key") || strings.Contains(lowerK, "secret") || strings.Contains(lowerK, "token") || strings.Contains(lowerK, "password") || strings.Contains(lowerK, "auth") {
				if _, ok := item.(string); ok {
					res[k] = "[REDACTED]"
					continue
				}
			}
			res[k] = redactValue(item)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			res[i] = redactValue(item)
		}
		return res
	default:
		return val
	}
}

// EventType categorizes an enterprise audit event.
type EventType string

const (
	EventSpecDeliberation EventType = "spec.deliberation"
	EventCodeConvergence  EventType = "code.convergence"
	EventReviewerVerdict   EventType = "reviewer.verdict"
	EventSteeringBind      EventType = "steering.bind"
	EventWorktreeGC        EventType = "worktree.gc"
	EventSandboxExecution  EventType = "sandbox.execution"
	EventBudgetExhausted   EventType = "budget.exhausted"
	EventSecurityViolation EventType = "security.violation"
)

// GenesisHash is the root parent hash for the first record in an audit stream.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// AuditEvent represents a structured, SIEM/OTEL-compatible, hash-chained audit record.
type AuditEvent struct {
	EventID         string         `json:"eventId"`
	Timestamp       time.Time      `json:"timestamp"`
	EventType       EventType      `json:"eventType"`
	Actor           string         `json:"actor"`
	Workspace       string         `json:"workspace"`
	Status          string         `json:"status"` // "SUCCESS", "REJECTED", "FAILED", "INFO"
	DurationMs      int64          `json:"durationMs,omitempty"`
	ModelID         string         `json:"modelId,omitempty"`
	ModelFamily     string         `json:"modelFamily,omitempty"`
	PromptHash      string         `json:"promptHash,omitempty"`
	ResponseHash    string         `json:"responseHash,omitempty"`
	StorySpecID     string         `json:"storySpecId,omitempty"`
	DiffHash        string         `json:"diffHash,omitempty"`
	Round           int            `json:"round,omitempty"`
	Cost            float64        `json:"cost,omitempty"`
	SandboxExitCode int            `json:"sandboxExitCode,omitempty"`
	Approver        string         `json:"approver,omitempty"`
	PolicyHash      string         `json:"policyHash,omitempty"`
	Details         map[string]any `json:"details,omitempty"`
	PrevHash        string         `json:"prevHash"`
	RecordHash      string         `json:"recordHash"`
	Signature       string         `json:"signature,omitempty"`
}

// ComputeRecordHash deterministically calculates the SHA-256 hash of the audit record.
func ComputeRecordHash(e *AuditEvent) string {
	var detailsJSON []byte
	if e.Details != nil {
		detailsJSON, _ = json.Marshal(e.Details)
	}
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d|%s|%s|%s|%s|%s|%s|%d|%.6f|%d|%s|%s|%s",
		e.PrevHash,
		e.EventID,
		e.Timestamp.UTC().Format(time.RFC3339Nano),
		e.EventType,
		e.Actor,
		e.Workspace,
		e.Status,
		e.DurationMs,
		e.ModelID,
		e.ModelFamily,
		e.PromptHash,
		e.ResponseHash,
		e.StorySpecID,
		e.DiffHash,
		e.Round,
		e.Cost,
		e.SandboxExitCode,
		e.Approver,
		e.PolicyHash,
		string(detailsJSON),
	)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// SignRecord produces an HMAC-SHA256 signature for a record hash.
func SignRecord(recordHash string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(recordHash))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyRecordSignature validates the HMAC signature of a record.
func VerifyRecordSignature(recordHash, signature, secret string) bool {
	expected := SignRecord(recordHash, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// SignRecordEd25519 signs a record hash using an asymmetric Ed25519 private key.
func SignRecordEd25519(recordHash string, privKey ed25519.PrivateKey) string {
	sig := ed25519.Sign(privKey, []byte(recordHash))
	return hex.EncodeToString(sig)
}

// VerifyRecordSignatureEd25519 verifies an Ed25519 signature of a record hash against a public key.
func VerifyRecordSignatureEd25519(recordHash, signatureHex string, pubKey ed25519.PublicKey) bool {
	sigBytes, err := hex.DecodeString(strings.TrimSpace(signatureHex))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pubKey, []byte(recordHash), sigBytes)
}

type spooledSinkItem struct {
	sink policy.RemoteSinkConfig
	data []byte
}

// Logger persists, hash-chains, and emits structured audit records with spooled remote delivery.
type Logger struct {
	workspaceDir   string
	logPath        string
	signingKey     string
	asymSigningKey ed25519.PrivateKey
	remoteSinks    []policy.RemoteSinkConfig
	lastHash       string
	initError      error
	mu             sync.Mutex
	httpClient     *http.Client
	spoolChan      chan spooledSinkItem
	stopChan       chan struct{}
	spoolWg        sync.WaitGroup
}

// InitError returns any fatal error encountered during logger initialization.
func (l *Logger) InitError() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.initError
}

var (
	defaultLogger *Logger
	loggerMu      sync.Mutex
)

// Default returns the process-wide default audit logger.
func Default(workspaceDir string) *Logger {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	// Recreate if workspaceDir changed or not yet initialized
	if defaultLogger == nil || (workspaceDir != "" && defaultLogger.workspaceDir != workspaceDir) {
		defaultLogger = NewLogger(workspaceDir)
	}
	return defaultLogger
}

// SetDefaultLogger sets the global default logger (useful for tests).
func SetDefaultLogger(l *Logger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	defaultLogger = l
}

// NewLogger creates a structured audit logger for a workspace honoring policy.
func NewLogger(workspaceDir string) *Logger {
	logFile := policy.EffectiveAuditLogPath(workspaceDir)

	pol := policy.Active()
	signingKey := pol.AuditSigningKey
	if signingKey == "" {
		signingKey = os.Getenv("ARTIX_AUDIT_SIGNING_KEY")
	}

	l := &Logger{
		workspaceDir: workspaceDir,
		logPath:      logFile,
		signingKey:   signingKey,
		remoteSinks:  pol.AuditRemoteSinks,
		lastHash:     GenesisHash,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		spoolChan:    make(chan spooledSinkItem, 1024),
		stopChan:     make(chan struct{}),
	}

	// G7: Asymmetric audit signing wired by default in enterprise mode.
	// Enterprise mode must require an ed25519 key path outside sandbox-readable paths and refuse otherwise.
	if policy.IsEnterprise() {
		keyPath := pol.AuditPrivateKeyPath
		if keyPath == "" {
			keyPath = os.Getenv("ARTIX_AUDIT_PRIVATE_KEY_PATH")
		}
		if keyPath == "" {
			keyPath = os.Getenv("ARTIX_AUDIT_KEY_PATH")
		}
		if keyPath == "" {
			l.initError = fmt.Errorf("enterprise audit policy violation: ed25519 private key path is required in enterprise mode (set pol.AuditPrivateKeyPath or ARTIX_AUDIT_PRIVATE_KEY_PATH outside sandbox-readable paths)")
		} else {
			if err := l.SetPrivateKeyPath(keyPath); err != nil {
				l.initError = fmt.Errorf("enterprise audit policy violation: %w", err)
			}
		}

		// Mandatory external sink under ARTIX_ENTERPRISE
		if len(l.remoteSinks) == 0 {
			if ep := os.Getenv("ARTIX_AUDIT_REMOTE_SINK"); ep != "" {
				l.remoteSinks = append(l.remoteSinks, policy.RemoteSinkConfig{Type: "http", Endpoint: ep})
			} else if ep := os.Getenv("ARTIX_AUDIT_HTTP_ENDPOINT"); ep != "" {
				l.remoteSinks = append(l.remoteSinks, policy.RemoteSinkConfig{Type: "http", Endpoint: ep})
			} else if ep := os.Getenv("ARTIX_AUDIT_OTEL_ENDPOINT"); ep != "" {
				l.remoteSinks = append(l.remoteSinks, policy.RemoteSinkConfig{Type: "http", Endpoint: ep})
			} else if ep := os.Getenv("ARTIX_AUDIT_SYSLOG_ENDPOINT"); ep != "" {
				l.remoteSinks = append(l.remoteSinks, policy.RemoteSinkConfig{Type: "syslog", Endpoint: ep})
			}
		}
		if len(l.remoteSinks) == 0 && l.initError == nil {
			l.initError = fmt.Errorf("enterprise audit policy violation: external sink (syslog/OTel/S3) is mandatory under ARTIX_ENTERPRISE")
		}
	}

	// Initialize lastHash from existing log file if available
	l.initLastHash()

	// Start spooled delivery worker for remote sinks
	l.spoolWg.Add(1)
	go l.spoolWorker()

	return l
}

func (l *Logger) initLastHash() {
	if l.logPath == "" {
		return
	}
	data, err := os.ReadFile(l.logPath)
	if err != nil || len(data) == 0 {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return
	}

	lastLine := strings.TrimSpace(lines[len(lines)-1])
	if lastLine == "" {
		return
	}

	var evt AuditEvent
	if err := json.Unmarshal([]byte(lastLine), &evt); err == nil && evt.RecordHash != "" {
		l.lastHash = evt.RecordHash
	}
}

// ReadEvents reads and parses all recorded audit events from the log file.
func (l *Logger) ReadEvents() ([]AuditEvent, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(l.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []AuditEvent
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var evt AuditEvent
		if err := json.Unmarshal([]byte(line), &evt); err == nil {
			events = append(events, evt)
		}
	}
	return events, nil
}

// SetSigningKey configures an HMAC key for cryptographic record signatures.
func (l *Logger) SetSigningKey(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.signingKey = key
}

// SetAsymmetricSigningKey configures an Ed25519 private key for asymmetric audit signing.
func (l *Logger) SetAsymmetricSigningKey(privKey ed25519.PrivateKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asymSigningKey = privKey
}

// AddRemoteSink adds an HTTP or syslog remote audit sink.
func (l *Logger) AddRemoteSink(sink policy.RemoteSinkConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remoteSinks = append(l.remoteSinks, sink)
	if l.initError != nil && strings.Contains(l.initError.Error(), "external sink") {
		l.initError = nil
	}
}

// LogPath returns the path to the active audit log file.
func (l *Logger) LogPath() string {
	return l.logPath
}

// RemoteSinks returns a copy of configured remote audit sinks.
func (l *Logger) RemoteSinks() []policy.RemoteSinkConfig {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := make([]policy.RemoteSinkConfig, len(l.remoteSinks))
	copy(copied, l.remoteSinks)
	return copied
}

// Close gracefully flushes pending spooled sinks and stops workers.
func (l *Logger) Close() {
	select {
	case <-l.stopChan:
		return // already closed
	default:
		close(l.stopChan)
		l.spoolWg.Wait()
	}
}

// Emit writes an AuditEvent to the JSONL log stream with hash chaining and dispatches to spooled sinks.
func (l *Logger) Emit(event AuditEvent) error {
	if event.EventID == "" {
		event.EventID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Actor == "" {
		event.Actor = os.Getenv("USER")
		if event.Actor == "" {
			event.Actor = "artix-agent"
		}
	}
	if event.Workspace == "" {
		event.Workspace = l.workspaceDir
	}

	// Secret redaction pass before recording or hashing
	if event.Details != nil {
		event.Details = redactValue(event.Details).(map[string]any)
	}
	if event.Actor != "" {
		event.Actor = RedactSecrets(event.Actor)
	}
	if event.Approver != "" {
		event.Approver = RedactSecrets(event.Approver)
	}
	if event.PolicyHash == "" {
		event.PolicyHash = policy.GetPolicyHash()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.initError != nil {
		return fmt.Errorf("audit logging refused: %w", l.initError)
	}
	if policy.IsEnterprise() && l.asymSigningKey == nil {
		return fmt.Errorf("audit logging refused: ed25519 asymmetric signing key is required in enterprise mode")
	}
	if policy.IsEnterprise() && len(l.remoteSinks) == 0 {
		return fmt.Errorf("enterprise audit policy violation: external sink (syslog/OTel/S3) is mandatory under ARTIX_ENTERPRISE")
	}

	// Hash Chaining
	if l.lastHash == "" {
		l.lastHash = GenesisHash
	}
	event.PrevHash = l.lastHash
	event.RecordHash = ComputeRecordHash(&event)

	// Cryptographic signing: prefer Ed25519 asymmetric signature if configured
	if l.asymSigningKey != nil {
		event.Signature = SignRecordEd25519(event.RecordHash, l.asymSigningKey)
	} else if l.signingKey != "" {
		event.Signature = SignRecord(event.RecordHash, l.signingKey)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}
	logLine := append(data, '\n')

	if l.logPath != "" {
		_ = os.MkdirAll(filepath.Dir(l.logPath), 0700)
		f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open audit log %s: %w", l.logPath, err)
		}
		if _, err := f.Write(logLine); err != nil {
			_ = f.Close()
			return fmt.Errorf("failed to write audit record: %w", err)
		}
		_ = f.Close()
	}

	l.lastHash = event.RecordHash

	// Dispatch to spooled sink queue
	for _, sink := range l.remoteSinks {
		select {
		case l.spoolChan <- spooledSinkItem{sink: sink, data: data}:
		default:
			// Spool channel saturated: fallback to asynchronous goroutine
			go l.sendToRemoteSinkWithRetry(sink, data, 1)
		}
	}

	return nil
}

func (l *Logger) spoolWorker() {
	defer l.spoolWg.Done()
	for {
		select {
		case item := <-l.spoolChan:
			l.sendToRemoteSinkWithRetry(item.sink, item.data, 3)
		case <-l.stopChan:
			// Drain remaining items before exiting
			for len(l.spoolChan) > 0 {
				item := <-l.spoolChan
				l.sendToRemoteSinkWithRetry(item.sink, item.data, 1)
			}
			return
		}
	}
}

func (l *Logger) sendToRemoteSinkWithRetry(sink policy.RemoteSinkConfig, data []byte, maxAttempts int) {
	delivered := false
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		success := l.sendToRemoteSink(sink, data)
		if success {
			delivered = true
			break
		}
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt*50) * time.Millisecond) // exponential backoff retry
		}
	}
	if !delivered {
		l.RecordSpoolLoss(sink.Endpoint, 1)
	}
}

func (l *Logger) sendToRemoteSink(sink policy.RemoteSinkConfig, data []byte) bool {
	switch strings.ToLower(sink.Type) {
	case "http", "https":
		req, err := http.NewRequest("POST", sink.Endpoint, bytes.NewReader(data))
		if err != nil {
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		if sink.AuthKey != "" {
			req.Header.Set("Authorization", "Bearer "+sink.AuthKey)
		}
		for k, v := range sink.Headers {
			req.Header.Set(k, v)
		}
		resp, err := l.httpClient.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode >= 200 && resp.StatusCode < 300

	case "syslog":
		endpoint := sink.Endpoint
		network := "udp"
		if strings.HasPrefix(endpoint, "tcp://") {
			network = "tcp"
			endpoint = strings.TrimPrefix(endpoint, "tcp://")
		} else if strings.HasPrefix(endpoint, "udp://") {
			endpoint = strings.TrimPrefix(endpoint, "udp://")
		}

		conn, err := net.DialTimeout(network, endpoint, 2*time.Second)
		if err != nil {
			return false
		}
		syslogMsg := fmt.Sprintf("<14>%s artix-audit: %s\n", time.Now().Format(time.RFC3339), string(data))
		_, err = conn.Write([]byte(syslogMsg))
		_ = conn.Close()
		return err == nil
	}
	return false
}

// VerificationResult summarizes the audit log chain verification.
type VerificationResult struct {
	TotalRecords int    `json:"totalRecords"`
	ValidRecords int    `json:"validRecords"`
	GenesisHash  string `json:"genesisHash"`
	LastHash     string `json:"lastHash"`
}

// VerifyLog verifies the cryptographic hash chain and optional signatures of an audit log file.
// Supports both HMAC symmetric keys and Ed25519 asymmetric public keys.
func VerifyLog(logPath string, signingKey ...string) (*VerificationResult, error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audit log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return &VerificationResult{TotalRecords: 0, ValidRecords: 0}, nil
	}

	if policy.IsEnterprise() {
		return nil, fmt.Errorf("enterprise audit policy violation: symmetric HMAC verification is prohibited in enterprise mode; use ed25519 asymmetric verification with public key")
	}

	expectedPrevHash := GenesisHash
	key := ""
	if len(signingKey) > 0 {
		key = signingKey[0]
	}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("tampering detected at record #%d: invalid JSON format: %w", i+1, err)
		}

		// 1. Check prev_hash linkage
		if event.PrevHash != expectedPrevHash {
			return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): broken hash chain: expected prev_hash %s, got %s",
				i+1, event.EventID, expectedPrevHash, event.PrevHash)
		}

		// 2. Check record_hash recalculation
		computedHash := ComputeRecordHash(&event)
		if event.RecordHash != computedHash {
			return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): record payload altered: expected hash %s, got %s",
				i+1, event.EventID, computedHash, event.RecordHash)
		}

		// 3. Optional signature verification
		if key != "" && event.Signature != "" {
			if !VerifyRecordSignature(event.RecordHash, event.Signature, key) {
				return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): invalid cryptographic signature", i+1, event.EventID)
			}
		}

		expectedPrevHash = event.RecordHash
	}

	return &VerificationResult{
		TotalRecords: len(lines),
		ValidRecords: len(lines),
		GenesisHash:  GenesisHash,
		LastHash:     expectedPrevHash,
	}, nil
}

// VerifyLogWithPubKey verifies audit records signed with an asymmetric Ed25519 private key.
func VerifyLogWithPubKey(logPath string, pubKey ed25519.PublicKey) (*VerificationResult, error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audit log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return &VerificationResult{TotalRecords: 0, ValidRecords: 0}, nil
	}

	expectedPrevHash := GenesisHash

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("tampering detected at record #%d: invalid JSON format: %w", i+1, err)
		}

		if event.PrevHash != expectedPrevHash {
			return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): broken hash chain", i+1, event.EventID)
		}

		computedHash := ComputeRecordHash(&event)
		if event.RecordHash != computedHash {
			return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): record payload altered", i+1, event.EventID)
		}

		if policy.IsEnterprise() {
			if event.Signature == "" {
				return nil, fmt.Errorf("enterprise audit violation: record #%d (event ID %s) has no ed25519 signature", i+1, event.EventID)
			}
			if len(pubKey) == 0 {
				return nil, fmt.Errorf("enterprise audit violation: ed25519 public key required to verify enterprise audit records")
			}
			if !VerifyRecordSignatureEd25519(event.RecordHash, event.Signature, pubKey) {
				return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): invalid ed25519 signature", i+1, event.EventID)
			}
		} else {
			if event.Signature != "" && len(pubKey) > 0 {
				if !VerifyRecordSignatureEd25519(event.RecordHash, event.Signature, pubKey) {
					return nil, fmt.Errorf("tampering detected at record #%d (event ID %s): invalid ed25519 signature", i+1, event.EventID)
				}
			}
		}

		expectedPrevHash = event.RecordHash
	}

	return &VerificationResult{
		TotalRecords: len(lines),
		ValidRecords: len(lines),
		GenesisHash:  GenesisHash,
		LastHash:     expectedPrevHash,
	}, nil
}

// SetPrivateKeyPath configures the Ed25519 private key from an external path.
// It strictly enforces that the private key must reside outside sandbox-readable paths.
func (l *Logger) SetPrivateKeyPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid private key path: %w", err)
	}
	if l.workspaceDir != "" {
		absWs, errWs := filepath.Abs(l.workspaceDir)
		if errWs == nil {
			rel, errRel := filepath.Rel(absWs, absPath)
			if errRel == nil && !strings.HasPrefix(rel, "..") {
				return fmt.Errorf("signing key refused: private key must be located outside sandbox-readable paths (%s is inside workspace %s)", absPath, absWs)
			}
		}
	}

	// Check if path is protected from sandbox reads via SensitiveReadDenyPaths (e.g. .artix, .ssh, .aws)
	isProtectedDenyPath := false
	for _, denyRel := range sandbox.SensitiveReadDenyPaths {
		if strings.Contains(absPath, "/"+denyRel+"/") || strings.HasSuffix(absPath, "/"+denyRel) || strings.Contains(absPath, "\\"+denyRel+"\\") {
			isProtectedDenyPath = true
			break
		}
	}

	// If not inside a protected denied directory, reject if located in open temp directories readable by sandbox
	if !isProtectedDenyPath {
		tempRoots := []string{os.TempDir(), "/tmp", "/private/tmp", "/var/tmp"}
		for _, tr := range tempRoots {
			if tr == "" {
				continue
			}
			absTr, errTr := filepath.Abs(tr)
			if errTr == nil {
				rel, errRel := filepath.Rel(absTr, absPath)
				if errRel == nil && !strings.HasPrefix(rel, "..") {
					return fmt.Errorf("signing key refused: private key must be located outside sandbox-readable temp paths (%s is inside temp dir %s)", absPath, absTr)
				}
			}
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat private key file: %w", err)
	}
	// Require owner-only mode (0600 or 0400), reject group/other read/write bits
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("signing key refused: private key file %s has insecure permissions %04o (must be owner-only 0600 or 0400)", absPath, info.Mode().Perm())
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	raw := strings.TrimSpace(string(data))
	if keyBytes, err := hex.DecodeString(raw); err == nil && len(keyBytes) == ed25519.PrivateKeySize {
		l.SetAsymmetricSigningKey(ed25519.PrivateKey(keyBytes))
		return nil
	} else if keyBytes, err := hex.DecodeString(raw); err == nil && len(keyBytes) == ed25519.SeedSize {
		priv := ed25519.NewKeyFromSeed(keyBytes)
		l.SetAsymmetricSigningKey(priv)
		return nil
	} else if len(data) == ed25519.PrivateKeySize {
		l.SetAsymmetricSigningKey(ed25519.PrivateKey(data))
		return nil
	} else if len(data) == ed25519.SeedSize {
		priv := ed25519.NewKeyFromSeed(data)
		l.SetAsymmetricSigningKey(priv)
		return nil
	}

	return fmt.Errorf("invalid ed25519 private key in file %s (expected 32-byte seed or 64-byte private key)", absPath)
}

// RecordSpoolLoss records that spooled audit records were lost due to unrecoverable delivery failure.
func (l *Logger) RecordSpoolLoss(sinkEndpoint string, count int) {
	_ = l.Emit(AuditEvent{
		EventType: EventType("audit.spool_loss"),
		Status:    "FAILED",
		Details: map[string]any{
			"sink":           sinkEndpoint,
			"droppedRecords": count,
			"reason":         "unrecoverable delivery failure or buffer saturation",
		},
	})
}

// VerifyOptions configures comprehensive audit log verification.
type VerifyOptions struct {
	PubKey           ed25519.PublicKey
	SigningKey       string
	ExpectedCount    int
	ExpectedLastHash string
}

// VerifyLogWithOptions verifies an audit log against custom constraints including expected count and last hash.
func VerifyLogWithOptions(logPath string, opts VerifyOptions) (*VerificationResult, error) {
	var res *VerificationResult
	var err error

	if opts.PubKey != nil {
		res, err = VerifyLogWithPubKey(logPath, opts.PubKey)
	} else if opts.SigningKey != "" {
		res, err = VerifyLog(logPath, opts.SigningKey)
	} else {
		res, err = VerifyLog(logPath)
	}

	if err != nil {
		return nil, err
	}

	if opts.ExpectedCount > 0 && res.ValidRecords < opts.ExpectedCount {
		return nil, fmt.Errorf("tampering detected: tail truncated: expected %d records, got %d", opts.ExpectedCount, res.ValidRecords)
	}

	if opts.ExpectedLastHash != "" && res.LastHash != opts.ExpectedLastHash {
		return nil, fmt.Errorf("tampering detected: tail truncated or altered: expected last hash %s, got %s", opts.ExpectedLastHash, res.LastHash)
	}

	return res, nil
}

