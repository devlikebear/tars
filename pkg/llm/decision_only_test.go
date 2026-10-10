package llm

import "testing"

func TestDecisionOnlyChatOptions(t *testing.T) {
	opts := DecisionOnlyChatOptions()
	if opts.ToolChoice == nil || opts.ToolChoice.Mode != ToolChoiceModeNone {
		t.Fatalf("ToolChoice = %+v, want none", opts.ToolChoice)
	}
	if opts.ResponseFormat == nil || opts.ResponseFormat.Type != ResponseFormatJSONObject {
		t.Fatalf("ResponseFormat = %+v, want json_object", opts.ResponseFormat)
	}
	if opts.ClaudeCodePermissionMode != "plan" {
		t.Fatalf("ClaudeCodePermissionMode = %q, want plan", opts.ClaudeCodePermissionMode)
	}
	h := opts.ClaudeCodeHarness
	if h == nil {
		t.Fatal("ClaudeCodeHarness = nil")
	}
	if len(h.Tools) != 0 {
		t.Fatalf("ClaudeCodeHarness.Tools = %v, want empty", h.Tools)
	}
	if !h.SafeMode || !h.StrictMCP || !h.DisableChrome {
		t.Fatalf("ClaudeCodeHarness = %+v, want SafeMode/StrictMCP/DisableChrome true", h)
	}
	if h.MaxTurns != 1 {
		t.Fatalf("ClaudeCodeHarness.MaxTurns = %d, want 1", h.MaxTurns)
	}
}

func TestSupportsDecisionOnly(t *testing.T) {
	cases := []struct {
		kind string
		want bool
	}{
		{"anthropic", true},
		{"openai", true},
		{"openai-codex", true},
		{"gemini", true},
		{"gemini-native", true},
		{"claude-code-cli", true},
		{"antigravity-cli", false},
		{"", true},
	}
	for _, c := range cases {
		if got := SupportsDecisionOnly(c.kind); got != c.want {
			t.Errorf("SupportsDecisionOnly(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}

func TestAttemptedTools(t *testing.T) {
	cases := []struct {
		name string
		resp ChatResponse
		want bool
	}{
		{"none", ChatResponse{}, false},
		{"tool call", ChatResponse{Message: ChatMessage{ToolCalls: []ToolCall{{ID: "t1"}}}}, true},
		{"provider executed", ChatResponse{ProviderExecutedTools: []ToolCall{{ID: "t1"}}}, true},
	}
	for _, c := range cases {
		if got := AttemptedTools(c.resp); got != c.want {
			t.Errorf("%s: AttemptedTools = %v, want %v", c.name, got, c.want)
		}
	}
}
