// Package config reads/writes .scribex/config.yaml. Secrets are never stored here: only env-var *names*.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"scribex/pkg/fsx"
	"scribex/pkg/prose"
	"scribex/pkg/safepath"
)

type AI struct {
	Provider   string // anthropic | openai (any OpenAI-compatible endpoint via base_url)
	Model      string
	APIKeyEnv  string // NAME of the env var holding the key
	BaseURL    string
	StyleGuide string // optional path to a repo style guide; sent in the cached system prompt
	MaxTokens  int
	MaxCalls   int // hard cap on LLM calls per run
}
type Wiki struct{ Dir, Home string }
type Config struct {
	AI   AI
	Wiki Wiki
}

func Default() Config {
	return Config{
		AI:   AI{"anthropic", "claude-sonnet-5-5", "ANTHROPIC_API_KEY", "", "", 1024, 20},
		Wiki: Wiki{"docs/wiki", "Home"},
	}
}

func Path(root string) string {
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		dir = filepath.Join(root, safepath.DirName)
	}
	return filepath.Join(dir, "config.yaml")
}

var (
	EnvName      = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	twelveDigits = regexp.MustCompile(`\d{12,}`)
)

// ValidateAPIKeyEnv checks whether v is a valid environment variable name for an API key.
// It rejects names not matching EnvName, matching prose.SecretShape, >64 chars, >=24 chars with no underscore,
// or containing a run of >= 12 digits.
func ValidateAPIKeyEnv(v string) error {
	if !EnvName.MatchString(v) ||
		prose.SecretShape(v) ||
		len(v) > 64 ||
		(len(v) >= 24 && !strings.Contains(v, "_")) ||
		twelveDigits.MatchString(v) {
		return errors.New("ai.api_key_env must be the NAME of an environment variable (e.g. ANTHROPIC_API_KEY), not a key")
	}
	return nil
}

// parse flattens indented `key:` sections into dotted keys.
func parse(s string) map[string]string {
	out := map[string]string{}
	type lvl struct {
		indent int
		key    string
	}
	var st []lvl
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || t[0] == '#' {
			continue
		}
		if i := strings.Index(t, " #"); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		ind := len(l) - len(strings.TrimLeft(l, " "))
		for len(st) > 0 && st[len(st)-1].indent >= ind {
			st = st[:len(st)-1]
		}
		k, v, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		path := k
		for i := len(st) - 1; i >= 0; i-- {
			path = st[i].key + "." + path
		}
		if v == "" {
			st = append(st, lvl{ind, k})
		} else {
			out[path] = v
		}
	}
	return out
}

func apply(c *Config, k, v string) error {
	num := func(dst *int) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("%s must be a non-negative integer", k)
		}
		*dst = n
		return nil
	}
	switch k {
	case "ai.provider":
		c.AI.Provider = v
	case "ai.model":
		c.AI.Model = v
	case "ai.api_key_env":
		if err := ValidateAPIKeyEnv(v); err != nil {
			return err
		}
		c.AI.APIKeyEnv = v
	case "ai.base_url":
		c.AI.BaseURL = v
	case "ai.style_guide":
		c.AI.StyleGuide = v
	case "ai.max_tokens":
		return num(&c.AI.MaxTokens)
	case "ai.max_calls":
		return num(&c.AI.MaxCalls)
	case "wiki.dir":
		c.Wiki.Dir = v
	case "wiki.home":
		c.Wiki.Home = v
	default:
		return fmt.Errorf("unknown key %q", k)
	}
	return nil
}

// Load returns defaults overlaid with the file (a missing file is fine).
func Load(root string) (Config, error) {
	c := Default()
	dir, err := safepath.ScribexDir(root, false)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	p := filepath.Join(dir, "config.yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	for k, v := range parse(string(b)) {
		if err := apply(&c, k, v); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (c Config) YAML() string {
	return fmt.Sprintf(`# Scribex config. Secrets never go here: api_key_env is the NAME of an env var
# (set it with "scribex secrets set" locally, or a CI secret).
ai:
  provider: %s
  model: %s
  api_key_env: %s
  base_url: %q
  style_guide: %q
  max_tokens: %d
  max_calls: %d
wiki:
  dir: %s
  home: %s
`, c.AI.Provider, c.AI.Model, c.AI.APIKeyEnv, c.AI.BaseURL, c.AI.StyleGuide, c.AI.MaxTokens, c.AI.MaxCalls, c.Wiki.Dir, c.Wiki.Home)
}

func Save(root string, c Config) error {
	if _, err := safepath.ScribexDir(root, true); err != nil {
		return err
	}
	return fsx.WriteFileIn(root, filepath.Join(safepath.DirName, "config.yaml"), []byte(c.YAML()), 0o644)
}

func Set(root, k, v string) error {
	c, err := Load(root)
	if err != nil {
		return err
	}
	if err := apply(&c, k, v); err != nil {
		return err
	}
	if p := c.Validate(); len(p) > 0 {
		return fmt.Errorf("%s", strings.Join(p, "; "))
	}
	return Save(root, c)
}

// Validate returns problems; empty means usable.
func (c Config) Validate() (p []string) {
	if c.AI.Provider != "anthropic" && c.AI.Provider != "openai" {
		p = append(p, "ai.provider must be anthropic or openai")
	}
	if c.AI.Model == "" {
		p = append(p, "ai.model is empty")
	}
	if err := ValidateAPIKeyEnv(c.AI.APIKeyEnv); err != nil {
		p = append(p, err.Error())
	}
	if c.AI.MaxTokens == 0 || c.AI.MaxCalls == 0 {
		p = append(p, "ai.max_tokens and ai.max_calls must be > 0")
	}
	return
}
