// Package mcp manages .scribex/mcp.json (standard `mcpServers` shape) and serves Scribex itself over MCP.
// Credentials are only ever ${ENV_VAR} references.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Server struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}
type File struct {
	Servers map[string]Server `json:"mcpServers"`
}

func Path(root string) string { return filepath.Join(root, ".scribex", "mcp.json") }

func Load(root string) (File, error) {
	f := File{Servers: map[string]Server{}}
	b, err := os.ReadFile(Path(root))
	if os.IsNotExist(err) {
		return f, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &f)
	}
	if f.Servers == nil {
		f.Servers = map[string]Server{}
	}
	return f, err
}

func Save(root string, f File) error {
	b, _ := json.MarshalIndent(f, "", "  ")
	if err := os.MkdirAll(filepath.Dir(Path(root)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(Path(root), append(b, '\n'), 0o644)
}

// Presets for common documentation systems. Package names are upstream's; verify before relying on them.
var Presets = map[string]Server{
	"scribex":          {Command: "scribex", Args: []string{"mcp", "serve", "-C", "."}},
	"confluence-oauth": {Command: "npx", Args: []string{"-y", "mcp-remote", "https://mcp.atlassian.com/v1/sse"}}, // browser OAuth, no secret
	"atlassian-token": {Command: "uvx", Args: []string{"mcp-atlassian"}, Env: map[string]string{
		"CONFLUENCE_URL": "${ATLASSIAN_URL}/wiki", "CONFLUENCE_USERNAME": "${ATLASSIAN_USER}", "CONFLUENCE_API_TOKEN": "${ATLASSIAN_TOKEN}",
		"JIRA_URL": "${ATLASSIAN_URL}", "JIRA_USERNAME": "${ATLASSIAN_USER}", "JIRA_API_TOKEN": "${ATLASSIAN_TOKEN}"}},
	"github": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}, Env: map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}"}},
	"notion": {Command: "npx", Args: []string{"-y", "@notionhq/notion-mcp-server"}, Env: map[string]string{"NOTION_TOKEN": "${NOTION_TOKEN}"}},
}

func PresetNames() (ns []string) {
	for k := range Presets {
		ns = append(ns, k)
	}
	sort.Strings(ns)
	return
}

var ref = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)
var secretKey = regexp.MustCompile(`(?i)token|key|secret|password|authorization`)

// Literals returns env/header entries that look like secrets but are not ${VAR} references.
func (s Server) Literals() (bad []string) {
	for _, m := range []map[string]string{s.Env, s.Headers} {
		for k, v := range m {
			if secretKey.MatchString(k) && !ref.MatchString(v) {
				bad = append(bad, k)
			}
		}
	}
	sort.Strings(bad)
	return
}

// Missing returns referenced env vars that are unset.
func (s Server) Missing() (miss []string) {
	seen := map[string]bool{}
	scan := func(v string) {
		for _, m := range ref.FindAllStringSubmatch(v, -1) {
			if os.Getenv(m[1]) == "" && !seen[m[1]] {
				seen[m[1]] = true
				miss = append(miss, m[1])
			}
		}
	}
	scan(s.URL)
	for _, v := range s.Env {
		scan(v)
	}
	for _, v := range s.Headers {
		scan(v)
	}
	sort.Strings(miss)
	return
}

func (s Server) Validate() error {
	if (s.Command == "") == (s.URL == "") {
		return fmt.Errorf("set exactly one of command or url")
	}
	if bad := s.Literals(); len(bad) > 0 {
		return fmt.Errorf("literal secret in %s: use a ${ENV_VAR} reference", strings.Join(bad, ", "))
	}
	return nil
}
