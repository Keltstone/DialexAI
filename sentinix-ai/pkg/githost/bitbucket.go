package githost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	return &Event{RepoPath: d.Repo.FullName, Repo: d.Repo.FullName, PR: strconv.Itoa(p.PR.ID), HeadSHA: s.Commit.Hash, HeadRef: s.Branch.Name,
		BaseRef: d.Branch.Name, CloneURL: s.Repo.Links.HTML.Href + ".git", BaseCloneURL: d.Repo.Links.HTML.Href + ".git",
		Fork: s.Repo.FullName != d.Repo.FullName, ConfiguredHost: b.Config.web()}, nil
}

func (b bitbucket) Status(ctx context.Context, e *Event, s State, desc string) error {
	st := "INPROGRESS"
	switch s {
	case Success:
		st = "SUCCESSFUL"
	case Neutral:
		st = "STOPPED"
	case Failure, Error:
		st = "FAILED"
	}
	return b.post(ctx, fmt.Sprintf("%s/repositories/%s/commit/%s/statuses/build", b.API, e.Repo, e.HeadSHA), b.hdr(),
		map[string]string{"key": "sentinix", "name": "sentinix", "state": st, "description": desc, "url": b.PublicURL})
}

func (b bitbucket) Comment(ctx context.Context, e *Event, body string) error {
	url := fmt.Sprintf("%s/repositories/%s/pullrequests/%s/comments", b.API, e.Repo, e.PR)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err == nil {
		for k, v := range b.hdr() {
			req.Header.Set(k, v)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var p struct {
				Values []struct {
					ID      int64 `json:"id"`
					Content struct {
						Raw string `json:"raw"`
					} `json:"content"`
				} `json:"values"`
			}
			if json.NewDecoder(resp.Body).Decode(&p) == nil {
				for _, c := range p.Values {
					if strings.Contains(c.Content.Raw, "<!-- sentinix-review-marker -->") || strings.HasPrefix(strings.TrimSpace(c.Content.Raw), "### Sentinix") {
						putURL := fmt.Sprintf("%s/repositories/%s/pullrequests/%s/comments/%d", b.API, e.Repo, e.PR, c.ID)
						return b.request(ctx, "PUT", putURL, b.hdr(), map[string]any{"content": map[string]string{"raw": body}})
					}
				}
			}
		}
	}
	return b.request(ctx, "POST", url, b.hdr(), map[string]any{"content": map[string]string{"raw": body}})
}
