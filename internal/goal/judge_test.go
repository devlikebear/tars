package goal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

func TestParseVerdict_PlainJSON(t *testing.T) {
	v, err := ParseVerdict(`{"satisfied": true, "reason": " all green "}`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !v.Satisfied {
		t.Fatal("expected satisfied=true")
	}
	if v.Reason != "all green" {
		t.Fatalf("reason mismatch: %q", v.Reason)
	}
}

func TestParseVerdict_CodeFence(t *testing.T) {
	v, err := ParseVerdict("```json\n{\"satisfied\": false, \"reason\": \"tests still failing\"}\n```")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.Satisfied {
		t.Fatal("expected satisfied=false")
	}
}

func TestParseVerdict_PreambleAndTrailingNoise(t *testing.T) {
	v, err := ParseVerdict("Here is the JSON:\n{\"satisfied\": false, \"reason\": \"x\"}\nThanks!")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.Satisfied {
		t.Fatal("expected satisfied=false")
	}
}

func TestParseVerdict_Empty(t *testing.T) {
	if _, err := ParseVerdict("   "); err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestParseVerdict_NoJSON(t *testing.T) {
	if _, err := ParseVerdict("yes it is satisfied"); err == nil {
		t.Fatal("expected error when no JSON object present")
	}
}

func TestParseVerdict_InvalidJSON(t *testing.T) {
	if _, err := ParseVerdict("{not valid"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

type stubClient struct {
	response string
	err      error
	lastMsgs []llm.ChatMessage
}

func (s *stubClient) Ask(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubClient) Chat(_ context.Context, msgs []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	s.lastMsgs = msgs
	if s.err != nil {
		return llm.ChatResponse{}, s.err
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: s.response}}, nil
}

type stubRouter struct {
	client *stubClient
	err    error
}

func (r *stubRouter) ClientFor(_ llm.Role) (llm.Client, llm.TierResolution, error) {
	if r.err != nil {
		return nil, llm.TierResolution{}, r.err
	}
	return r.client, llm.TierResolution{}, nil
}

func TestLLMJudger_SatisfiedRoundTrip(t *testing.T) {
	c := &stubClient{response: `{"satisfied": true, "reason": "done"}`}
	j := NewLLMJudger(&stubRouter{client: c}, "")

	v, err := j.Judge(context.Background(), "ship the feature", []llm.ChatMessage{
		{Role: "assistant", Content: "shipped"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !v.Satisfied {
		t.Fatal("expected satisfied")
	}
	if len(c.lastMsgs) != 2 || c.lastMsgs[0].Role != "system" {
		t.Fatalf("unexpected msgs: %+v", c.lastMsgs)
	}
}

func TestLLMJudger_BadJSONReturnsErrorWithFalseVerdict(t *testing.T) {
	c := &stubClient{response: `not json`}
	j := NewLLMJudger(&stubRouter{client: c}, "")
	v, err := j.Judge(context.Background(), "g", []llm.ChatMessage{{Role: "assistant", Content: "x"}})
	if err == nil {
		t.Fatal("expected error on unparseable response")
	}
	if v.Satisfied {
		t.Fatal("expected fail-open (false) verdict on parse error")
	}
}

func TestLLMJudger_RouterErrorPropagates(t *testing.T) {
	sentinel := errors.New("no client")
	j := NewLLMJudger(&stubRouter{err: sentinel}, "")
	_, err := j.Judge(context.Background(), "g", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}

func TestLLMJudger_EmptyGoalRejected(t *testing.T) {
	j := NewLLMJudger(&stubRouter{client: &stubClient{}}, "")
	if _, err := j.Judge(context.Background(), "  ", nil); err == nil {
		t.Fatal("expected error for empty goal")
	}
}

func TestBuildUserPayload_TruncatesWindow(t *testing.T) {
	msgs := make([]llm.ChatMessage, DefaultRecentMessageWindow+4)
	for i := range msgs {
		msgs[i] = llm.ChatMessage{Role: "assistant", Content: "msg"}
	}
	payload := buildUserPayload("g", msgs)
	count := strings.Count(payload, "[assistant]")
	if count != DefaultRecentMessageWindow {
		t.Fatalf("expected %d messages in payload, got %d", DefaultRecentMessageWindow, count)
	}
}

func TestBuildUserPayload_ClipsLongContent(t *testing.T) {
	long := strings.Repeat("x", MaxRecentMessageContentChars+500)
	payload := buildUserPayload("g", []llm.ChatMessage{{Role: "assistant", Content: long}})
	if !strings.Contains(payload, "…") {
		t.Fatalf("expected truncation marker in payload")
	}
}
