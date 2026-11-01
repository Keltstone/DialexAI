package llm

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"scribex/pkg/config"
	"scribex/pkg/httpx"
	"scribex/pkg/trust"
)

const ollamaDefaultURL = "http://127.0.0.1:11434"

func loopbackHost(h string) bool {
	return strings.EqualFold(h, "localhost") || h == "127.0.0.1" || h == "::1"
}

// newOllama talks to Ollama's OpenAI-compatible endpoint. Plain http is allowed only for loopback
// (so it cannot be pointed at an internal service over cleartext); any other host needs https and
// the same trust list as every other AI endpoint.
func newOllama(a config.AI) (Completer, error) {
	if a.Model == "" || !config.ValidModel(a.Model) {
		return nil, errors.New("ai.model is empty or invalid (see: ollama list)")
	}
	u := a.BaseURL
	if u == "" {
		u = ollamaDefaultURL
	}
	pu, err := url.Parse(u)
	if err != nil || pu.Host == "" || pu.User != nil || pu.RawQuery != "" || pu.Fragment != "" {
		return nil, fmt.Errorf("invalid ai.base_url %q", u)
	}
	pol := httpx.Policy{Timeout: 300 * time.Second, MaxBody: 2 << 20, MaxAttempts: 2, RetryPOST: false}
	var clean string
	if loopbackHost(pu.Hostname()) && (pu.Scheme == "http" || pu.Scheme == "https") {
		clean = strings.TrimSuffix(pu.Scheme+"://"+pu.Host+strings.TrimSuffix(pu.Path, "/"), "/")
		pol.AllowHTTP, pol.AllowLoopback = true, true
	} else {
		home, _ := os.UserHomeDir()
		hosts, err := trust.AIHosts("ollama", os.Getenv, home)
		if err != nil {
			return nil, fmt.Errorf("ai endpoint trust: %w", err)
		}
		clean, err = trust.CheckAIEndpoint("ollama", u, hosts, false)
		if err != nil {
			return nil, err
		}
	}
	return &Client{Provider: "openai", Model: a.Model, BaseURL: clean, MaxTokens: a.MaxTokens, http: httpx.New(pol)}, nil
}
