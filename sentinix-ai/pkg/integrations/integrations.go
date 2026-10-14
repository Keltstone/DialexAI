// Package integrations: chat alerts (Slack, Teams), outbound signed webhooks, and tickets (Jira, Linear).
package integrations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

type Webhook struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

type Config struct {
	Slack        string    `json:"slack"` // incoming webhook URL
	Teams        string    `json:"teams"` // incoming webhook / workflow URL
	Webhooks     []Webhook `json:"webhooks"`
	OnlyCritical bool      `json:"only_critical"` // alert only when new critical findings appear
	Jira         struct {
		Base, Email, Token, Project string
	} `json:"jira"`
	Linear struct {
		Key, Team string
	} `json:"linear"`
	PublicURL string `json:"public_url"` // dashboard link in messages
	LinearURL string `json:"-"`          // test override; defaults to the Linear API
}

type Set struct {
	Config
	HTTP *http.Client
}

// Load reads the config file; values like "$VAR" come from the environment.
func Load(path string) (*Set, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	e := func(v string) string {
		if strings.HasPrefix(v, "$") {
			return os.Getenv(v[1:])
		}
		return v
	}
	c.Slack, c.Teams = e(c.Slack), e(c.Teams)
	c.Jira.Token, c.Linear.Key = e(c.Jira.Token), e(c.Linear.Key)
	for i := range c.Webhooks {
		c.Webhooks[i].URL, c.Webhooks[i].Secret = e(c.Webhooks[i].URL), e(c.Webhooks[i].Secret)
	}
	return &Set{Config: c}, nil
}

func (s *Set) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *Set) send(ctx context.Context, url string, hdr map[string]string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", url, resp.Status) // body omitted: it may echo credentials
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Notify sends chat alerts and webhooks after a scan. With only_critical, nothing is sent unless a new critical appeared.
func (s *Set) Notify(ctx context.Context, p store.Project, sc store.Scan, fs []store.Finding) []error {
	if len(fs) == 0 {
		return nil
	}
	crit := 0
	for _, f := range fs {
		if f.Severity == engines.Critical {
			crit++
		}
	}
	var errs []error
	text := fmt.Sprintf("Sentinix: %d new finding(s) in %s (%d critical), scan %s on %s.", len(fs), p.Name, crit, sc.Mode, sc.Commit)
	if s.PublicURL != "" {
		text += " " + s.PublicURL
	}
	if !s.OnlyCritical || crit > 0 {
		body, _ := json.Marshal(map[string]string{"text": text})
		for _, u := range []string{s.Slack, s.Teams} {
			if u != "" {
				if err := s.send(ctx, u, nil, body, nil); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	type item struct{ Rule, File, Severity string }
	var items []item
	for _, f := range fs {
		items = append(items, item{f.Rule, f.File, f.Severity.String()})
	}
	payload, _ := json.Marshal(map[string]any{"project": p.Name, "scan": sc.ID, "commit": sc.Commit, "mode": sc.Mode, "new": items, "critical": crit})
	for _, w := range s.Webhooks {
		m := hmac.New(sha256.New, []byte(w.Secret))
		m.Write(payload)
		if err := s.send(ctx, w.URL, map[string]string{"X-Sentinix-Signature": "sha256=" + hex.EncodeToString(m.Sum(nil))}, payload, nil); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Ticket files the finding in the chosen tracker and returns its key or URL.
func (s *Set) Ticket(ctx context.Context, tracker string, f store.Finding, project string, due time.Time) (string, error) {
	title := fmt.Sprintf("[%s] %s in %s", f.Severity, f.Rule, f.File)
	desc := fmt.Sprintf("%s\n\nProject: %s\nLocation: %s:%d\nCWE: %s\nFix: %s", f.Message, project, f.File, f.Line, f.CWE, f.Fix)
	switch tracker {
	case "jira":
		j := s.Jira
		if j.Base == "" || j.Project == "" {
			return "", fmt.Errorf("jira is not configured")
		}
		fields := map[string]any{"project": map[string]string{"key": j.Project}, "summary": title, "description": desc, "issuetype": map[string]string{"name": "Task"}}
		if !due.IsZero() {
			fields["duedate"] = due.Format("2006-01-02")
		}
		b, _ := json.Marshal(map[string]any{"fields": fields})
		var out struct{ Key string }
		err := s.send(ctx, strings.TrimRight(j.Base, "/")+"/rest/api/2/issue", map[string]string{"Authorization": "Basic " + basic(j.Email, j.Token)}, b, &out)
		return out.Key, err
	case "linear":
		l := s.Linear
		if l.Key == "" || l.Team == "" {
			return "", fmt.Errorf("linear is not configured")
		}
		in := map[string]string{"teamId": l.Team, "title": title, "description": desc}
		if !due.IsZero() {
			in["dueDate"] = due.Format("2006-01-02")
		}
		b, _ := json.Marshal(map[string]any{
			"query":     "mutation($i: IssueCreateInput!){issueCreate(input:$i){success issue{identifier url}}}",
			"variables": map[string]any{"i": in}})
		var out struct {
			Data struct {
				IssueCreate struct {
					Success bool
					Issue   struct{ Identifier, URL string }
				}
			}
			Errors []struct{ Message string }
		}
		lurl := s.LinearURL
		if lurl == "" {
			lurl = "https://api.linear.app/graphql"
		}
		err := s.send(ctx, lurl, map[string]string{"Authorization": l.Key}, b, &out)
		if err == nil && (!out.Data.IssueCreate.Success || len(out.Errors) > 0) {
			err = fmt.Errorf("linear rejected the issue")
		}
		return out.Data.IssueCreate.Issue.Identifier, err
	}
	return "", fmt.Errorf("unknown tracker %q", tracker)
}

func basic(user, pass string) string {
	return base64Std(user + ":" + pass)
}

func base64Std(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
