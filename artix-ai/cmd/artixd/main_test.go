package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"artix/pkg/knowledge"
)

func TestBuildDaemonServer_Success(t *testing.T) {
	tmpDir := t.TempDir()
	server, err := BuildDaemonServer(DaemonOptions{
		Addr:      ":9090",
		GHSecret:  "whsec_12345",
		GLToken:   "gl_tok_67890",
		WorkDir:   tmpDir,
		JobsToken: "jobs_auth_token",
	})
	if err != nil {
		t.Fatalf("expected successful BuildDaemonServer, got: %v", err)
	}
	if server == nil {
		t.Fatalf("expected server to be non-nil")
	}

	handler := server.Handler()

	// Verify healthz endpoint responds 200 OK
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK from /healthz, got %d", rec.Code)
	}
}

func TestBuildDaemonServer_FailsWithoutSecretInEnterpriseMode(t *testing.T) {
	t.Setenv("ARTIX_ENTERPRISE", "1")

	server, err := BuildDaemonServer(DaemonOptions{
		Addr:     ":9090",
		GHSecret: "",
		GLToken:  "",
	})
	if err == nil {
		t.Fatalf("expected error when building daemon server without secrets in enterprise mode, got nil")
	}
	if server != nil {
		t.Errorf("expected server to be nil on error")
	}
}

func TestDaemonServer_JobsRouteAuth(t *testing.T) {
	tmpDir := t.TempDir()
	server, err := BuildDaemonServer(DaemonOptions{
		Addr:      ":9090",
		GHSecret:  "whsec_12345",
		WorkDir:   tmpDir,
		JobsToken: "secret-jobs-token",
	})
	if err != nil {
		t.Fatalf("BuildDaemonServer failed: %v", err)
	}

	handler := server.Handler()

	// 1. Unauthenticated /jobs request -> 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, unauthReq)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for /jobs without token, got %d", rec.Code)
	}

	// 2. Authenticated with token and tenant -> 200
	authReq := httptest.NewRequest(http.MethodGet, "/jobs?tenant=default", nil)
	authReq.Header.Set("Authorization", "Bearer secret-jobs-token")
	recAuth := httptest.NewRecorder()
	handler.ServeHTTP(recAuth, authReq)
	if recAuth.Code != http.StatusOK {
		t.Errorf("expected 200 OK for authenticated /jobs request, got %d", recAuth.Code)
	}
}

func TestBuildDaemonServer_RefusesNonLoopbackWithoutAuth(t *testing.T) {
	// Ensure not in enterprise mode
	t.Setenv("ARTIX_ENTERPRISE", "")
	t.Setenv("KRITIX_ENTERPRISE", "")

	// 1. Non-loopback (:8080 or 0.0.0.0:8080) without webhook secret -> rejected
	_, err := BuildDaemonServer(DaemonOptions{
		Addr:      ":8080",
		GHSecret:  "",
		GLToken:   "",
		JobsToken: "some-jobs-token",
	})
	if err == nil {
		t.Errorf("expected non-loopback bind without webhook secret to be rejected")
	}

	// 2. Non-loopback without jobs token -> rejected
	_, err = BuildDaemonServer(DaemonOptions{
		Addr:      "0.0.0.0:8080",
		GHSecret:  "whsec_123",
		JobsToken: "",
	})
	if err == nil {
		t.Errorf("expected non-loopback bind without jobs token to be rejected")
	}

	// 3. Loopback without auth in non-enterprise dev mode -> permitted
	server, err := BuildDaemonServer(DaemonOptions{
		Addr:      "127.0.0.1:8080",
		GHSecret:  "",
		GLToken:   "",
		JobsToken: "",
	})
	if err != nil {
		t.Errorf("expected loopback bind in non-enterprise mode to succeed, got: %v", err)
	}
	if server == nil {
		t.Errorf("expected server to be non-nil")
	}
}

func TestDaemon_PRReviewCommentToSynthesizedRule_IngestionAndRatification(t *testing.T) {
	tmpDir := t.TempDir()
	secret := "whsec_test_secret_r3"

	server, err := BuildDaemonServer(DaemonOptions{
		Addr:      "127.0.0.1:8080",
		GHSecret:  secret,
		WorkDir:   tmpDir,
		JobsToken: "test-jobs-token",
	})
	if err != nil {
		t.Fatalf("BuildDaemonServer failed: %v", err)
	}

	handler := server.Handler()

	payload := map[string]any{
		"action": "created",
		"pull_request": map[string]any{
			"number": 42,
			"title":  "Add Coroutine Worker",
		},
		"comment": map[string]any{
			"body":               "Never use GlobalScope.launch in our viewmodels, always use viewModelScope!",
			"author_association": "MEMBER",
			"html_url":           "https://github.com/org/repo/pull/42#discussion_r998877",
			"user": map[string]any{
				"login": "senior-engineer",
			},
		},
		"repository": map[string]any{
			"name":      "my-repo",
			"clone_url": "https://github.com/org/my-repo.git",
			"owner": map[string]any{
				"login": "org",
			},
			"default_branch": "main",
		},
		"sender": map[string]any{
			"login":              "senior-engineer",
			"author_association": "MEMBER",
		},
	}
	bodyBytes, _ := json.Marshal(payload)

	// Compute HMAC signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(bodyBytes)
	sigHex := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(bodyBytes))
	req.Header.Set("X-GitHub-Event", "pull_request_review_comment")
	req.Header.Set("X-Hub-Signature-256", sigHex)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
		t.Fatalf("expected 200/202 from PR review comment webhook, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		RuleID string `json:"ruleId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if resp.Status != "proposed" {
		t.Errorf("expected synthesized rule to start with status 'proposed', got %q", resp.Status)
	}
	if resp.RuleID == "" {
		t.Fatalf("expected non-empty ruleId in webhook response")
	}

	// Verify the rule was persisted in the knowledge store
	kStore := knowledge.NewStore(tmpDir)
	ki, err := kStore.Get(resp.RuleID)
	if err != nil {
		t.Fatalf("expected stored rule %s in knowledge store, got: %v", resp.RuleID, err)
	}

	if ki.Status != "proposed" || ki.IsActive() {
		t.Errorf("expected stored rule to be proposed (inactive), got status=%q isActive=%v", ki.Status, ki.IsActive())
	}

	// Ratify rule as CODEOWNER
	if err := ki.Ratify("senior-codeowner", []string{"senior-codeowner"}); err != nil {
		t.Fatalf("failed to ratify rule: %v", err)
	}
	if !ki.IsActive() || ki.Status != "active" {
		t.Errorf("expected ratified rule to be active, got status=%q isActive=%v", ki.Status, ki.IsActive())
	}
}

