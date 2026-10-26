// Package llm is a minimal Anthropic / OpenAI-compatible chat client.
package llm

import (
	"fmt"
	"os"
	"time"

	"scribex/pkg/config"
	"scribex/pkg/httpx"
	"scribex/pkg/trust"
)

type Client struct {
	Provider, Model, Key, BaseURL string
	MaxTokens                     int
	http                          *httpx.Client
}

func New(a config.AI) (*Client, error) {
	u := a.BaseURL
	if u == "" {
		u = map[string]string{"anthropic": "https://api.anthropic.com", "openai": "https://api.openai.com"}[a.Provider]
	}

	home, _ := os.UserHomeDir()
	hosts, err := trust.AIHosts(a.Provider, os.Getenv, home)
	if err != nil {
		return nil, fmt.Errorf("ai endpoint trust: %w", err)
	}

	insecureLocal := os.Getenv("SCRIBEX_ALLOW_INSECURE_LOCAL") == "1"
	cleanURL, err := trust.CheckAIEndpoint(a.Provider, u, hosts, insecureLocal)
	if err != nil {
		return nil, err
	}

	// Key is read only after endpoint validation passes (fail-closed)
	key := os.Getenv(a.APIKeyEnv)
	if key == "" {
		return nil, fmt.Errorf("env var %s is empty (scribex secrets set %s)", a.APIKeyEnv, a.APIKeyEnv)
	}

	pol := httpx.Policy{
		Secrets:       []string{key},
		Timeout:       90 * time.Second,
		MaxBody:       2 << 20, // 2 MiB
		MaxAttempts:   3,
		RetryPOST:     true,
		AllowHTTP:     insecureLocal,
		AllowLoopback: insecureLocal,
	}

	return &Client{
		Provider:  a.Provider,
		Model:     a.Model,
		Key:       key,
		BaseURL:   cleanURL,
		MaxTokens: a.MaxTokens,
		http:      httpx.New(pol),
	}, nil
}

// Complete sends a system+user prompt. The system prompt is marked cacheable (Anthropic prompt caching).
func (c *Client) Complete(system, user string) (string, error) {
	if c.Provider == "openai" {
		var out struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		err := c.http.Do("POST", c.BaseURL+"/v1/chat/completions", map[string]string{"Authorization": "Bearer " + c.Key}, map[string]any{
			"model": c.Model, "max_tokens": c.MaxTokens,
			"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		}, &out)
		if err != nil {
			return "", err
		}
		if len(out.Choices) == 0 {
			return "", fmt.Errorf("empty response choices")
		}
		if out.Choices[0].FinishReason == "length" {
			return "", fmt.Errorf("response truncated (finish_reason: length)")
		}
		text := out.Choices[0].Message.Content
		if text == "" {
			return "", fmt.Errorf("empty text in response")
		}
		if len(text) > 1<<20 {
			return "", fmt.Errorf("response exceeds 1 MB (%d bytes)", len(text))
		}
		return text, nil
	}
	var out struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	err := c.http.Do("POST", c.BaseURL+"/v1/messages", map[string]string{"x-api-key": c.Key, "anthropic-version": "2023-06-01"}, map[string]any{
		"model": c.Model, "max_tokens": c.MaxTokens,
		"system":   []map[string]any{{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}}},
		"messages": []map[string]string{{"role": "user", "content": user}},
	}, &out)
	if err != nil {
		return "", err
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("empty response content")
	}
	if out.StopReason == "max_tokens" {
		return "", fmt.Errorf("response truncated (stop_reason: max_tokens)")
	}
	if out.Content[0].Type != "text" {
		return "", fmt.Errorf("expected text content block, got %q", out.Content[0].Type)
	}
	text := out.Content[0].Text
	if text == "" {
		return "", fmt.Errorf("empty text in response")
	}
	if len(text) > 1<<20 {
		return "", fmt.Errorf("response exceeds 1 MB (%d bytes)", len(text))
	}
	return text, nil
}
