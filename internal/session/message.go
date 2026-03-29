package session

import "time"

// Message represents a single chat message in a session transcript.
// Tool fields are optional (omitempty) for backward compatibility with existing transcripts.
type Message struct {
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	ToolName   string    `json:"tool_name,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	ToolArgs   string    `json:"tool_args,omitempty"`
}
