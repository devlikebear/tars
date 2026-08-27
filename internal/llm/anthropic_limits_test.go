package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/llmdefaults"
)

// captureAnthropicRequest runs one Chat call against a stub server and
// returns the decoded request body plus the headers it carried.
func captureAnthropicRequest(t *testing.T, config ClientConfig, model string) (map[string]any, http.Header) {
	t.Helper()
	var body map[string]any
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	client, err := newAnthropicClientWithConfig(srv.URL, "k", model, config)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	return body, headers
}

func TestResolveAnthropicMaxTokens(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		configured int
		want       int
	}{
		{"explicit setting wins over documented default", "claude-opus-5", 8192, 8192},
		{"explicit setting wins for unknown model", "MiniMax-M2.7", 32000, 32000},
		// An explicit setting is uncapped — reaching the model's full
		// ceiling is exactly what the tier field is for.
		{"explicit setting is not capped", "claude-opus-5", 128000, 128000},
		{"unset is capped below the documented ceiling", "claude-opus-5", 0, anthropicDefaultMaxTokensCeiling},
		{"unset is capped for haiku too", "claude-haiku-4-5", 0, anthropicDefaultMaxTokensCeiling},
		{"unknown model falls back", "MiniMax-M2.7", 0, anthropicFallbackMaxTokens},
		{"negative is treated as unset", "claude-opus-5", -1, anthropicDefaultMaxTokensCeiling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveAnthropicMaxTokens(tc.model, tc.configured); got != tc.want {
				t.Fatalf("resolveAnthropicMaxTokens(%q, %d) = %d, want %d", tc.model, tc.configured, got, tc.want)
			}
		})
	}
}

func TestNewProvider_AnthropicTierMaxTokensReachesRequest(t *testing.T) {
	// The regression this guards: MaxTokens was structurally always 0, so
	// every tier shipped a 4096 ceiling no matter what config said.
	body, _ := captureAnthropicRequest(t, ClientConfig{MaxTokens: resolveAnthropicMaxTokens("claude-opus-5", 32000)}, "claude-opus-5")
	if got := body["max_tokens"]; got != float64(32000) {
		t.Fatalf("max_tokens = %v, want 32000", got)
	}
}

func TestNewProvider_AnthropicDefaultsFromModel(t *testing.T) {
	client, err := NewProvider(ProviderOptions{
		Provider: "anthropic",
		BaseURL:  "https://example.invalid",
		APIKey:   "k",
		Model:    "claude-opus-5",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	anthropic, ok := client.(*AnthropicClient)
	if !ok {
		t.Fatalf("expected *AnthropicClient, got %T", client)
	}
	if anthropic.config.MaxTokens != anthropicDefaultMaxTokensCeiling {
		t.Fatalf("MaxTokens = %d, want %d (capped default)", anthropic.config.MaxTokens, anthropicDefaultMaxTokensCeiling)
	}
}

func TestResolveAnthropicMaxTokens_DefaultStaysUnderTheNonStreamingBudget(t *testing.T) {
	// Ask/askFromSinglePrompt never streams and shares the 30s HTTPTimeout,
	// so an unset tier must not inherit a ceiling that turns a clean
	// truncation into a timeout. Explicit settings are the escape hatch.
	if anthropicDefaultMaxTokensCeiling >= 128000 {
		t.Fatalf("default ceiling %d does not cap the documented 128000", anthropicDefaultMaxTokensCeiling)
	}
	for _, model := range []string{"claude-opus-5", "claude-fable-5", "claude-sonnet-5", "claude-haiku-4-5"} {
		if got := resolveAnthropicMaxTokens(model, 0); got > anthropicDefaultMaxTokensCeiling {
			t.Errorf("default for %q = %d, want <= %d", model, got, anthropicDefaultMaxTokensCeiling)
		}
	}
}

func TestNewProvider_AnthropicUnknownModelUsesFallback(t *testing.T) {
	client, err := NewProvider(ProviderOptions{
		Provider: "anthropic",
		BaseURL:  "https://example.invalid",
		APIKey:   "k",
		Model:    "MiniMax-M2.7",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	anthropic, ok := client.(*AnthropicClient)
	if !ok {
		t.Fatalf("expected *AnthropicClient, got %T", client)
	}
	if anthropic.config.MaxTokens != anthropicFallbackMaxTokens {
		t.Fatalf("MaxTokens = %d, want %d (fallback)", anthropic.config.MaxTokens, anthropicFallbackMaxTokens)
	}
}

func TestNewAnthropicClient_SharesTheProviderDefaulting(t *testing.T) {
	// Both construction paths must agree; external consumers of pkg/llm
	// reach this one.
	client, err := NewAnthropicClient("https://example.invalid", "k", llmdefaults.AnthropicModel, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.config.MaxTokens != anthropicDefaultMaxTokensCeiling {
		t.Fatalf("MaxTokens = %d, want %d", client.config.MaxTokens, anthropicDefaultMaxTokensCeiling)
	}
}

func TestAnthropicBetaHeader(t *testing.T) {
	cases := []struct {
		name     string
		features []string
		want     string
	}{
		{"unset sends nothing", nil, ""},
		{"empty slice sends nothing", []string{}, ""},
		{"blank entries are dropped", []string{"  ", ""}, ""},
		{"single flag", []string{"context-1m-2025-08-07"}, "context-1m-2025-08-07"},
		{"multiple flags join with commas", []string{"context-1m-2025-08-07", "interleaved-thinking-2025-05-14"}, "context-1m-2025-08-07,interleaved-thinking-2025-05-14"},
		{"surrounding space is trimmed", []string{" context-1m-2025-08-07 "}, "context-1m-2025-08-07"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anthropicBetaHeader(tc.features); got != tc.want {
				t.Fatalf("anthropicBetaHeader(%v) = %q, want %q", tc.features, got, tc.want)
			}
		})
	}
}

func TestAnthropicChat_OmitsBetaHeaderByDefault(t *testing.T) {
	// Prompt caching went GA, so the flag this client used to send
	// unconditionally did nothing except occupy the only header slot —
	// and third-party gateways received it uninvited.
	_, headers := captureAnthropicRequest(t, ClientConfig{MaxTokens: 4096}, "claude-opus-5")
	if _, present := headers["Anthropic-Beta"]; present {
		t.Fatalf("anthropic-beta header present with no opt-in: %q", headers.Get("Anthropic-Beta"))
	}
}

func TestAnthropicChat_SendsOptedInBetaFeatures(t *testing.T) {
	_, headers := captureAnthropicRequest(t, ClientConfig{
		MaxTokens:    4096,
		BetaFeatures: []string{"context-1m-2025-08-07", "interleaved-thinking-2025-05-14"},
	}, "claude-opus-5")
	want := "context-1m-2025-08-07,interleaved-thinking-2025-05-14"
	if got := headers.Get("Anthropic-Beta"); got != want {
		t.Fatalf("anthropic-beta = %q, want %q", got, want)
	}
}

func TestAnthropicChat_ExplicitlyEmptyBetaListOmitsHeader(t *testing.T) {
	// A gateway tier that clears the list must send no header at all —
	// not an empty one the gateway still has to interpret.
	_, headers := captureAnthropicRequest(t, ClientConfig{
		MaxTokens:    4096,
		BetaFeatures: []string{},
	}, "MiniMax-M2.7")
	if _, present := headers["Anthropic-Beta"]; present {
		t.Fatalf("anthropic-beta header present for an explicitly empty list")
	}
}

func TestProviderClientConfig_ClonesBetaFeatures(t *testing.T) {
	features := []string{"context-1m-2025-08-07"}
	config := providerClientConfig(ProviderOptions{BetaFeatures: features})
	features[0] = "mutated"
	if config.BetaFeatures[0] != "context-1m-2025-08-07" {
		t.Fatalf("client config aliased the caller's slice: %q", config.BetaFeatures[0])
	}
}
