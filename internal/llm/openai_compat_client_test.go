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
		ToolChoice:      ToolChoiceRequired(),
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

// TestOpenAICompatibleChat_SpecificToolChoice asserts that
// ToolChoiceSpecific marshals to OpenAI's object form
// {"type":"function","function":{"name":...}} — the bare string form would
// be rejected by the OpenAI server when a specific tool is required.
func TestOpenAICompatibleChat_SpecificToolChoice(t *testing.T) {
	type captured struct {
		ToolChoice json.RawMessage `json:"tool_choice"`
	}
	var got captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, ChatOptions{
		ToolChoice: ToolChoiceSpecific("memory_search"),
		Tools: []ToolSchema{
			{Type: "function", Function: ToolFunctionSchema{Name: "memory_search", Parameters: json.RawMessage(`{"type":"object"}`)}},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	var asObject struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(got.ToolChoice, &asObject); err != nil {
		t.Fatalf("expected object tool_choice, got %s: %v", string(got.ToolChoice), err)
	}
	if asObject.Type != "function" || asObject.Function.Name != "memory_search" {
		t.Fatalf("unexpected tool_choice payload: %+v", asObject)
	}
}

// TestOpenAICompatibleChat_ResponseFormatJSONSchema asserts that a
// json_schema ResponseFormat with Strict=true serializes to OpenAI's
// nested response_format envelope.
func TestOpenAICompatibleChat_ResponseFormatJSONSchema(t *testing.T) {
	type captured struct {
		ResponseFormat map[string]any `json:"response_format"`
	}
	var got captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"steps\":[]}"}}]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	schema := json.RawMessage(`{"type":"object","properties":{"steps":{"type":"array"}}}`)
	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "plan"}}, ChatOptions{
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, Name: "subagents_plan", Schema: schema, Strict: true},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if got.ResponseFormat["type"] != "json_schema" {
		t.Fatalf("expected response_format.type=json_schema, got %v", got.ResponseFormat["type"])
	}
	js, ok := got.ResponseFormat["json_schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected json_schema sub-object, got %T", got.ResponseFormat["json_schema"])
	}
	if js["name"] != "subagents_plan" {
		t.Fatalf("unexpected schema name: %v", js["name"])
	}
	if js["strict"] != true {
		t.Fatalf("expected strict=true, got %v", js["strict"])
	}
	if _, ok := js["schema"]; !ok {
		t.Fatalf("schema field missing from envelope: %+v", js)
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
