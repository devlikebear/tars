package llm

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultClaudeCodeCLITimeoutOutlastsASilentBashCall(t *testing.T) {
	// A single Bash tool call may run up to 10 minutes without printing.
	if defaultClaudeCodeCLITimeout != 15*time.Minute {
		t.Fatalf("default idle timeout = %s, want 15m", defaultClaudeCodeCLITimeout)
	}
}

// The clock bounds silence, not the whole turn: every sign of life starts
// the window over.
func TestClaudeCodeTurnClockTouchRestartsIdleWindow(t *testing.T) {
	ctx, clock := newClaudeCodeTurnClock(context.Background(), 80*time.Millisecond)
	defer clock.stop()

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		clock.touch()
		time.Sleep(20 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatalf("expired while output kept coming: %v", context.Cause(ctx))
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the clock never expired once output stopped")
	}
}

// Output that arrives while a prompt is open must not restart the clock
// underneath the hold.
func TestClaudeCodeTurnClockTouchDuringPromptKeepsItStopped(t *testing.T) {
	ctx, clock := newClaudeCodeTurnClock(context.Background(), 40*time.Millisecond)
	defer clock.stop()

	clock.hold()
	clock.touch()
	time.Sleep(150 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatalf("expired while a prompt was open: %v", context.Cause(ctx))
	}
	clock.release()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the clock never resumed after the prompt closed")
	}
}

func TestParseClaudeCodeCLIStreamReportsActivityPerLine(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","session_id":"s1"}`,
		``,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"result","result":"hi","is_error":false}`,
	}, "\n")
	lines := 0
	_, err := parseClaudeCodeCLIStream(strings.NewReader(stream), ChatOptions{}, claudeCodeStreamHooks{
		activity: func() { lines++ },
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if lines != 3 {
		t.Fatalf("activity fired %d times, want 3 (once per non-empty line)", lines)
	}
}

// Consecutive assistant messages must not run together in the live stream:
// "…읽겠습니다.The store is in place." The final body already separates them.
func TestParseClaudeCodeCLIStreamSeparatesAssistantMessagesInDeltas(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"먼저 읽겠습니다."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"The store is in place."}]}}`,
		`{"type":"result","result":"done","is_error":false}`,
	}, "\n")
	var deltas []string
	_, err := parseClaudeCodeCLIStream(strings.NewReader(stream), ChatOptions{
		OnDelta: func(s string) { deltas = append(deltas, s) },
	}, claudeCodeStreamHooks{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := strings.Join(deltas, "")
	if want := "먼저 읽겠습니다.\n\nThe store is in place."; got != want {
		t.Fatalf("streamed text = %q, want %q", got, want)
	}
	if len(deltas) == 0 || strings.HasPrefix(deltas[0], "\n") {
		t.Fatalf("the first delta must not start with a separator: %q", deltas)
	}
}

// A turn that keeps printing outlives CLAUDE_CODE_CLI_TIMEOUT: the limit is
// on silence, not on the whole call.
func TestClaudeCodeCLIChat_OutputKeepsTurnAlivePastTimeout(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	script := "#!/bin/sh\n" +
		"i=0\n" +
		"while [ $i -lt 8 ]; do\n" +
		"  echo '{\"type\":\"system\",\"session_id\":\"s1\"}'\n" +
		"  sleep 0.3\n" +
		"  i=$((i+1))\n" +
		"done\n" +
		"echo '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"done\"}]}}'\n" +
		"echo '{\"type\":\"result\",\"result\":\"done\",\"is_error\":false}'\n"
	ccWriteStub(t, scriptPath, script)
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "1s")

	client := ccNewClient(t, dir)
	resp, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{})
	if err != nil {
		t.Fatalf("a turn still printing was cut off: %v", err)
	}
	if resp.Message.Content != "done" {
		t.Fatalf("content = %q, want done", resp.Message.Content)
	}
}
