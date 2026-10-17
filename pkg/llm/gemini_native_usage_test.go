package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// geminiCachedThinkingUsage is a usageMetadata block as the Gemini API sends
// it for a thinking model whose prompt prefix hit the implicit cache.
const geminiCachedThinkingUsage = `"usageMetadata":{"promptTokenCount":3000,"cachedContentTokenCount":2048,"candidatesTokenCount":40,"thoughtsTokenCount":160,"totalTokenCount":3200}`

func TestGeminiNativeUsage_ReportsCachedAndThinkingTokens(t *testing.T) {
	want := Usage{InputTokens: 3000, CachedTokens: 2048, OutputTokens: 200}

	t.Run("non-streaming", func(t *testing.T) {
		client := newGeminiUsageTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],` + geminiCachedThinkingUsage + `}`))
		})
		resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{})
		if err != nil {
			t.Fatalf("chat: %v", err)
		}
		if resp.Usage != want {
			t.Fatalf("usage = %+v, want %+v", resp.Usage, want)
		}
	})

	t.Run("streaming", func(t *testing.T) {
		client := newGeminiUsageTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"o"}]}}],"usageMetadata":{"promptTokenCount":3000,"cachedContentTokenCount":2048}}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"k"}]},"finishReason":"STOP"}],` + geminiCachedThinkingUsage + `}` + "\n\n"))
		})
		resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{OnDelta: func(string) {}})
		if err != nil {
			t.Fatalf("chat: %v", err)
		}
		if resp.Usage != want {
			t.Fatalf("usage = %+v, want %+v", resp.Usage, want)
		}
	})
}

func TestGeminiNativeThinkingConfig_WireShape(t *testing.T) {
	cases := []struct {
		name   string
		config ProviderOptions
		want   string
	}{
		{name: "effort sends the API's level name", config: ProviderOptions{ReasoningEffort: "low"}, want: `{"thinkingLevel":"LOW"}`},
		{name: "minimal effort", config: ProviderOptions{ReasoningEffort: "minimal"}, want: `{"thinkingLevel":"MINIMAL"}`},
		{name: "budget alone", config: ProviderOptions{ThinkingBudget: 2048}, want: `{"thinkingBudget":2048}`},
		{name: "budget wins over effort", config: ProviderOptions{ReasoningEffort: "high", ThinkingBudget: 2048}, want: `{"thinkingBudget":2048}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			client := newGeminiUsageTestClientWithOptions(t, tc.config, func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
			})
			if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{}); err != nil {
				t.Fatalf("chat: %v", err)
			}
			var generation struct {
				ThinkingConfig json.RawMessage `json:"thinkingConfig"`
			}
			if err := json.Unmarshal(body["generationConfig"], &generation); err != nil {
				t.Fatalf("generationConfig: %v (%s)", err, body["generationConfig"])
			}
			if string(generation.ThinkingConfig) != tc.want {
				t.Fatalf("thinkingConfig = %s, want %s", generation.ThinkingConfig, tc.want)
			}
		})
	}
}

func newGeminiUsageTestClient(t *testing.T, generate http.HandlerFunc) Client {
	t.Helper()
	return newGeminiUsageTestClientWithOptions(t, ProviderOptions{}, generate)
}

func newGeminiUsageTestClientWithOptions(t *testing.T, opts ProviderOptions, generate http.HandlerFunc) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"models/gemini-test","supportedGenerationMethods":["generateContent"]}`))
			return
		}
		generate(w, r)
	}))
	t.Cleanup(srv.Close)

	opts.Provider = "gemini-native"
	opts.BaseURL = srv.URL + "/v1beta"
	opts.APIKey = "gemini-key"
	opts.Model = "gemini-test"
	client, err := NewProvider(opts)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return client
}
