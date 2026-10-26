// Package jira reality-checks a PR's contract changes against its Jira ticket.
package jira

import (
	"fmt"
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

var keyRe = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

// Key finds the first issue key (PROJ-1234) in s, e.g. a branch name or PR title.
func Key(s string) string { return keyRe.FindString(s) }

// Text returns summary + description (acceptance criteria live there).
func (c Client) Text(key string) (string, error) {
	var out struct {
		Fields struct{ Summary, Description string } `json:"fields"`
	}
	err := c.client().Do("GET", fmt.Sprintf("%s/rest/api/2/issue/%s?fields=summary,description", strings.TrimRight(c.Base, "/"), key), c.hdr(), nil, &out)
	return out.Fields.Summary + "\n" + out.Fields.Description, err
}

func (c Client) Comment(key, body string) error {
	return c.client().Do("POST", fmt.Sprintf("%s/rest/api/2/issue/%s/comment", strings.TrimRight(c.Base, "/"), key), c.hdr(), map[string]string{"body": body}, nil)
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
