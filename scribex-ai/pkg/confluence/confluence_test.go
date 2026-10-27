package confluence

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type recordedReq struct {
	Method string
	Path   string
	Query  string
	Body   string
}

func setupTestServer(t *testing.T) (*httptest.Server, *[]recordedReq) {
	t.Helper()
	os.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	var mu sync.Mutex
	var reqs []recordedReq

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, recordedReq{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   string(body),
		})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/content/123") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"body":{"storage":{"value":"<p>Storage Content</p>"}}}`))
			return
		}
		if r.Method == "POST" && r.URL.Path == "/wiki/rest/api/content" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"draft-999"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(func() {
		srv.Close()
	})
	return srv, &reqs
}

func TestPullValidation(t *testing.T) {
	srv, reqs := setupTestServer(t)
	c := Client{Base: srv.URL, User: "u", Token: "t"}

	badIDs := []string{
		"123/../../x",
		"1?expand=x",
		"",
		strings.Repeat("9", 21),
	}

	for _, bad := range badIDs {
		t.Run("bad_"+bad, func(t *testing.T) {
			*reqs = nil
			_, err := c.Pull(bad)
			if err == nil {
				t.Fatalf("Pull(%q) should fail", bad)
			}
			if len(*reqs) != 0 {
				t.Fatalf("Pull(%q) made %d requests, want 0", bad, len(*reqs))
			}
		})
	}

	t.Run("valid_pull", func(t *testing.T) {
		*reqs = nil
		val, err := c.Pull("123")
		if err != nil {
			t.Fatalf("Pull(123) failed: %v", err)
		}
		if val != "<p>Storage Content</p>" {
			t.Fatalf("unexpected body: %q", val)
		}
		if len(*reqs) != 1 {
			t.Fatalf("expected 1 request, got %d", len(*reqs))
		}
		r := (*reqs)[0]
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Path != "/wiki/rest/api/content/123" {
			t.Errorf("expected path /wiki/rest/api/content/123, got %s", r.Path)
		}
		if r.Query != "expand=body.storage" {
			t.Errorf("expected query expand=body.storage, got %s", r.Query)
		}
	})
}

func TestCreateDraft(t *testing.T) {
	srv, reqs := setupTestServer(t)
	c := Client{Base: srv.URL, User: "u", Token: "t"}

	// Bad spaces & titles
	badCalls := []struct {
		name  string
		space string
		title string
	}{
		{"invalid space", "!SPACE", "Valid Title"},
		{"empty title", "ENG", ""},
		{"too long title", "ENG", strings.Repeat("A", 256)},
		{"newline in title", "ENG", "Title\nNewline"},
	}

	for _, bc := range badCalls {
		t.Run(bc.name, func(t *testing.T) {
			*reqs = nil
			_, err := c.CreateDraft(bc.space, bc.title, "<p>body</p>")
			if err == nil {
				t.Fatalf("CreateDraft(%q, %q) should fail", bc.space, bc.title)
			}
			if len(*reqs) != 0 {
				t.Fatalf("expected 0 requests, got %d", len(*reqs))
			}
		})
	}

	// Valid draft creation
	t.Run("valid_draft", func(t *testing.T) {
		*reqs = nil
		id, err := c.CreateDraft("ENG", "Valid Title", "<p>content</p>")
		if err != nil {
			t.Fatalf("CreateDraft failed: %v", err)
		}
		if id != "draft-999" {
			t.Fatalf("expected draft id 'draft-999', got %q", id)
		}
		if len(*reqs) != 1 {
			t.Fatalf("expected 1 request, got %d", len(*reqs))
		}
		r := (*reqs)[0]
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Path != "/wiki/rest/api/content" {
			t.Errorf("expected path /wiki/rest/api/content, got %s", r.Path)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(r.Body), &parsed); err != nil {
			t.Fatalf("invalid json body: %v", err)
		}
		if parsed["status"] != "draft" {
			t.Errorf("expected status 'draft', got %v", parsed["status"])
		}
	})
}

func TestStorageEscapingAndSizeCap(t *testing.T) {
	sigs := map[string]string{
		"pkg/a.go#<script>alert(1)</script>": "func Test() string { return `]]>` && '&' }",
	}
	body, err := Body(sigs)
	if err != nil {
		t.Fatalf("Body failed: %v", err)
	}

	// Must contain no raw unescaped <script or ]]>
	if strings.Contains(body, "<script>") || strings.Contains(body, "</script>") {
		t.Errorf("raw <script> tag present in body:\n%s", body)
	}
	if strings.Contains(body, "]]>") {
		t.Errorf("raw ]]> present in body:\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "]]&gt;") {
		t.Errorf("escaped entities missing in body:\n%s", body)
	}

	// 100 kB signature passes
	sigs100k := map[string]string{
		"pkg/big.go#Big": strings.Repeat("x", 100*1024),
	}
	if _, err := Body(sigs100k); err != nil {
		t.Fatalf("100 kB body should succeed: %v", err)
	}

	// > 1 MiB body fails
	sigsBig := map[string]string{
		"pkg/huge.go#Huge": strings.Repeat("x", 1024*1024+100),
	}
	if _, err := Body(sigsBig); err == nil {
		t.Fatalf("body > 1 MiB should return error")
	}
}
