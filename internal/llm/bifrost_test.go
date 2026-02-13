package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBifrostClientAsk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok response"}}]}`))
	}))
	defer srv.Close()

	client, err := NewBifrostClient(srv.URL+"/v1", "test-key", "test-model")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if resp != "ok response" {
		t.Fatalf("expected 'ok response', got %q", resp)
	}
}

func TestNewBifrostClientRequiresConfig(t *testing.T) {
	_, err := NewBifrostClient("", "k", "m")
	if err == nil {
		t.Fatal("expected error for empty base url")
	}
	_, err = NewBifrostClient("http://localhost", "", "m")
	if err == nil {
		t.Fatal("expected error for empty api key")
	}
	_, err = NewBifrostClient("http://localhost", "k", "")
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestOpenAIClientAsk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"openai response"}}]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := client.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if resp != "openai response" {
		t.Fatalf("unexpected response: %q", resp)
	}
}
