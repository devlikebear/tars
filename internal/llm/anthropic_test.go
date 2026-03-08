package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAnthropicChat_IncludesToolsAndParsesToolUse(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[
				{"type":"tool_use","id":"tool_1","name":"memory_search","input":{"query":"coffee"}}
			],
			"usage":{"input_tokens":7,"output_tokens":3},
			"stop_reason":"tool_use"
		}`))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "find memory"},
	}, ChatOptions{
		ToolChoice:     "required",
		ThinkingBudget: 4096,
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "memory_search",
					Description: "search memory",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	toolsRaw, ok := captured["tools"].([]any)
	if !ok || len(toolsRaw) != 1 {
		t.Fatalf("expected one tool in request, got %+v", captured["tools"])
	}
	toolMap, ok := toolsRaw[0].(map[string]any)
	if !ok || toolMap["name"] != "memory_search" {
		t.Fatalf("unexpected tool payload: %+v", toolsRaw[0])
	}
	choiceRaw, ok := captured["tool_choice"].(map[string]any)
	if !ok || choiceRaw["type"] != "any" {
		t.Fatalf("expected tool_choice any, got %+v", captured["tool_choice"])
	}
	thinkingRaw, ok := captured["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking config in request, got %+v", captured["thinking"])
	}
	if thinkingRaw["type"] != "enabled" {
		t.Fatalf("expected thinking.type enabled, got %+v", thinkingRaw)
	}
	if thinkingRaw["budget_tokens"] != float64(4096) {
		t.Fatalf("expected thinking.budget_tokens 4096, got %+v", thinkingRaw)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "memory_search" {
		t.Fatalf("unexpected tool name: %q", resp.Message.ToolCalls[0].Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(resp.Message.ToolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("tool args should be valid json: %v", err)
	}
	if args["query"] != "coffee" {
		t.Fatalf("unexpected tool args: %+v", args)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("unexpected stop reason: %q", resp.StopReason)
	}
}

func TestAnthropicChat_StreamParsesToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"message\":{\"usage\":{\"input_tokens\":11}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tool_1\",\"name\":\"memory_search\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"partial_json\":\"{\\\"query\\\":\\\"co\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"partial_json\":\"ffee\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var streamed strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "find memory"},
	}, ChatOptions{
		OnDelta: func(text string) {
			streamed.WriteString(text)
		},
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "memory_search",
					Description: "search memory",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if streamed.String() != "" {
		t.Fatalf("expected empty text stream for tool-call response, got %q", streamed.String())
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(resp.Message.ToolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("tool args should be valid json: %v", err)
	}
	if args["query"] != "coffee" {
		t.Fatalf("unexpected tool args: %+v", args)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("expected stop reason tool_use, got %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestAnthropicChat_StreamToolUseStartInputAndPartialJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"message\":{\"usage\":{\"input_tokens\":11}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tool_1\",\"name\":\"exec\",\"input\":{}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"partial_json\":\"{\\\"command\\\":\\\"p\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"partial_json\":\"wd\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "pwd"},
	}, ChatOptions{
		OnDelta: func(string) {},
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "exec",
					Description: "run command",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(resp.Message.ToolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("tool args should be valid json: %v", err)
	}
	if args["command"] != "pwd" {
		t.Fatalf("unexpected tool args: %+v", args)
	}
}

func TestAnthropicChat_RequestUsesAnthropicToolWireFormat(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{
					ID:        "call_1",
					Name:      "memory_search",
					Arguments: `{"query":"coffee"}`,
				},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    `{"results":[]}`,
		},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	messagesRaw, ok := captured["messages"].([]any)
	if !ok || len(messagesRaw) != 2 {
		t.Fatalf("expected two messages, got %+v", captured["messages"])
	}

	assistantMsg, ok := messagesRaw[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid assistant message payload: %+v", messagesRaw[0])
	}
	if assistantMsg["role"] != "assistant" {
		t.Fatalf("expected assistant role, got %+v", assistantMsg["role"])
	}
	assistantBlocks, ok := assistantMsg["content"].([]any)
	if !ok || len(assistantBlocks) != 1 {
		t.Fatalf("expected assistant content block with tool_use, got %+v", assistantMsg["content"])
	}
	toolUse, ok := assistantBlocks[0].(map[string]any)
	if !ok || toolUse["type"] != "tool_use" || toolUse["id"] != "call_1" || toolUse["name"] != "memory_search" {
		t.Fatalf("unexpected tool_use block: %+v", assistantBlocks[0])
	}

	toolResultMsg, ok := messagesRaw[1].(map[string]any)
	if !ok {
		t.Fatalf("invalid tool result message payload: %+v", messagesRaw[1])
	}
	if toolResultMsg["role"] != "user" {
		t.Fatalf("expected user role for tool result, got %+v", toolResultMsg["role"])
	}
	toolResultBlocks, ok := toolResultMsg["content"].([]any)
	if !ok || len(toolResultBlocks) != 1 {
		t.Fatalf("expected tool_result content block, got %+v", toolResultMsg["content"])
	}
	toolResult, ok := toolResultBlocks[0].(map[string]any)
	if !ok || toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "call_1" {
		t.Fatalf("unexpected tool_result block: %+v", toolResultBlocks[0])
	}
}
