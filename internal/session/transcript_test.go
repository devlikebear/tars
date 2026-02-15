package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendAndReadMessages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	msg1 := Message{
		Role:      "user",
		Content:   "hello",
		Timestamp: time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC),
	}
	msg2 := Message{
		Role:      "assistant",
		Content:   "hi there",
		Timestamp: time.Date(2026, 2, 14, 10, 0, 1, 0, time.UTC),
	}

	if err := AppendMessage(path, msg1); err != nil {
		t.Fatalf("append msg1: %v", err)
	}
	if err := AppendMessage(path, msg2); err != nil {
		t.Fatalf("append msg2: %v", err)
	}

	messages, err := ReadMessages(path)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "hello" {
		t.Fatalf("unexpected first message: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "hi there" {
		t.Fatalf("unexpected second message: %+v", messages[1])
	}
}

func TestReadMessages_EmptyOrMissing(t *testing.T) {
	// Non-existent file should return nil slice, nil error
	messages, err := ReadMessages("/tmp/does-not-exist-transcript.jsonl")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if messages != nil {
		t.Fatalf("expected nil slice for missing file, got: %v", messages)
	}

	// Empty file should return nil slice, nil error
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(emptyPath, []byte(""), 0o644); err != nil {
		t.Fatalf("create empty file: %v", err)
	}
	messages, err = ReadMessages(emptyPath)
	if err != nil {
		t.Fatalf("expected nil error for empty file, got: %v", err)
	}
	if messages != nil {
		t.Fatalf("expected nil slice for empty file, got: %v", messages)
	}
}

func TestLoadHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	// Append 10 messages, each with ~40 chars content
	for i := 0; i < 10; i++ {
		msg := Message{
			Role:      "user",
			Content:   fmt.Sprintf("message number %d with some padding text", i),
			Timestamp: time.Date(2026, 2, 14, 10, 0, i, 0, time.UTC),
		}
		if err := AppendMessage(path, msg); err != nil {
			t.Fatalf("append msg %d: %v", i, err)
		}
	}

	// Load with large budget — should get all 10
	all, err := LoadHistory(path, 100000)
	if err != nil {
		t.Fatalf("load all: %v", err)
	}
	if len(all) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(all))
	}
	// Messages should be in chronological order (oldest first)
	if all[0].Content != "message number 0 with some padding text" {
		t.Fatalf("expected first message to be oldest, got %q", all[0].Content)
	}

	// Load with tiny budget — should get fewer messages (most recent ones)
	few, err := LoadHistory(path, 20)
	if err != nil {
		t.Fatalf("load few: %v", err)
	}
	if len(few) >= 10 {
		t.Fatalf("expected fewer than 10 messages with small budget, got %d", len(few))
	}
	if len(few) == 0 {
		t.Fatal("expected at least 1 message even with small budget")
	}
	// The returned messages should be the most recent ones, in chronological order
	lastMsg := few[len(few)-1]
	if lastMsg.Content != "message number 9 with some padding text" {
		t.Fatalf("expected last message to be most recent, got %q", lastMsg.Content)
	}

	// Load from missing file — should empty result
	empty, err := LoadHistory("/tmp/no-such-file.jsonl", 100000)
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 messages for missing file, got %d", len(empty))
	}
}

func TestLoadHistory_IncludeCompactionBoundarySummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compacted.jsonl")

	summary := Message{
		Role:      "system",
		Content:   "[COMPACTION SUMMARY]\nCompacted 50 messages.",
		Timestamp: time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC),
	}
	if err := AppendMessage(path, summary); err != nil {
		t.Fatalf("append summary: %v", err)
	}
	for i := 0; i < 6; i++ {
		if err := AppendMessage(path, Message{
			Role:      "user",
			Content:   fmt.Sprintf("recent message %d %s", i, strings.Repeat("x", 48)),
			Timestamp: time.Date(2026, 2, 15, 12, 0, i+1, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append recent message %d: %v", i, err)
		}
	}

	history, err := LoadHistory(path, 24)
	if err != nil {
		t.Fatalf("load compacted history: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected summary + recent messages, got %d", len(history))
	}
	if history[0].Role != "system" || !strings.Contains(history[0].Content, "[COMPACTION SUMMARY]") {
		t.Fatalf("expected first message to be compaction summary, got %+v", history[0])
	}
	if history[len(history)-1].Content != "recent message 5 "+strings.Repeat("x", 48) {
		t.Fatalf("expected latest message kept, got %q", history[len(history)-1].Content)
	}
}

func TestLoadHistory_DoesNotDuplicateCompactionBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compacted-full.jsonl")

	if err := AppendMessage(path, Message{
		Role:      "system",
		Content:   "[COMPACTION SUMMARY]\nCompacted 20 messages.",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}
	if err := AppendMessage(path, Message{
		Role:      "assistant",
		Content:   "latest",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append latest: %v", err)
	}

	history, err := LoadHistory(path, 1000)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if strings.Count(history[0].Content, "[COMPACTION SUMMARY]") != 1 {
		t.Fatalf("expected summary message once, got %q", history[0].Content)
	}
}
