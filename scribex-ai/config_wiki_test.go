package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scribex/pkg/anchors"
	"scribex/pkg/config"
	"scribex/pkg/llm"
	"scribex/pkg/mcp"
	"scribex/pkg/secrets"
	"scribex/pkg/wiki"
)

func TestConfigAndSecrets(t *testing.T) {
	d := t.TempDir()
	if err := config.Set(d, "ai.model", "m1"); err != nil {
		t.Fatal(err)
	}
	if c, _ := config.Load(d); c.AI.Model != "m1" || c.Wiki.Dir != "docs/wiki" {
		t.Fatalf("%+v", c)
	}
	if config.Set(d, "ai.api_key_env", "sk-ant-abc123") == nil {
		t.Fatal("pasted key must be rejected")
	}
	if config.Set(d, "nope", "x") == nil {
		t.Fatal("unknown key")
	}
	if err := secrets.Set(d, "MY_KEY", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(secrets.File(d)); st.Mode().Perm() != 0o600 {
		t.Fatal("perm", st.Mode())
	}
	if b, _ := os.ReadFile(filepath.Join(d, ".gitignore")); !strings.Contains(string(b), "secrets.env") {
		t.Fatal("not gitignored")
	}
	os.Unsetenv("MY_KEY")
	secrets.Load(d)
	if os.Getenv("MY_KEY") != "s3cret" {
		t.Fatal("load")
	}
}

func TestAIRewriteOnlyForChangedSyncedAnchors(t *testing.T) {
	look := func(f, s string) (string, bool, error) { return "func A(x int)", true, nil }
	calls := 0
	prose := func(ref, sig, old string) (string, error) { calls++; return "new prose", nil }
	// never synced -> no LLM call
	_, out := anchors.Process("<!-- @scribex:anchor(a.go#A) -->\nold\n<!-- @scribex:end -->", look, true, prose)
	if calls != 0 || !strings.Contains(out, "old") {
		t.Fatal("first sync must not spend tokens", calls, out)
	}
	// synced, then signature changes -> rewrite
	stale := strings.Replace(out, "sig=", "sig=0", 1)
	_, out2 := anchors.Process(stale, look, true, prose)
	if calls != 1 || !strings.Contains(out2, "new prose") || strings.Contains(out2, "\nold") {
		t.Fatal(calls, out2)
	}
}

func TestLLMAnthropicShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if r.Header.Get("x-api-key") != "k" || in["model"] != "m" {
			w.WriteHeader(400)
		}
		w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
	}))
	defer srv.Close()
	os.Setenv("TEST_LLM_KEY", "k")
	t.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
	u, _ := url.Parse(srv.URL)
	t.Setenv("SCRIBEX_AI_ALLOWED_HOSTS", u.Host)
	c, err := llm.New(config.AI{Provider: "anthropic", Model: "m", APIKeyEnv: "TEST_LLM_KEY", BaseURL: srv.URL, MaxTokens: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Complete("s", "u"); err != nil || got != "hi" {
		t.Fatal(got, err)
	}
}

func TestMCPConfigAndServer(t *testing.T) {
	if (mcp.Server{Command: "x", Env: map[string]string{"API_TOKEN": "literal"}}).Validate() == nil {
		t.Fatal("literal secret accepted")
	}
	s := mcp.Presets["atlassian-token"]
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("ATLASSIAN_TOKEN")
	if len(s.Missing()) == 0 {
		t.Fatal("missing not detected")
	}
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "docs/wiki"), 0o755)
	os.WriteFile(filepath.Join(d, "docs/wiki/Home.md"), []byte("# Home\n[[Auth Guide]]\n"), 0o644)
	os.WriteFile(filepath.Join(d, "docs/wiki/Auth-Guide.md"), []byte("# Auth\nsee [[Nope]]\n"), 0o644)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scribex_wiki_lint","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := mcp.Serve(strings.NewReader(in), &out, d); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "scribex_check") || !strings.Contains(lines[2], "broken link") {
		t.Fatal(out.String())
	}
}

func TestWikiLintAndRender(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "Home.md"), []byte("# Home\n[[getting started|Start]] and [x](Other.md)\n"), 0o644)
	os.WriteFile(filepath.Join(d, "Getting-Started.md"), []byte("---\ntitle: Getting Started\ntags: [intro]\n---\nback to [[Home]] [[Ghost]]\n"), 0o644)
	os.WriteFile(filepath.Join(d, "Lonely.md"), []byte("# Lonely\n"), 0o644)
	w, err := wiki.Load(d)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(w.Lint("Home"), "\n")
	for _, want := range []string{"Other.md", "[[Ghost]]", "Lonely.md orphan"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "Getting-Started.md orphan") {
		t.Fatal("linked page flagged orphan")
	}
	out := filepath.Join(t.TempDir(), "pub")
	if err := w.Render(out, "Home"); err != nil {
		t.Fatal(err)
	}
	h, _ := os.ReadFile(filepath.Join(out, "Home.md"))
	g, _ := os.ReadFile(filepath.Join(out, "Getting-Started.md"))
	if !strings.Contains(string(h), "[Start](Getting-Started.md)") || !strings.Contains(string(g), "Linked from: [Home](Home.md)") {
		t.Fatalf("%s\n---\n%s", h, g)
	}
	if _, err := os.Stat(filepath.Join(out, "_Sidebar.md")); err != nil {
		t.Fatal(err)
	}
}
