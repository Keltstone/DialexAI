package httpx

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchemeRules(t *testing.T) {
	client := New(Policy{
		AllowHTTP:     false,
		AllowLoopback: false,
	})

	// Userinfo in URL rejected
	err := client.Do("GET", "https://user:pass@example.com/api", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("expected userinfo error, got %v", err)
	}

	// Plain http rejected when not allowed
	err = client.Do("GET", "http://example.com/api", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "http scheme not allowed") {
		t.Errorf("expected http not allowed error, got %v", err)
	}

	// file: scheme rejected
	err = client.Do("GET", "file:///etc/passwd", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected scheme error for file:, got %v", err)
	}

	// ftp: scheme rejected
	err = client.Do("GET", "ftp://example.com/resource", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected scheme error for ftp:, got %v", err)
	}
}

func TestRedirects(t *testing.T) {
	// Server 2 (evil target)
	var server2Requests int32
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&server2Requests, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server2.Close()

	// Server 1 redirects to Server 2
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cross" {
			http.Redirect(w, r, server2.URL+"/pwn", http.StatusFound)
			return
		}
		if r.URL.Path == "/same1" {
			http.Redirect(w, r, "/same2", http.StatusFound)
			return
		}
		if r.URL.Path == "/same2" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"target":"reached"}`))
			return
		}
		if r.URL.Path == "/hop1" {
			http.Redirect(w, r, "/hop2", http.StatusFound)
			return
		}
		if r.URL.Path == "/hop2" {
			http.Redirect(w, r, "/hop3", http.StatusFound)
			return
		}
		if r.URL.Path == "/hop3" {
			http.Redirect(w, r, "/hop4", http.StatusFound)
			return
		}
		if r.URL.Path == "/hop4" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"target":"4"}`))
			return
		}
	}))
	defer server1.Close()

	client := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
	})

	// 1. Cross-host redirect refused: server 2 must receive ZERO requests
	fakeKey := "my-" + "secret-" + "key"
	err := client.Do("GET", server1.URL+"/cross", map[string]string{"x-api-key": fakeKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "cross-host redirect") {
		t.Errorf("expected cross-host redirect error, got %v", err)
	}
	if atomic.LoadInt32(&server2Requests) != 0 {
		t.Errorf("server2 received %d requests, expected 0", server2Requests)
	}

	// 2. Same-host redirect succeeds
	var sameOut map[string]string
	err = client.Do("GET", server1.URL+"/same1", nil, nil, &sameOut)
	if err != nil {
		t.Fatalf("same-host redirect failed: %v", err)
	}
	if sameOut["target"] != "reached" {
		t.Errorf("unexpected output: %v", sameOut)
	}

	// 3. 4th hop refused (stopped after 3 hops)
	err = client.Do("GET", server1.URL+"/hop1", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "stopped after 3 redirects") {
		t.Errorf("expected 3-hop limit error, got %v", err)
	}

	// 4. https -> http redirect refused
	serverHTTPS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/insecure", http.StatusFound)
	}))
	defer serverHTTPS.Close()

	pool := x509.NewCertPool()
	pool.AddCert(serverHTTPS.Certificate())
	clientHTTPS := New(Policy{
		AllowLoopback: true,
		AllowHTTP:     true,
		RootCAs:       pool,
	})
	err = clientHTTPS.Do("GET", serverHTTPS.URL+"/redirect", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "redirect scheme changed") {
		t.Errorf("expected redirect scheme changed error, got %v", err)
	}
}

func TestBodyCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Stream 10 MiB of 'a'
		chunk := strings.Repeat("a", 64*1024)
		for i := 0; i < 160; i++ {
			_, _ = w.Write([]byte(chunk))
		}
	}))
	defer server.Close()

	client := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		MaxBody:       1 << 20, // 1 MiB cap
	})

	err := client.Do("GET", server.URL+"/stream", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Errorf("expected response exceeds cap error, got %v", err)
	}
}

func TestErrorScrubbing(t *testing.T) {
	secretKey := "sk-" + strings.Repeat("k", 24)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// Server echoes the secret from body or header
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"unauthorized with key %s"}`, secretKey)))
	}))
	defer server.Close()

	client := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		Secrets:       []string{secretKey},
	})

	reqURL := fmt.Sprintf("%s/api?token=%s", server.URL, secretKey)
	err := client.Do("GET", reqURL, map[string]string{"Authorization": "Bearer " + secretKey}, nil, nil)
	if err == nil {
		t.Fatal("expected 401 error, got nil")
	}

	errStr := err.Error()
	if strings.Contains(errStr, secretKey) {
		t.Errorf("secret was leaked in error string: %s", errStr)
	}
	if strings.Contains(errStr, "token=") {
		t.Errorf("query parameter was leaked in error string: %s", errStr)
	}
	if !strings.Contains(errStr, "[redacted]") {
		t.Errorf("expected [redacted] in error string, got %s", errStr)
	}
}

func TestRetryBehavior(t *testing.T) {
	// 1. Retry 429 with Retry-After
	var attempts429 int32
	var sleptDurations []time.Duration
	server429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts429, 1)
		if att == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"slow down"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server429.Close()

	client429 := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		MaxAttempts:   3,
		Sleep: func(d time.Duration) {
			sleptDurations = append(sleptDurations, d)
		},
	})

	var out429 map[string]string
	err := client429.Do("GET", server429.URL, nil, nil, &out429)
	if err != nil {
		t.Fatalf("unexpected error on 429 retry: %v", err)
	}
	if attempts429 != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts429)
	}
	if len(sleptDurations) != 1 || sleptDurations[0] != 2*time.Second {
		t.Errorf("expected slept duration 2s, got %v", sleptDurations)
	}

	// 2. 503 x 3 fails after exactly 3 attempts
	var attempts503 int32
	server503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts503, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
	}))
	defer server503.Close()

	client503 := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		MaxAttempts:   3,
		Sleep:         func(d time.Duration) {},
	})

	err = client503.Do("GET", server503.URL, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error after 3 failed attempts, got nil")
	}
	if attempts503 != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", attempts503)
	}

	// 3. POST 500 not retried
	var attemptsPost500 int32
	serverPost500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptsPost500, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer serverPost500.Close()

	clientPost500 := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		MaxAttempts:   3,
		RetryPOST:     true, // Even with RetryPOST, 500 is not 429/502/503/504
		Sleep:         func(d time.Duration) {},
	})

	err = clientPost500.Do("POST", serverPost500.URL, nil, map[string]string{"data": "x"}, nil)
	if err == nil {
		t.Fatal("expected 500 error, got nil")
	}
	if attemptsPost500 != 1 {
		t.Errorf("expected exactly 1 attempt for 500, got %d", attemptsPost500)
	}

	// 4. LLM POST 429 retried, body re-sent identically
	var attemptsLLM int32
	var receivedBodies []string
	serverLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(b))
		att := atomic.AddInt32(&attemptsLLM, 1)
		if att == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"reply":"hello"}`))
	}))
	defer serverLLM.Close()

	clientLLM := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		MaxAttempts:   3,
		RetryPOST:     true,
		Sleep:         func(d time.Duration) {},
	})

	payload := map[string]string{"prompt": "say hi"}
	var outLLM map[string]string
	err = clientLLM.Do("POST", serverLLM.URL, nil, payload, &outLLM)
	if err != nil {
		t.Fatalf("unexpected error on LLM POST retry: %v", err)
	}
	if attemptsLLM != 2 {
		t.Errorf("expected 2 attempts, got %d", attemptsLLM)
	}
	if len(receivedBodies) != 2 || receivedBodies[0] != receivedBodies[1] {
		t.Errorf("bodies were not identical on retry: %v", receivedBodies)
	}
}

func TestTLSRootCAs(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"secure"}`))
	}))
	defer server.Close()

	// 1. Without RootCAs, custom TLS cert fails
	clientDefault := New(Policy{
		AllowLoopback: true,
	})
	err := clientDefault.Do("GET", server.URL, nil, nil, nil)
	if err == nil {
		t.Fatal("expected cert verification failure without RootCAs, got nil")
	}

	// 2. With RootCAs set to test server cert pool, succeeds
	certPool := x509.NewCertPool()
	certPool.AddCert(server.Certificate())

	clientTLS := New(Policy{
		AllowLoopback: true,
		RootCAs:       certPool,
	})
	var out map[string]string
	err = clientTLS.Do("GET", server.URL, nil, nil, &out)
	if err != nil {
		t.Fatalf("expected TLS success with RootCAs, got %v", err)
	}
	if out["status"] != "secure" {
		t.Errorf("unexpected output: %v", out)
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Policy{
		AllowHTTP:     true,
		AllowLoopback: true,
		Timeout:       50 * time.Millisecond,
		MaxAttempts:   1, // no retry
	})

	start := time.Now()
	err := client.Do("GET", server.URL, nil, nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestNoInsecureSkipVerify(t *testing.T) {
	// Walk all .go files under scribex-ai and fail if InsecureSkipVerify appears outside this test
	root := "../.."
	var found []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip this test file itself
		if strings.HasSuffix(path, "httpx_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "InsecureSkipVerify") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Fatalf("InsecureSkipVerify found in files: %v", found)
	}
}
