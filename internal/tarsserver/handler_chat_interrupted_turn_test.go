package tarsserver

import (
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// A turn that ends in an error with no reply text keeps every tool call it
// made, the tools TARS ran itself included. Only an upstream CLI provider's
// tools used to be kept, so a native-provider turn stopped by a loop guard
// left the user's message and nothing else.
func TestPersistInterruptedTurnKeepsNativeToolCalls(t *testing.T) {
	transcript := filepath.Join(t.TempDir(), "session.jsonl")
	state := chatRunState{transcriptPath: transcript}
	calls := []ToolCallRecord{
		{ToolName: "exec", ToolCallID: "call_1", ToolArgs: `{"command":"go build ./..."}`, ToolResult: "ok"},
		{ToolName: "edit_file", ToolCallID: "call_2", ToolArgs: `{"path":"a.go"}`, ToolResult: "boom", ToolIsError: true},
		{ToolName: "Bash", ToolCallID: "toolu_b", ToolResult: providerToolPendingResult, upstream: true},
	}

	persistInterruptedTurn(state, "do the work", llm.ChatResponse{}, calls, zerolog.Nop())

	messages, err := session.ReadMessages(transcript)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected the three tool calls on record, got %+v", messages)
	}
	for i, want := range []struct {
		id, name, content string
		isError           bool
	}{
		{"call_1", "exec", "ok", false},
		{"call_2", "edit_file", "boom", true},
		{"toolu_b", "Bash", providerToolInterruptedResult, false},
	} {
		got := messages[i]
		if got.Role != "tool" || got.ToolCallID != want.id || got.ToolName != want.name || got.Content != want.content || got.ToolIsError != want.isError {
			t.Fatalf("message %d = %+v, want %+v", i, got, want)
		}
	}
	// With no assistant reply after them the calls are for the console only:
	// the next turn's model history does not replay them.
	history := buildLLMMessageHistory(append(messages, session.Message{Role: "user", Content: "continue"}))
	for _, m := range history {
		if m.Role == "tool" || len(m.ToolCalls) > 0 {
			t.Fatalf("orphan tool calls must not reach the model, got %+v", history)
		}
	}
}
