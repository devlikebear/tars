package session

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildCompactionSummaryWithOptions_StructuredSectionsAndIdentifiers(t *testing.T) {
	messages := []Message{
		{
			Role:      "user",
			Content:   "Use $tars-github-flow and implement `/compact` for `internal/session/compaction.go` with issue #130 on branch feat/session-compaction-upgrade.",
			Timestamp: time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC),
		},
		{
			Role:      "assistant",
			Content:   "Current code already compacts through /v1/compact and `BuildCompactionSummary`; next likely edits are `internal/tarsserver/helpers_chat.go` and `internal/tarsclient/commands_session.go`.",
			Timestamp: time.Date(2026, 3, 22, 9, 1, 0, 0, time.UTC),
		},
		{
			Role:      "user",
			Content:   "Focus on decisions and open questions.",
			Timestamp: time.Date(2026, 3, 22, 9, 2, 0, 0, time.UTC),
		},
	}

	summary := BuildCompactionSummaryWithOptions(messages, CompactionSummaryOptions{
		FocusInstructions: "Focus on decisions and open questions",
	})

	for _, needle := range []string{
		"[COMPACTION SUMMARY]",
		"Requested Focus:",
		"Current Goal:",
		"Identifiers To Preserve:",
		"Recent Context:",
		"Open State:",
		"/compact",
		"internal/session/compaction.go",
		"feat/session-compaction-upgrade",
		"#130",
	} {
		if !strings.Contains(summary, needle) {
			t.Fatalf("expected summary to contain %q, got:\n%s", needle, summary)
		}
	}
}

func TestCompactTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	for i := 0; i < 12; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if err := AppendMessage(path, Message{
			Role:      role,
			Content:   fmt.Sprintf("message %02d content", i),
			Timestamp: time.Date(2026, 2, 14, 12, 0, i, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	result, err := CompactTranscript(path, 5, time.Date(2026, 2, 14, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("compact transcript: %v", err)
	}
	if !result.Compacted {
		t.Fatalf("expected compacted=true")
	}
	if result.OriginalCount != 12 {
		t.Fatalf("expected original count 12, got %d", result.OriginalCount)
	}
	if result.CompactedCount != 7 {
		t.Fatalf("expected compacted count 7, got %d", result.CompactedCount)
	}
	if result.FinalCount != 6 {
		t.Fatalf("expected final count 6, got %d", result.FinalCount)
	}

	messages, err := ReadMessages(path)
	if err != nil {
		t.Fatalf("read compacted transcript: %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("expected 6 messages after compaction, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Fatalf("expected first message role system, got %q", messages[0].Role)
	}
	if !strings.Contains(messages[0].Content, "[COMPACTION SUMMARY]") {
		t.Fatalf("expected compaction summary marker, got %q", messages[0].Content)
	}
	if messages[1].Content != "message 07 content" {
		t.Fatalf("expected first kept message to be message 07, got %q", messages[1].Content)
	}
	if messages[len(messages)-1].Content != "message 11 content" {
		t.Fatalf("expected latest message to be kept, got %q", messages[len(messages)-1].Content)
	}
}

func TestCompactTranscript_NoOpWhenSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.jsonl")
	for i := 0; i < 3; i++ {
		if err := AppendMessage(path, Message{
			Role:      "user",
			Content:   fmt.Sprintf("small %d", i),
			Timestamp: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	result, err := CompactTranscript(path, 5, time.Now().UTC())
	if err != nil {
		t.Fatalf("compact small transcript: %v", err)
	}
	if result.Compacted {
		t.Fatalf("expected no compaction on small transcript")
	}
}

func TestCompactTranscript_WithTokenBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "budget.jsonl")

	for i := 0; i < 10; i++ {
		if err := AppendMessage(path, Message{
			Role:      "user",
			Content:   fmt.Sprintf("token budget message %d %s", i, strings.Repeat("x", 80)),
			Timestamp: time.Date(2026, 2, 14, 12, 0, i, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	result, err := CompactTranscriptWithOptions(path, 50, time.Now().UTC(), CompactOptions{
		KeepRecentTokens: 45,
	})
	if err != nil {
		t.Fatalf("compact with token budget: %v", err)
	}
	if !result.Compacted {
		t.Fatalf("expected compaction with token budget")
	}

	msgs, err := ReadMessages(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected summary + 2 recent messages, got %d", len(msgs))
	}
	if msgs[1].Content != "token budget message 8 "+strings.Repeat("x", 80) {
		t.Fatalf("unexpected first kept message: %q", msgs[1].Content)
	}
}

func TestCutoffIndexByRecentTokenShare_PrefersUserBoundary(t *testing.T) {
	messages := make([]Message, 0, 30)
	for i := 0; i < 30; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		messages = append(messages, Message{
			Role:      role,
			Content:   fmt.Sprintf("message %02d %s", i, strings.Repeat("x", 80)),
			Timestamp: time.Date(2026, 3, 22, 9, 0, i, 0, time.UTC),
		})
	}

	cutoff := cutoffIndexByRecentTokenShare(messages, 0.30, 1, 5)
	if cutoff != 22 {
		t.Fatalf("expected cutoff at next user boundary 22, got %d", cutoff)
	}
}

func TestCompactTranscriptWithOptions_KeepsToolBlocksBehindUserBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-block.jsonl")
	messages := []Message{
		{Role: "user", Content: "request one", Timestamp: time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)},
		{Role: "assistant", Content: "checking files", Timestamp: time.Date(2026, 3, 22, 9, 0, 1, 0, time.UTC)},
		{Role: "tool", Content: "file list", ToolName: "list_dir", ToolCallID: "call_1", Timestamp: time.Date(2026, 3, 22, 9, 0, 2, 0, time.UTC)},
		{Role: "assistant", Content: "here is the summary", Timestamp: time.Date(2026, 3, 22, 9, 0, 3, 0, time.UTC)},
		{Role: "user", Content: "request two", Timestamp: time.Date(2026, 3, 22, 9, 0, 4, 0, time.UTC)},
		{Role: "assistant", Content: "final answer", Timestamp: time.Date(2026, 3, 22, 9, 0, 5, 0, time.UTC)},
		{Role: "user", Content: "request three", Timestamp: time.Date(2026, 3, 22, 9, 0, 6, 0, time.UTC)},
		{Role: "assistant", Content: "follow-up", Timestamp: time.Date(2026, 3, 22, 9, 0, 7, 0, time.UTC)},
		{Role: "user", Content: "request four", Timestamp: time.Date(2026, 3, 22, 9, 0, 8, 0, time.UTC)},
		{Role: "assistant", Content: "done", Timestamp: time.Date(2026, 3, 22, 9, 0, 9, 0, time.UTC)},
	}
	for _, msg := range messages {
		if err := AppendMessage(path, msg); err != nil {
			t.Fatalf("append message: %v", err)
		}
	}

	result, err := CompactTranscriptWithOptions(path, 5, time.Now().UTC(), CompactOptions{
		KeepRecentTokens:   5,
		KeepRecentFraction: 0.20,
	})
	if err != nil {
		t.Fatalf("compact transcript: %v", err)
	}
	if !result.Compacted {
		t.Fatalf("expected compaction to happen")
	}

	compacted, err := ReadMessages(path)
	if err != nil {
		t.Fatalf("read compacted transcript: %v", err)
	}
	if len(compacted) < 3 {
		t.Fatalf("expected summary + latest user turn, got %+v", compacted)
	}
	if compacted[1].Role != "user" {
		t.Fatalf("expected tail to restart at next user boundary, got %+v", compacted)
	}
	for _, msg := range compacted[1:] {
		if msg.Role == "tool" && msg.ToolCallID == "call_1" {
			t.Fatalf("expected old tool block to be compacted away, got %+v", compacted)
		}
	}
}
