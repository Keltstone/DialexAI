// Package jira reality-checks a PR's contract changes against its Jira ticket.
package jira

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"scribex/pkg/docs"
	"scribex/pkg/httpx"
)

type Client struct{ Base, User, Token string }

func (c Client) client() *httpx.Client {
	p := httpx.Default()
	if c.Token != "" {
		p.Secrets = []string{c.Token}
	}
	return httpx.New(p)
}

func (c Client) hdr() map[string]string {
	return map[string]string{"Authorization": httpx.Basic(c.User, c.Token)}
}

var validKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,30}-[1-9][0-9]{0,9}$`)

// ValidKey reports whether k matches ^[A-Z][A-Z0-9_]{1,30}-[1-9][0-9]{0,9}$.
func ValidKey(k string) bool {
	return validKeyRe.MatchString(k)
}

var keyCandidateRe = regexp.MustCompile(`[A-Z][A-Z0-9_]{1,30}-[0-9]+`)

// Key finds the first issue key (PROJ-1234) in s that satisfies ValidKey.
func Key(s string) string {
	for _, cand := range keyCandidateRe.FindAllString(s, -1) {
		if ValidKey(cand) {
			return cand
		}
	}
	return ""
}

// Text returns summary + description (acceptance criteria live there).
func (c Client) Text(key string) (string, error) {
	if !ValidKey(key) {
		return "", errors.New("invalid Jira key")
	}
	escapedKey := url.PathEscape(key)
	var out struct {
		Fields struct{ Summary, Description string } `json:"fields"`
	}
	err := c.client().Do("GET", fmt.Sprintf("%s/rest/api/2/issue/%s?fields=summary,description", strings.TrimRight(c.Base, "/"), escapedKey), c.hdr(), nil, &out)
	return out.Fields.Summary + "\n" + out.Fields.Description, err
}

func (c Client) Comment(key, body string) error {
	if !ValidKey(key) {
		return errors.New("invalid Jira key")
	}
	escapedKey := url.PathEscape(key)
	return c.client().Do("POST", fmt.Sprintf("%s/rest/api/2/issue/%s/comment", strings.TrimRight(c.Base, "/"), escapedKey), c.hdr(), map[string]string{"body": body}, nil)
}

// PostRealityCheck posts body to the issue if it has not already been posted.
// It searches up to 5 pages of 100 comments for the marker.
// Returns (posted bool, err error).
func (c Client) PostRealityCheck(key, body string) (bool, error) {
	if !ValidKey(key) {
		return false, errors.New("invalid Jira key")
	}
	h := sha256.Sum256([]byte(body))
	marker := fmt.Sprintf("<!-- scribex-reality-check:%s -->", hex.EncodeToString(h[:4]))

	escapedKey := url.PathEscape(key)
	startAt := 0
	for page := 0; page < 5; page++ {
		var out struct {
			StartAt    int `json:"startAt"`
			MaxResults int `json:"maxResults"`
			Total      int `json:"total"`
			Comments   []struct {
				Body string `json:"body"`
			} `json:"comments"`
		}
		endpoint := fmt.Sprintf("%s/rest/api/2/issue/%s/comment?startAt=%d&maxResults=100", strings.TrimRight(c.Base, "/"), escapedKey, startAt)
		err := c.client().Do("GET", endpoint, c.hdr(), nil, &out)
		if err != nil {
			return false, err
		}
		for _, comm := range out.Comments {
			if strings.Contains(comm.Body, marker) {
				return false, nil // already posted, idempotent skip
			}
		}
		if len(out.Comments) == 0 || startAt+len(out.Comments) >= out.Total {
			break
		}
		startAt += len(out.Comments)
	}

	postedBody := body + "\n\n" + marker
	if err := c.Comment(key, postedBody); err != nil {
		return false, err
	}
	return true, nil
}

// Check flags contract changes the ticket never mentions.
// ponytail: substring match on the symbol name; swap for LLM judgement if too noisy.
func Check(ticket string, ds []docs.Delta) (flags []string) {
	t := strings.ToLower(ticket)
	for _, d := range ds {
		name := d.Sym[strings.LastIndex(d.Sym, ".")+1:]
		if strings.Contains(t, strings.ToLower(name)) {
			continue
		}
		if d.Breaking() {
			flags = append(flags, fmt.Sprintf("undeclared breaking change: %s#%s (%s)", d.File, d.Sym, d.Kind))
		} else {
			flags = append(flags, fmt.Sprintf("possible scope creep: new public %s#%s", d.File, d.Sym))
		}
	}
	return
}
