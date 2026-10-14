package githost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type bitbucket struct{ Config }

func (b bitbucket) Name() string                { return b.Config.Name }
func (b bitbucket) CloneAuth() (string, string) { return "x-token-auth", b.Token }
func (b bitbucket) hdr() map[string]string {
	return map[string]string{"Authorization": "Bearer " + b.Token}
}

func (b bitbucket) Parse(r *http.Request, body []byte) (*Event, error) {
	if !hmacOK(b.Secret, r.Header.Get("X-Hub-Signature"), body) {
		return nil, fmt.Errorf("bad signature")
	}
	if k := r.Header.Get("X-Event-Key"); k != "pullrequest:created" && k != "pullrequest:updated" {
		return nil, nil
	}
	type repo struct {
		FullName string `json:"full_name"`
		Links    struct{ HTML struct{ Href string } }
	}
	var p struct {
		PR struct {
			ID  int
			Src struct {
				Commit struct{ Hash string }
				Branch struct{ Name string }
				Repo   repo `json:"repository"`
			} `json:"source"`
			Dst struct {
				Branch struct{ Name string }
				Repo   repo `json:"repository"`
			} `json:"destination"`
		} `json:"pullrequest"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	s, d := p.PR.Src, p.PR.Dst
	return &Event{Repo: d.Repo.FullName, PR: strconv.Itoa(p.PR.ID), HeadSHA: s.Commit.Hash, HeadRef: s.Branch.Name,
		BaseRef: d.Branch.Name, CloneURL: s.Repo.Links.HTML.Href + ".git", BaseCloneURL: d.Repo.Links.HTML.Href + ".git",
		Fork: s.Repo.FullName != d.Repo.FullName}, nil
}

func (b bitbucket) Status(ctx context.Context, e *Event, s State, desc string) error {
	st := [...]string{"INPROGRESS", "SUCCESSFUL", "FAILED"}[s]
	return b.post(ctx, fmt.Sprintf("%s/repositories/%s/commit/%s/statuses/build", b.API, e.Repo, e.HeadSHA), b.hdr(),
		map[string]string{"key": "sentinix", "name": "sentinix", "state": st, "description": desc, "url": b.PublicURL})
}

func (b bitbucket) Comment(ctx context.Context, e *Event, body string) error {
	return b.post(ctx, fmt.Sprintf("%s/repositories/%s/pullrequests/%s/comments", b.API, e.Repo, e.PR), b.hdr(),
		map[string]any{"content": map[string]string{"raw": body}})
}
