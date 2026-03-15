package llm

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAskFromSinglePrompt_ReturnsAssistantContent(t *testing.T) {
	t.Parallel()

	var gotMessages []ChatMessage
	got, err := askFromSinglePrompt(
		context.Background(),
		func(_ context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
			gotMessages = messages
			if !reflect.DeepEqual(opts, ChatOptions{}) {
				t.Fatalf("expected empty chat options, got %+v", opts)
			}
			return ChatResponse{
				Message: ChatMessage{
					Role:    "assistant",
					Content: "hello",
				},
			}, nil
		},
		"ping",
	)
	if err != nil {
		t.Fatalf("ask from single prompt: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected assistant content hello, got %q", got)
	}
	wantMessages := []ChatMessage{{Role: "user", Content: "ping"}}
	if !reflect.DeepEqual(gotMessages, wantMessages) {
		t.Fatalf("expected messages %+v, got %+v", wantMessages, gotMessages)
	}
}

func TestAskFromSinglePrompt_WithNormalizer_TransformsAssistantContent(t *testing.T) {
	t.Parallel()

	got, err := askFromSinglePrompt(
		context.Background(),
		func(_ context.Context, _ []ChatMessage, _ ChatOptions) (ChatResponse, error) {
			return ChatResponse{
				Message: ChatMessage{
					Role:    "assistant",
					Content: "  hello  \n",
				},
			}, nil
		},
		"ping",
		func(content string) string {
			return "trimmed:" + content[2:7]
		},
	)
	if err != nil {
		t.Fatalf("ask from single prompt: %v", err)
	}
	if got != "trimmed:hello" {
		t.Fatalf("expected normalized content, got %q", got)
	}
}

func TestAskFromSinglePrompt_PropagatesChatError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	_, err := askFromSinglePrompt(
		context.Background(),
		func(_ context.Context, _ []ChatMessage, _ ChatOptions) (ChatResponse, error) {
			return ChatResponse{}, wantErr
		},
		"ping",
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}
