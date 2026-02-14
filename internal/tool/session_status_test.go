package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSessionStatusTool(t *testing.T) {
	tl := NewSessionStatusTool(func(_ context.Context) (SessionStatus, error) {
		return SessionStatus{
			SessionID:       "sess-1",
			HistoryMessages: 3,
		}, nil
	})

	result, err := tl.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected one content block, got %d", len(result.Content))
	}

	var parsed SessionStatus
	if err := json.Unmarshal([]byte(result.Content[0].Text), &parsed); err != nil {
		t.Fatalf("parse result text: %v", err)
	}
	if parsed.SessionID != "sess-1" {
		t.Fatalf("unexpected session id: %q", parsed.SessionID)
	}
	if parsed.HistoryMessages != 3 {
		t.Fatalf("unexpected history message count: %d", parsed.HistoryMessages)
	}
}
