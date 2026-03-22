package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiNativeClientChat_NonStreamingParsesToolCall(t *testing.T) {
	var captured map[string]any
	var preflightCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1beta/models/gemini-2.5-flash" {
			preflightCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]}`))
			return
		}
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:generateContent" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-key" {
			t.Fatalf("expected x-goog-api-key header, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{
				"content":{
					"role":"model",
					"parts":[
						{"text":"let me check"},
						{"functionCall":{"id":"call_2","name":"memory_search","args":{"query":"coffee"}},"thoughtSignature":"bW9kZWxfc2ln"}
					]
				},
				"finishReason":"STOP"
			}],
			"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":6}
		}`))
	}))
	defer srv.Close()

	client, err := NewGeminiNativeClient(srv.URL+"/v1beta", "gemini-key", "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	messages := []ChatMessage{
		{Role: "system", Content: "system rule"},
		{Role: "user", Content: "search memory"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "memory_search", Arguments: `{"query":"tea"}`, ThoughtSignature: "c2ln"},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    `{"items":[]}`,
		},
	}
	opts := ChatOptions{
		ReasoningEffort: "minimal",
		ThinkingBudget:  2048,
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "memory_search",
					Description: "search memory",
					Parameters: json.RawMessage(`{
						"type":"object",
						"additionalProperties": false,
						"properties":{
							"query":{"type":"string"},
							"options":{
								"type":"object",
								"additionalProperties": false,
								"properties":{"limit":{"type":"integer"}}
							}
						}
					}`),
				},
			},
		},
		ToolChoice: "required",
	}
	resp, err := client.Chat(context.Background(), messages, opts)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "memory_search" {
		t.Fatalf("unexpected tool name: %q", resp.Message.ToolCalls[0].Name)
	}
	if resp.Message.ToolCalls[0].Arguments != `{"query":"coffee"}` {
		t.Fatalf("unexpected tool arguments: %q", resp.Message.ToolCalls[0].Arguments)
	}
	if resp.Message.ToolCalls[0].ThoughtSignature != "bW9kZWxfc2ln" {
		t.Fatalf("unexpected thought signature: %q", resp.Message.ToolCalls[0].ThoughtSignature)
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("expected tool_calls stop reason, got %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 6 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}

	toolConfig, ok := captured["toolConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected toolConfig in request, got %+v", captured["toolConfig"])
	}
	fc, ok := toolConfig["functionCallingConfig"].(map[string]any)
	if !ok || fc["mode"] != "ANY" {
		t.Fatalf("expected functionCallingConfig mode ANY, got %+v", toolConfig["functionCallingConfig"])
	}
	systemInstruction, ok := captured["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("expected systemInstruction in request, got %+v", captured["systemInstruction"])
	}
	builtConfig := client.buildGenerateContentConfig(messages, opts)
	if builtConfig.ThinkingConfig == nil {
		t.Fatal("expected ThinkingConfig in generated config")
	}
	if builtConfig.ThinkingConfig.ThinkingLevel != geminiThinkingMinimal {
		t.Fatalf("expected thinkingLevel THINKING_LEVEL_MINIMAL, got %+v", builtConfig.ThinkingConfig.ThinkingLevel)
	}
	if builtConfig.ThinkingConfig.ThinkingBudget == nil || *builtConfig.ThinkingConfig.ThinkingBudget != 2048 {
		t.Fatalf("expected thinkingBudget 2048, got %+v", builtConfig.ThinkingConfig.ThinkingBudget)
	}
	parts, ok := systemInstruction["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("expected systemInstruction parts, got %+v", systemInstruction["parts"])
	}
	contents, ok := captured["contents"].([]any)
	if !ok || len(contents) < 3 {
		t.Fatalf("expected converted contents in request, got %+v", captured["contents"])
	}
	foundReplay := false
	for _, item := range contents {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msg["parts"].([]any)
		if !ok {
			continue
		}
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			functionCall, ok := part["functionCall"].(map[string]any)
			if !ok {
				continue
			}
			if functionCall["name"] != "memory_search" {
				continue
			}
			foundReplay = true
			if functionCall["id"] != "call_1" {
				t.Fatalf("expected replay call id call_1, got %+v", functionCall["id"])
			}
			if part["thoughtSignature"] != "c2ln" {
				t.Fatalf("expected replay thoughtSignature c2ln, got %+v", part["thoughtSignature"])
			}
		}
	}
	if !foundReplay {
		t.Fatalf("expected assistant functionCall replay in request contents: %+v", contents)
	}
	toolsPayload, ok := captured["tools"].([]any)
	if !ok || len(toolsPayload) == 0 {
		t.Fatalf("expected tools in request, got %+v", captured["tools"])
	}
	firstTool, ok := toolsPayload[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object, got %+v", toolsPayload[0])
	}
	functionDeclarations, ok := firstTool["functionDeclarations"].([]any)
	if !ok || len(functionDeclarations) == 0 {
		t.Fatalf("expected functionDeclarations, got %+v", firstTool["functionDeclarations"])
	}
	firstDecl, ok := functionDeclarations[0].(map[string]any)
	if !ok {
		t.Fatalf("expected function declaration object, got %+v", functionDeclarations[0])
	}
	paramsJSONSchema, ok := firstDecl["parametersJsonSchema"].(map[string]any)
	if !ok {
		t.Fatalf("expected parametersJsonSchema in request, got %+v", firstDecl)
	}
	if _, exists := paramsJSONSchema["additionalProperties"]; !exists {
		t.Fatalf("expected additionalProperties in parametersJsonSchema, got %+v", paramsJSONSchema)
	}
	if preflightCalls != 1 {
		t.Fatalf("expected exactly one preflight call, got %d", preflightCalls)
	}
}

func TestGeminiNativeClientChat_StreamingParsesDeltaAndToolCall(t *testing.T) {
	var preflightCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1beta/models/gemini-2.5-flash" {
			preflightCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]}`))
			return
		}
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:streamGenerateContent" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("alt"); got != "sse" {
			t.Fatalf("expected alt=sse, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hel\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"lo\"},{\"functionCall\":{\"name\":\"exec\",\"args\":{\"command\":\"pwd\"}}}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":9,\"candidatesTokenCount\":3}}\n\n"))
	}))
	defer srv.Close()

	client, err := NewGeminiNativeClient(srv.URL+"/v1beta", "gemini-key", "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	var streamed strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "run pwd"},
	}, ChatOptions{
		OnDelta: func(text string) {
			streamed.WriteString(text)
		},
		Tools: []ToolSchema{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:       "exec",
					Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
				},
			},
		},
		ToolChoice: "required",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if streamed.String() != "Hello" {
		t.Fatalf("unexpected streamed text: %q", streamed.String())
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.Message.ToolCalls)
	}
	if resp.Message.ToolCalls[0].Name != "exec" {
		t.Fatalf("unexpected tool name: %q", resp.Message.ToolCalls[0].Name)
	}
	if resp.Message.ToolCalls[0].Arguments != `{"command":"pwd"}` {
		t.Fatalf("unexpected tool args: %q", resp.Message.ToolCalls[0].Arguments)
	}
	if resp.StopReason != "tool_calls" {
		t.Fatalf("expected tool_calls stop reason, got %q", resp.StopReason)
	}
	if preflightCalls != 1 {
		t.Fatalf("expected exactly one preflight call, got %d", preflightCalls)
	}
}

func TestGeminiNativeClientChat_PreflightRejectsUnsupportedModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1beta/models/gemini-2.5-flash" {
			t.Fatalf("unexpected request in preflight test: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["countTokens"]}`))
	}))
	defer srv.Close()

	client, err := NewGeminiNativeClient(srv.URL+"/v1beta", "gemini-key", "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hello"},
	}, ChatOptions{})
	if err == nil {
		t.Fatal("expected preflight error for unsupported model")
	}
	if !strings.Contains(err.Error(), "does not support generateContent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsKeyRecursive(v any, key string) bool {
	switch typed := v.(type) {
	case map[string]any:
		for k, child := range typed {
			if k == key {
				return true
			}
			if containsKeyRecursive(child, key) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsKeyRecursive(item, key) {
				return true
			}
		}
	}
	return false
}
