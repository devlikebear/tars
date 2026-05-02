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

func TestOpenAICompatibleChat_KimiSkipsReasoningAndServiceTierWithTools(t *testing.T) {
	type captured struct {
		ReasoningEffort string `json:"reasoning_effort"`
		ServiceTier     string `json:"service_tier"`
		Tools           []any  `json:"tools"`
	}

	var got captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	client := &OpenAICompatibleClient{
		label:      "kimi",
		baseURL:    srv.URL + "/v1",
		apiKey:     "kimi-key",
		model:      "moonshot-v1-auto",
		config:     DefaultClientConfig(),
		httpClient: http.DefaultClient,
	}

	_, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "check"}}, ChatOptions{
		ReasoningEffort: "high",
		ServiceTier:     "priority",
		Tools: []ToolSchema{
			{Type: "function", Function: ToolFunctionSchema{Name: "current_time", Parameters: json.RawMessage(`{"type":"object","properties":{"noop":{"type":"string"}}}`)}},
		},
		ToolChoice: ToolChoiceRequired(),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if got.ReasoningEffort != "" {
		t.Fatalf("expected reasoning_effort to be omitted for kimi tool call, got %q", got.ReasoningEffort)
	}
	if got.ServiceTier != "" {
		t.Fatalf("expected service_tier to be omitted for kimi tool call, got %q", got.ServiceTier)
	}
	if len(got.Tools) != 1 {
		t.Fatalf("expected tool definition to be sent, got %+v", got.Tools)
	}
}

func TestOpenAICompatibleChat_KimiRequestIncludesReasoningContentForAssistantToolCallMessage(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role             string `json:"role"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
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

	client := &OpenAICompatibleClient{
		label:      "kimi",
		baseURL:    srv.URL + "/v1",
		apiKey:     "kimi-key",
		model:      "moonshot-v1-auto",
		config:     DefaultClientConfig(),
		httpClient: http.DefaultClient,
	}

	_, err := client.Chat(context.Background(), []ChatMessage{
		{
			Role:             "assistant",
			ReasoningContent: "preparing tool call",
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "memory_search", Arguments: `{"query":"coffee"}`},
			},
		},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if len(captured.Messages) == 0 {
		t.Fatalf("expected messages in request, got none")
	}
	if captured.Messages[0].Role != "assistant" {
		t.Fatalf("unexpected first role: %q", captured.Messages[0].Role)
	}
	if captured.Messages[0].ReasoningContent != "preparing tool call" {
		t.Fatalf("expected reasoning_content to be forwarded, got %q", captured.Messages[0].ReasoningContent)
	}
	if len(captured.Messages[0].ToolCalls) != 1 {
		t.Fatalf("expected one tool_call in request, got %+v", captured.Messages[0].ToolCalls)
	}
	if captured.Messages[0].ToolCalls[0].Function.Name != "memory_search" {
		t.Fatalf("unexpected tool name in request: %q", captured.Messages[0].ToolCalls[0].Function.Name)
	}
}

func TestOpenAICompatibleChat_KimiFiltersToolMessagesWithoutMatchingAssistantToolCalls(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			Reasoning  string `json:"reasoning_content"`
			Content    string `json:"content"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
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

	client := &OpenAICompatibleClient{
		label:      "kimi",
		baseURL:    srv.URL + "/v1",
		apiKey:     "kimi-key",
		model:      "kimi-k2.6",
		config:     DefaultClientConfig(),
		httpClient: http.DefaultClient,
	}

	_, err := client.Chat(context.Background(), []ChatMessage{
		{
			Role:    "assistant",
			Content: "already finished previously",
		},
		{
			Role:       "tool",
			ToolCallID: "session:0",
			Content:    `{"result":"ok"}`,
		},
		{
			Role:    "user",
			Content: "다시 확인해줘",
		},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if len(captured.Messages) != 2 {
		t.Fatalf("expected orphan tool message to be filtered, got %d messages", len(captured.Messages))
	}
	if captured.Messages[0].Role != "assistant" {
		t.Fatalf("unexpected first role: %q", captured.Messages[0].Role)
	}
	if captured.Messages[1].Role != "user" {
		t.Fatalf("unexpected second role: %q", captured.Messages[1].Role)
	}
}

func TestOpenAICompatibleChat_KimiFiltersStaleToolMessagesAndKeepsLatestForDuplicatedToolCallID(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			Content    string `json:"content"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
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

	client := &OpenAICompatibleClient{
		label:      "kimi",
		baseURL:    srv.URL + "/v1",
		apiKey:     "kimi-key",
		model:      "kimi-k2.6",
		config:     DefaultClientConfig(),
		httpClient: http.DefaultClient,
	}

	_, err := client.Chat(context.Background(), []ChatMessage{
		{
			Role:    "assistant",
			Content: "old assistant message",
			ToolCalls: []ToolCall{
				{ID: "cron:1", Name: "cron", Arguments: `{"action":"list"}`},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "cron:1",
			Content:    `{"old":"result"}`,
		},
		{
			Role:    "assistant",
			Content: "intermediate",
			ToolCalls: []ToolCall{
				{ID: "cron:1", Name: "cron", Arguments: `{"action":"list"}`},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "cron:1",
			Content:    `{"new":"result"}`,
		},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if len(captured.Messages) != 4 {
		t.Fatalf("expected 4 messages in request, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "assistant" || captured.Messages[0].Content != "old assistant message" {
		t.Fatalf("unexpected first message: %+v", captured.Messages[0])
	}
	if captured.Messages[1].Role != "tool" || captured.Messages[1].ToolCallID != "cron:1" {
		t.Fatalf("expected old cron tool message to be dropped, got %+v", captured.Messages[1])
	}
	if captured.Messages[1].Content != `{"new":"result"}` {
		t.Fatalf("expected only latest cron tool result to remain, got content %q", captured.Messages[1].Content)
	}
	if captured.Messages[2].Role != "assistant" || captured.Messages[2].ToolCalls[0].ID != "cron:1" {
		t.Fatalf("expected second assistant tool call message, got %+v", captured.Messages[2])
	}
	if captured.Messages[3].Role != "tool" || captured.Messages[3].ToolCallID != "cron:1" {
		t.Fatalf("expected latest tool message to be present, got %+v", captured.Messages[3])
	}
}

func TestOpenAICompatibleChat_KimiTrimsToolCallIDsForWirePayload(t *testing.T) {
	var captured struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
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

	client := &OpenAICompatibleClient{
		label:      "kimi",
		baseURL:    srv.URL + "/v1",
		apiKey:     "kimi-key",
		model:      "kimi-k2.6",
		config:     DefaultClientConfig(),
		httpClient: http.DefaultClient,
	}

	_, err := client.Chat(context.Background(), []ChatMessage{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: " cron:1 ", Name: "cron", Arguments: `{"action":"list"}`},
			},
		},
		{
			Role:       "tool",
			ToolCallID: " cron:1 ",
			Content:    `{"result":"ok"}`,
		},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if len(captured.Messages) != 2 {
		t.Fatalf("expected 2 messages in request, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "assistant" || len(captured.Messages[0].ToolCalls) != 1 {
		t.Fatalf("expected assistant toolcall message, got %+v", captured.Messages[0])
	}
	if captured.Messages[0].ToolCalls[0].ID != "cron:1" {
		t.Fatalf("expected trimmed toolcall id, got %q", captured.Messages[0].ToolCalls[0].ID)
	}
	if captured.Messages[1].Role != "tool" || captured.Messages[1].ToolCallID != "cron:1" {
		t.Fatalf("expected trimmed tool_call_id, got %+v", captured.Messages[1])
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

// TestOpenAICompatibleChat_PDFUnsupportedError covers RF-046 — Chat
// Completions does not accept PDF document blocks, so the build path now
// returns a structured error instead of silently inserting placeholder
// text the model would treat as throwaway.
func TestOpenAICompatibleChat_PDFUnsupportedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatalf("server should not be reached")
		_ = w
	}))
	defer srv.Close()

	client, err := NewOpenAIClient(srv.URL+"/v1", "k", "m")
	if err != nil {
		t.Fatalf("new client: %v", err)
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
