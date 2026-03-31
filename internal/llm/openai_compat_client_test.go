package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestOpenAICompatibleChat_IncludesToolsAndParsesToolCalls(t *testing.T) {
	var captured struct {
		Tools           []ToolSchema `json:"tools"`
		ToolChoice      string       `json:"tool_choice"`
		ReasoningEffort string       `json:"reasoning_effort"`
		ServiceTier     string       `json:"service_tier"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[
				{
					"message":{
						"content":"",
						"tool_calls":[
							{
								"id":"call_1",
								"type":"function",
								"function":{"name":"memory_search","arguments":"{\"query\":\"coffee\"}"}
							}
						]
					},
					"finish_reason":"tool_calls"
				}
			],
			"usage":{"prompt_tokens":11,"completion_tokens":3}
		}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "find memory"},
	}, ChatOptions{
		ToolChoice:      "required",
		ReasoningEffort: "high",
		ServiceTier:     "priority",
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

	if len(captured.Tools) != 1 || captured.Tools[0].Function.Name != "memory_search" {
		t.Fatalf("expected tools in request, got %+v", captured.Tools)
	}
	if captured.ToolChoice != "required" {
		t.Fatalf("expected tool_choice=required, got %q", captured.ToolChoice)
	}
	if captured.ReasoningEffort != "high" {
		t.Fatalf("expected reasoning_effort=high, got %q", captured.ReasoningEffort)
	}
	if captured.ServiceTier != "priority" {
		t.Fatalf("expected service_tier=priority, got %q", captured.ServiceTier)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool_call in response, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "memory_search" {
		t.Fatalf("unexpected tool name: %q", resp.Message.ToolCalls[0].Name)
	}
	if resp.Message.ToolCalls[0].Arguments != `{"query":"coffee"}` {
		t.Fatalf("unexpected tool args: %q", resp.Message.ToolCalls[0].Arguments)
	}
}

func TestOpenAICompatibleChat_StreamParsesToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"memory_search\",\"arguments\":\"{\\\"query\\\":\\\"co\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"ffee\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
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
		t.Fatalf("expected one tool_call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Arguments != `{"query":"coffee"}` {
		t.Fatalf("unexpected assembled args: %q", resp.Message.ToolCalls[0].Arguments)
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("expected stop_reason tool_calls, got %q", resp.StopReason)
	}
}

func TestOpenAICompatibleChat_RequestUsesWireToolCallFormat(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role      string `json:"role"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Name     string `json:"name"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
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

	if len(captured.Messages) < 1 || len(captured.Messages[0].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool_calls in request, got %+v", captured.Messages)
	}
	tc := captured.Messages[0].ToolCalls[0]
	if tc.Type != "function" {
		t.Fatalf("expected type=function, got %q", tc.Type)
	}
	if tc.Function.Name != "memory_search" {
		t.Fatalf("expected function.name memory_search, got %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"query":"coffee"}` {
		t.Fatalf("expected function.arguments JSON string, got %q", tc.Function.Arguments)
	}
	if tc.Name != "" {
		t.Fatalf("wire format should not use top-level name field, got %q", tc.Name)
	}
}
