package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebTool_FetchActionDelegatesToFetchTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	web := NewWebTool(WebOptions{
		FetchEnabled: true,
		Fetch: WebFetchOptions{
			AllowPrivateHosts: true,
			HTTPClient:        &http.Client{Timeout: 5 * time.Second},
		},
	})

	args := []byte(`{"action":"fetch","url":"` + srv.URL + `"}`)
	res, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Text())
	}
	if !strings.Contains(res.Text(), "hello world") {
		t.Fatalf("missing fetched content: %s", res.Text())
	}
}

func TestWebTool_SearchActionDelegatesToSearchTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"t","url":"https://x","description":"d"}]}}`))
	}))
	defer srv.Close()

	web := NewWebTool(WebOptions{
		SearchEnabled: true,
		Search: WebSearchOptions{
			Provider:     "brave",
			BraveAPIKey:  "test-key",
			BraveBaseURL: srv.URL,
			HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		},
	})

	args := []byte(`{"action":"search","query":"hello","count":3}`)
	res, err := web.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Text())
	}
	var payload struct {
		Provider string `json:"provider"`
		Count    int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Provider != "brave" || payload.Count != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestWebTool_RejectsUnknownAction(t *testing.T) {
	web := NewWebTool(WebOptions{SearchEnabled: true, FetchEnabled: true})
	res, err := web.Execute(context.Background(), []byte(`{"action":"explode"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for unknown action, got: %s", res.Text())
	}
}

func TestWebTool_DisabledActionsReportDisabled(t *testing.T) {
	web := NewWebTool(WebOptions{}) // both disabled
	fetchRes, err := web.Execute(context.Background(), []byte(`{"action":"fetch","url":"https://example.test"}`))
	if err != nil {
		t.Fatalf("execute fetch: %v", err)
	}
	if !fetchRes.IsError || !strings.Contains(fetchRes.Text(), "disabled") {
		t.Fatalf("expected fetch disabled error, got: %s", fetchRes.Text())
	}
	searchRes, err := web.Execute(context.Background(), []byte(`{"action":"search","query":"hi"}`))
	if err != nil {
		t.Fatalf("execute search: %v", err)
	}
	if !searchRes.IsError || !strings.Contains(searchRes.Text(), "disabled") {
		t.Fatalf("expected search disabled error, got: %s", searchRes.Text())
	}
}
