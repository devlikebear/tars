//go:build integration

package llm

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestAntigravityCLILive drives a real `agy` process end to end. It needs the
// CLI installed and signed in — the credential lives in the system keyring, not
// in any env var — so it skips when the binary cannot be resolved.
//
//	go test -tags integration ./internal/llm/ -run TestAntigravityCLILive -v
func TestAntigravityCLILive(t *testing.T) {
	if _, err := FindAntigravityCLIPath(); err != nil {
		t.Skipf("antigravity cli not available: %v", err)
	}

	client, err := NewProvider(ProviderOptions{
		Provider: "antigravity-cli",
		WorkDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var deltas []string
	resp, err := client.Chat(ctx, []ChatMessage{
		{Role: "system", Content: "Answer with a single word and nothing else."},
		{Role: "user", Content: "Reply with exactly: PONG"},
	}, ChatOptions{OnDelta: func(text string) { deltas = append(deltas, text) }})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if !strings.Contains(strings.ToUpper(resp.Message.Content), "PONG") {
		t.Errorf("content = %q, want it to contain PONG", resp.Message.Content)
	}
	// The conversation id is what makes multi-turn resume possible.
	if strings.TrimSpace(resp.SessionID) == "" {
		t.Error("SessionID is empty; --conversation resume would be impossible")
	}
	if resp.StopReason != "SUCCESS" {
		t.Errorf("stop reason = %q, want SUCCESS", resp.StopReason)
	}
	if resp.Usage.InputTokens <= 0 || resp.Usage.OutputTokens <= 0 {
		t.Errorf("usage = %+v, want non-zero token counts", resp.Usage)
	}
	if len(deltas) == 0 {
		t.Error("OnDelta never fired; text_delta parsing is broken")
	}
	t.Logf("content=%q session=%s usage=%+v turns=%d deltas=%d",
		resp.Message.Content, resp.SessionID, resp.Usage, resp.Turns, len(deltas))
}

// TestAntigravityCLILive_Resume proves the conversation id round-trips: a
// second turn resumed with it must recall the first turn's content.
func TestAntigravityCLILive_Resume(t *testing.T) {
	if _, err := FindAntigravityCLIPath(); err != nil {
		t.Skipf("antigravity cli not available: %v", err)
	}

	client, err := NewProvider(ProviderOptions{Provider: "antigravity-cli", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	first, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "Remember the word GRAPEFRUIT. Reply with just: OK"},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if strings.TrimSpace(first.SessionID) == "" {
		t.Fatal("first turn returned no SessionID")
	}

	second, err := client.Chat(ctx, []ChatMessage{
		{Role: "user", Content: "What word did I ask you to remember? Reply with just that word."},
	}, ChatOptions{ResumeSessionID: first.SessionID})
	if err != nil {
		t.Fatalf("resumed turn: %v", err)
	}
	if !strings.Contains(strings.ToUpper(second.Message.Content), "GRAPEFRUIT") {
		t.Errorf("resumed content = %q, want it to recall GRAPEFRUIT", second.Message.Content)
	}
	// The id must stay stable across the resume, otherwise a third turn would
	// silently start a new conversation.
	if second.SessionID != first.SessionID {
		t.Errorf("resumed session id = %q, want %q", second.SessionID, first.SessionID)
	}
	t.Logf("first=%s second=%q", first.SessionID, second.Message.Content)
}
