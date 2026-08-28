package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// anthropicCaptureServer returns a server that records the decoded request
// body and replies with a fixed JSON response.
func anthropicCaptureServer(t *testing.T, captured *map[string]any, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
}

func TestAnthropicThinkingBudgetForEffort(t *testing.T) {
	cases := []struct {
		effort string
		want   int
	}{
		{"", 0},
		{"none", 0},
		{"minimal", 1024},
		{"low", 2048},
		{"medium", 8192},
		{"high", 16384},
	}
	for _, tc := range cases {
		if got := anthropicThinkingBudgetForEffort(tc.effort); got != tc.want {
			t.Fatalf("effort %q: got %d want %d", tc.effort, got, tc.want)
		}
	}
}

// The whole point of LP-003: an effort level set on an anthropic tier must
// change the request payload instead of being silently dropped.
func TestBuildAnthropicThinking_EffortMapsToBudget(t *testing.T) {
	cases := []struct {
		name      string
		effort    string
		maxTokens int
		want      int // 0 means "no thinking block"
	}{
		{"none disables", "none", 64000, 0},
		{"unset disables", "", 64000, 0},
		{"minimal", "minimal", 64000, 1024},
		{"low", "low", 64000, 2048},
		{"medium", "medium", 64000, 8192},
		{"high", "high", 64000, 16384},
		// max_tokens is pinned at 4096 until LP-004, so every level above
		// minimal is clamped to leave the answer its headroom.
		{"clamped to headroom at 4096", "high", 4096, 3072},
		{"minimal still fits at 4096", "minimal", 4096, 1024},
		// Below the floor there is no legal budget, so thinking degrades off
		// rather than shipping a request the provider rejects.
		{"disabled when headroom below floor", "high", 1500, 0},
		// Exactly enough room for the floor still ships thinking.
		{"floor fits exactly", "high", 2048, 1024},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAnthropicThinking(ClientConfig{}, ChatOptions{ReasoningEffort: tc.effort}, tc.maxTokens)
			if tc.want == 0 {
				if got != nil {
					t.Fatalf("expected no thinking block, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected thinking block with budget %d, got nil", tc.want)
			}
			if got["type"] != "enabled" {
				t.Fatalf("type got %v want enabled", got["type"])
			}
			if got["budget_tokens"] != tc.want {
				t.Fatalf("budget_tokens got %v want %d", got["budget_tokens"], tc.want)
			}
		})
	}
}

// An explicit thinking_budget is the more specific knob and outranks the
// effort level, but still has to fit under max_tokens.
func TestBuildAnthropicThinking_ExplicitBudgetOutranksEffort(t *testing.T) {
	got := buildAnthropicThinking(ClientConfig{}, ChatOptions{ThinkingBudget: 5000, ReasoningEffort: "minimal"}, 64000)
	if got == nil || got["budget_tokens"] != 5000 {
		t.Fatalf("explicit budget got %v want 5000", got)
	}
	// Below the provider floor it is raised, not dropped.
	got = buildAnthropicThinking(ClientConfig{}, ChatOptions{ThinkingBudget: 10}, 64000)
	if got == nil || got["budget_tokens"] != anthropicMinThinkingBudget {
		t.Fatalf("sub-floor budget got %v want %d", got, anthropicMinThinkingBudget)
	}
}

func TestBuildAnthropicThinking_ClientConfigEffortApplies(t *testing.T) {
	got := buildAnthropicThinking(ClientConfig{ReasoningEffort: "medium"}, ChatOptions{}, 64000)
	if got == nil || got["budget_tokens"] != 8192 {
		t.Fatalf("config effort got %v want 8192", got)
	}
}

func TestAnthropicChat_ReasoningEffortReachesRequestPayload(t *testing.T) {
	var captured map[string]any
	srv := anthropicCaptureServer(t, &captured, `{"content":[{"type":"text","text":"ok"}]}`)
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		ReasoningEffort: "minimal",
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	thinking, ok := captured["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("request carries no thinking block: %v", captured)
	}
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking type got %v", thinking["type"])
	}
	if thinking["budget_tokens"] != float64(1024) {
		t.Fatalf("budget_tokens got %v want 1024", thinking["budget_tokens"])
	}
}

func TestAnthropicChat_NoReasoningEffortLeavesThinkingOff(t *testing.T) {
	var captured map[string]any
	srv := anthropicCaptureServer(t, &captured, `{"content":[{"type":"text","text":"ok"}]}`)
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if _, ok := captured["thinking"]; ok {
		t.Fatalf("unexpected thinking block: %v", captured["thinking"])
	}
}

func TestParseAnthropicContentBlocks_CapturesThinkingAndRedacted(t *testing.T) {
	blocks := []anthropicContentBlock{
		{Type: "thinking", Thinking: "step one", Signature: "sig-a"},
		{Type: "redacted_thinking", Data: "opaque-payload"},
		{Type: "text", Text: "answer"},
		{Type: "tool_use", ID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"a"}`)},
	}
	content, toolCalls, reasoning := parseAnthropicContentBlocks(blocks)
	if content != "answer" {
		t.Fatalf("content got %q", content)
	}
	if len(toolCalls) != 1 || toolCalls[0].ID != "call_1" {
		t.Fatalf("tool calls got %+v", toolCalls)
	}
	if len(reasoning) != 2 {
		t.Fatalf("reasoning blocks got %+v", reasoning)
	}
	if reasoning[0] != (ReasoningBlock{Type: ReasoningBlockThinking, Text: "step one", Signature: "sig-a"}) {
		t.Fatalf("thinking block got %+v", reasoning[0])
	}
	if reasoning[1] != (ReasoningBlock{Type: ReasoningBlockRedacted, Data: "opaque-payload"}) {
		t.Fatalf("redacted block got %+v", reasoning[1])
	}
}

func TestAnthropicChat_NonStreamingSurfacesReasoningBlocks(t *testing.T) {
	var captured map[string]any
	srv := anthropicCaptureServer(t, &captured, `{"content":[
		{"type":"thinking","thinking":"weighing options","signature":"sig-1"},
		{"type":"text","text":"done"}
	]}`)
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.Message.ReasoningBlocks) != 1 {
		t.Fatalf("reasoning blocks got %+v", resp.Message.ReasoningBlocks)
	}
	if resp.Message.ReasoningBlocks[0].Signature != "sig-1" {
		t.Fatalf("signature got %q", resp.Message.ReasoningBlocks[0].Signature)
	}
	// The console reasoning stream reads ReasoningContent; non-streaming
	// responses must populate it too.
	if resp.Message.ReasoningContent != "weighing options" {
		t.Fatalf("ReasoningContent got %q", resp.Message.ReasoningContent)
	}
}

func TestAnthropicChat_StreamCapturesThinkingSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"Let me \"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"think...\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig-stream\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":1,\"content_block\":{\"type\":\"redacted_thinking\",\"data\":\"opaque\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":2,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"read_file\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"a\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var reasoning strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		OnDelta:          func(string) {},
		OnReasoningDelta: func(text string) { reasoning.WriteString(text) },
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if got, want := reasoning.String(), "Let me think..."; got != want {
		t.Fatalf("reasoning stream got %q want %q", got, want)
	}
	blocks := resp.Message.ReasoningBlocks
	if len(blocks) != 2 {
		t.Fatalf("reasoning blocks got %+v", blocks)
	}
	if blocks[0] != (ReasoningBlock{Type: ReasoningBlockThinking, Text: "Let me think...", Signature: "sig-stream"}) {
		t.Fatalf("thinking block got %+v", blocks[0])
	}
	if blocks[1] != (ReasoningBlock{Type: ReasoningBlockRedacted, Data: "opaque"}) {
		t.Fatalf("redacted block got %+v", blocks[1])
	}
	// The redacted block must not swallow the tool call's partial JSON.
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Arguments != `{"path":"a"}` {
		t.Fatalf("tool calls got %+v", resp.Message.ToolCalls)
	}
}

// Anthropic rejects an assistant turn that carries tool_use with extended
// thinking enabled unless the signed thinking blocks come back with it, in
// their original order and ahead of the tool_use blocks.
func TestToAnthropicAssistantMessage_ReEmitsThinkingAheadOfToolUse(t *testing.T) {
	msg := ChatMessage{
		Role:    "assistant",
		Content: "let me look",
		ReasoningBlocks: []ReasoningBlock{
			{Type: ReasoningBlockThinking, Text: "first", Signature: "sig-1"},
			{Type: ReasoningBlockRedacted, Data: "opaque"},
		},
		ToolCalls: []ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"a"}`}},
	}
	wire := toAnthropicAssistantMessage(msg)
	blocks, ok := wire.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content is not a block array: %T", wire.Content)
	}
	if len(blocks) != 4 {
		t.Fatalf("block count got %d want 4: %+v", len(blocks), blocks)
	}
	if blocks[0]["type"] != "thinking" || blocks[0]["thinking"] != "first" || blocks[0]["signature"] != "sig-1" {
		t.Fatalf("block 0 got %+v", blocks[0])
	}
	if blocks[1]["type"] != "redacted_thinking" || blocks[1]["data"] != "opaque" {
		t.Fatalf("block 1 got %+v", blocks[1])
	}
	if blocks[2]["type"] != "text" {
		t.Fatalf("block 2 got %+v", blocks[2])
	}
	if blocks[3]["type"] != "tool_use" {
		t.Fatalf("block 3 got %+v", blocks[3])
	}
}

// A thinking block with no signature was never signed by the provider, so
// echoing it back is an error. Drop it instead of shipping a rejected request.
func TestToAnthropicAssistantMessage_DropsUnsignedThinking(t *testing.T) {
	msg := ChatMessage{
		Role:            "assistant",
		ReasoningBlocks: []ReasoningBlock{{Type: ReasoningBlockThinking, Text: "unsigned"}},
		ToolCalls:       []ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{}`}},
	}
	wire := toAnthropicAssistantMessage(msg)
	blocks, ok := wire.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content is not a block array: %T", wire.Content)
	}
	for _, b := range blocks {
		if b["type"] == "thinking" {
			t.Fatalf("unsigned thinking block was emitted: %+v", blocks)
		}
	}
}

// Historical assistant turns without tool_use do not need their thinking
// blocks echoed — Anthropic ignores them — so the message stays a plain
// string and the transcript stays small.
func TestToAnthropicAssistantMessage_PlainTurnStaysString(t *testing.T) {
	msg := ChatMessage{
		Role:            "assistant",
		Content:         "answer",
		ReasoningBlocks: []ReasoningBlock{{Type: ReasoningBlockThinking, Text: "x", Signature: "sig"}},
	}
	wire := toAnthropicAssistantMessage(msg)
	if got, ok := wire.Content.(string); !ok || got != "answer" {
		t.Fatalf("content got %#v want plain string %q", wire.Content, "answer")
	}
}

// A full tool-loop replay: the assistant turn that requested the tool comes
// back with its signed thinking blocks intact.
func TestAnthropicChat_ToolLoopReplayCarriesThinkingBlocks(t *testing.T) {
	var captured map[string]any
	srv := anthropicCaptureServer(t, &captured, `{"content":[{"type":"text","text":"final"}]}`)
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	messages := []ChatMessage{
		{Role: "user", Content: "read a"},
		{
			Role:            "assistant",
			ReasoningBlocks: []ReasoningBlock{{Type: ReasoningBlockThinking, Text: "need the file", Signature: "sig-1"}},
			ToolCalls:       []ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"a"}`}},
		},
		{Role: "tool", ToolCallID: "call_1", Content: "contents"},
	}
	if _, err := client.Chat(context.Background(), messages, ChatOptions{ReasoningEffort: "minimal"}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	wire, _ := captured["messages"].([]any)
	if len(wire) != 3 {
		t.Fatalf("wire messages got %d: %v", len(wire), wire)
	}
	assistant, _ := wire[1].(map[string]any)
	blocks, _ := assistant["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("assistant blocks got %v", blocks)
	}
	first, _ := blocks[0].(map[string]any)
	if first["type"] != "thinking" || first["signature"] != "sig-1" {
		t.Fatalf("first block got %v", first)
	}
	last, _ := blocks[1].(map[string]any)
	if last["type"] != "tool_use" {
		t.Fatalf("last block got %v", last)
	}
}
