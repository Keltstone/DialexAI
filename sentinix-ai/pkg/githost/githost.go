// Package githost hides GitHub, GitLab and Bitbucket (cloud or self-hosted via API URL)
// behind one interface: verify+parse a PR/MR webhook, set a status, post a comment.
package githost

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type State int

const (
	Pending State = iota
	Success
	Failure
	Neutral
	Error
)

var stateNames = []string{"pending", "success", "failure", "neutral", "error"}

func (s State) String() string {
	if int(s) >= 0 && int(s) < len(stateNames) {
		return stateNames[s]
	}
	return "unknown"
}

// Event is the provider-neutral view of a PR/MR that needs scanning.
type Event struct {
	RepoPath     string // human path "owner/name", matches Project.Repo
	Repo         string // API identifier of the target repo (full name, or project id for GitLab)
	PR           string
	HeadSHA      string // may be short (Bitbucket); HeadRef is the fallback for fetching
	HeadRef      string
	BaseRef      string
	BaseSHA      string // payload base SHA where the host provides it
	CloneURL     string // head repo
	BaseCloneURL   string
	Fork           bool
	ConfiguredHost string
}

type Host interface {
	Name() string
	// Parse verifies the signature/token, then returns the event, or nil for events to ignore.
	Parse(r *http.Request, body []byte) (*Event, error)
	Status(ctx context.Context, e *Event, s State, desc string) error
	StatusNamed(ctx context.Context, e *Event, name string, s State, desc string) error
	Comment(ctx context.Context, e *Event, body string) error
	// CloneAuth is the user/token pair for HTTPS clones.
	CloneAuth() (user, token string)
	// RepoURL is the HTTPS clone URL for a repo path.
	RepoURL(path string) string
	ConfiguredHost() string
}

type Config struct {
	Name      string `json:"name"` // webhook path: /webhook/<name>
	Kind      string `json:"kind"` // github | gitlab | bitbucket
	API       string `json:"api"`  // override for GitHub Enterprise / self-managed GitLab
	Secret    string `json:"secret"`
	Token     string `json:"token"`
	PublicURL string `json:"public_url"` // linked from Bitbucket build statuses
	WebURL    string `json:"web_url"`    // git host web root; derived from api when empty
}

// web returns the host's web root used to build clone URLs.
func (c Config) web() string {
	if c.WebURL != "" {
		return strings.TrimRight(c.WebURL, "/")
	}
	for _, suf := range []string{"/api/v3", "/api/v4", "/2.0"} {
		if strings.HasSuffix(c.API, suf) {
			w := strings.TrimSuffix(c.API, suf)
			return strings.Replace(w, "://api.", "://", 1)
		}
	}
	return strings.Replace(c.API, "://api.", "://", 1)
}

func (c Config) RepoURL(path string) string { return c.web() + "/" + path + ".git" }
func (c Config) ConfiguredHost() string      { return c.web() }

func New(c Config) (Host, error) {
	if c.Secret == "" || c.Token == "" {
		return nil, fmt.Errorf("host %q: secret and token are required", c.Name) // fail closed
	}
	c.API = strings.TrimRight(c.API, "/")
	switch c.Kind {
	case "github":
		if c.API == "" {
			c.API = "https://api.github.com"
		}
		return github{c}, nil
	case "gitlab":
		if c.API == "" {
			c.API = "https://gitlab.com/api/v4"
		}
		return gitlab{c}, nil
	case "bitbucket":
		// Bitbucket Cloud only; Server/Data Center has a different API.
		if c.API == "" {
			c.API = "https://api.bitbucket.org/2.0"
		}
		return bitbucket{c}, nil
	}
	return nil, fmt.Errorf("host %q: unknown kind %q", c.Name, c.Kind)
}

func hmacOK(secret, header string, body []byte) bool {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	want := "sha256=" + hex.EncodeToString(m.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(header)) == 1
}

func (c Config) post(ctx context.Context, url string, hdr map[string]string, payload any) error {
	return c.request(ctx, "POST", url, hdr, payload)
}

func (c Config) request(ctx context.Context, method, url string, hdr map[string]string, payload any) error {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s: %s %s", url, resp.Status, msg)
	}
	return nil
}

// clip keeps status descriptions under the strictest host limit (GitHub: 140 chars); an over-long
// description is rejected with 422 and the PR would be left pending forever.
func clip(desc string) string {
	r := []rune(strings.Join(strings.Fields(desc), " "))
	if len(r) > 140 {
		return string(r[:137]) + "..."
	}
	return string(r)
}
