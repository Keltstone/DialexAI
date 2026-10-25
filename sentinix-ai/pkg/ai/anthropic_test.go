package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnthropicAndProviderConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "k" {
			http.Error(w, "bad", 400)
			return
		}
		w.Write([]byte(`{"content":[{"text":"{\"ok\":true}"}]}`))
	}))
	defer srv.Close()
	p := filepath.Join(t.TempDir(), "m.json")
	os.WriteFile(p, []byte(`[{"name":"a","provider":"anthropic","url":"`+srv.URL+`","key":"k","model":"m"},{"name":"b","url":"http://x","model":"m","local":true}]`), 0o600)
	ms, err := LoadModels(p)
	if err != nil || len(ms) != 2 {
		t.Fatal(ms, err)
	}
	if _, ok := ms[1].(Chat); !ok {
		t.Fatal("openai-compatible default lost")
	}
	out, err := ms[0].Complete(context.Background(), "s", "u")
	if err != nil || !strings.Contains(out, "ok") {
		t.Fatal(out, err)
	}
}
