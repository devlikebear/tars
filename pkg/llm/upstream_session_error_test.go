package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpstreamSessionIDFromError(t *testing.T) {
	base := errors.New("cli timed out")
	err := withUpstreamSession(base, "sess-1")
	if got := UpstreamSessionIDFromError(err); got != "sess-1" {
		t.Fatalf("id = %q", got)
	}
	if err.Error() != base.Error() || !errors.Is(err, base) {
		t.Fatalf("the wrapper must keep the message and chain: %v", err)
	}
	if got := UpstreamSessionIDFromError(fmt.Errorf("loop: %w", err)); got != "sess-1" {
		t.Fatalf("wrapped id = %q", got)
	}
	if withUpstreamSession(base, "  ") != base || withUpstreamSession(nil, "x") != nil {
		t.Fatal("no id or no error: nothing to wrap")
	}
	if UpstreamSessionIDFromError(base) != "" || UpstreamSessionIDFromError(nil) != "" {
		t.Fatal("plain errors carry no id")
	}
}

// ccSessionThenHang prints the init event with a session id, then goes
// silent past CLAUDE_CODE_CLI_TIMEOUT — the first turn of a long coding
// session cut off by the idle limit.
func ccSessionThenHang(t *testing.T) Client {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	ccWriteStub(t, scriptPath, "#!/bin/sh\n"+
		`printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-saved"}'`+"\n"+
		"sleep 30\n")
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "1s")
	return ccNewClient(t, dir)
}

// A saved upstream session outlives the call that failed: the error carries
// its id so the next turn can --resume it.
func TestClaudeCodeCLIChat_TimeoutKeepsSavedSessionID(t *testing.T) {
	client := ccSessionThenHang(t)
	_, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{PersistSession: true})
	if err == nil || !strings.Contains(err.Error(), "no output for 1s") {
		t.Fatalf("expected the idle timeout, got %v", err)
	}
	if got := UpstreamSessionIDFromError(err); got != "sess-saved" {
		t.Fatalf("session id on error = %q, want sess-saved", got)
	}
}

// A resumed session is on disk too.
func TestClaudeCodeCLIChat_TimeoutOnResumeKeepsSessionID(t *testing.T) {
	client := ccSessionThenHang(t)
	_, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{ResumeSessionID: "sess-old"})
	if got := UpstreamSessionIDFromError(err); got != "sess-saved" {
		t.Fatalf("session id on error = %q (err %v)", got, err)
	}
}

// A one-shot call runs with --no-session-persistence: its id names nothing
// that could be resumed.
func TestClaudeCodeCLIChat_UnsavedSessionIDNotReported(t *testing.T) {
	client := ccSessionThenHang(t)
	_, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{})
	if err == nil {
		t.Fatal("expected the idle timeout")
	}
	if got := UpstreamSessionIDFromError(err); got != "" {
		t.Fatalf("session id on error = %q, want none", got)
	}
}

// Cancellation (the person pressing stop) keeps it as well.
func TestClaudeCodeCLIChat_CancelKeepsSavedSessionID(t *testing.T) {
	client := ccSessionThenHang(t)
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "30s")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(700 * time.Millisecond)
		cancel()
	}()
	_, err := client.Chat(ctx, ccUserMsg(), ChatOptions{PersistSession: true})
	if err == nil {
		t.Fatal("expected a cancellation error")
	}
	if got := UpstreamSessionIDFromError(err); got != "sess-saved" {
		t.Fatalf("session id on error = %q (err %v)", got, err)
	}
}

// An error result from the CLI (is_error) keeps it too.
func TestClaudeCodeCLIChat_ErrorResultKeepsSavedSessionID(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	ccWriteStub(t, scriptPath, "#!/bin/sh\n"+
		`printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-saved"}'`+"\n"+
		`printf '%s\n' '{"type":"result","is_error":true,"result":"API Error: overloaded","session_id":"sess-saved"}'`+"\n")
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client := ccNewClient(t, dir)
	_, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{PersistSession: true})
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("expected the CLI's error, got %v", err)
	}
	if got := UpstreamSessionIDFromError(err); got != "sess-saved" {
		t.Fatalf("session id on error = %q", got)
	}
}

// Antigravity saves every conversation, so a failed call reports the
// conversation it started whatever PersistSession says.
func TestParseAntigravityCLIStreamKeepsConversationOnError(t *testing.T) {
	_, err := parseAntigravityCLIStream(strings.NewReader(agyInitEvent()), ChatOptions{})
	if err == nil {
		t.Fatal("a stream without a result must fail")
	}
	if got := UpstreamSessionIDFromError(err); got != agyConvID {
		t.Fatalf("conversation on error = %q", got)
	}
}

func TestAntigravityCLIClient_ChatFailureKeepsConversation(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "agy-stub"+exeSuffix())
	if err := os.WriteFile(stub, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(antigravityCLIPathEnv, stub)
	t.Setenv(antigravityCLITimeoutEnv, "5s")
	t.Setenv("GO_WANT_ANTIGRAVITY_DYING_HELPER", "1")
	client, err := NewAntigravityCLIClient(dir, "")
	if err != nil {
		t.Fatalf("NewAntigravityCLIClient: %v", err)
	}
	client.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestAntigravityCLIDyingHelperProcess")
	}
	_, err = client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "go"}}, ChatOptions{})
	if err == nil {
		t.Fatal("expected the dead CLI to fail the call")
	}
	if got := UpstreamSessionIDFromError(err); got != agyConvID {
		t.Fatalf("conversation on error = %q (err %v)", got, err)
	}
}

// TestAntigravityCLIDyingHelperProcess starts a conversation and dies
// before the result.
func TestAntigravityCLIDyingHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ANTIGRAVITY_DYING_HELPER") != "1" {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, agyInitEvent())
	os.Exit(3)
}
