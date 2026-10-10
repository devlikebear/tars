package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lmStudioTemplateErrorStream is what LM Studio sent for a Qwen-family model
// whose chat template rejected the request: HTTP 200, an event stream, and
// the failure as an SSE error event.
const lmStudioTemplateErrorStream = "event: error\n" +
	`data: {"error":{"message":"Engine protocol predict request returned 500: Error rendering prompt with jinja template: \"Jinja Exception: System message must be at the beginning.\""},"message":"Engine protocol predict request returned 500: Error rendering prompt with jinja template: \"Jinja Exception: System message must be at the beginning.\""}` +
	"\n\n"

func streamOpenAICompat(t *testing.T, body string) (ChatResponse, string, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var streamed strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		OnDelta: func(text string) { streamed.WriteString(text) },
	})
	return resp, streamed.String(), err
}

func TestOpenAICompatibleChat_StreamErrorIsReturned(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "lm studio error event",
			body: lmStudioTemplateErrorStream,
			want: "System message must be at the beginning",
		},
		{
			name: "error object without an event line",
			body: `data: {"error":{"message":"model crashed","type":"server_error"}}` + "\n\ndata: [DONE]\n\n",
			want: "model crashed",
		},
		{
			name: "error after content",
			body: `data: {"choices":[{"delta":{"content":"par"}}]}` + "\n\n" +
				`data: {"error":{"message":"context length exceeded"}}` + "\n\n",
			want: "context length exceeded",
		},
		{
			name: "error as a string",
			body: `data: {"error":"overloaded"}` + "\n\n",
			want: "overloaded",
		},
		{
			name: "error object without a message falls back to the top-level message",
			body: `data: {"error":{"code":500},"message":"engine failed"}` + "\n\n",
			want: "engine failed",
		},
		{
			name: "error event with an unrecognized payload",
			body: "event: error\n" + `data: {"detail":"boom"}` + "\n\n",
			want: `{"detail":"boom"}`,
		},
		{
			name: "error event with a payload that is not json",
			body: "event: error\ndata: upstream unavailable\n\n",
			want: "upstream unavailable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := streamOpenAICompat(t, tc.body)
			if err == nil {
				t.Fatalf("expected the in-stream error, got none")
			}
			var provErr *ProviderError
			if !errors.As(err, &provErr) {
				t.Fatalf("expected a ProviderError, got %T: %v", err, err)
			}
			if provErr.Provider != "openai" || provErr.Operation != "stream" {
				t.Fatalf("unexpected provider error: %+v", provErr)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not carry %q", err.Error(), tc.want)
			}
		})
	}
}

// An event name belongs to one event only: a named event that is not an
// error, and the chunks after an error-free one, are still read as chunks.
func TestOpenAICompatibleChat_StreamIgnoresOtherEventNames(t *testing.T) {
	body := "event: message\n" + `data: {"choices":[{"delta":{"content":"hel"}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"content":"lo"}}],"error":null}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"
	resp, streamed, err := streamOpenAICompat(t, body)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "hello" || streamed != "hello" {
		t.Fatalf("unexpected content %q (streamed %q)", resp.Message.Content, streamed)
	}
	if resp.StopReason != "stop" {
		t.Fatalf("unexpected stop reason %q", resp.StopReason)
	}
}
