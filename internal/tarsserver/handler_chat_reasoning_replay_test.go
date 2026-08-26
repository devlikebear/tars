package tarsserver

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// A tool loop replayed from a stored transcript must hand the assistant turn
// back with its signed thinking blocks. Without them Anthropic rejects the
// turn that a tool_result answers, so a server restart mid-conversation would
// break the next request.
func TestBuildLLMMessageHistory_ReplaysPersistedReasoningBlocks(t *testing.T) {
	now := time.Now().UTC()
	history := []session.Message{
		{Role: "user", Content: "read a", Timestamp: now},
		{
			Role:       "tool",
			Content:    "file contents",
			ToolName:   "read_file",
			ToolCallID: "call_1",
			ToolArgs:   `{"path":"a"}`,
			Timestamp:  now,
		},
		{
			Role:      "assistant",
			Content:   "here it is",
			Timestamp: now,
			ReasoningBlocks: []session.ReasoningBlock{
				{Type: "thinking", Text: "need the file", Signature: "sig-1"},
				{Type: "redacted_thinking", Data: "opaque"},
			},
		},
	}

	messages := buildLLMMessageHistory(history)
	var assistant *llm.ChatMessage
	for i := range messages {
		if messages[i].Role == "assistant" {
			assistant = &messages[i]
			break
		}
	}
	if assistant == nil {
		t.Fatalf("no assistant message rebuilt: %+v", messages)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_1" {
		t.Fatalf("tool calls got %+v", assistant.ToolCalls)
	}
	want := []llm.ReasoningBlock{
		{Type: llm.ReasoningBlockThinking, Text: "need the file", Signature: "sig-1"},
		{Type: llm.ReasoningBlockRedacted, Data: "opaque"},
	}
	if len(assistant.ReasoningBlocks) != len(want) {
		t.Fatalf("reasoning blocks got %+v want %+v", assistant.ReasoningBlocks, want)
	}
	for i := range want {
		if assistant.ReasoningBlocks[i] != want[i] {
			t.Fatalf("block %d got %+v want %+v", i, assistant.ReasoningBlocks[i], want[i])
		}
	}
}

// Transcripts written before reasoning blocks existed must keep replaying
// unchanged.
func TestBuildLLMMessageHistory_LegacyTranscriptHasNoReasoningBlocks(t *testing.T) {
	now := time.Now().UTC()
	messages := buildLLMMessageHistory([]session.Message{
		{Role: "user", Content: "hi", Timestamp: now},
		{Role: "assistant", Content: "hello", Timestamp: now},
	})
	if len(messages) != 2 {
		t.Fatalf("messages got %+v", messages)
	}
	if messages[1].ReasoningBlocks != nil {
		t.Fatalf("legacy assistant turn gained reasoning blocks: %+v", messages[1].ReasoningBlocks)
	}
}

// A redacted block must survive the transcript byte-identically: it is an
// opaque provider token and any reformatting invalidates it.
func TestSessionReasoningBlocks_RoundTripThroughTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	original := []llm.ReasoningBlock{
		{Type: llm.ReasoningBlockThinking, Text: "step one\nstep two", Signature: "c2lnLXZhbHVlPT0="},
		{Type: llm.ReasoningBlockRedacted, Data: "EroBCkYIBBgCKkDd/opaque+payload=="},
	}
	if err := session.AppendMessage(path, session.Message{
		Role:            "assistant",
		Content:         "done",
		Timestamp:       time.Now().UTC(),
		ReasoningBlocks: toSessionReasoningBlocks(original),
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var stored session.Message
	if err := json.Unmarshal(bytes.TrimSpace(raw), &stored); err != nil {
		t.Fatalf("decode transcript line: %v", err)
	}
	restored := toLLMReasoningBlocks(stored.ReasoningBlocks)
	if len(restored) != len(original) {
		t.Fatalf("restored %+v want %+v", restored, original)
	}
	for i := range original {
		if restored[i] != original[i] {
			t.Fatalf("block %d restored %+v want %+v", i, restored[i], original[i])
		}
	}
}
