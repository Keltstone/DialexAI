// Package confluence pushes draft revisions and pulls page bodies via the Atlassian REST API.
package confluence

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"scribex/pkg/httpx"
	"scribex/pkg/safepath"
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

var spaceRe = regexp.MustCompile(`^[A-Za-z0-9~][A-Za-z0-9_~-]{0,254}$`)

// CreateDraft opens a draft page for a tech lead to review; returns its id.
func (c Client) CreateDraft(space, title, storage string) (string, error) {
	if !spaceRe.MatchString(space) {
		return "", errors.New("invalid space")
	}
	runes := []rune(title)
	if len(runes) == 0 || len(runes) > 255 {
		return "", errors.New("invalid title length")
	}
	for _, r := range runes {
		if unicode.IsControl(r) {
			return "", errors.New("title contains control characters")
		}
	}

	in := map[string]any{
		"type": "page", "status": "draft", "title": title,
		"space": map[string]string{"key": space},
		"body":  map[string]any{"storage": map[string]string{"value": storage, "representation": "storage"}},
	}
	var out struct{ ID string }
	err := c.client().Do("POST", strings.TrimRight(c.Base, "/")+"/wiki/rest/api/content", c.hdr(), in, &out)
	return out.ID, err
}

// Pull returns a page's storage-format body.
func (c Client) Pull(id string) (string, error) {
	if !safepath.ValidPageID(id) {
		return "", errors.New("invalid page id")
	}
	escapedID := url.PathEscape(id)
	var out struct {
		Body struct {
			Storage struct{ Value string } `json:"storage"`
		} `json:"body"`
	}
	err := c.client().Do("GET", fmt.Sprintf("%s/wiki/rest/api/content/%s?expand=body.storage", strings.TrimRight(c.Base, "/"), escapedID), c.hdr(), nil, &out)
	return out.Body.Storage.Value, err
}

// Body renders symbol signatures as Confluence storage format. Capped at 1 MiB.
func Body(sigs map[string]string) (string, error) {
	keys := make([]string, 0, len(sigs))
	for k := range sigs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "<h3>%s</h3><pre>%s</pre>", html.EscapeString(k), html.EscapeString(sigs[k]))
	}
	if b.Len() > 1024*1024 {
		return "", errors.New("body exceeds 1 MiB limit")
	}
	return b.String(), nil
}
