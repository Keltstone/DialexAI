package audit

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"artix/pkg/policy"
)

func TestAuditEventEmissionAndHashChaining(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	logger.SetSigningKey("test-signing-secret")

	// Emit 3 events
	events := []AuditEvent{
		{
			EventType:    EventSpecDeliberation,
			Status:       "SUCCESS",
			ModelID:      "claude-3-7-sonnet",
			PromptHash:   "abc123prompt",
			ResponseHash: "def456resp",
			StorySpecID:  "STORY-101",
			Cost:         0.042,
		},
		{
			EventType:   EventCodeConvergence,
			Status:      "SUCCESS",
			ModelID:     "claude-3-7-sonnet",
			StorySpecID: "STORY-101",
			DiffHash:    "diff789hash",
			Round:       1,
			Cost:        0.125,
		},
		{
			EventType:   EventReviewerVerdict,
			Status:      "SUCCESS",
			ModelID:     "gpt-4o",
			StorySpecID: "STORY-101",
			DiffHash:    "diff789hash",
			Cost:        0.015,
		},
	}

	for _, e := range events {
		if err := logger.Emit(e); err != nil {
			t.Fatalf("Emit failed: %v", err)
		}
	}

	logFile := logger.LogPath()
	res, err := VerifyLog(logFile, "test-signing-secret")
	if err != nil {
		t.Fatalf("VerifyLog failed on valid chain: %v", err)
	}

	if res.ValidRecords != 3 {
		t.Errorf("expected 3 valid records, got %d", res.ValidRecords)
	}
}

func TestTamperDetectionModifiedPayload(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)

	_ = logger.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS", Cost: 0.10})
	_ = logger.Emit(AuditEvent{EventType: EventReviewerVerdict, Status: "SUCCESS", Cost: 0.05})

	logFile := logger.LogPath()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var evt AuditEvent
	_ = json.Unmarshal([]byte(lines[0]), &evt)

	// Tamper with the cost
	evt.Cost = 0.00
	tamperedLine, _ := json.Marshal(evt)
	lines[0] = string(tamperedLine)

	_ = os.WriteFile(logFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	_, err = VerifyLog(logFile)
	if err == nil {
		t.Fatalf("expected tamper detection on modified payload, but verification succeeded")
	}
	if !strings.Contains(err.Error(), "tampering detected") {
		t.Errorf("expected 'tampering detected' in error, got: %v", err)
	}
}

func TestTamperDetectionBrokenChain(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)

	_ = logger.Emit(AuditEvent{EventType: EventSpecDeliberation, Status: "SUCCESS"})
	_ = logger.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS"})
	_ = logger.Emit(AuditEvent{EventType: EventReviewerVerdict, Status: "SUCCESS"})

	logFile := logger.LogPath()
	data, _ := os.ReadFile(logFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Delete the middle record to break the chain
	newLines := []string{lines[0], lines[2]}
	_ = os.WriteFile(logFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644)

	_, err := VerifyLog(logFile)
	if err == nil {
		t.Fatalf("expected tamper detection on broken hash chain, but verification succeeded")
	}
	if !strings.Contains(err.Error(), "broken hash chain") {
		t.Errorf("expected 'broken hash chain' in error, got: %v", err)
	}
}

func TestAuditLogCannotBeDisabledInEnterpriseMode(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_AUDIT_LOG", "/dev/null")

	logger := NewLogger(tempDir)
	expected := filepath.Join(tempDir, ".artix", "audit.jsonl")
	if logger.LogPath() != expected {
		t.Errorf("expected audit log to ignore /dev/null in enterprise mode; expected %s, got %s", expected, logger.LogPath())
	}
}

func TestRemoteHTTPSink(t *testing.T) {
	received := make(chan AuditEvent, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var evt AuditEvent
		if err := json.NewDecoder(r.Body).Decode(&evt); err == nil {
			received <- evt
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	logger.AddRemoteSink(policy.RemoteSinkConfig{
		Type:     "http",
		Endpoint: server.URL,
	})

	err := logger.Emit(AuditEvent{
		EventType:   EventSpecDeliberation,
		Status:      "SUCCESS",
		StorySpecID: "STORY-REMOTE-1",
	})
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	select {
	case evt := <-received:
		if evt.StorySpecID != "STORY-REMOTE-1" {
			t.Errorf("remote sink received unexpected payload: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for remote HTTP sink delivery")
	}
}

func TestSecretRedactionInAuditLog(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)

	err := logger.Emit(AuditEvent{
		EventType:   EventSecurityViolation,
		Status:      "INFO",
		StorySpecID: "STORY-SEC-1",
		Actor:       "Bearer ghp_123456789012345678901234567890123456",
		Details: map[string]any{
			"authorization": "Bearer ya29.a0AfH6SMB_secret_bearer_token_123456789",
			"github_token":  "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
			"config": map[string]any{
				"apiKey": "super_secret_api_key_value",
			},
		},
	})
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	data, err := os.ReadFile(logger.LogPath())
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}
	raw := string(data)

	// Ensure sensitive values are not present
	if strings.Contains(raw, "ya29.a0AfH6SMB") {
		t.Errorf("log leaked bearer token")
	}
	if strings.Contains(raw, "ghp_abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Errorf("log leaked GitHub PAT")
	}
	if strings.Contains(raw, "super_secret_api_key_value") {
		t.Errorf("log leaked API key")
	}

	// Verify the hash chain still verifies
	if _, err := VerifyLog(logger.LogPath()); err != nil {
		t.Fatalf("verification failed for redacted log: %v", err)
	}
}

func TestAuditLogFilePermissions(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)

	_ = logger.Emit(AuditEvent{
		EventType: EventSpecDeliberation,
		Status:    "SUCCESS",
	})

	info, err := os.Stat(logger.LogPath())
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	// Mode should be 0600 (-rw-------)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected audit log permission 0600, got %o", perm)
	}
}

func TestAuditAsymmetricSigningEd25519(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey failed: %v", err)
	}

	logger.SetAsymmetricSigningKey(priv)

	err = logger.Emit(AuditEvent{
		EventType:   EventCodeConvergence,
		Status:      "SUCCESS",
		StorySpecID: "STORY-ASYM-1",
		Actor:       "sec-ops",
		Cost:        0.05,
	})
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Verify using public key (proves signature was generated with private key)
	res, err := VerifyLogWithPubKey(logger.LogPath(), pub)
	if err != nil {
		t.Fatalf("VerifyLogWithPubKey failed: %v", err)
	}
	if res.ValidRecords != 1 {
		t.Errorf("expected 1 valid record, got %d", res.ValidRecords)
	}

	// Verify with a different public key must fail
	otherPub, _, _ := ed25519.GenerateKey(nil)
	_, err = VerifyLogWithPubKey(logger.LogPath(), otherPub)
	if err == nil {
		t.Fatalf("expected verification failure with non-matching public key, got nil")
	}
}

func TestAuditRemoteSinkSpooledRetry(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := attempts.Add(1)
		if att < 2 {
			// Fail first attempt to trigger spool worker retry
			http.Error(w, "temporary gateway error", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger.AddRemoteSink(policy.RemoteSinkConfig{
		Type:     "http",
		Endpoint: srv.URL,
	})

	err := logger.Emit(AuditEvent{
		EventType: EventSpecDeliberation,
		Status:    "SUCCESS",
	})
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Wait briefly for spool retry worker
	deadline := time.Now().Add(2 * time.Second)
	for attempts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts from spooled retry worker, got %d", attempts.Load())
	}
}

func TestG7_TamperLine(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	pub, priv, _ := ed25519.GenerateKey(nil)
	logger.SetAsymmetricSigningKey(priv)

	_ = logger.Emit(AuditEvent{EventType: EventSpecDeliberation, Status: "SUCCESS", Cost: 0.01})
	_ = logger.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS", Cost: 0.05})
	_ = logger.Emit(AuditEvent{EventType: EventReviewerVerdict, Status: "SUCCESS", Cost: 0.02})

	logFile := logger.LogPath()
	data, _ := os.ReadFile(logFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Tamper line 2
	var evt AuditEvent
	_ = json.Unmarshal([]byte(lines[1]), &evt)
	evt.Cost = 999.99 // modified payload
	tamperedLine, _ := json.Marshal(evt)
	lines[1] = string(tamperedLine)
	_ = os.WriteFile(logFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	_, err := VerifyLogWithPubKey(logFile, pub)
	if err == nil {
		t.Fatalf("expected tamper detection on tampered line, got nil error")
	}
	if !strings.Contains(err.Error(), "tampering detected") {
		t.Fatalf("expected error mentioning tampering detected, got: %v", err)
	}
}

func TestG7_DeleteLine(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	pub, priv, _ := ed25519.GenerateKey(nil)
	logger.SetAsymmetricSigningKey(priv)

	_ = logger.Emit(AuditEvent{EventType: EventSpecDeliberation, Status: "SUCCESS"})
	_ = logger.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS"})
	_ = logger.Emit(AuditEvent{EventType: EventReviewerVerdict, Status: "SUCCESS"})

	logFile := logger.LogPath()
	data, _ := os.ReadFile(logFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Delete line 2
	newLines := []string{lines[0], lines[2]}
	_ = os.WriteFile(logFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644)

	_, err := VerifyLogWithPubKey(logFile, pub)
	if err == nil {
		t.Fatalf("expected tamper detection on deleted line, got nil error")
	}
	if !strings.Contains(err.Error(), "tampering detected") && !strings.Contains(err.Error(), "broken hash chain") {
		t.Fatalf("expected broken hash chain or tampering detected, got: %v", err)
	}
}

func TestG7_TruncateTail(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	pub, priv, _ := ed25519.GenerateKey(nil)
	logger.SetAsymmetricSigningKey(priv)

	_ = logger.Emit(AuditEvent{EventType: EventSpecDeliberation, Status: "SUCCESS"})
	_ = logger.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS"})
	_ = logger.Emit(AuditEvent{EventType: EventReviewerVerdict, Status: "SUCCESS"})

	logFile := logger.LogPath()
	data, _ := os.ReadFile(logFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	var lastEvt AuditEvent
	_ = json.Unmarshal([]byte(lines[2]), &lastEvt)
	expectedLastHash := lastEvt.RecordHash

	// Truncate the tail: delete line 3
	newLines := []string{lines[0], lines[1]}
	_ = os.WriteFile(logFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644)

	_, err := VerifyLogWithOptions(logFile, VerifyOptions{
		PubKey:           pub,
		ExpectedCount:    3,
		ExpectedLastHash: expectedLastHash,
	})
	if err == nil {
		t.Fatalf("expected tamper detection on truncated tail, got nil error")
	}
	if !strings.Contains(err.Error(), "tail truncated") && !strings.Contains(err.Error(), "tampering detected") {
		t.Fatalf("expected error mentioning tail truncated, got: %v", err)
	}
}

func TestG7_ForgeWithHMACKeyFromPolicy(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	pub, _, _ := ed25519.GenerateKey(nil)

	// Attacker tries to forge a record using HMAC key taken from policy file
	hmacKey := "attacker-extracted-hmac-key"
	logger.SetSigningKey(hmacKey)

	_ = logger.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS"})

	logFile := logger.LogPath()

	// Enterprise audit verification requires public key verification and must reject the HMAC forgery
	_, err := VerifyLogWithPubKey(logFile, pub)
	if err == nil {
		t.Fatalf("expected verification failure for record forged with HMAC key, got nil error")
	}
	if !strings.Contains(err.Error(), "invalid ed25519 signature") && !strings.Contains(err.Error(), "tampering detected") {
		t.Fatalf("expected ed25519 signature error, got: %v", err)
	}
}

func TestG7_PrivateKeyOutsideSandboxRefusal(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	// Place private key inside sandbox/workspace directory
	insidePath := filepath.Join(tempDir, "agent_accessible_private.key")
	_ = os.WriteFile(insidePath, []byte("fake-key"), 0600)

	err := logger.SetPrivateKeyPath(insidePath)
	if err == nil {
		t.Fatalf("expected error when private key path is inside sandbox-readable workspace, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox-readable") && !strings.Contains(err.Error(), "outside sandbox") {
		t.Fatalf("expected error mentioning sandbox path restriction, got: %v", err)
	}
}

func TestG7_SinkOutageSpoolsAndRetriesWithoutDropping(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	var receivedCount atomic.Int32
	var outageActive atomic.Bool
	outageActive.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if outageActive.Load() {
			http.Error(w, "sink outage: 503 service unavailable", http.StatusServiceUnavailable)
			return
		}
		receivedCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger.AddRemoteSink(policy.RemoteSinkConfig{
		Type:     "http",
		Endpoint: srv.URL,
	})

	// Emit 5 events while sink is suffering an outage
	for i := 0; i < 5; i++ {
		_ = logger.Emit(AuditEvent{
			EventType: EventCodeConvergence,
			Status:    "SUCCESS",
			Round:     i + 1,
		})
	}

	// Heal outage: sink is now operational
	outageActive.Store(false)

	// Wait for spooled retry worker to deliver all 5 events
	deadline := time.Now().Add(3 * time.Second)
	for receivedCount.Load() < 5 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}

	if receivedCount.Load() < 5 {
		t.Fatalf("expected all 5 events to be delivered after sink recovered, but only received %d", receivedCount.Load())
	}
}

func TestG7_SpoolLossIsItselfAudited(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	// Simulate unrecoverable spool loss
	logger.RecordSpoolLoss("https://failing-sink.example.com", 10)

	logFile := logger.LogPath()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if !strings.Contains(string(data), "audit.spool_loss") && !strings.Contains(string(data), "spool.loss") {
		t.Fatalf("expected audit log to record audit.spool_loss event, got: %s", string(data))
	}
}

// TestR2_6_EnterpriseMode_RequiresExternalEd25519KeyAndRefusesOtherwise asserts that
// in enterprise mode, NewLogger requires an ed25519 key path outside sandbox-readable paths
// and refuses to operate otherwise.
func TestR2_6_EnterpriseMode_RequiresExternalEd25519KeyAndRefusesOtherwise(t *testing.T) {
	tempDir := t.TempDir()

	// Enable enterprise policy
	entPol := &policy.Policy{
		EnterpriseMode:      true,
		RequireSignedPolicy: true,
		IsVerified:          true,
		AuditRemoteSinks:    []policy.RemoteSinkConfig{{Type: "syslog", Endpoint: "127.0.0.1:514"}},
	}
	policy.SetActivePolicyForTest(entPol)
	defer policy.ResetTestPolicy()

	// 1. Missing private key path in enterprise mode -> must refuse
	l1 := NewLogger(tempDir)
	defer l1.Close()
	if err := l1.InitError(); err == nil {
		t.Fatalf("expected enterprise NewLogger to refuse when no ed25519 key path is provided, got nil")
	}
	if err := l1.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "TEST"}); err == nil {
		t.Fatalf("expected Emit to fail when logger initialized without ed25519 key in enterprise, got nil")
	}

	// 2. Private key path set to a path INSIDE workspace -> must refuse
	insideKeyPath := filepath.Join(tempDir, "inside_workspace.key")
	_ = os.WriteFile(insideKeyPath, []byte("key"), 0600)
	entPol.AuditPrivateKeyPath = insideKeyPath
	l2 := NewLogger(tempDir)
	defer l2.Close()
	if err := l2.InitError(); err == nil {
		t.Fatalf("expected enterprise NewLogger to refuse when key is inside workspace, got nil")
	}

	// 3. Private key path set to outside workspace with valid Ed25519 key -> succeeds
	extDir := t.TempDir()
	secDir := filepath.Join(extDir, ".artix")
	_ = os.MkdirAll(secDir, 0700)
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	extKeyPath := filepath.Join(secDir, "outside_ed25519.key")
	if err := os.WriteFile(extKeyPath, []byte(hex.EncodeToString(privKey)), 0600); err != nil {
		t.Fatal(err)
	}
	entPol.AuditPrivateKeyPath = extKeyPath
	entPol.AuditPublicKey = hex.EncodeToString(pubKey)

	l3 := NewLogger(tempDir)
	defer l3.Close()
	if err := l3.InitError(); err != nil {
		t.Fatalf("expected enterprise NewLogger to succeed with valid outside key, got error: %v", err)
	}
	if err := l3.Emit(AuditEvent{EventType: EventCodeConvergence, Status: "SUCCESS"}); err != nil {
		t.Fatalf("expected successful emit, got: %v", err)
	}

	// Verify the log verifies cleanly with the public key
	res, err := VerifyLogWithOptions(l3.LogPath(), VerifyOptions{PubKey: pubKey})
	if err != nil {
		t.Fatalf("expected log to verify with public key, got: %v", err)
	}
	if res.ValidRecords != 1 {
		t.Fatalf("expected 1 valid record, got %d", res.ValidRecords)
	}
}

// TestR2_6_EnterpriseMode_ForgedRecordWithPolicyHMACKeyIsRejected asserts that an audit record
// signed using the HMAC key (from policy or env) is strictly rejected in enterprise mode,
// where only Ed25519 asymmetric signatures from the external private key are valid.
func TestR2_6_EnterpriseMode_ForgedRecordWithPolicyHMACKeyIsRejected(t *testing.T) {
	tempDir := t.TempDir()
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	hmacSecret := "leaked-or-readable-policy-hmac-key"
	entPol := &policy.Policy{
		RequireSignedPolicy: true,
		IsVerified:          true,
		AuditSigningKey:     hmacSecret,
		AuditPublicKey:      hex.EncodeToString(pubKey),
	}
	policy.SetActivePolicyForTest(entPol)
	defer policy.ResetTestPolicy()

	logPath := filepath.Join(tempDir, "enterprise_audit.jsonl")

	// Craft an audit event signed with the HMAC secret
	event := AuditEvent{
		EventID:   "EVT-FORGED-001",
		Timestamp: time.Now().UTC(),
		EventType: EventCodeConvergence,
		Status:    "SUCCESS",
		PrevHash:  GenesisHash,
	}
	event.RecordHash = ComputeRecordHash(&event)
	// Attacker attempts to forge with HMAC key
	event.Signature = SignRecord(event.RecordHash, hmacSecret)

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	// Verification in enterprise mode MUST reject the HMAC-forged signature
	_, verifyErr := VerifyLogWithOptions(logPath, VerifyOptions{PubKey: pubKey})
	if verifyErr == nil {
		t.Fatalf("SECURITY VIOLATION: enterprise verification accepted record signed with HMAC key! Must strictly require Ed25519 signature.")
	}
	if !strings.Contains(verifyErr.Error(), "invalid ed25519 signature") && !strings.Contains(verifyErr.Error(), "signature") {
		t.Fatalf("expected signature rejection error, got: %v", verifyErr)
	}
}

// TestR3_5_PrivateKeyPath_TestedAgainstSandboxReadablePaths asserts that SetPrivateKeyPath
// rejects private key paths located inside any sandbox-readable directory (workspace, /tmp, os.TempDir())
// and accepts keys in protected/external paths.
func TestR3_5_PrivateKeyPath_TestedAgainstSandboxReadablePaths(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	defer logger.Close()

	// 1. Inside workspace -> rejected
	insideWs := filepath.Join(tempDir, "audit.key")
	_ = os.WriteFile(insideWs, []byte("key"), 0600)
	if err := logger.SetPrivateKeyPath(insideWs); err == nil {
		t.Fatalf("expected error for private key inside workspace, got nil")
	}

	// 2. In /tmp or temp dir root -> rejected if it is world-accessible / readable by sandbox
	inTmp := filepath.Join(os.TempDir(), "artix_world_readable_test.key")
	_ = os.WriteFile(inTmp, []byte("key"), 0600)
	defer os.Remove(inTmp)
	if err := logger.SetPrivateKeyPath(inTmp); err == nil {
		t.Fatalf("expected error for private key in sandbox-accessible temp root %s, got nil", inTmp)
	}

	// 3. In protected directory or external secure directory -> accepted
	extDir := t.TempDir()
	secSubdir := filepath.Join(extDir, ".artix")
	_ = os.MkdirAll(secSubdir, 0700)
	pub, priv, _ := ed25519.GenerateKey(nil)
	extKey := filepath.Join(secSubdir, "audit_ed25519.key")
	_ = os.WriteFile(extKey, []byte(hex.EncodeToString(priv)), 0600)
	_ = pub

	if err := logger.SetPrivateKeyPath(extKey); err != nil {
		t.Fatalf("expected success for private key in secure external path, got: %v", err)
	}
}

func TestR9_SetPrivateKeyPath_MalformedKeys_FailHard(t *testing.T) {
	wsDir := t.TempDir()
	secDir := filepath.Join(t.TempDir(), "secure_keys", ".ssh")
	_ = os.MkdirAll(secDir, 0700)

	logger := NewLogger(wsDir)

	// Case 1: Garbage content
	garbageKey := filepath.Join(secDir, "garbage.key")
	_ = os.WriteFile(garbageKey, []byte("this is not a valid ed25519 key at all"), 0600)
	if err := logger.SetPrivateKeyPath(garbageKey); err == nil {
		t.Fatalf("expected error for garbage private key file, got nil")
	}

	// Case 2: Truncated hex key (10 bytes hex instead of 64 or 128)
	truncatedKey := filepath.Join(secDir, "truncated.key")
	_ = os.WriteFile(truncatedKey, []byte("deadbeef01"), 0600)
	if err := logger.SetPrivateKeyPath(truncatedKey); err == nil {
		t.Fatalf("expected error for truncated private key file, got nil")
	}

	// Case 3: Wrong length raw bytes (e.g. 16 bytes)
	wrongLenKey := filepath.Join(secDir, "wrong_len.key")
	_ = os.WriteFile(wrongLenKey, make([]byte, 16), 0600)
	if err := logger.SetPrivateKeyPath(wrongLenKey); err == nil {
		t.Fatalf("expected error for wrong length raw private key file, got nil")
	}
}

func TestR9_SetPrivateKeyPath_Permissions_RequireOwnerOnly(t *testing.T) {
	wsDir := t.TempDir()
	secDir := filepath.Join(t.TempDir(), "secure_keys", ".ssh")
	_ = os.MkdirAll(secDir, 0700)

	logger := NewLogger(wsDir)
	_, priv, _ := ed25519.GenerateKey(nil)

	// 0644 insecure permissions (group/world readable)
	insecureKey := filepath.Join(secDir, "insecure_0644.key")
	_ = os.WriteFile(insecureKey, []byte(hex.EncodeToString(priv)), 0644)
	_ = os.Chmod(insecureKey, 0644)

	if err := logger.SetPrivateKeyPath(insecureKey); err == nil {
		t.Fatalf("expected error for 0644 permissions on private key file, got nil")
	}

	// 0600 secure permissions (owner only)
	secureKey := filepath.Join(secDir, "secure_0600.key")
	_ = os.WriteFile(secureKey, []byte(hex.EncodeToString(priv)), 0600)
	_ = os.Chmod(secureKey, 0600)

	if err := logger.SetPrivateKeyPath(secureKey); err != nil {
		t.Fatalf("expected success for 0600 permissions, got: %v", err)
	}
}

func TestBoard2_MandatoryExternalSink_EnterpriseMode(t *testing.T) {
	tempDir := t.TempDir()
	secDir := filepath.Join(t.TempDir(), "keys", ".ssh")
	_ = os.MkdirAll(secDir, 0700)
	_, priv, _ := ed25519.GenerateKey(nil)
	keyPath := filepath.Join(secDir, "audit.key")
	_ = os.WriteFile(keyPath, []byte(hex.EncodeToString(priv)), 0600)

	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_AUDIT_PRIVATE_KEY_PATH", keyPath)
	policy.ResetCache()
	defer policy.ResetCache()

	// 1. Without remote sink under ARTIX_ENTERPRISE -> Must fail closed
	loggerNoSink := NewLogger(tempDir)
	if err := loggerNoSink.InitError(); err == nil {
		if errEmit := loggerNoSink.Emit(AuditEvent{EventType: EventSpecDeliberation, Status: "SUCCESS"}); errEmit == nil {
			t.Errorf("NewLogger under ARTIX_ENTERPRISE must fail closed without mandatory external sink (syslog/OTel/S3)")
		}
	}

	// 2. With remote sink under ARTIX_ENTERPRISE -> Sinks allowed and emission succeeds
	received := make(chan AuditEvent, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var evt AuditEvent
		_ = json.NewDecoder(r.Body).Decode(&evt)
		received <- evt
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	loggerWithSink := NewLogger(tempDir)
	loggerWithSink.AddRemoteSink(policy.RemoteSinkConfig{
		Type:     "http",
		Endpoint: server.URL,
	})
	if err := loggerWithSink.Emit(AuditEvent{EventType: EventSpecDeliberation, Status: "SUCCESS"}); err != nil {
		t.Errorf("Emit with mandatory sink failed under enterprise: %v", err)
	}
}

func TestBoard2_AuditLogRedirect_CannotDisableInEnterprise(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ARTIX_ENTERPRISE", "1")
	policy.ResetCache()
	defer policy.ResetCache()

	// 1. ARTIX_AUDIT_LOG=/dev/null or empty/none must NOT disable audit log
	t.Setenv("ARTIX_AUDIT_LOG", "/dev/null")
	path := policy.EffectiveAuditLogPath(tempDir)
	if path == "/dev/null" || path == "" || path == "none" {
		t.Errorf("ARTIX_AUDIT_LOG=/dev/null must not disable audit under enterprise mode; got: %s", path)
	}

	// 2. Valid redirect path should be honored if non-empty and not /dev/null
	customLog := filepath.Join(tempDir, "custom_enterprise_audit.jsonl")
	t.Setenv("ARTIX_AUDIT_LOG", customLog)
	pathCustom := policy.EffectiveAuditLogPath(tempDir)
	if pathCustom != customLog {
		t.Errorf("ARTIX_AUDIT_LOG should allow redirecting log path, expected %s, got %s", customLog, pathCustom)
	}
}

func TestBoard2_VerifyAudit_DetectsTruncationAndEdits(t *testing.T) {
	tempDir := t.TempDir()
	logger := NewLogger(tempDir)
	logger.SetSigningKey("test-board2-key")

	for i := 1; i <= 5; i++ {
		_ = logger.Emit(AuditEvent{
			EventType:   EventCodeConvergence,
			Status:      "SUCCESS",
			Round:       i,
			Cost:        float64(i) * 0.05,
			StorySpecID: "STORY-B2",
		})
	}

	logFile := logger.LogPath()
	res, err := VerifyLog(logFile, "test-board2-key")
	if err != nil {
		t.Fatalf("valid log failed verification: %v", err)
	}
	if res.ValidRecords != 5 {
		t.Fatalf("expected 5 valid records, got %d", res.ValidRecords)
	}

	// 1. Edit record payload -> must be detected
	data, _ := os.ReadFile(logFile)
	tamperedData := strings.Replace(string(data), `"round":3`, `"round":99`, 1)
	tamperedFile := filepath.Join(tempDir, "tampered.jsonl")
	_ = os.WriteFile(tamperedFile, []byte(tamperedData), 0644)
	if _, err := VerifyLog(tamperedFile, "test-board2-key"); err == nil {
		t.Errorf("VerifyLog must detect edited record payload")
	}

	// 2. Truncate middle record -> must detect broken hash chain
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	deletedMiddle := append([]string{}, lines[:2]...)
	deletedMiddle = append(deletedMiddle, lines[3:]...)
	deletedFile := filepath.Join(tempDir, "deleted_middle.jsonl")
	_ = os.WriteFile(deletedFile, []byte(strings.Join(deletedMiddle, "\n")+"\n"), 0644)
	if _, err := VerifyLog(deletedFile, "test-board2-key"); err == nil {
		t.Errorf("VerifyLog must detect deleted middle record")
	}

	// 3. Truncate tail record with expected count -> must detect truncation
	truncatedTail := strings.Join(lines[:3], "\n") + "\n"
	truncFile := filepath.Join(tempDir, "truncated_tail.jsonl")
	_ = os.WriteFile(truncFile, []byte(truncatedTail), 0644)
	opts := VerifyOptions{
		SigningKey:    "test-board2-key",
		ExpectedCount: 5,
	}
	if _, err := VerifyLogWithOptions(truncFile, opts); err == nil {
		t.Errorf("VerifyLogWithOptions must detect tail truncation when expected count is 5")
	}
}




