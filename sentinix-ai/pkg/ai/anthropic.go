package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Anthropic is the native Messages API. The system prompt is cacheable, which cuts cost for hunt's repeated calls.
type Anthropic struct {
	Label, URL, Key, ModelID string
	MaxTokens                int
	IsLocal                  bool
	HTTP                     *http.Client
}

func (a Anthropic) Name() string { return a.Label }
func (a Anthropic) Local() bool  { return a.IsLocal }

func (a Anthropic) Complete(ctx context.Context, system, user string) (string, error) {
	u := a.URL
	if u == "" {
		u = "https://api.anthropic.com"
	}
	mt := a.MaxTokens
	if mt == 0 {
		mt = 1024
	}
	body, _ := json.Marshal(map[string]any{"model": a.ModelID, "max_tokens": mt, "temperature": 0,
		"system":   []map[string]any{{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}}},
		"messages": []map[string]string{{"role": "user", "content": user}}})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(u, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.Key)
	req.Header.Set("anthropic-version", "2023-06-01")
	h := a.HTTP
	if h == nil {
		h = &http.Client{Timeout: 3 * time.Minute}
	}
	resp, err := h.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s: %s", a.Label, resp.Status)
	}
	var out struct{ Content []struct{ Text string } }
	if err := json.Unmarshal(data, &out); err != nil || len(out.Content) == 0 {
		return "", fmt.Errorf("%s: unreadable response", a.Label)
	}
	return out.Content[0].Text, nil
}
