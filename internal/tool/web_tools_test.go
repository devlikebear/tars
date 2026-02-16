package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchTool_Disabled(t *testing.T) {
	t1 := NewWebFetchTool(false)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("web_fetch execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "disabled") {
		t.Fatalf("expected disabled error, got %s", res.Text())
	}
}

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

func TestWebFetchTool_ParsesHTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Hello</h1><p>World</p></body></html>`))
	}))
	defer ts.Close()

	t1 := newWebFetchToolWithHTTP(true, ts.Client())
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"url":"`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("web_fetch execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "Hello") || !strings.Contains(res.Text(), "World") {
		t.Fatalf("expected extracted text, got %s", res.Text())
	}
}
