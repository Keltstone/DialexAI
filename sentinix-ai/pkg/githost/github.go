package githost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type github struct{ Config }

func (g github) Name() string                { return g.Config.Name }
func (g github) CloneAuth() (string, string) { return "x-access-token", g.Token }

func (g github) hdr() map[string]string {
	return map[string]string{"Authorization": "Bearer " + g.Token, "Accept": "application/vnd.github+json"}
}

func (g github) Parse(r *http.Request, body []byte) (*Event, error) {
	if !hmacOK(g.Secret, r.Header.Get("X-Hub-Signature-256"), body) {
		return nil, fmt.Errorf("bad signature")
	}
	if r.Header.Get("X-GitHub-Event") != "pull_request" {
		return nil, nil
	}
	type repo struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	}
	var p struct {
		Action string
		Number int
		PR     struct {
			Head struct {
				SHA, Ref string
				Repo     repo
			}
			Base struct {
				SHA, Ref string
				Repo     repo
			}
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	switch p.Action {
	case "opened", "synchronize", "reopened":
	default:
		return nil, nil
	}
	h, b := p.PR.Head, p.PR.Base
	return &Event{RepoPath: b.Repo.FullName, Repo: b.Repo.FullName, PR: strconv.Itoa(p.Number), HeadSHA: h.SHA, HeadRef: h.Ref,
		BaseRef: b.Ref, BaseSHA: b.SHA, CloneURL: h.Repo.CloneURL, BaseCloneURL: b.Repo.CloneURL,
		Fork: h.Repo.FullName != b.Repo.FullName, ConfiguredHost: g.Config.web()}, nil
}

func (g github) StatusNamed(ctx context.Context, e *Event, name string, s State, desc string) error {
	desc = clip(desc)
	if s == Neutral {
		return g.post(ctx, fmt.Sprintf("%s/repos/%s/check-runs", g.API, e.Repo), g.hdr(),
			map[string]any{
				"name":       name,
				"head_sha":   e.HeadSHA,
				"status":     "completed",
				"conclusion": "neutral",
				"output": map[string]string{
					"title":   name,
					"summary": desc,
				},
			})
	}
	st := "pending"
	switch s {
	case Success:
		st = "success"
	case Failure:
		st = "failure"
	case Error:
		st = "error"
	}
	return g.post(ctx, fmt.Sprintf("%s/repos/%s/statuses/%s", g.API, e.Repo, e.HeadSHA), g.hdr(),
		map[string]string{"state": st, "description": desc, "context": name})
}

func (g github) Status(ctx context.Context, e *Event, s State, desc string) error {
	return g.StatusNamed(ctx, e, "sentinix", s, desc)
}

func (g github) Comment(ctx context.Context, e *Event, body string) error {
	url := fmt.Sprintf("%s/repos/%s/issues/%s/comments", g.API, e.Repo, e.PR)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err == nil {
		for k, v := range g.hdr() {
			req.Header.Set(k, v)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var comments []struct {
				ID   int64  `json:"id"`
				Body string `json:"body"`
			}
			if json.NewDecoder(resp.Body).Decode(&comments) == nil {
				for _, c := range comments {
					if strings.Contains(c.Body, "<!-- sentinix-review-marker -->") || strings.HasPrefix(strings.TrimSpace(c.Body), "### Sentinix") {
						patchURL := fmt.Sprintf("%s/repos/%s/issues/comments/%d", g.API, e.Repo, c.ID)
						return g.request(ctx, "PATCH", patchURL, g.hdr(), map[string]string{"body": body})
					}
				}
			}
		}
	}
	return g.request(ctx, "POST", url, g.hdr(), map[string]string{"body": body})
}
