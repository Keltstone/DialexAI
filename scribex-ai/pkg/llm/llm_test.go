package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"scribex/pkg/config"
)

func TestLLMResponseHandling(t *testing.T) {
	os.Setenv("TEST_LLM_KEY", "test-key-12345678")

	tests := []struct {
		name       string
		provider   string
		response   any
		wantErr    bool
		errContain string
		wantText   string
	}{
		// Anthropic tests
		{
			name:     "anthropic_success",
			provider: "anthropic",
			response: map[string]any{
				"stop_reason": "end_turn",
				"content": []map[string]any{
					{"type": "text", "text": "Valid response."},
				},
			},
			wantText: "Valid response.",
		},
		{
			name:     "anthropic_stop_reason_max_tokens",
			provider: "anthropic",
			response: map[string]any{
				"stop_reason": "max_tokens",
				"content": []map[string]any{
					{"type": "text", "text": "Truncated reply here..."},
				},
			},
			wantErr:    true,
			errContain: "max_tokens",
		},
		{
			name:     "anthropic_content_empty",
			provider: "anthropic",
			response: map[string]any{
				"stop_reason": "end_turn",
				"content":     []map[string]any{},
			},
			wantErr:    true,
			errContain: "empty response content",
		},
		{
			name:     "anthropic_block_not_text",
			provider: "anthropic",
			response: map[string]any{
				"stop_reason": "end_turn",
				"content": []map[string]any{
					{"type": "tool_use", "name": "bash"},
				},
			},
			wantErr:    true,
			errContain: "expected text content block",
		},
		{
			name:     "anthropic_text_empty",
			provider: "anthropic",
			response: map[string]any{
				"stop_reason": "end_turn",
				"content": []map[string]any{
					{"type": "text", "text": ""},
				},
			},
			wantErr:    true,
			errContain: "empty text in response",
		},
		{
			name:     "anthropic_text_over_1mb",
			provider: "anthropic",
			response: map[string]any{
				"stop_reason": "end_turn",
				"content": []map[string]any{
					{"type": "text", "text": strings.Repeat("A", (1<<20)+1)},
				},
			},
			wantErr:    true,
			errContain: "exceeds 1 MB",
		},

		// OpenAI tests
		{
			name:     "openai_success",
			provider: "openai",
			response: map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message":       map[string]any{"content": "Valid OpenAI response."},
					},
				},
			},
			wantText: "Valid OpenAI response.",
		},
		{
			name:     "openai_finish_reason_length",
			provider: "openai",
			response: map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "length",
						"message":       map[string]any{"content": "Partial response..."},
					},
				},
			},
			wantErr:    true,
			errContain: "finish_reason: length",
		},
		{
			name:     "openai_choices_empty",
			provider: "openai",
			response: map[string]any{
				"choices": []map[string]any{},
			},
			wantErr:    true,
			errContain: "empty response choices",
		},
		{
			name:     "openai_text_empty",
			provider: "openai",
			response: map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message":       map[string]any{"content": ""},
					},
				},
			},
			wantErr:    true,
			errContain: "empty text in response",
		},
		{
			name:     "openai_text_over_1mb",
			provider: "openai",
			response: map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message":       map[string]any{"content": strings.Repeat("B", (1<<20)+1)},
					},
				},
			},
			wantErr:    true,
			errContain: "exceeds 1 MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			os.Setenv("SCRIBEX_ALLOW_INSECURE_LOCAL", "1")
			u, _ := url.Parse(srv.URL)
			os.Setenv("SCRIBEX_AI_ALLOWED_HOSTS", u.Host)

			cl, err := New(config.AI{
				Provider:  tt.provider,
				Model:     "test-model",
				APIKeyEnv: "TEST_LLM_KEY",
				BaseURL:   srv.URL,
				MaxTokens: 100,
			})
			if err != nil {
				t.Fatalf("New failed: %v", err)
			}

			got, err := cl.Complete("sys", "user")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContain)
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantText {
				t.Fatalf("got %q, want %q", got, tt.wantText)
			}
		})
	}
}
