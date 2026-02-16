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
