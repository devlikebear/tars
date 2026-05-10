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

func TestOpenAICodexClient_CapturesRateLimitHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-codex-primary-used-percent", "37.5")
		w.Header().Set("x-codex-primary-window-minutes", "300")
		w.Header().Set("x-codex-primary-reset-after-seconds", "1200")
		w.Header().Set("x-codex-secondary-used-percent", "8")
		w.Header().Set("x-codex-secondary-window-minutes", "10080")
		w.Header().Set("x-codex-credits-remaining", "9001")
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
			return auth.CodexCredential{AccessToken: "tok"}, nil
		},
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			t.Fatal("refresh should not be called")
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	if _, ok := client.LastCodexRateLimit(); ok {
		t.Fatal("expected no snapshot before any request")
	}

	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		OnDelta: func(string) {},
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	snap, ok := client.LastCodexRateLimit()
	if !ok {
		t.Fatal("expected snapshot after request")
	}
	if snap.Primary == nil || snap.Primary.UsedPercent != 37.5 || snap.Primary.WindowMinutes != 300 {
		t.Errorf("primary: %+v", snap.Primary)
	}
	if snap.Secondary == nil || snap.Secondary.UsedPercent != 8 || snap.Secondary.WindowMinutes != 10080 {
		t.Errorf("secondary: %+v", snap.Secondary)
	}
	if got := snap.RawHeaders["x-codex-credits-remaining"]; got != "9001" {
		t.Errorf("raw credits-remaining: got %q, want 9001", got)
	}

	// Sanity: client also satisfies the CodexRateLimitSource interface.
	var _ CodexRateLimitSource = client
}

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

// TestOpenAICodexClient_ToolChoice_Specific covers RF-048: when the caller
// passes ToolChoiceSpecific, the request body must use the object form
// (Responses API parity with Chat Completions). The previous hardcoded
// "auto" silently dropped the request.
func TestOpenAICodexClient_ToolChoice_Specific(t *testing.T) {
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(srv.URL, "gpt-5.3-codex", "oauth", "openai-codex", "",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) { return auth.CodexCredential{AccessToken: "t"}, nil },
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		OnDelta:    func(string) {},
		ToolChoice: ToolChoiceSpecific("exec"),
		Tools:      []ToolSchema{{Type: "function", Function: ToolFunctionSchema{Name: "exec", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	tc, ok := got["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_choice object, got %#v", got["tool_choice"])
	}
	fn, _ := tc["function"].(map[string]any)
	if tc["type"] != "function" || fn["name"] != "exec" {
		t.Fatalf("unexpected tool_choice payload: %#v", tc)
	}
}

// TestOpenAICodexClient_ResponseFormat_JSONSchema covers RF-048: the
// Responses API uses text.format (not response_format) for structured
// outputs. Strict + named schema must serialize at the top of the format
// envelope.
func TestOpenAICodexClient_ResponseFormat_JSONSchema(t *testing.T) {
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(srv.URL, "gpt-5.3-codex", "oauth", "openai-codex", "",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) { return auth.CodexCredential{AccessToken: "t"}, nil },
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	schema := json.RawMessage(`{"type":"object","properties":{"steps":{"type":"array"}}}`)
	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "plan"}}, ChatOptions{
		OnDelta:        func(string) {},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, Name: "plan", Schema: schema, Strict: true},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	text, ok := got["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text envelope, got %#v", got["text"])
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("expected text.format, got %#v", text["format"])
	}
	if format["type"] != "json_schema" || format["name"] != "plan" || format["strict"] != true {
		t.Fatalf("unexpected text.format: %#v", format)
	}
	if _, ok := format["schema"]; !ok {
		t.Fatalf("schema missing from text.format envelope: %#v", format)
	}
}

// TestOpenAICodexClient_ReasoningEffortAndServiceTier covers RF-049:
// previously ReasoningEffort and ServiceTier were silently dropped.
// Responses API expects reasoning.effort (object) and service_tier
// (string).
func TestOpenAICodexClient_ReasoningEffortAndServiceTier(t *testing.T) {
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(srv.URL, "gpt-5.3-codex", "oauth", "openai-codex", "",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) { return auth.CodexCredential{AccessToken: "t"}, nil },
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		OnDelta:         func(string) {},
		ReasoningEffort: "high",
		ServiceTier:     "priority",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	reasoning, ok := got["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("expected reasoning.effort=high, got %#v", got["reasoning"])
	}
	if got["service_tier"] != "priority" {
		t.Fatalf("expected service_tier=priority, got %#v", got["service_tier"])
	}
}

// TestOpenAICodexClient_PDFUnsupportedError verifies RF-046: codex must
// surface a structured error rather than silently turning a PDF into
// throwaway placeholder text.
func TestOpenAICodexClient_PDFUnsupportedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := newOpenAICodexClientWithConfig(srv.URL, "gpt-5.3-codex", "oauth", "openai-codex", "",
		DefaultClientConfig(),
		func() (auth.CodexCredential, error) { return auth.CodexCredential{AccessToken: "t"}, nil },
		func(context.Context, auth.CodexCredential) (auth.CodexCredential, error) {
			return auth.CodexCredential{}, nil
		},
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{
		{Role: "user", ContentBlocks: []ContentBlock{{Type: "document", MediaType: "application/pdf", Data: "JVBERi0..."}}},
	}, ChatOptions{})
	if err == nil {
		t.Fatalf("expected pdf_unsupported error, got nil")
	}
	if !strings.Contains(err.Error(), "pdf_unsupported_by_provider") {
		t.Fatalf("unexpected error: %v", err)
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

func TestOpenAICodexClient_StreamReasoningSummary(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.reasoning_summary_text.delta","delta":"Plan: "}`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"do it"}`,
		`data: {"type":"response.output_text.delta","delta":"Done"}`,
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

	var content, reasoning strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		OnDelta:          func(s string) { content.WriteString(s) },
		OnReasoningDelta: func(s string) { reasoning.WriteString(s) },
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if got, want := content.String(), "Done"; got != want {
		t.Fatalf("content stream got %q want %q", got, want)
	}
	if got, want := reasoning.String(), "Plan: do it"; got != want {
		t.Fatalf("reasoning stream got %q want %q", got, want)
	}
	if resp.Message.ReasoningContent != "Plan: do it" {
		t.Fatalf("ReasoningContent buffer got %q", resp.Message.ReasoningContent)
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
