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
