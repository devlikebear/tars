package session

import "time"

// Message represents a single chat message in a session transcript.
// Tool fields are optional (omitempty) for backward compatibility with existing transcripts.
type Message struct {
	ID          string    `json:"id,omitempty"`
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	ToolName    string    `json:"tool_name,omitempty"`
	ToolCallID  string    `json:"tool_call_id,omitempty"`
	ToolArgs    string    `json:"tool_args,omitempty"`
	ToolIsError bool      `json:"tool_is_error,omitempty"`
	// ReasoningBlocks preserves the provider-native reasoning blocks of an
	// assistant turn so a transcript replayed after a restart can hand them
	// back to the provider. Anthropic validates the signed thinking sequence
	// of an assistant turn that carries tool_use, so a turn rebuilt without
	// them is not the same turn.
	ReasoningBlocks []ReasoningBlock `json:"reasoning_blocks,omitempty"`
	// Interim marks assistant text said partway through a turn, before the
	// tool calls that follow it in the transcript. A turn is saved in the
	// order it streamed — text, tools, text, tools, reply — and its reply is
	// the last assistant message that is not interim. Transcripts written
	// before this field have no interim messages: all of a turn's tools, then
	// one reply holding all of its text.
	Interim bool `json:"interim,omitempty"`
}

// ReasoningBlock mirrors llm.ReasoningBlock on the transcript.
//
// It is duplicated rather than imported so the session package stays free of
// a dependency on internal/llm — a transcript is a storage format, not a
// provider type. Converters live in the server package that owns both.
type ReasoningBlock struct {
	// Type is "thinking" or "redacted_thinking".
	Type string `json:"type"`
	// Text is the visible reasoning; empty for redacted blocks.
	Text string `json:"text,omitempty"`
	// Signature authenticates Text. Opaque — store and replay verbatim.
	Signature string `json:"signature,omitempty"`
	// Data is the opaque payload of a redacted block.
	Data string `json:"data,omitempty"`
}
