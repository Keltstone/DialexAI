package githost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
				Ref  string
				Repo repo
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
		BaseRef: b.Ref, CloneURL: h.Repo.CloneURL, BaseCloneURL: b.Repo.CloneURL,
		Fork: h.Repo.FullName != b.Repo.FullName}, nil
}

func (g github) Status(ctx context.Context, e *Event, s State, desc string) error {
	st := [...]string{"pending", "success", "failure"}[s]
	return g.post(ctx, fmt.Sprintf("%s/repos/%s/statuses/%s", g.API, e.Repo, e.HeadSHA), g.hdr(),
		map[string]string{"state": st, "description": desc, "context": "sentinix"})
}

func (g github) Comment(ctx context.Context, e *Event, body string) error {
	return g.post(ctx, fmt.Sprintf("%s/repos/%s/issues/%s/comments", g.API, e.Repo, e.PR), g.hdr(),
		map[string]string{"body": body})
}
