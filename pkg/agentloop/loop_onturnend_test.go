package agentloop

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
)

func TestLoop_Run_OnTurnEndNoInjectStops(t *testing.T) {
	reg := tool.NewRegistry()
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", Content: "first stop"}},
		},
	}
	loop := NewLoop(client, reg)

	hookCalls := 0
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "go"},
	}, RunOptions{
		OnTurnEnd: func(_ context.Context, _ llm.ChatResponse) (string, error) {
			hookCalls++
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Message.Content != "first stop" {
		t.Fatalf("got %q want first stop", resp.Message.Content)
	}
	if hookCalls != 1 {
		t.Fatalf("hook called %d times, want 1", hookCalls)
	}
	if client.callIndex != 1 {
		t.Fatalf("expected 1 chat call, got %d", client.callIndex)
	}
}

func TestLoop_Run_OnTurnEndInjectContinues(t *testing.T) {
	reg := tool.NewRegistry()
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", Content: "round 1"}},
			{Message: llm.ChatMessage{Role: "assistant", Content: "round 2"}},
			{Message: llm.ChatMessage{Role: "assistant", Content: "round 3 final"}},
		},
	}
	loop := NewLoop(client, reg)

	calls := 0
	resp, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "start"},
	}, RunOptions{
		OnTurnEnd: func(_ context.Context, _ llm.ChatResponse) (string, error) {
			calls++
			if calls < 3 {
				return "keep going", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Message.Content != "round 3 final" {
		t.Fatalf("got %q want round 3 final", resp.Message.Content)
	}
	if client.callIndex != 3 {
		t.Fatalf("expected 3 chat calls, got %d", client.callIndex)
	}
	// Second and third chat invocations should have observed the injected
	// user-role continuation messages appended after the assistant turns.
	if len(client.seenInputs) < 3 {
		t.Fatalf("expected 3 seen inputs")
	}
	last := client.seenInputs[2]
	foundInjected := 0
	for _, m := range last {
		if m.Role == "user" && m.Content == "keep going" {
			foundInjected++
		}
	}
	if foundInjected != 2 {
		t.Fatalf("expected 2 injected user messages, got %d (msgs=%v)", foundInjected, last)
	}
}

func TestLoop_Run_OnTurnEndErrorAborts(t *testing.T) {
	reg := tool.NewRegistry()
	client := &scriptedLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}},
		},
	}
	loop := NewLoop(client, reg)

	sentinel := errors.New("judge failed")
	_, err := loop.Run(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "go"},
	}, RunOptions{
		OnTurnEnd: func(_ context.Context, _ llm.ChatResponse) (string, error) {
			return "", sentinel
		},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}
