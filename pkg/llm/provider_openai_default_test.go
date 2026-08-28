package llm

import "testing"

// Regression: the "openai" provider must default its base URL and model like
// the other providers, so callers that pass only a provider id + API key (no
// custom endpoint) get a working client instead of "openai base url is required".
func TestNewProvider_OpenAIDefaultsBaseURLAndModel(t *testing.T) {
	client, err := NewProvider(ProviderOptions{Provider: "openai", APIKey: "sk-test"})
	if err != nil {
		t.Fatalf("NewProvider(openai): %v", err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
}

// Same regression for anthropic: it errors at request time, not construction,
// so assert the base URL is defaulted. Empty base URL produced a relative
// "/v1/messages" → "unsupported protocol scheme".
func TestNewProvider_AnthropicDefaultsBaseURLAndModel(t *testing.T) {
	client, err := NewProvider(ProviderOptions{Provider: "anthropic", APIKey: "sk-ant-test"})
	if err != nil {
		t.Fatalf("NewProvider(anthropic): %v", err)
	}
	ac, ok := client.(*AnthropicClient)
	if !ok {
		t.Fatalf("expected *AnthropicClient, got %T", client)
	}
	if ac.baseURL != "https://api.anthropic.com" {
		t.Fatalf("base url = %q, want https://api.anthropic.com", ac.baseURL)
	}
	if ac.model == "" {
		t.Fatal("model should be defaulted")
	}
}
