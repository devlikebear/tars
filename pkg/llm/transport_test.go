package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
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

func TestExecuteJSONChatRequest_ReturnsRequestAndResponse(t *testing.T) {
	var capturedAuth string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	req, resp, err := executeJSONChatRequest(context.Background(), jsonRequestSpec{
		Provider: "openai",
		URL:      server.URL,
		Headers: map[string]string{
			"Authorization": "Bearer test-key",
			"Content-Type":  "application/json",
		},
		Body: map[string]any{"model": "gpt-test"},
	}, &http.Client{Timeout: time.Second}, false)
	if err != nil {
		t.Fatalf("execute json chat request: %v", err)
	}
	defer resp.Body.Close()

	if req == nil {
		t.Fatal("expected request")
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if got := req.URL.String(); got != server.URL {
		t.Fatalf("expected request URL %q, got %q", server.URL, got)
	}
	if capturedAuth != "Bearer test-key" {
		t.Fatalf("expected authorization header, got %q", capturedAuth)
	}
	if capturedBody["model"] != "gpt-test" {
		t.Fatalf("unexpected request body: %+v", capturedBody)
	}
}

func TestLogChatRequestStart_LogsStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	prev := zlog.Logger
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	defer func() { zlog.Logger = prev }()

	logChatRequestStart("openai", "gpt-4o-mini", "https://example.com/v1/chat/completions", 2, true, 1, "required")

	logged := buf.String()
	for _, want := range []string{
		`"provider":"openai"`,
		`"model":"gpt-4o-mini"`,
		`"url":"https://example.com/v1/chat/completions"`,
		`"message_count":2`,
		`"stream":true`,
		`"tool_count":1`,
		`"tool_choice":"required"`,
		`"message":"llm request start"`,
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("expected log to contain %q, got %q", want, logged)
		}
	}
}

type transportRecorder struct{}

func (t *transportRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req)
}
