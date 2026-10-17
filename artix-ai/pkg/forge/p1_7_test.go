package forge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestP1_7_LargeUnauthenticatedBody_RejectedByMaxBytesReader(t *testing.T) {
	srv := NewWebhookServer(WebhookServerConfig{
		GitHubSecret: "test-secret",
	})
	handler := srv.Handler()

	// 15 MB dummy payload
	largeBody := bytes.Repeat([]byte("A"), 15*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(largeBody))
	req.Header.Set("X-GitHub-Event", "issues")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 413 Request Entity Too Large or 400 Bad Request, got: %d", rec.Code)
	}
}

func TestP1_7_AtomicQueueCapEnforcement(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewWebhookServer(WebhookServerConfig{
		JobsAuthToken:     "secret-token",
		MaxConcurrentJobs: 2,
		MaxQueuedJobs:     0, // zero queue: only 2 slots total
		AllowedRoots:      []string{tmpDir},
		AllowedRepoIDs: map[string]string{
			"repo1": tmpDir,
		},
	})
	handler := srv.Handler()

	n := 100
	var wg sync.WaitGroup
	wg.Add(n)

	statusCodes := make([]int, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			body := []byte(`{"prompt":"test","repoId":"repo1"}`)
			req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer secret-token")
			req.Header.Set("X-Artix-Tenant", "tenant-1")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			statusCodes[idx] = rec.Code
		}(i)
	}
	wg.Wait()

	accepted := 0
	rateLimited := 0
	for _, code := range statusCodes {
		if code == http.StatusAccepted {
			accepted++
		} else if code == http.StatusTooManyRequests {
			rateLimited++
		}
	}

	if accepted > 2 {
		t.Fatalf("SECURITY FLAW: queue cap not enforced atomically! Accepted %d jobs when cap was 2", accepted)
	}
	if rateLimited != n-accepted {
		t.Fatalf("expected remaining requests to receive 429, accepted: %d, rateLimited: %d", accepted, rateLimited)
	}
}

func TestP1_7_RestartWedge_RecoversStaleRunningJobs(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	// Simulate 8 jobs left in "running" state from a prior crash
	staleJobs := make(map[string]*JobStatus)
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("stale-%d", i)
		staleJobs[id] = &JobStatus{
			ID:        id,
			Status:    "running",
			CreatedAt: time.Now().Add(-1 * time.Hour),
		}
	}
	stateData, _ := json.Marshal(daemonPersistentState{Jobs: staleJobs})
	_ = os.WriteFile(stateFile, stateData, 0600)

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:       stateFile,
		JobsAuthToken:     "secret-token",
		MaxConcurrentJobs: 4,
		MaxQueuedJobs:     4,
		AllowedRoots:      []string{tmpDir},
		AllowedRepoIDs: map[string]string{
			"repo1": tmpDir,
		},
	})

	// Verify all stale jobs were marked failed
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("stale-%d", i)
		j, ok := srv.GetJob(id)
		if !ok || j.Status == "running" {
			t.Fatalf("RESTART WEDGE: job %s still has status %q after server restart", id, j.Status)
		}
	}

	// Server must be able to accept new tasks now!
	handler := srv.Handler()
	body := []byte(`{"prompt":"test","repoId":"repo1"}`)
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Artix-Tenant", "tenant-1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("RESTART WEDGE: server rejected new task with code %d: %s", rec.Code, rec.Body.String())
	}
}

func TestP1_7_DefaultDaemon_RequiresAuth(t *testing.T) {
	srv := NewWebhookServer(WebhookServerConfig{
		RequireAuth:        true,
		RequireWebhookAuth: true,
	})
	handler := srv.Handler()

	for _, path := range []string{"/jobs", "/tasks", "/readyz", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
			t.Fatalf("SECURITY VULNERABILITY: endpoint %s returned code %d unauthenticated in default build!", path, rec.Code)
		}
	}
}
