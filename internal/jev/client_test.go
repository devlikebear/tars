package jev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAskSendsWireRequestAndParsesAnswers(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"model":"kev-0.8b","answers":{"quiet":{"noul":0.83}},"usage":{"input_tokens":120}}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, APIKey: "k-123", Model: "kev-latest"})
	resp, err := c.Ask(context.Background(), "now: Tue", map[string]Question{
		"quiet": {Type: "noul", Instructions: "Did the user ask for quiet?"},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if auth != "Bearer k-123" {
		t.Fatalf("auth header = %q", auth)
	}
	if got["state"] != "now: Tue" || got["model"] != "kev-latest" {
		t.Fatalf("request body = %v", got)
	}
	a := resp.Answers["quiet"]
	if a.Noul == nil || *a.Noul != 0.83 || resp.Usage.InputTokens != 120 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAskRetriesOverloadThenSucceeds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(529)
			return
		}
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer srv.Close()
	var slept []time.Duration
	c := New(Options{BaseURL: srv.URL, Sleep: func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}})
	if _, err := c.Ask(context.Background(), "s", map[string]Question{}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if calls != 3 || len(slept) != 2 || slept[0] != 500*time.Millisecond || slept[1] != time.Second {
		t.Fatalf("calls=%d slept=%v", calls, slept)
	}
}

func TestAskGivesUpAfterRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL, Sleep: func(context.Context, time.Duration) error { return nil }})
	_, err := c.Ask(context.Background(), "s", map[string]Question{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusTooManyRequests || calls != 4 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestAskReturnsRedactedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key sk-live-SECRETSECRET"}` + strings.Repeat("x", 500)))
	}))
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL, APIKey: "sk-live-SECRETSECRET"})
	_, err := c.Ask(context.Background(), "s", map[string]Question{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(httpErr.Message, "SECRETSECRET") || len(httpErr.Message) > 220 {
		t.Fatalf("message not redacted/truncated: %q", httpErr.Message)
	}
}

func TestAskNotConfigured(t *testing.T) {
	if _, err := New(Options{}).Ask(context.Background(), "s", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func TestIsLoopback(t *testing.T) {
	for url, want := range map[string]bool{
		"http://127.0.0.1:8009":   true,
		"http://localhost:8009":   true,
		"http://[::1]:8009":       true,
		"https://api.typesafe.ai": false,
		"http://192.168.0.5:8009": false,
		"":                        false,
	} {
		if got := New(Options{BaseURL: url}).IsLoopback(); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", url, got, want)
		}
	}
}
