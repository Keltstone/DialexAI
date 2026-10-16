package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/store"
)

// TestProbe2RateLimitStarvesWebhooks tests that:
// 1. Webhooks (/webhook/*) and /healthz do not return 429 even after 400 requests from one proxy IP.
// 2. Behind a trusted proxy, rate limiting keys on X-Forwarded-For per client, not the proxy IP.
func TestProbe2RateLimitStarvesWebhooks(t *testing.T) {
	st, _ := store.Open("sqlite::memory:")
	a := &App{
		Store:          st,
		Sess:           auth.Sessions{Key: []byte("test-session-key-0123456789012345")},
		TrustedProxies: []string{"10.0.0.5"},
	}
	h := a.Handler()

	// 1. Send 400 webhooks from ingress/proxy IP: none should receive 429
	codes := map[int]int{}
	for i := 0; i < 400; i++ {
		r := httptest.NewRequest("POST", "/webhook/gh", nil)
		r.RemoteAddr = "10.0.0.5:4444"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		codes[w.Code]++
	}
	if codes[http.StatusTooManyRequests] > 0 {
		t.Fatalf("webhooks were rate-limited: got %d 429s in %v", codes[http.StatusTooManyRequests], codes)
	}

	// 2. /healthz must not return 429 afterwards
	rHealth := httptest.NewRequest("GET", "/healthz", nil)
	rHealth.RemoteAddr = "10.0.0.5:4444"
	wHealth := httptest.NewRecorder()
	h.ServeHTTP(wHealth, rHealth)
	t.Logf("webhook status histogram from one proxy IP: %v ; /healthz afterwards: %d", codes, wHealth.Code)
	if wHealth.Code != http.StatusOK {
		t.Fatalf("/healthz returned %d, expected 200", wHealth.Code)
	}

	// 3. Behind a trusted proxy, rate limiting is per-client IP via X-Forwarded-For
	// Send 305 API requests from client A (1.2.3.4)
	var clientACodes map[int]int = map[int]int{}
	for i := 0; i < 305; i++ {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.RemoteAddr = "10.0.0.5:4444"
		r.Header.Set("X-Forwarded-For", "1.2.3.4")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		clientACodes[w.Code]++
	}
	t.Logf("Client A (1.2.3.4) codes: %v", clientACodes)
	if clientACodes[http.StatusTooManyRequests] == 0 {
		t.Fatalf("expected client A to be rate-limited after 300 requests, got %v", clientACodes)
	}

	// Client B (1.2.3.5) from the same proxy IP must NOT be rate-limited (has fresh budget)
	rB := httptest.NewRequest("GET", "/api/me", nil)
	rB.RemoteAddr = "10.0.0.5:4444"
	rB.Header.Set("X-Forwarded-For", "1.2.3.5")
	wB := httptest.NewRecorder()
	h.ServeHTTP(wB, rB)
	t.Logf("Client B (1.2.3.5) code: %d", wB.Code)
	if wB.Code == http.StatusTooManyRequests {
		t.Fatalf("client B should not be rate-limited by client A's requests through same proxy")
	}
}

func TestRateLimiterPrune(t *testing.T) {
	st, _ := store.Open("sqlite::memory:")
	a := &App{
		Store: st,
		Sess:  auth.Sessions{Key: []byte("test-session-key-0123456789012345")},
	}
	rl := newRateLimiter(a)
	rl.clients["expired-client"] = &clientRate{
		count:     10,
		resetTime: time.Now().Add(-10 * time.Minute),
	}
	rl.clients["active-client"] = &clientRate{
		count:     5,
		resetTime: time.Now().Add(10 * time.Minute),
	}

	rl.prune(time.Now())

	if _, exists := rl.clients["expired-client"]; exists {
		t.Errorf("expired-client should have been pruned")
	}
	if _, exists := rl.clients["active-client"]; !exists {
		t.Errorf("active-client should still exist")
	}
}
