package llm

// DecisionOnlyChatOptions returns the ChatOptions for a judgment-only call:
// one that must never execute a tool and must answer with strict JSON. This
// is the isolation computer use (tars#973) established and initiative
// (tars#1219) reuses: ToolChoiceNone, a JSON response format, and on Claude
// Code a plan permission mode with an empty harness tool list, strict MCP,
// Chrome disabled and a single turn. Callers may copy the result and set
// additional fields (e.g. Model) before calling Chat.
func DecisionOnlyChatOptions() ChatOptions {
	return ChatOptions{
		ToolChoice:               ToolChoiceNone(),
		ResponseFormat:           &ResponseFormat{Type: ResponseFormatJSONObject},
		ClaudeCodePermissionMode: "plan",
		ClaudeCodeHarness: &ClaudeCodeHarnessOptions{
			Tools:         []string{},
			SafeMode:      true,
			StrictMCP:     true,
			DisableChrome: true,
			MaxTurns:      1,
		},
	}
}

// SupportsDecisionOnly reports whether a provider kind can be forced into a
// decision-only call that never executes a tool. antigravity-cli has no flag
// that disables its own native tool use for a single call, so it is
// excluded from every role that requires this isolation (computer_use,
// initiative).
func SupportsDecisionOnly(kind string) bool {
	return kind != "antigravity-cli"
}

// AttemptedTools reports whether a decision-only response tried to use a
// tool despite ToolChoiceNone. A caller that asked for strict JSON must
// reject such a response rather than parse it.
func AttemptedTools(resp ChatResponse) bool {
	return len(resp.Message.ToolCalls) > 0 || len(resp.ProviderExecutedTools) > 0
}
