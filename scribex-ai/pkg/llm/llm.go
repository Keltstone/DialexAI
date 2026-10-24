// Package llm is a minimal Anthropic / OpenAI-compatible chat client.
package llm

import (
	"fmt"
	"os"
	"strings"

	"scribex/pkg/config"
	"scribex/pkg/httpx"
)

type Client struct {
	Provider, Model, Key, BaseURL string
	MaxTokens                     int
}

func New(a config.AI) (*Client, error) {
	key := os.Getenv(a.APIKeyEnv)
	if key == "" {
		return nil, fmt.Errorf("env var %s is empty (scribex secrets set %s)", a.APIKeyEnv, a.APIKeyEnv)
	}
	u := a.BaseURL
	if u == "" {
		u = map[string]string{"anthropic": "https://api.anthropic.com", "openai": "https://api.openai.com"}[a.Provider]
	}
	return &Client{a.Provider, a.Model, key, strings.TrimRight(u, "/"), a.MaxTokens}, nil
}

// Complete sends a system+user prompt. The system prompt is marked cacheable (Anthropic prompt caching).
func (c *Client) Complete(system, user string) (string, error) {
	if c.Provider == "openai" {
		var out struct {
			Choices []struct{ Message struct{ Content string } }
		}
		err := httpx.Do("POST", c.BaseURL+"/v1/chat/completions", map[string]string{"Authorization": "Bearer " + c.Key}, map[string]any{
			"model": c.Model, "max_tokens": c.MaxTokens,
			"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		}, &out)
		if err == nil && len(out.Choices) == 0 {
			err = fmt.Errorf("empty response")
		}
		if err != nil {
			return "", err
		}
		return out.Choices[0].Message.Content, nil
	}
	var out struct{ Content []struct{ Text string } }
	err := httpx.Do("POST", c.BaseURL+"/v1/messages", map[string]string{"x-api-key": c.Key, "anthropic-version": "2023-06-01"}, map[string]any{
		"model": c.Model, "max_tokens": c.MaxTokens,
		"system":   []map[string]any{{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}}},
		"messages": []map[string]string{{"role": "user", "content": user}},
	}, &out)
	if err == nil && len(out.Content) == 0 {
		err = fmt.Errorf("empty response")
	}
	if err != nil {
		return "", err
	}
	return out.Content[0].Text, nil
}
