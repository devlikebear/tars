package session

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
