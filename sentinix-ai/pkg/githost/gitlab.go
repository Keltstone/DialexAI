package githost

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type gitlab struct{ Config }

func (g gitlab) Name() string                { return g.Config.Name }
func (g gitlab) CloneAuth() (string, string) { return "oauth2", g.Token }
func (g gitlab) hdr() map[string]string      { return map[string]string{"PRIVATE-TOKEN": g.Token} }

func (g gitlab) Parse(r *http.Request, body []byte) (*Event, error) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Gitlab-Token")), []byte(g.Secret)) != 1 {
		return nil, fmt.Errorf("bad token")
	}
	var p struct {
		Project struct {
			Path string `json:"path_with_namespace"`
		}
		Kind string `json:"object_kind"`
		A    struct {
			Action       string
			IID          int
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			SourceProj   int    `json:"source_project_id"`
			TargetProj   int    `json:"target_project_id"`
			OldRev       string
			LastCommit   struct{ ID string } `json:"last_commit"`
			Source       struct {
				GitHTTPURL string `json:"git_http_url"`
			} `json:"source"`
			Target struct {
				GitHTTPURL string `json:"git_http_url"`
			} `json:"target"`
		} `json:"object_attributes"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	a := p.A
	// "update" also fires on title edits; only a push carries oldrev.
	if p.Kind != "merge_request" || !(a.Action == "open" || a.Action == "reopen" || a.Action == "update" && a.OldRev != "") {
		return nil, nil
	}
	return &Event{RepoPath: p.Project.Path, Repo: strconv.Itoa(a.TargetProj), PR: strconv.Itoa(a.IID), HeadSHA: a.LastCommit.ID,
		HeadRef: a.SourceBranch, BaseRef: a.TargetBranch, CloneURL: a.Source.GitHTTPURL,
		BaseCloneURL: a.Target.GitHTTPURL, Fork: a.SourceProj != a.TargetProj,
		ConfiguredHost: g.Config.web()}, nil
}

// ponytail: fork MRs have their commit in the source project, so the status call may 404; the comment still posts.
func (g gitlab) StatusNamed(ctx context.Context, e *Event, name string, s State, desc string) error {
	desc = clip(desc)
	st := "pending"
	switch s {
	case Success:
		st = "success"
	case Neutral:
		st = "success"
		desc = "scanner unavailable"
	case Failure, Error:
		st = "failed"
	}
	return g.post(ctx, fmt.Sprintf("%s/projects/%s/statuses/%s", g.API, e.Repo, e.HeadSHA), g.hdr(),
		map[string]string{"state": st, "name": name, "description": desc})
}

func (g gitlab) Status(ctx context.Context, e *Event, s State, desc string) error {
	return g.StatusNamed(ctx, e, "sentinix", s, desc)
}

func (g gitlab) Comment(ctx context.Context, e *Event, body string) error {
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes", g.API, e.Repo, e.PR)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err == nil {
		for k, v := range g.hdr() {
			req.Header.Set(k, v)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var notes []struct {
				ID   int64  `json:"id"`
				Body string `json:"body"`
			}
			if json.NewDecoder(resp.Body).Decode(&notes) == nil {
				for _, n := range notes {
					if strings.Contains(n.Body, "<!-- sentinix-review-marker -->") || strings.HasPrefix(strings.TrimSpace(n.Body), "### Sentinix") {
						putURL := fmt.Sprintf("%s/projects/%s/merge_requests/%s/notes/%d", g.API, e.Repo, e.PR, n.ID)
						return g.request(ctx, "PUT", putURL, g.hdr(), map[string]string{"body": body})
					}
				}
			}
		}
	}
	return g.request(ctx, "POST", url, g.hdr(), map[string]string{"body": body})
}
