package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

func fnd(sev engines.Severity) store.Finding {
	return store.Finding{Finding: engines.Finding{Rule: "r", File: "a.kt", Line: 3, Severity: sev, Message: "m"}}
}

func TestNotifyRespectsOnlyCriticalAndSignsWebhooks(t *testing.T) {
	var chat, hooks int
	var sigOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/chat" {
			chat++
			return
		}
		hooks++
		m := hmac.New(sha256.New, []byte("s3"))
		m.Write(b)
		sigOK = r.Header.Get("X-Sentinix-Signature") == "sha256="+hex.EncodeToString(m.Sum(nil))
	}))
	defer srv.Close()
	s := &Set{Config: Config{Slack: srv.URL + "/chat", Teams: srv.URL + "/chat", OnlyCritical: true, Webhooks: []Webhook{{srv.URL + "/hook", "s3"}}}}
	p, sc := store.Project{Name: "p"}, store.Scan{Mode: "full"}
	if errs := s.Notify(context.Background(), p, sc, []store.Finding{fnd(engines.High)}); len(errs) != 0 || chat != 0 || hooks != 1 || !sigOK {
		t.Fatalf("high: errs=%v chat=%d hooks=%d sig=%v", errs, chat, hooks, sigOK)
	}
	s.Notify(context.Background(), p, sc, []store.Finding{fnd(engines.Critical)})
	if chat != 2 {
		t.Fatalf("critical should alert both channels, got %d", chat)
	}
}

func TestTickets(t *testing.T) {
	var jiraAuth, jiraBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.HasPrefix(r.URL.Path, "/rest/api/2/issue") {
			jiraAuth, jiraBody = r.Header.Get("Authorization"), string(b)
			w.Write([]byte(`{"key":"SEC-7"}`))
			return
		}
		w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"identifier":"ENG-9","url":"u"}}}}`))
	}))
	defer srv.Close()
	s := &Set{}
	s.Jira.Base, s.Jira.Email, s.Jira.Token, s.Jira.Project = srv.URL, "u@example.test", "tok", "SEC"
	s.Linear.Key, s.Linear.Team, s.LinearURL = "k", "t", srv.URL+"/graphql"
	due := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if k, err := s.Ticket(context.Background(), "jira", fnd(engines.High), "p", due); err != nil || k != "SEC-7" || !strings.HasPrefix(jiraAuth, "Basic ") || !strings.Contains(jiraBody, `"duedate":"2026-11-01"`) {
		t.Fatalf("%q %v %s", k, err, jiraBody)
	}
	if k, err := s.Ticket(context.Background(), "linear", fnd(engines.High), "p", due); err != nil || k != "ENG-9" {
		t.Fatalf("%q %v", k, err)
	}
	if _, err := (&Set{}).Ticket(context.Background(), "jira", fnd(engines.High), "p", due); err == nil {
		t.Fatal("unconfigured tracker accepted")
	}
}
