package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/secrets"
)

// errorsAs keeps the assertions below readable.
func errorsAs(err error, target any) bool { return errors.As(err, target) }

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient(Config{APIKey: "k-test", BaseURL: srv.URL, Model: "jev-latest"})
	c.httpClient = srv.Client()
	c.sleep = func(time.Duration) {}
	return c
}

func TestAsk_SendsExpectedRequestShape(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"op":{"type":"choice","choice":"click","confidence":0.9,"probabilities":{"click":0.9,"none":0.1}},"risky":{"type":"noul","noul":0.05}},"usage":{"input_tokens":10,"output_tokens":2}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	resp, err := c.Ask(context.Background(), "STATE", map[string]Question{
		"op":    Choice("which op", map[string]string{"click": "click it", "none": "nothing"}),
		"risky": Noul("is it risky", "hard to undo", "reversible"),
		"mood":  Score("how", []string{"Calm", "Angry"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer k-test" {
		t.Errorf("auth = %q", auth)
	}
	if got["state"] != "STATE" || got["model"] != "jev-latest" {
		t.Errorf("state/model = %v/%v", got["state"], got["model"])
	}
	qs := got["questions"].(map[string]any)
	if qs["op"].(map[string]any)["type"] != "choice" {
		t.Errorf("op type = %v", qs["op"])
	}
	if crit := qs["risky"].(map[string]any)["criteria"].(map[string]any); crit["true"] != "hard to undo" || crit["false"] != "reversible" {
		t.Errorf("noul criteria = %v", crit)
	}
	if levels := qs["mood"].(map[string]any)["criteria"].([]any); len(levels) != 2 || levels[0] != "Calm" {
		t.Errorf("score criteria = %v", levels)
	}
	if resp.Answers["op"].Choice != "click" || resp.Answers["op"].Confidence != 0.9 {
		t.Errorf("op answer = %+v", resp.Answers["op"])
	}
	if resp.Answers["risky"].Noul != 0.05 || resp.Usage.InputTokens != 10 {
		t.Errorf("risky/usage = %+v / %+v", resp.Answers["risky"], resp.Usage)
	}
}

func TestAsk_RetriesOn429ThenSucceeds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{}}`))
	}))
	defer srv.Close()
	var slept []time.Duration
	c := newTestClient(t, srv)
	c.sleep = func(d time.Duration) { slept = append(slept, d) }
	if _, err := c.Ask(context.Background(), "s", nil); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(slept) != 2 || slept[0] != 500*time.Millisecond || slept[1] != time.Second {
		t.Errorf("calls=%d slept=%v", calls, slept)
	}
}

func TestAsk_GivesUpAfterThreeRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(529)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.Ask(context.Background(), "s", nil)
	var jerr *Error
	if !errorsAs(err, &jerr) || jerr.Status != 529 || calls != 4 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestAsk_401IsNotRetriedAndBodyIsTruncated(t *testing.T) {
	calls := 0
	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'x'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(long)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.Ask(context.Background(), "s", nil)
	var jerr *Error
	if !errorsAs(err, &jerr) || jerr.Status != 401 || calls != 1 || len(jerr.Message) > 200 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestAsk_ErrorBodyNeverContainsAPIKey(t *testing.T) {
	secrets.ResetForTests()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"bad key k-test in header"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.Ask(context.Background(), "s", nil)
	var jerr *Error
	if !errorsAs(err, &jerr) || jerr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(jerr.Message, "k-test") {
		t.Errorf("api key leaked into error message: %q", jerr.Message)
	}
	if strings.Contains(err.Error(), "k-test") {
		t.Errorf("api key leaked into Error(): %q", err.Error())
	}
}

func TestAsk_NoAPIKey(t *testing.T) {
	c := NewClient(Config{})
	if c.Configured() {
		t.Fatal("expected not configured")
	}
	if _, err := c.Ask(context.Background(), "s", nil); err != ErrNoAPIKey {
		t.Fatalf("err = %v", err)
	}
}
