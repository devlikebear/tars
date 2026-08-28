package llm

import (
	"context"
	"testing"
)

func TestFakeClientChatRecordsOptions(t *testing.T) {
	client := &FakeClient{Label: "light"}
	_, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		ReasoningEffort: "low",
		OnDelta:         func(string) {},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if client.ChatCalls != 1 {
		t.Fatalf("expected one chat call, got %d", client.ChatCalls)
	}
	if client.LastChatOptions.ReasoningEffort != "low" {
		t.Fatalf("expected recorded reasoning effort, got %q", client.LastChatOptions.ReasoningEffort)
	}
	if client.LastChatOptions.OnDelta == nil {
		t.Fatal("expected recorded OnDelta callback")
	}
}
