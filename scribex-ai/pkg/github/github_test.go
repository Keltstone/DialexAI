package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestValidRepo(t *testing.T) {
	tests := []struct {
		repo string
		want bool
	}{
		{"owner/name", true},
		{"my-org_1.0/my-repo.git", true},
		{"../../orgs/x", false},
		{"a/b/c", false},
		{"a/..", false},
		{"../b", false},
		{"a/.", false},
		{"./b", false},
		{"/x", false},
		{"x/", false},
		{"a /b", false},
		{"a/b c", false},
		{"owner/репо", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.repo, func(t *testing.T) {
			if got := ValidRepo(tc.repo); got != tc.want {
				t.Errorf("ValidRepo(%q) = %v, want %v", tc.repo, got, tc.want)
			}
		})
	}
}

func TestUpsertZeroRequestsOnInvalidArgs(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	var reqCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	badCases := []struct {
		repo string
		pr   int
	}{
		{"../../orgs/x", 1},
		{"invalid", 1},
		{"owner/repo", 0},
		{"owner/repo", -1},
		{"owner/repo", 2147483648},
	}

	for _, bc := range badCases {
		reqCount = 0
		_, err := Upsert(srv.URL, "token", bc.repo, bc.pr, "report")
		if err == nil {
			t.Errorf("Upsert(%q, %d) should fail", bc.repo, bc.pr)
		}
		if reqCount != 0 {
			t.Errorf("Upsert(%q, %d) made %d requests, want 0", bc.repo, bc.pr, reqCount)
		}
	}
}

// 1. Pagination: 250 comments, ours on page 3 -> PATCHed, not duplicated
func TestPaginationFindsOurCommentOnPage3(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "my-bot")

	var patchedID int64
	var posted bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/comments") {
			q := r.URL.Query()
			page := q.Get("page")
			var comments []Comment
			switch page {
			case "1":
				for i := 1; i <= 100; i++ {
					comments = append(comments, Comment{ID: int64(i), Body: fmt.Sprintf("other comment %d", i), User: User{Login: "user1"}})
				}
			case "2":
				for i := 101; i <= 200; i++ {
					comments = append(comments, Comment{ID: int64(i), Body: fmt.Sprintf("other comment %d", i), User: User{Login: "user2"}})
				}
			case "3":
				for i := 201; i <= 249; i++ {
					comments = append(comments, Comment{ID: int64(i), Body: fmt.Sprintf("other comment %d", i), User: User{Login: "user3"}})
				}
				// Ours is comment 250 on page 3
				comments = append(comments, Comment{
					ID:   250,
					Body: "<!-- scribex-report:v1 -->\nold report",
					User: User{Login: "my-bot", Type: "Bot"},
				})
			default:
				comments = []Comment{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(comments)
			return
		}

		if r.Method == "PATCH" && strings.Contains(r.URL.Path, "/issues/comments/250") {
			patchedID = 250
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(Comment{ID: 250, Body: payload["body"], User: User{Login: "my-bot"}})
			return
		}

		if r.Method == "POST" {
			posted = true
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Comment{ID: 999})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	res, err := Upsert(srv.URL, "tok", "owner/repo", 10, "new report")
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if posted {
		t.Errorf("expected no POST, but POST was called")
	}
	if patchedID != 250 {
		t.Errorf("expected comment 250 to be PATCHed, got ID %d", patchedID)
	}
	if res.Action != ActionUpdated || res.ID != 250 {
		t.Errorf("unexpected result: %+v", res)
	}
}

// 2. Pagination exceeds 5000 comments -> error "too many comments"
func TestPaginationLimitExceeded(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "my-bot")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 100 items per page
		var comments []Comment
		for i := 0; i < 100; i++ {
			comments = append(comments, Comment{ID: int64(i + 1), Body: "text", User: User{Login: "someone"}})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comments)
	}))
	defer srv.Close()

	_, err := Upsert(srv.URL, "tok", "owner/repo", 10, "report")
	if err == nil || !strings.Contains(err.Error(), "too many comments") {
		t.Fatalf("expected 'too many comments' error, got %v", err)
	}
}

// 3. Foreign comment starting with marker is ignored (no PATCH, POSTs new)
func TestForeignCommentStartingWithMarkerIgnored(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "scribex-bot")

	var patched bool
	var posted bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/comments") {
			// A hostile / foreign user created a comment starting with marker
			comments := []Comment{
				{
					ID:   42,
					Body: "<!-- scribex-report:v1 -->\nhijacked comment",
					User: User{Login: "attacker", Type: "User"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(comments)
			return
		}
		if r.Method == "PATCH" {
			patched = true
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" {
			posted = true
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Comment{ID: 101, User: User{Login: "scribex-bot", Type: "Bot"}})
			return
		}
	}))
	defer srv.Close()

	res, err := Upsert(srv.URL, "tok", "owner/repo", 10, "my safe report")
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if patched {
		t.Errorf("attacker's comment was hijacked / patched!")
	}
	if !posted {
		t.Errorf("expected new comment to be posted")
	}
	if res.Action != ActionCreated {
		t.Errorf("expected ActionCreated, got %v", res.Action)
	}
}

// 4. Author via /user and via Bot fallback
func TestAuthorResolution(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")

	// Case A: /user returns 200 with user login
	t.Run("UserViaAPI200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/user" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(User{Login: "api-user", Type: "User"})
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/comments") {
				comments := []Comment{
					{ID: 1, Body: "<!-- scribex-report:v1 -->\nold", User: User{Login: "api-user"}},
				}
				json.NewEncoder(w).Encode(comments)
				return
			}
			if r.Method == "PATCH" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}))
		defer srv.Close()

		res, err := Upsert(srv.URL, "tok", "owner/repo", 1, "updated")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != ActionUpdated {
			t.Errorf("expected updated via resolved api-user, got %v", res.Action)
		}
	})

	// Case B: /user returns 401/403 -> falls back to Bot github-actions[bot]
	t.Run("BotFallbackOn403", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/user" {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/comments") {
				comments := []Comment{
					{ID: 2, Body: "<!-- scribex-report:v1 -->\nold", User: User{Login: "github-actions[bot]", Type: "Bot"}},
				}
				json.NewEncoder(w).Encode(comments)
				return
			}
			if r.Method == "PATCH" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}))
		defer srv.Close()

		res, err := Upsert(srv.URL, "tok", "owner/repo", 1, "updated")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != ActionUpdated || res.ID != 2 {
			t.Errorf("expected updated via bot fallback, got %+v", res)
		}
	})
}

// 5. Idempotent upsert: unchanged body -> zero writes
func TestIdempotentUpsertZeroWrites(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "scribex-bot")

	writeCount := 0
	reportContent := "exact identical report content"
	fullBody := MarkerV1 + "\n" + reportContent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			comments := []Comment{
				{ID: 55, Body: fullBody, User: User{Login: "scribex-bot"}},
			}
			json.NewEncoder(w).Encode(comments)
			return
		}
		writeCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res, err := Upsert(srv.URL, "tok", "owner/repo", 1, reportContent)
	if err != nil {
		t.Fatal(err)
	}
	if writeCount != 0 {
		t.Errorf("expected 0 writes for unchanged body, got %d", writeCount)
	}
	if res.Action != ActionUnchanged || res.ID != 55 {
		t.Errorf("expected ActionUnchanged, got %+v", res)
	}
}

// 6. Race repair: concurrent duplicate created -> lowest ID kept, others deleted
func TestRaceRepairKeepsLowestIDAndDeletesOthers(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "scribex-bot")

	var mu sync.Mutex
	comments := []Comment{}
	deletedIDs := []int64{}
	patchedIDs := []int64{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == "GET" {
			json.NewEncoder(w).Encode(comments)
			return
		}

		if r.Method == "POST" {
			// Simultaneously inject a racing competing comment with a lower ID
			competing := Comment{
				ID:   10,
				Body: "<!-- scribex-report:v1 -->\nracing comment",
				User: User{Login: "scribex-bot"},
			}
			newCreated := Comment{
				ID:   20,
				Body: "<!-- scribex-report:v1 -->\nmy comment",
				User: User{Login: "scribex-bot"},
			}
			comments = append(comments, competing, newCreated)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newCreated)
			return
		}

		if r.Method == "PATCH" {
			var id int64
			fmt.Sscanf(r.URL.Path, "/repos/owner/repo/issues/comments/%d", &id)
			patchedIDs = append(patchedIDs, id)
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == "DELETE" {
			var id int64
			fmt.Sscanf(r.URL.Path, "/repos/owner/repo/issues/comments/%d", &id)
			deletedIDs = append(deletedIDs, id)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}))
	defer srv.Close()

	res, err := Upsert(srv.URL, "tok", "owner/repo", 1, "final report")
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Should keep lowest ID (10), delete the other (20), and patch lowest (10)
	if res.ID != 10 {
		t.Errorf("expected lowest ID 10 kept, got %d", res.ID)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(deletedIDs) != 1 || deletedIDs[0] != 20 {
		t.Errorf("expected ID 20 deleted, got deleted list: %v", deletedIDs)
	}
	if len(patchedIDs) != 1 || patchedIDs[0] != 10 {
		t.Errorf("expected ID 10 patched, got patched list: %v", patchedIDs)
	}
}

// 7. Legacy marker accepted and migrated to v1 on update
func TestLegacyMarkerAcceptedAndMigrated(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "scribex-bot")

	var patchedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			comments := []Comment{
				{
					ID:   88,
					Body: "<!-- scribex-report -->\nlegacy report",
					User: User{Login: "scribex-bot"},
				},
			}
			json.NewEncoder(w).Encode(comments)
			return
		}
		if r.Method == "PATCH" {
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			patchedBody = payload["body"]
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer srv.Close()

	res, err := Upsert(srv.URL, "tok", "owner/repo", 1, "new migrated report")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != ActionUpdated || res.ID != 88 {
		t.Errorf("expected updated comment 88, got %+v", res)
	}
	if !strings.HasPrefix(patchedBody, MarkerV1+"\n") {
		t.Errorf("expected patched body to start with MarkerV1, got: %s", patchedBody)
	}
}

// 8. Body length cap: body exceeding 60 000 chars rejected
func TestBodyLengthCapRejection(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	t.Setenv("SCRIBEX_COMMENT_AUTHOR", "scribex-bot")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Comment{})
	}))
	defer srv.Close()

	oversized := strings.Repeat("A", 60001)
	_, err := Upsert(srv.URL, "tok", "owner/repo", 1, oversized)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected length cap error, got %v", err)
	}
}

// 9. Token never appears in output or errors
func TestTokenNeverLeakedInErrors(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	secretToken := "ghp_SECRET_TOKEN_CANARY_98765"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error occurred"))
	}))
	defer srv.Close()

	_, err := Upsert(srv.URL, secretToken, "owner/repo", 1, "report")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Fatalf("token leaked in error: %v", err)
	}
}

// 10. Degrade on forks / read-only token
func TestDegradeOnFork403(t *testing.T) {
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	tmpSummary := t.TempDir() + "/step_summary.md"
	t.Setenv("GITHUB_STEP_SUMMARY", tmpSummary)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "read-only-token")
	t.Setenv("GITHUB_API_URL", srv.URL)

	// Test Upsert directly reports 403 Resource not accessible
	_, err := Upsert(srv.URL, "read-only-token", "owner/repo", 1, "report")
	if err == nil || !strings.Contains(err.Error(), "Resource not accessible") {
		t.Fatalf("expected 403 Resource not accessible, got %v", err)
	}
}
