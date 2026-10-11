package initiative

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

func TestSanitizeSpeechResponseTable(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain one line", "Welcome back!", "Welcome back!"},
		{"strips code fence", "```\nHey, good to see you.\n```", "Hey, good to see you."},
		{"keeps at most three lines", "one\ntwo\nthree\nfour\nfive", "one\ntwo\nthree"},
		{"drops blank lines", "one\n\n\ntwo", "one\ntwo"},
		{"strips list markers and wrapping quotes", "- \"Hey there\"", "Hey there"},
		{"empty input is empty", "   \n  ", ""},
		{"control characters are dropped", "Hey\x00 there\x07!", "Hey there!"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeSpeechResponse(c.in)
			if got != c.want {
				t.Fatalf("sanitizeSpeechResponse(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeSpeechResponseCapsTotalLength(t *testing.T) {
	long := strings.Repeat("x", maxSpeechTotalRunes+200)
	got := sanitizeSpeechResponse(long)
	if len([]rune(got)) > maxSpeechTotalRunes {
		t.Fatalf("sanitized length = %d, want <= %d", len([]rune(got)), maxSpeechTotalRunes)
	}
}

func TestBuildSpeechMessagesIncludesTextOnlyWhenSendsText(t *testing.T) {
	in := ComposeInput{
		Intent: IntentGreet, Now: at(9, 0), Location: seoul, Identity: "Dry, concise, a little sardonic.",
		SendsText: true, Profile: "likes tea", RecentUser: []UserMessage{{At: at(8, 55), Text: "오늘 피곤하다"}},
	}
	msgs := buildSpeechMessages(in)
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("messages = %+v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "Dry, concise, a little sardonic.") {
		t.Fatalf("system message missing identity tone: %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[1].Content, "오늘 피곤하다") || !strings.Contains(msgs[1].Content, "likes tea") {
		t.Fatalf("user message missing text context: %q", msgs[1].Content)
	}
}

func TestBuildSpeechMessagesOmitsTextWhenNotSendsText(t *testing.T) {
	in := ComposeInput{
		Intent: IntentCheckIn, Now: at(9, 0), Location: seoul, Identity: "Dry, concise.",
		SendsText: false, Profile: "likes tea", RecentUser: []UserMessage{{At: at(8, 55), Text: "오늘 피곤하다"}},
	}
	msgs := buildSpeechMessages(in)
	if strings.Contains(msgs[1].Content, "오늘 피곤하다") || strings.Contains(msgs[1].Content, "likes tea") {
		t.Fatalf("user message must not leak text when SendsText is false: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "intent: check_in") {
		t.Fatalf("user message missing intent metadata: %q", msgs[1].Content)
	}
}

func TestComposeIsolationMatchesDecisionOnlyMinusJSON(t *testing.T) {
	client := &stubChatClient{resp: contentResponse("Welcome back!")}
	composer := &SpeechComposer{Client: client, Model: "sonnet"}
	text, err := composer.Compose(context.Background(), ComposeInput{Intent: IntentGreet, Now: at(9, 0), Location: seoul})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if text != "Welcome back!" {
		t.Fatalf("text = %q", text)
	}
	if client.lastOpt.ToolChoice == nil || client.lastOpt.ToolChoice.Mode != llm.ToolChoiceModeNone {
		t.Fatalf("expected ToolChoiceNone, got %+v", client.lastOpt.ToolChoice)
	}
	if client.lastOpt.ResponseFormat != nil {
		t.Fatalf("speech must not force a JSON response format, got %+v", client.lastOpt.ResponseFormat)
	}
	if client.lastOpt.ClaudeCodeHarness == nil || len(client.lastOpt.ClaudeCodeHarness.Tools) != 0 {
		t.Fatalf("expected an empty Claude Code harness tool list, got %+v", client.lastOpt.ClaudeCodeHarness)
	}
	if client.lastOpt.ResumeSessionID != "" || client.lastOpt.PersistSession {
		t.Fatalf("speech compose must never resume or persist an upstream session, got %+v", client.lastOpt)
	}
}

func TestComposeEmptyResponseIsErrSpeechEmpty(t *testing.T) {
	client := &stubChatClient{resp: contentResponse("   \n ```\n```\n")}
	composer := &SpeechComposer{Client: client}
	_, err := composer.Compose(context.Background(), ComposeInput{Intent: IntentGreet})
	if !errors.Is(err, ErrSpeechEmpty) {
		t.Fatalf("err = %v, want ErrSpeechEmpty", err)
	}
}

func TestComposeToolAttemptIsRejected(t *testing.T) {
	client := &stubChatClient{resp: llm.ChatResponse{Message: llm.ChatMessage{
		Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "bash"}},
	}}}
	composer := &SpeechComposer{Client: client}
	_, err := composer.Compose(context.Background(), ComposeInput{Intent: IntentGreet})
	if !errors.Is(err, ErrSpeechToolAttempt) {
		t.Fatalf("err = %v, want ErrSpeechToolAttempt", err)
	}
}

func TestComposeProviderErrorDoesNotLeakRequestText(t *testing.T) {
	client := &stubChatClient{err: &llm.ProviderError{Provider: "anthropic", StatusCode: 500, Message: "오늘 피곤하다 echoed back"}}
	composer := &SpeechComposer{Client: client}
	_, err := composer.Compose(context.Background(), ComposeInput{
		Intent: IntentGreet, SendsText: true, RecentUser: []UserMessage{{At: at(9, 0), Text: "오늘 피곤하다"}},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	// describeSystemOneError-style handling happens in the caller (tarsserver);
	// this only checks Compose itself returns the raw error for the caller
	// to classify, never a sanitized/pre-baked message of its own that
	// might embed the state. The provider error's own Message field is the
	// provider's concern, not this package's — Compose does not add to it.
	var provErr *llm.ProviderError
	if !errors.As(err, &provErr) || provErr.StatusCode != 500 {
		t.Fatalf("err = %v, want the provider error to pass through", err)
	}
}

func TestComposeNilClientIsConfigured(t *testing.T) {
	composer := &SpeechComposer{}
	if _, err := composer.Compose(context.Background(), ComposeInput{Intent: IntentGreet}); err == nil {
		t.Fatal("expected an error for an unconfigured composer")
	}
}
