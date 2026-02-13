package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicClientAsk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"anthropic response"}]}`))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := client.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if resp != "anthropic response" {
		t.Fatalf("unexpected response: %q", resp)
	}
}
