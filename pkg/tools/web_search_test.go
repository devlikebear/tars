package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	t1 := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "brave", BraveAPIKey: "key", BraveBaseURL: ts.URL, CacheTTL: 0, HTTPClient: ts.Client()})
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

func TestWebSearchTool_RustXNGProvider(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"tokio","results":[
			{"title":" Tokio ","url":"https://tokio.rs","content":" runtime ","engine":"wikipedia","score":1.5},
			{"title":"Extra","url":"https://example.com/extra","content":"past the limit"}
		]}`))
	}))
	defer ts.Close()

	for _, base := range []string{ts.URL, ts.URL + "/", ts.URL + "/search"} {
		t1 := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "rustxng", RustXNGBaseURL: base, HTTPClient: ts.Client()})
		res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tokio","count":1}`))
		if err != nil {
			t.Fatalf("web_search execute: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success, got %s", res.Text())
		}
		if gotPath != "/search" {
			t.Fatalf("base %q: expected /search, got %q", base, gotPath)
		}
		if gotQuery.Get("q") != "tokio" || gotQuery.Get("format") != "json" || gotQuery.Get("max_results") != "1" {
			t.Fatalf("unexpected query: %v", gotQuery)
		}
		if gotQuery.Has("include_raw_content") {
			t.Fatalf("page text must stay off: %v", gotQuery)
		}
		var payload webSearchResponse
		if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		want := webSearchResult{Title: "Tokio", URL: "https://tokio.rs", Description: "runtime"}
		if payload.Provider != "rustxng" || len(payload.Results) != 1 || payload.Results[0] != want {
			t.Fatalf("unexpected result: %+v", payload)
		}
	}
}

func TestWebSearchTool_RustXNGStatusError(t *testing.T) {
	// A server with JSON output turned off answers 403, as SearXNG does.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	t1 := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "rustxng", RustXNGBaseURL: ts.URL, HTTPClient: ts.Client()})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tokio"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "rustxng status 403") {
		t.Fatalf("expected status error, got %s", res.Text())
	}
}

func TestWebSearchTool_RustXNGInvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer ts.Close()

	t1 := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "rustxng", RustXNGBaseURL: ts.URL, HTTPClient: ts.Client()})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tokio"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "decode rustxng response failed") {
		t.Fatalf("expected decode error, got %s", res.Text())
	}
}

func TestWebSearchTool_RustXNGBadBaseURL(t *testing.T) {
	t1 := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "rustxng", RustXNGBaseURL: "http://bad host"})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"query":"tokio"}`))
	if err != nil {
		t.Fatalf("web_search execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "invalid rustxng base url") {
		t.Fatalf("expected base url error, got %s", res.Text())
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
