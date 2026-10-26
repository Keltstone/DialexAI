// Package github posts the Scribex report as a single, updated-in-place PR comment.
package github

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"scribex/pkg/httpx"
)

const (
	MarkerV1     = "<!-- scribex-report:v1 -->"
	MarkerLegacy = "<!-- scribex-report -->"
	MaxBodyLen   = 60000
	MaxPages     = 50
	PerPage      = 100
)

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

// ValidRepo reports whether s is owner/name matching ^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$
// with neither part equal to . or ..
func ValidRepo(s string) bool {
	if !repoRe.MatchString(s) {
		return false
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return false
	}
	if parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return false
	}
	return true
}

type Action string

const (
	ActionCreated   Action = "created"
	ActionUpdated   Action = "updated"
	ActionUnchanged Action = "unchanged"
)

type Result struct {
	Action Action
	ID     int64
}

type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User User   `json:"user"`
}

type options struct {
	author string
}

type Option func(*options)

// WithAuthor overrides the author login matched against comments.
func WithAuthor(author string) Option {
	return func(o *options) {
		o.author = author
	}
}

func resolveAuthor(cl *httpx.Client, api, token, overrideAuthor string) (login string, isBot bool, err error) {
	if overrideAuthor != "" {
		return overrideAuthor, false, nil
	}
	if envAuthor := os.Getenv("SCRIBEX_COMMENT_AUTHOR"); envAuthor != "" {
		return envAuthor, false, nil
	}

	var u User
	err = cl.Do("GET", fmt.Sprintf("%s/user", strings.TrimRight(api, "/")), map[string]string{"Authorization": "Bearer " + token}, nil, &u)
	if err == nil && u.Login != "" {
		return u.Login, u.Type == "Bot", nil
	}

	// On error (or if GITHUB_TOKEN in Actions cannot call /user), fall back to Bot / github-actions[bot]
	if err != nil {
		return "github-actions[bot]", true, nil
	}
	return "github-actions[bot]", true, nil
}

func isOurComment(c Comment, myLogin string, isBot bool, overrideAuthor string) bool {
	// First line must be MarkerV1 or MarkerLegacy
	firstLine := c.Body
	if idx := strings.Index(c.Body, "\n"); idx >= 0 {
		firstLine = c.Body[:idx]
	}
	firstLine = strings.TrimRight(firstLine, "\r")
	if firstLine != MarkerV1 && firstLine != MarkerLegacy {
		return false
	}

	if overrideAuthor != "" {
		return c.User.Login == overrideAuthor
	}
	if envAuthor := os.Getenv("SCRIBEX_COMMENT_AUTHOR"); envAuthor != "" {
		return c.User.Login == envAuthor
	}
	if isBot {
		return c.User.Type == "Bot" && c.User.Login == "github-actions[bot]"
	}
	return c.User.Login == myLogin
}

func listAllComments(cl *httpx.Client, base string, pr int, h map[string]string) ([]Comment, error) {
	var all []Comment
	for page := 1; page <= MaxPages; page++ {
		var batch []Comment
		url := fmt.Sprintf("%s/issues/%d/comments?per_page=%d&page=%d", base, pr, PerPage, page)
		if err := cl.Do("GET", url, h, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < PerPage {
			return all, nil
		}
	}
	return nil, errors.New("too many comments")
}

// Upsert creates or updates the Scribex comment on a PR.
func Upsert(api, token, repo string, pr int, body string, optList ...Option) (Result, error) {
	if !ValidRepo(repo) {
		return Result{}, errors.New("invalid repository name")
	}
	if pr <= 0 || pr > 2147483647 {
		return Result{}, errors.New("invalid pull request number")
	}

	var opts options
	for _, o := range optList {
		o(&opts)
	}

	p := httpx.Default()
	if token != "" {
		p.Secrets = []string{token}
	}
	cl := httpx.New(p)

	myLogin, isBot, err := resolveAuthor(cl, api, token, opts.author)
	if err != nil {
		return Result{}, err
	}

	h := map[string]string{"Authorization": "Bearer " + token}
	base := fmt.Sprintf("%s/repos/%s", strings.TrimRight(api, "/"), repo)

	existing, err := listAllComments(cl, base, pr, h)
	if err != nil {
		return Result{}, err
	}

	targetBody := MarkerV1 + "\n" + body
	if len(targetBody) > MaxBodyLen {
		return Result{}, fmt.Errorf("comment body length %d exceeds %d characters", len(targetBody), MaxBodyLen)
	}

	var ours []Comment
	for _, c := range existing {
		if isOurComment(c, myLogin, isBot, opts.author) {
			ours = append(ours, c)
		}
	}

	sort.Slice(ours, func(i, j int) bool {
		return ours[i].ID < ours[j].ID
	})

	if len(ours) > 0 {
		target := ours[0]
		if target.Body == targetBody {
			return Result{Action: ActionUnchanged, ID: target.ID}, nil
		}
		payload := map[string]string{"body": targetBody}
		if err := cl.Do("PATCH", fmt.Sprintf("%s/issues/comments/%d", base, target.ID), h, payload, nil); err != nil {
			return Result{}, err
		}
		return Result{Action: ActionUpdated, ID: target.ID}, nil
	}

	// No existing comment; POST new
	payload := map[string]string{"body": targetBody}
	var created Comment
	if err := cl.Do("POST", fmt.Sprintf("%s/issues/%d/comments", base, pr), h, payload, &created); err != nil {
		return Result{}, err
	}

	// Race repair: re-list to detect any competing comment created concurrently
	relisted, err := listAllComments(cl, base, pr, h)
	if err == nil {
		var currentOurs []Comment
		for _, c := range relisted {
			if isOurComment(c, myLogin, isBot, opts.author) {
				currentOurs = append(currentOurs, c)
			}
		}
		sort.Slice(currentOurs, func(i, j int) bool {
			return currentOurs[i].ID < currentOurs[j].ID
		})

		if len(currentOurs) > 1 {
			lowest := currentOurs[0]
			if lowest.Body != targetBody {
				_ = cl.Do("PATCH", fmt.Sprintf("%s/issues/comments/%d", base, lowest.ID), h, payload, nil)
			}
			for _, other := range currentOurs[1:] {
				_ = cl.Do("DELETE", fmt.Sprintf("%s/issues/comments/%d", base, other.ID), h, nil, nil)
			}
			return Result{Action: ActionCreated, ID: lowest.ID}, nil
		}
	}

	return Result{Action: ActionCreated, ID: created.ID}, nil
}
