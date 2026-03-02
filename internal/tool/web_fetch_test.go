package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
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

func TestWebFetchTool_BlocksPrivateHostByDefault(t *testing.T) {
	t1 := NewWebFetchToolWithOptions(WebFetchOptions{
		Enabled: true,
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"url":"http://127.0.0.1:8080"}`))
	if err != nil {
		t.Fatalf("web_fetch execute: %v", err)
	}
	if !res.IsError || !strings.Contains(strings.ToLower(res.Text()), "private host is blocked") {
		t.Fatalf("expected private host block, got %s", res.Text())
	}
}

func TestWebFetchTool_AllowsPrivateHostFromAllowlist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	t1 := NewWebFetchToolWithOptions(WebFetchOptions{
		Enabled:              true,
		PrivateHostAllowlist: []string{"127.0.0.1"},
		HTTPClient:           ts.Client(),
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"url":"`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("web_fetch execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected allowlist success, got %s", res.Text())
	}
}

func TestWebFetchTool_RevalidatesRedirectTargets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:8080/internal", http.StatusFound)
	}))
	defer ts.Close()

	t1 := NewWebFetchToolWithOptions(WebFetchOptions{
		Enabled:    true,
		HTTPClient: ts.Client(),
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"url":"`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("web_fetch execute: %v", err)
	}
	if !res.IsError || !strings.Contains(strings.ToLower(res.Text()), "private host is blocked") {
		t.Fatalf("expected redirect private host block, got %s", res.Text())
	}
}

func TestWebFetchTool_AllowsPrivateHostWhenOptionEnabled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	var logBuf bytes.Buffer
	originalLogOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(originalLogOut)

	t1 := NewWebFetchToolWithOptions(WebFetchOptions{
		Enabled:           true,
		AllowPrivateHosts: true,
		HTTPClient:        ts.Client(),
	})
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"url":"`+ts.URL+`"}`))
	if err != nil {
		t.Fatalf("web_fetch execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with allow private hosts, got %s", res.Text())
	}
	if !strings.Contains(logBuf.String(), "web_fetch_private_host_allowed") {
		t.Fatalf("expected private host warning event log, got %q", logBuf.String())
	}
}
