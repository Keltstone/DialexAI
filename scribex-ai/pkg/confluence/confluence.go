// Package confluence pushes draft revisions and pulls page bodies via the Atlassian REST API.
package confluence

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"scribex/pkg/httpx"
)

type Client struct{ Base, User, Token string }

func (c Client) hdr() map[string]string {
	return map[string]string{"Authorization": httpx.Basic(c.User, c.Token)}
}

// CreateDraft opens a draft page for a tech lead to review; returns its id.
func (c Client) CreateDraft(space, title, storage string) (string, error) {
	in := map[string]any{
		"type": "page", "status": "draft", "title": title,
		"space": map[string]string{"key": space},
		"body":  map[string]any{"storage": map[string]string{"value": storage, "representation": "storage"}},
	}
	var out struct{ ID string }
	err := httpx.Do("POST", strings.TrimRight(c.Base, "/")+"/wiki/rest/api/content", c.hdr(), in, &out)
	return out.ID, err
}

// Pull returns a page's storage-format body.
func (c Client) Pull(id string) (string, error) {
	var out struct {
		Body struct {
			Storage struct{ Value string } `json:"storage"`
		} `json:"body"`
	}
	err := httpx.Do("GET", fmt.Sprintf("%s/wiki/rest/api/content/%s?expand=body.storage", strings.TrimRight(c.Base, "/"), id), c.hdr(), nil, &out)
	return out.Body.Storage.Value, err
}

// Body renders symbol signatures as Confluence storage format.
func Body(sigs map[string]string) string {
	keys := make([]string, 0, len(sigs))
	for k := range sigs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "<h3>%s</h3><pre>%s</pre>", html.EscapeString(k), html.EscapeString(sigs[k]))
	}
	return b.String()
}
