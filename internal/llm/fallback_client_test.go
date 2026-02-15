package llm

import (
	"context"
	"errors"
	"testing"
)

type stubClient struct {
	askValue   string
	askErr     error
	chatValue  ChatResponse
	chatErr    error
	askCalls   int
	chatCalls  int
	lastPrompt string
}

func (c *stubClient) Ask(_ context.Context, prompt string) (string, error) {
	c.askCalls++
	c.lastPrompt = prompt
	return c.askValue, c.askErr
}

func (c *stubClient) Chat(_ context.Context, _ []ChatMessage, _ ChatOptions) (ChatResponse, error) {
	c.chatCalls++
	return c.chatValue, c.chatErr
}

func TestFallbackClient_AskUsesFallbackOnPrimaryError(t *testing.T) {
	primary := &stubClient{
		askErr: newProviderError("primary", "request", errors.New("blocked")),
	}
	fallback := &stubClient{
		askValue: "fallback ok",
	}
	client := newFallbackClient(primary, fallback)

	got, err := client.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if got != "fallback ok" {
		t.Fatalf("unexpected ask response: %q", got)
	}
	if primary.askCalls != 1 || fallback.askCalls != 1 {
		t.Fatalf("expected ask calls 1/1, got %d/%d", primary.askCalls, fallback.askCalls)
	}
}

func TestFallbackClient_ChatUsesFallbackOnPrimaryError(t *testing.T) {
	primary := &stubClient{
		chatErr: newProviderError("primary", "request", errors.New("forbidden")),
	}
	fallback := &stubClient{
		chatValue: ChatResponse{
			Message: ChatMessage{
				Role:    "assistant",
				Content: "fallback chat",
			},
		},
	}
	client := newFallbackClient(primary, fallback)

	got, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if got.Message.Content != "fallback chat" {
		t.Fatalf("unexpected chat response: %q", got.Message.Content)
	}
	if primary.chatCalls != 1 || fallback.chatCalls != 1 {
		t.Fatalf("expected chat calls 1/1, got %d/%d", primary.chatCalls, fallback.chatCalls)
	}
}

func TestFallbackClient_DoesNotFallbackForContextCanceled(t *testing.T) {
	primary := &stubClient{
		chatErr: context.Canceled,
	}
	fallback := &stubClient{}
	client := newFallbackClient(primary, fallback)

	_, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}}, ChatOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if fallback.chatCalls != 0 {
		t.Fatalf("expected fallback not to run, got calls=%d", fallback.chatCalls)
	}
}
