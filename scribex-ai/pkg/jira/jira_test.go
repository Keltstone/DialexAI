package jira

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestValidKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PROJ-1", true},
		{"AB_C-12", true},
		{"AB-9999999999", true}, // 10 digits
		{"proj-1", false},       // lowercase
		{"PROJ-0", false},       // ends in 0 or begins with 0
		{"PROJ-1/../x", false},
		{"PROJ-1?x=1", false},
		{"PROJ-", false},
		{"PROJ-123456789012", false}, // 12-digit number (> 10 digits)
		{"PROJ-1\n", false},
		{"PROJ-１２３", false}, // unicode digits
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			if got := ValidKey(tc.key); got != tc.want {
				t.Errorf("ValidKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestKey(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"feature/PROJ-123-add-x", "PROJ-123"},
		{"proj-1", ""},
		{"feature/PROJ-1/../../x", "PROJ-1"},
		{"bugfix/AB_C-99-fix", "AB_C-99"},
		{"no-key-here", ""},
	}

	for _, tc := range tests {
		t.Run(tc.branch, func(t *testing.T) {
			got := Key(tc.branch)
			if got != tc.want {
				t.Errorf("Key(%q) = %q, want %q", tc.branch, got, tc.want)
			}
			if got != "" && !ValidKey(got) {
				t.Errorf("Key(%q) returned %q which fails ValidKey", tc.branch, got)
			}
		})
	}
}

func TestClientZeroRequestsOnInvalidKey(t *testing.T) {
	os.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	var reqCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := Client{Base: srv.URL, User: "u", Token: "t"}

	badKeys := []string{"proj-1", "PROJ-0", "PROJ-1/../../x", ""}
	for _, k := range badKeys {
		reqCount = 0
		if _, err := c.Text(k); err == nil {
			t.Errorf("Text(%q) should fail", k)
		}
		if reqCount != 0 {
			t.Errorf("Text(%q) made %d requests, want 0", k, reqCount)
		}

		reqCount = 0
		if err := c.Comment(k, "body"); err == nil {
			t.Errorf("Comment(%q) should fail", k)
		}
		if reqCount != 0 {
			t.Errorf("Comment(%q) made %d requests, want 0", k, reqCount)
		}
	}
}

func TestPostRealityCheckIdempotent(t *testing.T) {
	os.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	var mu sync.Mutex
	var comments []string
	postCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/comment") {
			// Return current comments
			type comm struct {
				Body string `json:"body"`
			}
			var list []comm
			for _, c := range comments {
				list = append(list, comm{Body: c})
			}
			out := map[string]any{
				"startAt":    0,
				"maxResults": 100,
				"total":      len(list),
				"comments":   list,
			}
			b, _ := json.Marshal(out)
			w.Write(b)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/comment") {
			postCount++
			bodyBytes, _ := io.ReadAll(r.Body)
			var payload map[string]string
			json.Unmarshal(bodyBytes, &payload)
			comments = append(comments, payload["body"])
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"1001"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := Client{Base: srv.URL, User: "u", Token: "t"}
	key := "PROJ-123"
	body := "Scribex reality-check for PROJ-123:\n* undeclared breaking change"

	// First post: should succeed and POST
	posted, err := c.PostRealityCheck(key, body)
	if err != nil {
		t.Fatalf("first PostRealityCheck failed: %v", err)
	}
	if !posted {
		t.Fatalf("first PostRealityCheck should have posted")
	}
	if postCount != 1 {
		t.Fatalf("expected 1 POST, got %d", postCount)
	}

	// Second post with identical body: should skip posting (idempotent)
	posted2, err := c.PostRealityCheck(key, body)
	if err != nil {
		t.Fatalf("second PostRealityCheck failed: %v", err)
	}
	if posted2 {
		t.Fatalf("second PostRealityCheck should NOT have posted")
	}
	if postCount != 1 {
		t.Fatalf("expected postCount to remain 1, got %d", postCount)
	}

	// Third test: if stub already lists a comment with the marker upfront, no POST
	postCount = 0
	key2 := "PROJ-456"
	body2 := "Some pre-existing body"
	h2 := sha256.Sum256([]byte(body2))
	comments = []string{
		fmt.Sprintf("Existing comment\n\n<!-- scribex-reality-check:%s -->", hex.EncodeToString(h2[:4])),
	}
	posted3, err := c.PostRealityCheck(key2, body2)
	if err != nil {
		t.Fatalf("PostRealityCheck failed: %v", err)
	}
	if posted3 || postCount != 0 {
		t.Fatalf("expected 0 POST when stub lists comment with marker, got posted=%v postCount=%d", posted3, postCount)
	}
}
