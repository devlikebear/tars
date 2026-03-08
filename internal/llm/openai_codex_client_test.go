package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/auth"
)

func TestOpenAICodexClient_Headers_SetsRequired(t *testing.T) {
	var gotAuth, gotBeta, gotOriginator, gotAccept, gotAccountID, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBeta = r.Header.Get("OpenAI-Beta")
		gotOriginator = r.Header.Get("originator")
		gotAccept = r.Header.Get("Accept")
		gotAccountID = r.Header.Get("chatgpt-account-id")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{
				AccessToken:  "token-1",
				RefreshToken: "refresh-1",
				AccountID:    "acc-1",
			}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	var delta strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "say hello"},
	}, ChatOptions{
		OnDelta: func(s string) { delta.WriteString(s) },
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "hello" {
		t.Fatalf("expected assistant content hello, got %q", resp.Message.Content)
	}
	if delta.String() != "hello" {
		t.Fatalf("expected stream delta hello, got %q", delta.String())
	}
	if gotPath != "/codex/responses" {
		t.Fatalf("expected codex responses path, got %q", gotPath)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("expected bearer header, got %q", gotAuth)
	}
	if gotBeta != "responses=experimental" {
		t.Fatalf("expected OpenAI-Beta responses=experimental, got %q", gotBeta)
	}
	if gotOriginator != "tars" {
		t.Fatalf("expected originator tars, got %q", gotOriginator)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("expected Accept text/event-stream, got %q", gotAccept)
	}
	if gotAccountID != "acc-1" {
		t.Fatalf("expected chatgpt-account-id acc-1, got %q", gotAccountID)
	}
}

func TestOpenAICodexClient_RequestBody_IncludesRequiredFields(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","usage":{"input_tokens":2,"output_tokens":1},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
	}, ChatOptions{
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "exec",
					Description: "execute a shell command",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("expected assistant content ok, got %q", resp.Message.Content)
	}

	if got["model"] != "gpt-5.3-codex" {
		t.Fatalf("expected model gpt-5.3-codex, got %#v", got["model"])
	}
	if got["store"] != false {
		t.Fatalf("expected store=false, got %#v", got["store"])
	}
	if got["stream"] != false {
		t.Fatalf("expected stream=false, got %#v", got["stream"])
	}
	if got["tool_choice"] != "auto" {
		t.Fatalf("expected tool_choice=auto, got %#v", got["tool_choice"])
	}
	if got["parallel_tool_calls"] != true {
		t.Fatalf("expected parallel_tool_calls=true, got %#v", got["parallel_tool_calls"])
	}
	include, ok := got["include"].([]any)
	if !ok || !containsAnyString(include, "reasoning.encrypted_content") {
		t.Fatalf("expected include reasoning.encrypted_content, got %#v", got["include"])
	}
	if _, ok := got["input"].([]any); !ok {
		t.Fatalf("expected input array, got %#v", got["input"])
	}
	if _, ok := got["tools"].([]any); !ok {
		t.Fatalf("expected tools array, got %#v", got["tools"])
	}
}

func TestOpenAICodexClient_ToolNames_SanitizeAndRestore(t *testing.T) {
	var got map[string]any
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"mcp_filesystem_read_file","arguments":"{\"path\":\"README.md\"}"}}`,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "read readme"},
	}, ChatOptions{
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "mcp.filesystem.read_file",
					Description: "read file from mcp filesystem",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
				},
			},
		},
		OnDelta: func(string) {},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool in request body, got %#v", got["tools"])
	}
	tool0, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object, got %#v", tools[0])
	}
	if tool0["name"] != "mcp_filesystem_read_file" {
		t.Fatalf("expected sanitized tool name mcp_filesystem_read_file, got %#v", tool0["name"])
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "mcp.filesystem.read_file" {
		t.Fatalf("expected restored tool name mcp.filesystem.read_file, got %q", resp.Message.ToolCalls[0].Name)
	}
}

func TestOpenAICodexClient_StreamEvents_ParsesLifecycle(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		`data: {"type":"response.content_part.added","part":{"type":"output_text","text":""}}`,
		`data: {"type":"response.output_text.delta","delta":"Hel"}`,
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		`data: {"type":"response.output_item.done","item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":3}}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	var delta strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}}, ChatOptions{
		OnDelta: func(s string) { delta.WriteString(s) },
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "Hello" {
		t.Fatalf("expected Hello, got %q", resp.Message.Content)
	}
	if delta.String() != "Hello" {
		t.Fatalf("expected stream delta Hello, got %q", delta.String())
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestOpenAICodexClient_ToolCallStream_ParsesToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec","arguments":"{\"command\":\"pwd\"}"}}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":1}}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "run pwd"}}, ChatOptions{
		OnDelta: func(string) {},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %q", resp.Message.ToolCalls[0].ID)
	}
	if resp.Message.ToolCalls[0].Name != "exec" {
		t.Fatalf("expected tool call name exec, got %q", resp.Message.ToolCalls[0].Name)
	}
	if resp.Message.ToolCalls[0].Arguments != `{"command":"pwd"}` {
		t.Fatalf("expected arguments {\"command\":\"pwd\"}, got %q", resp.Message.ToolCalls[0].Arguments)
	}
}

func TestOpenAICodexClient_ToolCallStream_PrefersLatestArguments(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec","arguments":"{\"command\":\"p\"}"}}`,
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec","arguments":"{\"command\":\"pwd\"}"}}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":1}}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "run pwd"}}, ChatOptions{
		OnDelta: func(string) {},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Arguments != `{"command":"pwd"}` {
		t.Fatalf("expected latest arguments {\"command\":\"pwd\"}, got %q", resp.Message.ToolCalls[0].Arguments)
	}
}

func TestOpenAICodexClient_RefreshRetry401_RetriesOnce(t *testing.T) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer new-token" {
			t.Fatalf("expected refreshed token, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	refreshCount := 0
	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{
				AccessToken:  "old-token",
				RefreshToken: "old-refresh",
				AccountID:    "acc-1",
			}, nil
		},
		func(_ context.Context, cred auth.CodexCredential) (auth.CodexCredential, error) {
			refreshCount++
			if cred.RefreshToken != "old-refresh" {
				return auth.CodexCredential{}, fmt.Errorf("unexpected refresh token %q", cred.RefreshToken)
			}
			return auth.CodexCredential{
				AccessToken:  "new-token",
				RefreshToken: "new-refresh",
				AccountID:    "acc-1",
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}}, ChatOptions{
		OnDelta: func(string) {},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("expected assistant content ok, got %q", resp.Message.Content)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
	if refreshCount != 1 {
		t.Fatalf("expected refresh once, got %d", refreshCount)
	}
}

func TestOpenAICodexClient_StreamRequiredFallback_RetriesWithStream(t *testing.T) {
	var requestCount int
	var streamValues []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		stream, _ := body["stream"].(bool)
		streamValues = append(streamValues, stream)
		if !stream {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"Stream must be set to true"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hello"},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("expected fallback streamed response ok, got %q", resp.Message.Content)
	}
	if requestCount != 2 {
		t.Fatalf("expected two requests with stream fallback, got %d", requestCount)
	}
	if len(streamValues) != 2 || streamValues[0] || !streamValues[1] {
		t.Fatalf("expected stream flags [false,true], got %#v", streamValues)
	}
}

func TestOpenAICodexClient_RetryOnceOnInternalErrorEvent(t *testing.T) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount == 1 {
			_, _ = w.Write([]byte("data: {\"type\":\"error\",\"message\":\"stream ID 9; INTERNAL_ERROR; received from peer\"}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(
		srv.URL,
		"gpt-5.3-codex",
		"oauth",
		"openai-codex",
		"",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) {
			return auth.CodexCredential{AccessToken: "token-1"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hello"},
	}, ChatOptions{
		OnDelta: func(string) {},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("expected retried streamed response ok, got %q", resp.Message.Content)
	}
	if requestCount != 2 {
		t.Fatalf("expected two requests with internal error retry, got %d", requestCount)
	}
}

func containsAnyString(values []any, needle string) bool {
	for _, raw := range values {
		if s, ok := raw.(string); ok && s == needle {
			return true
		}
	}
	return false
}
