package forge

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"artix/pkg/policy"
)

func TestReadyz_HealthyReturns200(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(tempDir, "jobs.json"),
		OfflineCacheDir: cacheDir,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for healthy /readyz, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReadyz_UnwritableJobStore_Returns503(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	// Unwritable storage path: points to a read-only dir or invalid path
	readOnlyDir := filepath.Join(tempDir, "readonly")
	_ = os.MkdirAll(readOnlyDir, 0555)
	_ = os.Chmod(readOnlyDir, 0555)

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(readOnlyDir, "cannot_write", "jobs.json"),
		OfflineCacheDir: cacheDir,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unwritable job store, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReadyz_EnterpriseAuditSinkDown_Returns503(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	// Set enterprise mode with dead sink
	t.Setenv("ARTIX_ENTERPRISE", "1")
	t.Setenv("ARTIX_AUDIT_PRIVATE_KEY_PATH", filepath.Join(tempDir, "fake.key"))
	// Create fake key
	fakeKey := make([]byte, 64)
	_ = os.WriteFile(filepath.Join(tempDir, "fake.key"), []byte(hex.EncodeToString(fakeKey)), 0600)

	// Dead remote sink on closed local port
	t.Setenv("ARTIX_AUDIT_REMOTE_SINK", "http://127.0.0.1:59999/sink")
	policy.ResetCachedPolicy()

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(tempDir, "jobs.json"),
		OfflineCacheDir: cacheDir,
		JobsAuthToken:   "ent-token-123",
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Authorization", "Bearer ent-token-123")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when enterprise audit sink is down, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEnterpriseMode_NoToken_FailsClosed_Returns401_GenericBody(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	t.Setenv("ARTIX_ENTERPRISE", "1")
	policy.ResetCachedPolicy()

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(tempDir, "jobs.json"),
		OfflineCacheDir: cacheDir,
		JobsAuthToken:   "", // no token set in enterprise mode
	})

	for _, endpoint := range []string{"/readyz", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for %s in enterprise mode without token, got %d", endpoint, rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, tempDir) || strings.Contains(body, "/") {
			t.Errorf("expected generic error body without filesystem paths for %s, got: %q", endpoint, body)
		}
	}
}

func TestReadyz_MissingOfflineCache_Returns503(t *testing.T) {
	tempDir := t.TempDir()

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(tempDir, "jobs.json"),
		OfflineCacheDir: filepath.Join(tempDir, "non_existent_cache_dir"),
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when offline cache is missing, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReadyz_And_Metrics_AuthEnforcedWithToken(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(tempDir, "jobs.json"),
		OfflineCacheDir: cacheDir,
		JobsAuthToken:   "secret-token-123",
	})

	for _, endpoint := range []string{"/readyz", "/metrics"} {
		// 1. Unauthenticated -> 401
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauthenticated %s, got %d", endpoint, rec.Code)
		}

		// 2. Authenticated -> 200
		reqAuth := httptest.NewRequest(http.MethodGet, endpoint, nil)
		reqAuth.Header.Set("Authorization", "Bearer secret-token-123")
		recAuth := httptest.NewRecorder()
		srv.Handler().ServeHTTP(recAuth, reqAuth)

		if recAuth.Code != http.StatusOK {
			t.Errorf("expected 200 for authenticated %s, got %d: %s", endpoint, recAuth.Code, recAuth.Body.String())
		}
	}
}

func TestMetrics_PrometheusExposition_ContainsAllSeries(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	srv := NewWebhookServer(WebhookServerConfig{
		StoragePath:     filepath.Join(tempDir, "jobs.json"),
		OfflineCacheDir: cacheDir,
	})

	// Record conditions to increment metrics
	RecordJobMetric("completed", 2, 1500, 0.05)
	RecordJobMetric("failed", 1, 500, 0.01)
	RecordPhaseDuration("test", 1.25)
	RecordOfflineCacheMiss()
	RecordTimeout()
	RecordSkippedHostUnsupported()
	SetQueueDepth(3)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()

	requiredSeries := []string{
		"artix_jobs_total",
		"artix_rounds_per_job",
		"artix_tokens_total",
		"artix_cost_usd_total",
		"artix_phase_duration_seconds",
		"artix_queue_depth",
		"OFFLINE_CACHE_MISS",
		"TIMEOUT",
		"SKIPPED_HOST_UNSUPPORTED",
	}

	for _, s := range requiredSeries {
		if !strings.Contains(body, s) {
			t.Errorf("expected /metrics exposition to contain series %q, but body was:\n%s", s, body)
		}
	}
}
