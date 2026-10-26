package github

import (
	"net/http"
	"net/http/httptest"
	"os"
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
		{"owner/репо", false}, // unicode
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
	os.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
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
		{"owner/repo", 2147483648}, // > 2^31-1
	}

	for _, bc := range badCases {
		reqCount = 0
		err := Upsert(srv.URL, "token", bc.repo, bc.pr, "report")
		if err == nil {
			t.Errorf("Upsert(%q, %d) should fail", bc.repo, bc.pr)
		}
		if reqCount != 0 {
			t.Errorf("Upsert(%q, %d) made %d requests, want 0", bc.repo, bc.pr, reqCount)
		}
	}
}
