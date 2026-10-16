package githost

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sig(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func req(hdr map[string]string) *http.Request {
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func TestParse(t *testing.T) {
	gh := `{"action":"opened","number":7,"pull_request":{"head":{"sha":"abc","ref":"f","repo":{"full_name":"fork/r","clone_url":"https://h/fork/r.git"}},"base":{"ref":"main","repo":{"full_name":"o/r","clone_url":"https://h/o/r.git"}}}}`
	glb := `{"object_kind":"merge_request","object_attributes":{"action":"open","iid":3,"target_branch":"main","source_project_id":2,"target_project_id":1,"last_commit":{"id":"abc"}}}`
	bb := `{"pullrequest":{"id":9,"source":{"commit":{"hash":"abc"},"branch":{"name":"f"},"repository":{"full_name":"w/r","links":{"html":{"href":"https://bb/w/r"}}}},"destination":{"branch":{"name":"main"},"repository":{"full_name":"w/r","links":{"html":{"href":"https://bb/w/r"}}}}}}`
	cases := []struct {
		kind, body string
		hdr        map[string]string
		pr         string
		fork       bool
	}{
		{"github", gh, map[string]string{"X-Hub-Signature-256": sig("s", gh), "X-GitHub-Event": "pull_request"}, "7", true},
		{"gitlab", glb, map[string]string{"X-Gitlab-Token": "s"}, "3", true},
		{"bitbucket", bb, map[string]string{"X-Hub-Signature": sig("s", bb), "X-Event-Key": "pullrequest:updated"}, "9", false},
	}
	for _, c := range cases {
		h, err := New(Config{Name: c.kind, Kind: c.kind, Secret: "s", Token: "t"})
		if err != nil {
			t.Fatal(err)
		}
		ev, err := h.Parse(req(c.hdr), []byte(c.body))
		if err != nil || ev == nil || ev.PR != c.pr || ev.Fork != c.fork || ev.BaseRef != "main" || ev.HeadSHA != "abc" {
			t.Errorf("%s: %+v %v", c.kind, ev, err)
		}
		if ev != nil && ev.ConfiguredHost == "" {
			t.Errorf("%s: ConfiguredHost not set on Event", c.kind)
		}
		if h.ConfiguredHost() == "" {
			t.Errorf("%s: Host.ConfiguredHost() is empty", c.kind)
		}
		bad := map[string]string{}
		if _, err := h.Parse(req(bad), []byte(c.body)); err == nil {
			t.Errorf("%s: unsigned request accepted", c.kind)
		}
	}

	// Test self-hosted GHE and self-managed GitLab
	ghe, _ := New(Config{Name: "ghe", Kind: "github", API: "https://ghe.corp.example/api/v3", Secret: "s", Token: "t"})
	evGHE, err := ghe.Parse(req(map[string]string{"X-Hub-Signature-256": sig("s", gh), "X-GitHub-Event": "pull_request"}), []byte(gh))
	if err != nil || evGHE == nil || evGHE.ConfiguredHost != "https://ghe.corp.example" {
		t.Fatalf("GHE ConfiguredHost mismatch: got %v, err %v", evGHE, err)
	}

	glSelf, _ := New(Config{Name: "gl", Kind: "gitlab", API: "https://gitlab.corp.example/api/v4", Secret: "s", Token: "t"})
	evGL, err := glSelf.Parse(req(map[string]string{"X-Gitlab-Token": "s"}), []byte(glb))
	if err != nil || evGL == nil || evGL.ConfiguredHost != "https://gitlab.corp.example" {
		t.Fatalf("GitLab self-managed ConfiguredHost mismatch: got %v, err %v", evGL, err)
	}
}
