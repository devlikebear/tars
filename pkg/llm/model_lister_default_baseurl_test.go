package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/auth"
)

// recordingRoundTripper captures the request URL and returns a canned body,
// so we can assert which endpoint FetchModels resolved without real network.
type recordingRoundTripper struct {
	gotURL string
	body   string
}

func (rt *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.gotURL = req.URL.String()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Header:     make(http.Header),
	}, nil
}

func newDefaultURLFetcher(rt *recordingRoundTripper) *modelFetcher {
	return newModelFetcherWithDeps(modelFetcherDeps{
		httpClient: &http.Client{Transport: rt},
		resolveCredential: func(auth.ProviderAuthConfig) (auth.ProviderCredential, error) {
			return auth.ProviderCredential{AccessToken: "k"}, nil
		},
	})
}

// Regression: FetchModels must default the base URL per provider (like
// NewProvider) so an empty BaseURL does not fail with "invalid llm base url".
func TestModelFetcher_GeminiNativeDefaultsBaseURL(t *testing.T) {
	rt := &recordingRoundTripper{body: `{"models":[{"name":"models/gemini-2.5-flash"}]}`}
	fetcher := newDefaultURLFetcher(rt)

	models, err := fetcher.FetchModels(context.Background(), ProviderOptions{Provider: "gemini-native"})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if !strings.Contains(rt.gotURL, "generativelanguage.googleapis.com") || !strings.HasSuffix(rt.gotURL, "/v1beta/models") {
		t.Fatalf("expected defaulted gemini-native URL, got %q", rt.gotURL)
	}
	if len(models) != 1 {
		t.Fatalf("models = %v", models)
	}
}

func TestModelFetcher_OpenAIDefaultsBaseURL(t *testing.T) {
	rt := &recordingRoundTripper{body: `{"data":[{"id":"gpt-4o"}]}`}
	fetcher := newDefaultURLFetcher(rt)

	if _, err := fetcher.FetchModels(context.Background(), ProviderOptions{Provider: "openai"}); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if !strings.HasPrefix(rt.gotURL, "https://api.openai.com/v1") || !strings.HasSuffix(rt.gotURL, "/models") {
		t.Fatalf("expected defaulted openai URL, got %q", rt.gotURL)
	}
}
