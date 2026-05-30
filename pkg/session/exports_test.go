package session_test

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/pkg/session"
)

func TestExportedSessionRoundTrip(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.EnsureWorker("proj-1")
	if err != nil {
		t.Fatalf("EnsureWorker: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session id")
	}
	path := store.TranscriptPath(sess.ID)
	if path == "" {
		t.Fatal("expected non-empty transcript path")
	}
	now := time.Now()
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "안녕", Timestamp: now}); err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if err := session.AppendMessage(path, session.Message{Role: "assistant", Content: "반가워요", Timestamp: now.Add(time.Second)}); err != nil {
		t.Fatalf("AppendMessage assistant: %v", err)
	}
	msgs, err := session.ReadMessages(path)
	if err != nil {
		t.Fatalf("ReadMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "안녕" {
		t.Fatalf("msg0 = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "반가워요" {
		t.Fatalf("msg1 = %+v", msgs[1])
	}
}

func TestExportedLoadHistoryTokenBudget(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.EnsureWorker("proj-2")
	if err != nil {
		t.Fatalf("EnsureWorker: %v", err)
	}
	path := store.TranscriptPath(sess.ID)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := session.AppendMessage(path, session.Message{
			Role:      "user",
			Content:   "메시지 본문이 토큰 예산 동작을 검증할 만큼 충분히 길어야 한다 " + time.Duration(i).String(),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("AppendMessage %d: %v", i, err)
		}
	}
	all, err := session.LoadHistory(path, 100000)
	if err != nil {
		t.Fatalf("LoadHistory generous: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("generous budget want 3, got %d", len(all))
	}
	few, err := session.LoadHistory(path, 1)
	if err != nil {
		t.Fatalf("LoadHistory tiny: %v", err)
	}
	if len(few) >= len(all) {
		t.Fatalf("tiny budget should truncate: got %d (all=%d)", len(few), len(all))
	}
}

func TestExportedReadMessagesMissingFile(t *testing.T) {
	msgs, err := session.ReadMessages(t.TempDir() + "/does-not-exist.jsonl")
	if err != nil {
		t.Fatalf("ReadMessages missing should not error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("want 0 messages for missing file, got %d", len(msgs))
	}
}
