package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebSearchTool_MissingAPIKey(t *testing.T) {
	t1 := NewWebSearchTool(true, "")
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if !res.IsError || !strings.Contains(strings.ToLower(res.Text()), "api key") {
		t.Fatalf("expected api key error, got %s", res.Text())
	}
}

func TestWebSearchTool_ParsesResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"TARS","url":"https://tars.dev","description":"assistant"}]}}`))
	}))
	defer ts.Close()

	t1 := newWebSearchToolWithHTTP(ts.URL, true, "key", ts.Client())
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "https://tars.dev") {
		t.Fatalf("expected parsed url, got %s", res.Text())
	}
}

func TestWebSearchTool_PerplexityProvider(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "Bearer px-key") {
			t.Fatalf("expected authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"answer"}}],"citations":["https://example.com"]}`))
	}))
	defer ts.Close()

	t1 := NewWebSearchToolWithOptions(WebSearchOptions{
		Enabled:           true,
		Provider:          "perplexity",
		PerplexityAPIKey:  "px-key",
		PerplexityBaseURL: ts.URL,
		CacheTTL:          0,
		HTTPClient:        ts.Client(),
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "https://example.com") {
		t.Fatalf("expected citation url, got %s", res.Text())
	}
}

func TestWebSearchTool_PerplexityMissingAPIKey(t *testing.T) {
	t1 := NewWebSearchToolWithOptions(WebSearchOptions{
		Enabled:  true,
		Provider: "perplexity",
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if !res.IsError || !strings.Contains(strings.ToLower(res.Text()), "perplexity api key") {
		t.Fatalf("expected perplexity api key error, got %s", res.Text())
	}
}

func TestWebSearchTool_InvalidProvider(t *testing.T) {
	t1 := NewWebSearchToolWithOptions(WebSearchOptions{
		Enabled:     true,
		Provider:    "brave",
		BraveAPIKey: "key",
		CacheTTL:    0,
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tars","provider":"unknown"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if !res.IsError || !strings.Contains(strings.ToLower(res.Text()), "provider must be one of") {
		t.Fatalf("expected invalid provider error, got %s", res.Text())
	}
}

func TestWebSearchTool_UsesCache(t *testing.T) {
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"TARS","url":"https://tars.dev","description":"assistant"}]}}`))
	}))
	defer ts.Close()

	t1 := NewWebSearchToolWithOptions(WebSearchOptions{
		Enabled:      true,
		Provider:     "brave",
		BraveAPIKey:  "key",
		BraveBaseURL: ts.URL,
		CacheTTL:     5 * time.Second,
		HTTPClient:   ts.Client(),
	})
	_, _ = t1.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	_, _ = t1.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if hits.Load() != 1 {
		t.Fatalf("expected 1 network hit due to cache, got %d", hits.Load())
	}
}
