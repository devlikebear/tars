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
		ToolChoice:     ToolChoiceRequired(),
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
	// The requested budget equals max_tokens, which the provider rejects —
	// budget_tokens must sit strictly below it and leave room for the answer.
	// buildAnthropicThinking clamps it to max_tokens minus the output
	// headroom instead of shipping the invalid pair.
	if thinkingRaw["budget_tokens"] != float64(4096-anthropicThinkingOutputHeadroom) {
		t.Fatalf("expected thinking.budget_tokens clamped to %d, got %+v", 4096-anthropicThinkingOutputHeadroom, thinkingRaw)
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

func TestAnthropicChat_StreamThinkingDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"message\":{\"usage\":{\"input_tokens\":7}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"Let me \"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"think...\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client, err := NewAnthropicClient(srv.URL, "k", "claude-3-5-haiku-latest", 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var content, reasoning strings.Builder
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{
		OnDelta:          func(text string) { content.WriteString(text) },
		OnReasoningDelta: func(text string) { reasoning.WriteString(text) },
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if got, want := content.String(), "hello"; got != want {
		t.Fatalf("content stream got %q want %q", got, want)
	}
	if got, want := reasoning.String(), "Let me think..."; got != want {
		t.Fatalf("reasoning stream got %q want %q", got, want)
	}
	if resp.Message.ReasoningContent != "Let me think..." {
		t.Fatalf("ReasoningContent buffer got %q", resp.Message.ReasoningContent)
	}
	if resp.Message.Content != "hello" {
		t.Fatalf("Content got %q", resp.Message.Content)
	}
}

// A system prompt split into a stable head and a volatile tail must keep the
// cache breakpoint on the head. Marking the tail instead writes a fresh cache
// entry every turn and reads none — the LP-001 failure mode.
func TestToAnthropicSystemBlocks_BreakpointStaysOnStablePrefix(t *testing.T) {
	blocks := toAnthropicSystemBlocks([]string{"static prefix", "## Current Time\n\n2026-08-22T10:23:00Z"})
	if len(blocks) != 2 {
		t.Fatalf("expected one block per system message, got %d", len(blocks))
	}
	if blocks[0]["text"] != "static prefix" {
		t.Fatalf("expected stable prefix first, got %+v", blocks[0]["text"])
	}
	if _, ok := blocks[0]["cache_control"]; !ok {
		t.Fatal("expected cache_control on the stable prefix block")
	}
	if _, ok := blocks[1]["cache_control"]; ok {
		t.Fatal("expected no cache_control on the volatile tail block")
	}
}

func TestToAnthropicSystemBlocks_SingleMessageKeepsWholePromptCached(t *testing.T) {
	blocks := toAnthropicSystemBlocks([]string{"only block"})
	if len(blocks) != 1 {
		t.Fatalf("expected a single block, got %d", len(blocks))
	}
	if _, ok := blocks[0]["cache_control"]; !ok {
		t.Fatal("expected cache_control on the only system block")
	}
}

// --- LP-002 rolling message breakpoints ---

func userMsg(content string) ChatMessage   { return ChatMessage{Role: "user", Content: content} }
func assistMsg(content string) ChatMessage { return ChatMessage{Role: "assistant", Content: content} }

// wireCacheMarkedIndexes returns indexes of wire messages whose LAST content
// block carries cache_control.
func wireCacheMarkedIndexes(t *testing.T, wire []anthropicWireMessage) []int {
	t.Helper()
	marked := make([]int, 0)
	for i, msg := range wire {
		if anthropicWireMessageMarked(msg) {
			marked = append(marked, i)
		}
	}
	return marked
}

func anthropicWireMessageMarked(msg anthropicWireMessage) bool {
	blocks, ok := msg.Content.([]map[string]any)
	if !ok || len(blocks) == 0 {
		return false
	}
	_, marked := blocks[len(blocks)-1]["cache_control"]
	return marked
}

func TestAnthropicCompletedTurnEndIndexes(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		want     []int
	}{
		{"empty", nil, nil},
		{"single incoming user message", []ChatMessage{userMsg("hi")}, nil},
		{
			"one completed turn plus incoming",
			[]ChatMessage{userMsg("q1"), assistMsg("r1"), userMsg("q2")},
			[]int{1},
		},
		{
			"three completed turns plus incoming",
			[]ChatMessage{userMsg("q1"), assistMsg("r1"), userMsg("q2"), assistMsg("r2"), userMsg("q3"), assistMsg("r3"), userMsg("q4")},
			[]int{1, 3, 5},
		},
		{
			"tool loop tail stays in-flight",
			[]ChatMessage{
				userMsg("q1"), assistMsg("r1"),
				userMsg("q2"),
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "exec", Arguments: "{}"}}},
				{Role: "tool", ToolCallID: "c1", Content: "out"},
			},
			[]int{1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anthropicCompletedTurnEndIndexes(tt.messages)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v want %v", got, tt.want)
				}
			}
		})
	}
}

func TestApplyAnthropicRollingCacheBreakpoints_EmptyHistory(t *testing.T) {
	wire := toAnthropicWireMessages(nil)
	applyAnthropicRollingCacheBreakpoints(wire, nil, 2)
	if len(wire) != 0 {
		t.Fatalf("expected empty wire messages, got %d", len(wire))
	}
}

// The index-alignment guard is the only thing keeping a marker off an
// unrelated message if the wire conversion ever stops being 1:1.
func TestApplyAnthropicRollingCacheBreakpoints_LengthMismatchMarksNothing(t *testing.T) {
	messages := []ChatMessage{userMsg("q1"), assistMsg("r1"), userMsg("q2")}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages[:2], 0)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 0 {
		t.Fatalf("mismatched lengths must mark nothing, got %v", marked)
	}
}

// anthropicMessageCacheBudget is what keeps the request under the provider's
// total breakpoint limit, so every reservation level needs to be pinned.
func TestAnthropicMessageCacheBudget(t *testing.T) {
	tests := []struct {
		reserved int
		want     int
	}{
		{0, 2},
		{1, 2},
		{2, 2},
		{3, 1},
		{4, 0},
		{5, 0},
	}
	for _, tt := range tests {
		if got := anthropicMessageCacheBudget(tt.reserved); got != tt.want {
			t.Fatalf("reserved=%d: got budget %d want %d", tt.reserved, got, tt.want)
		}
		if total := tt.reserved + anthropicMessageCacheBudget(tt.reserved); tt.reserved <= anthropicMaxCacheBreakpoints && total > anthropicMaxCacheBreakpoints {
			t.Fatalf("reserved=%d: total %d exceeds provider limit %d", tt.reserved, total, anthropicMaxCacheBreakpoints)
		}
	}
}

// A heavily reserved request must spend fewer slots on messages rather than
// blow past the provider limit.
func TestApplyAnthropicRollingCacheBreakpoints_ReservedSlotsShrinkBudget(t *testing.T) {
	messages := []ChatMessage{
		userMsg("q1"), assistMsg("r1"),
		userMsg("q2"), assistMsg("r2"),
		userMsg("q3"), assistMsg("r3"),
		userMsg("q4"),
	}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 3)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 1 || marked[0] != 5 {
		t.Fatalf("expected a single breakpoint on the newest completed turn, got %v", marked)
	}
}

// When the newest completed turn cannot carry a marker, the slot must fall
// back to an older markable turn instead of being forfeited.
func TestApplyAnthropicRollingCacheBreakpoints_FallsBackWhenNewestTurnUnmarkable(t *testing.T) {
	messages := []ChatMessage{
		userMsg("q1"), assistMsg("r1"),
		userMsg("q2"), assistMsg("r2"),
		userMsg("q3"), assistMsg(""), // unmarkable: no content to hang cache_control on
		userMsg("q4"),
	}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 2)
	marked := wireCacheMarkedIndexes(t, wire)
	if len(marked) != 2 || marked[0] != 1 || marked[1] != 3 {
		t.Fatalf("expected the budget to fall back to older markable turns, got %v", marked)
	}
}

// The point of the second marker is that the previous turn's newest
// breakpoint — already warm — is retained as the fallback on the next turn.
func TestApplyAnthropicRollingCacheBreakpoints_WindowRollsAcrossTurns(t *testing.T) {
	markersFor := func(messages []ChatMessage) []int {
		wire := toAnthropicWireMessages(messages)
		applyAnthropicRollingCacheBreakpoints(wire, messages, 2)
		return wireCacheMarkedIndexes(t, wire)
	}

	turnA := []ChatMessage{userMsg("q1"), assistMsg("r1"), userMsg("q2"), assistMsg("r2"), userMsg("q3")}
	turnB := append(append([]ChatMessage{}, turnA...), assistMsg("r3"), userMsg("q4"))

	markedA := markersFor(turnA)
	markedB := markersFor(turnB)
	if len(markedA) != 2 || len(markedB) != 2 {
		t.Fatalf("expected two markers on both turns, got %v and %v", markedA, markedB)
	}
	if markedB[0] != markedA[1] {
		t.Fatalf("turn B's fallback (%d) must reuse turn A's newest breakpoint (%d) so it is already warm", markedB[0], markedA[1])
	}
	if markedB[1] <= markedA[1] {
		t.Fatalf("turn B's newest breakpoint (%d) must advance past turn A's (%d)", markedB[1], markedA[1])
	}
}

func TestApplyAnthropicRollingCacheBreakpoints_SingleIncomingMessage(t *testing.T) {
	messages := []ChatMessage{userMsg("hi")}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 0)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 0 {
		t.Fatalf("expected no breakpoints on bare history, got %v", marked)
	}
}

func TestApplyAnthropicRollingCacheBreakpoints_ShortHistoryMarksLastCompletedTurn(t *testing.T) {
	messages := []ChatMessage{userMsg("q1"), assistMsg("r1"), userMsg("q2")}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 0)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 1 || marked[0] != 1 {
		t.Fatalf("expected single breakpoint on the previous assistant reply, got %v", marked)
	}
	if !anthropicWireMessageMarked(wire[1]) {
		t.Fatal("expected breakpoint on message index 1")
	}
}

func TestApplyAnthropicRollingCacheBreakpoints_LongHistoryUsesRollingWindow(t *testing.T) {
	messages := []ChatMessage{
		userMsg("q1"), assistMsg("r1"),
		userMsg("q2"), assistMsg("r2"),
		userMsg("q3"), assistMsg("r3"),
		userMsg("q4"), assistMsg("r4"),
		userMsg("q5"),
	}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 2)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 2 || marked[0] != 5 || marked[1] != 7 {
		t.Fatalf("expected rolling breakpoints on the two newest completed turns, got %v", marked)
	}
	if anthropicWireMessageMarked(wire[8]) {
		t.Fatal("the in-flight turn must not be marked")
	}
}

func TestApplyAnthropicRollingCacheBreakpoints_MidToolLoopKeepsStablePlacement(t *testing.T) {
	messages := []ChatMessage{
		userMsg("q1"), assistMsg("r1"),
		userMsg("q2"), assistMsg("r2"),
		userMsg("q3"),
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "exec", Arguments: "{}"}}},
		{Role: "tool", ToolCallID: "c1", Content: "out"},
	}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 2)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 2 || marked[0] != 1 || marked[1] != 3 {
		t.Fatalf("expected breakpoints frozen on completed turns, got %v", marked)
	}
	for _, idx := range []int{4, 5, 6} {
		if anthropicWireMessageMarked(wire[idx]) {
			t.Fatalf("message %d must stay unmarked during the tool loop", idx)
		}
	}
}

// A breakpoint may sit ON a tool_result message (both halves of the exchange
// land inside the cached prefix), but never between an assistant tool_use and
// its matching result.
func TestApplyAnthropicRollingCacheBreakpoints_TurnMayEndOnCompleteToolPair(t *testing.T) {
	messages := []ChatMessage{
		userMsg("q1"),
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "exec", Arguments: "{}"}}},
		{Role: "tool", ToolCallID: "c1", Content: "out"},
		userMsg("q2"),
	}
	wire := toAnthropicWireMessages(messages)
	applyAnthropicRollingCacheBreakpoints(wire, messages, 0)
	if marked := wireCacheMarkedIndexes(t, wire); len(marked) != 1 || marked[0] != 2 {
		t.Fatalf("expected the breakpoint on the completed tool_result message, got %v", marked)
	}
	resultBlock, ok := wire[2].Content.([]map[string]any)
	if !ok || resultBlock[0]["type"] != "tool_result" {
		t.Fatalf("unexpected wire content at index 2: %+v", wire[2].Content)
	}
	if _, ok := resultBlock[0]["cache_control"]; !ok {
		t.Fatalf("expected cache_control on the tool_result block, got %+v", resultBlock[0])
	}
}

func TestMarkAnthropicCacheBreakpoint_RefusesUnmatchedToolUse(t *testing.T) {
	msg := &anthropicWireMessage{
		Role: "assistant",
		Content: []map[string]any{
			{"type": "tool_use", "id": "c1", "name": "exec", "input": map[string]any{}},
		},
	}
	markAnthropicCacheBreakpoint(msg)
	blocks := msg.Content.([]map[string]any)
	if _, ok := blocks[len(blocks)-1]["cache_control"]; ok {
		t.Fatal("must never place a breakpoint between tool_use and its tool_result")
	}
}

func TestMarkAnthropicCacheBreakpoint_SkipsBlankStringContent(t *testing.T) {
	msg := &anthropicWireMessage{Role: "assistant", Content: ""}
	markAnthropicCacheBreakpoint(msg)
	if text, ok := msg.Content.(string); !ok || text != "" {
		t.Fatalf("blank string content must be left untouched, got %#v", msg.Content)
	}
}

func TestAnthropicChat_TotalCacheBreakpointsWithinProviderLimit(t *testing.T) {
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
	messages := []ChatMessage{
		{Role: "system", Content: "stable body"},
		userMsg("q1"), assistMsg("r1"),
		userMsg("q2"), assistMsg("r2"),
		userMsg("q3"), assistMsg("r3"),
		userMsg("q4"), assistMsg("r4"),
		userMsg("q5"),
	}
	_, err = client.Chat(context.Background(), messages, ChatOptions{
		Tools: []ToolSchema{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name:        "memory_search",
				Description: "search memory",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	countCacheControl := func(value any) int {
		count := 0
		var walk func(any)
		walk = func(v any) {
			switch typed := v.(type) {
			case map[string]any:
				if _, ok := typed["cache_control"]; ok {
					count++
				}
				for _, child := range typed {
					walk(child)
				}
			case []any:
				for _, child := range typed {
					walk(child)
				}
			}
		}
		walk(value)
		return count
	}

	systemRaw, _ := captured["system"].([]any)
	toolsRaw, _ := captured["tools"].([]any)
	messagesRaw, _ := captured["messages"].([]any)
	total := countCacheControl(systemRaw) + countCacheControl(toolsRaw) + countCacheControl(messagesRaw)
	if total > 4 {
		t.Fatalf("provider allows at most 4 breakpoints, request carried %d", total)
	}
	if total != 4 {
		t.Fatalf("expected system + tools + two rolling message markers = 4, got %d", total)
	}
}

// Turn N+1's newest message-level breakpoint must cover the whole transcript
// through turn N — everything before the incoming user message becomes one
// cacheable prefix, and its coverage grows as the conversation grows.
func TestAnthropicChat_CacheablePrefixGrowsWithConversation(t *testing.T) {
	newestMarkedIndex := func(t *testing.T, history []ChatMessage) int {
		t.Helper()
		reqBody := buildTestChatRequest(t, history)
		messagesRaw, ok := reqBody["messages"].([]anthropicWireMessage)
		if !ok {
			t.Fatalf("expected wire messages, got %+v", reqBody["messages"])
		}
		marked := wireCacheMarkedIndexes(t, messagesRaw)
		if len(marked) == 0 {
			t.Fatalf("expected message breakpoints for history of %d messages", len(history))
		}
		return marked[len(marked)-1]
	}

	base := []ChatMessage{userMsg("q1"), assistMsg("r1"), userMsg("q2")}
	turnTwoNewest := newestMarkedIndex(t, base)

	longer := append(append([]ChatMessage{}, base...), assistMsg("r2"), userMsg("q3"))
	turnThreeNewest := newestMarkedIndex(t, longer)

	if turnThreeNewest <= turnTwoNewest {
		t.Fatalf("cacheable prefix must grow with conversation length: turn2=%d turn3=%d", turnTwoNewest, turnThreeNewest)
	}
	incoming := len(longer) - 1
	if turnThreeNewest >= incoming {
		t.Fatalf("newest breakpoint must not cover the incoming turn: newest=%d incoming=%d", turnThreeNewest, incoming)
	}
}

func buildTestChatRequest(t *testing.T, history []ChatMessage) map[string]any {
	t.Helper()
	client, err := newAnthropicClientWithConfig("https://api.anthropic.com", "k", "claude-3-5-haiku-latest", DefaultClientConfig())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client.buildChatRequest(history, ChatOptions{}, false)
}

func TestAnthropicChat_EmitsSystemTailOutsideCachedPrefix(t *testing.T) {
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
		{Role: "system", Content: "stable body"},
		{Role: "system", Content: "## Current Time\n\nCurrent time: 2026-08-22T10:23:00Z"},
		{Role: "user", Content: "hi"},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	systemRaw, ok := captured["system"].([]any)
	if !ok || len(systemRaw) != 2 {
		t.Fatalf("expected two system blocks, got %+v", captured["system"])
	}
	head, ok := systemRaw[0].(map[string]any)
	if !ok || head["text"] != "stable body" {
		t.Fatalf("unexpected head block: %+v", systemRaw[0])
	}
	if _, ok := head["cache_control"]; !ok {
		t.Fatalf("expected cache_control on head block, got %+v", head)
	}
	tail, ok := systemRaw[1].(map[string]any)
	if !ok {
		t.Fatalf("invalid tail block: %+v", systemRaw[1])
	}
	tailText, _ := tail["text"].(string)
	if !strings.Contains(tailText, "Current time:") {
		t.Fatalf("expected the clock in the tail block, got %+v", tail["text"])
	}
	if _, ok := tail["cache_control"]; ok {
		t.Fatalf("expected no cache_control on tail block, got %+v", tail)
	}
}
