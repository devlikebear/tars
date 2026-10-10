package initiative

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

// stubChatClient answers Chat with a fixed response or error, and records
// the options it was called with so tests can assert isolation.
type stubChatClient struct {
	calls   int
	resp    llm.ChatResponse
	err     error
	lastOpt llm.ChatOptions
}

func (c *stubChatClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *stubChatClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.calls++
	c.lastOpt = opts
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	return c.resp, nil
}

func contentResponse(content string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: content}, StopReason: "stop"}
}

func TestLLMTextBackendStrictBooleans(t *testing.T) {
	client := &stubChatClient{resp: contentResponse(`{"quiet_requested":true,"user_strained":false,"special_day":false}`)}
	backend := &LLMTextBackend{Client: client, Model: "haiku"}
	got, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0)))
	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	if !got.QuietRequested || got.UserStrained || got.SpecialDay {
		t.Fatalf("got %+v", got)
	}
	if got.Probabilities != nil {
		t.Fatalf("llm backend must not report probabilities, got %v", got.Probabilities)
	}
	if client.lastOpt.ToolChoice == nil || client.lastOpt.ToolChoice.Mode != llm.ToolChoiceModeNone {
		t.Fatalf("expected ToolChoiceNone, got %+v", client.lastOpt.ToolChoice)
	}
}

func TestLLMTextBackendAcceptsOneCodeFence(t *testing.T) {
	client := &stubChatClient{resp: contentResponse("```json\n{\"quiet_requested\":false,\"user_strained\":true,\"special_day\":false}\n```")}
	backend := &LLMTextBackend{Client: client}
	got, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0)))
	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	if got.QuietRequested || !got.UserStrained || got.SpecialDay {
		t.Fatalf("got %+v", got)
	}
}

func TestLLMTextBackendRejectsBrokenJSON(t *testing.T) {
	client := &stubChatClient{resp: contentResponse("not json")}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error for broken JSON")
	}
}

func TestLLMTextBackendRejectsMissingKey(t *testing.T) {
	client := &stubChatClient{resp: contentResponse(`{"quiet_requested":true,"user_strained":false}`)}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestLLMTextBackendRejectsUnknownKey(t *testing.T) {
	client := &stubChatClient{resp: contentResponse(`{"quiet_requested":true,"user_strained":false,"special_day":false,"extra":true}`)}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestLLMTextBackendRejectsNonBoolean(t *testing.T) {
	client := &stubChatClient{resp: contentResponse(`{"quiet_requested":0.8,"user_strained":false,"special_day":false}`)}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error for a probability instead of a boolean")
	}
}

func TestLLMTextBackendRejectsTrailingData(t *testing.T) {
	client := &stubChatClient{resp: contentResponse(`{"quiet_requested":true,"user_strained":false,"special_day":false}{}`)}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error for trailing data")
	}
}

func TestLLMTextBackendRejectsToolCallAttempt(t *testing.T) {
	client := &stubChatClient{resp: llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "bash"}}},
	}}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error when the model attempts a tool call")
	}
}

func TestLLMTextBackendRejectsProviderExecutedToolAttempt(t *testing.T) {
	client := &stubChatClient{resp: llm.ChatResponse{ProviderExecutedTools: []llm.ToolCall{{ID: "t1", Name: "bash"}}}}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error when a provider-executed tool is reported")
	}
}

func TestLLMTextBackendPropagatesProviderError(t *testing.T) {
	client := &stubChatClient{err: errors.New("upstream down")}
	backend := &LLMTextBackend{Client: client}
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected provider error to propagate")
	}
}

func TestLLMTextBackendNilClient(t *testing.T) {
	var backend *LLMTextBackend
	if _, err := backend.ReadText(context.Background(), "state", textQuestionsFor(at(9, 0))); err == nil {
		t.Fatal("expected error for nil backend")
	}
}

// TestTextReaderViaLLMBackend exercises the generic textReader (cache/skip
// semantics) through the llm adapter end to end, mirroring the jev-path
// coverage in textsignals_test.go.
func TestTextReaderViaLLMBackend(t *testing.T) {
	client := &stubChatClient{resp: contentResponse(`{"quiet_requested":true,"user_strained":false,"special_day":false}`)}
	r := newLLMTextReader(client, "haiku")
	got, err := r.Read(context.Background(), "k1", "state", textQuestionsFor(at(9, 0)))
	if err != nil || !got.QuietRequested || got.Source != "llm" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	again, _ := r.Read(context.Background(), "k1", "state moved", textQuestionsFor(at(9, 0)))
	if client.calls != 1 || again.Source != "cache" {
		t.Fatalf("expected cache hit, calls=%d got=%+v", client.calls, again)
	}
}

func TestTextReaderViaLLMBackendErrorIsNotCached(t *testing.T) {
	client := &stubChatClient{err: errors.New("down")}
	r := newLLMTextReader(client, "haiku")
	got, err := r.Read(context.Background(), "k", "state", textQuestionsFor(at(9, 0)))
	if err == nil || got.Source != "error" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	client.err = nil
	client.resp = contentResponse(`{"quiet_requested":false,"user_strained":false,"special_day":false}`)
	if _, err := r.Read(context.Background(), "k", "state", textQuestionsFor(at(9, 0))); err != nil || client.calls != 2 {
		t.Fatalf("expected retry after error, calls=%d err=%v", client.calls, err)
	}
}

func TestTextReaderViaLLMBackendNilClientSkips(t *testing.T) {
	r := newLLMTextReader(nil, "haiku")
	got, err := r.Read(context.Background(), "k", "state", textQuestionsFor(at(9, 0)))
	if err != nil || got.Source != "skipped" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
