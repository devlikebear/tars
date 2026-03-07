package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJSONRequestSpec_BuildRequest(t *testing.T) {
	req, err := jsonRequestSpec{
		Provider: "openai",
		URL:      "https://example.com/v1/chat/completions",
		Headers: map[string]string{
			"Authorization": "Bearer test-key",
			"Content-Type":  "application/json",
		},
		Body: map[string]any{
			"model": "gpt-test",
		},
	}.buildRequest(context.Background())
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("expected authorization header, got %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type header, got %q", got)
	}
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body["model"] != "gpt-test" {
		t.Fatalf("unexpected request body: %+v", body)
	}
}

func TestJSONRequestSpec_DoRequestUsesStreamingTransportWithoutClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	spec := jsonRequestSpec{
		Provider: "openai",
		URL:      server.URL,
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     map[string]any{"model": "gpt-test"},
	}
	transport := &transportRecorder{}
	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
	}

	streamClient := transportHTTPClient(client, true)
	if streamClient.Timeout != 0 {
		t.Fatalf("expected streaming client timeout=0, got %s", streamClient.Timeout)
	}
	if streamClient.Transport != transport {
		t.Fatal("expected streaming client to reuse base transport")
	}

	resp, err := doJSONRequest(spec, client, true)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_ = resp.Body.Close()
	if client.Timeout != 2*time.Second {
		t.Fatalf("expected original client timeout to remain unchanged")
	}
}

func TestJSONRequestSpec_DoRequestWrapsHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := doJSONRequest(jsonRequestSpec{
		Provider: "anthropic",
		URL:      server.URL,
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     map[string]any{"model": "claude"},
	}, &http.Client{Timeout: time.Second}, false)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %+v", providerErr)
	}
	if !strings.Contains(providerErr.Error(), "anthropic status 401: denied") {
		t.Fatalf("unexpected error message: %v", providerErr)
	}
}

type transportRecorder struct{}

func (t *transportRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req)
}
