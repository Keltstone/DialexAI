package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sentinix/pkg/auth"
	"sentinix/pkg/githost"
	"sentinix/pkg/store"
)

type webhookTestHost struct {
	name   string
	secret string
}

func (h *webhookTestHost) Name() string            { return h.name }
func (h *webhookTestHost) ConfiguredHost() string  { return "github.com" }
func (h *webhookTestHost) RepoURL(r string) string { return "https://github.com/" + r }
func (h *webhookTestHost) CloneAuth() (string, string) {
	return "x-access-token", "tok"
}
func (h *webhookTestHost) Status(ctx context.Context, e *githost.Event, s githost.State, desc string) error {
	return nil
}
func (h *webhookTestHost) Comment(ctx context.Context, e *githost.Event, body string) error {
	return nil
}
func (h *webhookTestHost) Parse(r *http.Request, body []byte) (*githost.Event, error) {
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		return nil, errors.New("bad signature")
	}
	var ev githost.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhook_PreAuthBoundedRead(t *testing.T) {
	st, err := store.Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	host := &webhookTestHost{name: "gh", secret: "test-secret"}
	app := &App{
		Store: st,
		Hosts: map[string]githost.Host{"gh": host},
		Sess:  auth.Sessions{Key: []byte("test-session-key")},
	}
	handler := app.Handler()

	// 1. Oversized body > 5 MiB should be rejected before auth with 400 Bad Request
	oversized := make([]byte, 6<<20) // 6 MiB
	req := httptest.NewRequest("POST", "/webhook/gh", bytes.NewReader(oversized))
	req.Header.Set("X-Hub-Signature-256", signBody(host.secret, oversized))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized payload > 5 MiB, got: %d", rec.Code)
	}

	// 2. Body within limit (< 5 MiB) with invalid signature should be 401 Unauthorized
	validPayload := []byte(`{"repo":"org/repo","pr":"1","head_sha":"abc","base_sha":"def"}`)
	reqBadSig := httptest.NewRequest("POST", "/webhook/gh", bytes.NewReader(validPayload))
	reqBadSig.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	recBadSig := httptest.NewRecorder()
	handler.ServeHTTP(recBadSig, reqBadSig)

	if recBadSig.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad signature, got: %d", recBadSig.Code)
	}
}

func TestWebhook_EnqueueFailureThenRedeliveryResultsInScan(t *testing.T) {
	st, err := store.Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	host := &webhookTestHost{name: "gh", secret: "test-secret"}
	app := &App{
		Store: st,
		Hosts: map[string]githost.Host{"gh": host},
		Sess:  auth.Sessions{Key: []byte("test-session-key")},
	}
	handler := app.Handler()

	payload := []byte(`{"repo":"org/repo","pr":"42","head_sha":"1111111111111111111111111111111111111111","base_sha":"0000000000000000000000000000000000000000"}`)
	sig := signBody(host.secret, payload)
	deliveryID := "delivery-uuid-9999"

	// Force EnqueueJob failure by temporarily renaming durable_queue table
	db := st.DB()
	if _, err := db.Exec("ALTER TABLE durable_queue RENAME TO durable_queue_backup"); err != nil {
		t.Fatal(err)
	}

	// First attempt: should fail at enqueue
	req1 := httptest.NewRequest("POST", "/webhook/gh", bytes.NewReader(payload))
	req1.Header.Set("X-Hub-Signature-256", sig)
	req1.Header.Set("X-GitHub-Delivery", deliveryID)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when EnqueueJob fails, got: %d", rec1.Code)
	}

	// Restore durable_queue table
	if _, err := db.Exec("ALTER TABLE durable_queue_backup RENAME TO durable_queue"); err != nil {
		t.Fatal(err)
	}

	// Redelivery with the EXACT same delivery ID: should succeed and result in a scan/job in queue
	req2 := httptest.NewRequest("POST", "/webhook/gh", bytes.NewReader(payload))
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("X-GitHub-Delivery", deliveryID)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on redelivery after enqueue fix, got: %d", rec2.Code)
	}

	// Verify that the job was indeed enqueued and ready for scan
	jobID, hostName, queuedPayload, err := st.DequeueJob()
	if err != nil {
		t.Fatalf("expected job to be in queue after redelivery, got err: %v", err)
	}
	if jobID == "" || hostName != "gh" || !strings.Contains(queuedPayload, "42") {
		t.Fatalf("expected queued job for PR 42, got jobID=%q, host=%q, payload=%q", jobID, hostName, queuedPayload)
	}
}
