package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseClaudeCodeCLITimeout(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", defaultClaudeCodeCLITimeout},
		{"   ", defaultClaudeCodeCLITimeout},
		{"300s", 300 * time.Second},
		{"5m", 5 * time.Minute},
		{"120", 120 * time.Second},
		{"0", defaultClaudeCodeCLITimeout},
		{"-3", defaultClaudeCodeCLITimeout},
		{"garbage", defaultClaudeCodeCLITimeout},
	}
	for _, tc := range cases {
		if got := parseClaudeCodeCLITimeout(tc.in); got != tc.want {
			t.Errorf("parseClaudeCodeCLITimeout(%q)=%s want %s", tc.in, got, tc.want)
		}
	}
}

func TestClaudeCodeCLIEnv_AddsStartupDefaults(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/home/x"}
	got := claudeCodeCLIEnv(base)
	for _, want := range []string{
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("expected env to contain %q, got %v", want, got)
		}
	}
	// base entries are preserved
	if !slices.Contains(got, "PATH=/usr/bin") || !slices.Contains(got, "HOME=/home/x") {
		t.Errorf("base env not preserved: %v", got)
	}
}

func TestClaudeCodeCLIEnv_PreservesUserOverride(t *testing.T) {
	base := []string{"DISABLE_TELEMETRY=0", "PATH=/usr/bin"}
	got := claudeCodeCLIEnv(base)
	if slices.Contains(got, "DISABLE_TELEMETRY=1") {
		t.Errorf("user DISABLE_TELEMETRY=0 should not be overridden; got %v", got)
	}
	if !slices.Contains(got, "DISABLE_TELEMETRY=0") {
		t.Errorf("user value lost; got %v", got)
	}
}

func TestClaudeCodeCLIChat_InjectsStartupEnvToProcess(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nenv > " + shellQuote(envPath) + "\n" +
		`printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"num_turns":1,"session_id":"s","stop_reason":"end_turn","usage":{},"result":"ok"}'` + "\n"
	ccWriteStub(t, scriptPath, script)
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client := ccNewClient(t, dir)
	if _, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env dump: %v", err)
	}
	env := string(data)
	for _, want := range []string{
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("expected process env to contain %q", want)
		}
	}
}

func TestClaudeCodeCLIChat_TimesOut(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	// Fork a long-lived grandchild (sleep) that inherits the stdout pipe and
	// hangs far past the deadline. If the timeout only killed the direct
	// child, the surviving grandchild would hold the pipe open and the call
	// would block ~30s; bounding elapsed well under that proves the process
	// group is killed. (No marker/count assertion here: that would race the
	// shell stub's own startup against the deadline and flake under load.)
	script := "#!/bin/sh\nsleep 30\n"
	ccWriteStub(t, scriptPath, script)
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	t.Setenv("CLAUDE_CODE_CLI_TIMEOUT", "1s")

	client := ccNewClient(t, dir)
	start := time.Now()
	_, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("timeout/process-group kill not enforced: took %s", elapsed)
	}
}

func TestRunClaudeCodeCLIWithRetry_NoRetryWhenContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // a done context models a timeout or cancellation
	calls := 0
	_, err := runClaudeCodeCLIWithRetry(ctx, func() (ChatResponse, error) {
		calls++
		return ChatResponse{}, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt when context is done (no retry), got %d", calls)
	}
}

func TestRunClaudeCodeCLIWithRetry_RetriesOnceWhenLive(t *testing.T) {
	calls := 0
	_, err := runClaudeCodeCLIWithRetry(context.Background(), func() (ChatResponse, error) {
		calls++
		return ChatResponse{}, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error after retry, got nil")
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts (initial + 1 retry), got %d", calls)
	}
}

func TestRunClaudeCodeCLIWithRetry_NoRetryOnSuccess(t *testing.T) {
	calls := 0
	resp, err := runClaudeCodeCLIWithRetry(context.Background(), func() (ChatResponse, error) {
		calls++
		return ChatResponse{SessionID: "ok"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt on success, got %d", calls)
	}
	if resp.SessionID != "ok" {
		t.Fatalf("expected response to pass through, got %+v", resp)
	}
}

func TestClaudeCodeCLIChat_RetriesTransientFailureOnce(t *testing.T) {
	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	scriptPath := filepath.Join(dir, "claude")
	// Fail on the first invocation, succeed on the second.
	script := "#!/bin/sh\n" +
		"printf x >> " + shellQuote(countPath) + "\n" +
		"n=$(wc -c < " + shellQuote(countPath) + " | tr -d ' ')\n" +
		`if [ "$n" -lt 2 ]; then echo "boom" >&2; exit 1; fi` + "\n" +
		`printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"num_turns":1,"session_id":"s2","stop_reason":"end_turn","usage":{},"result":"recovered"}'` + "\n"
	ccWriteStub(t, scriptPath, script)
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client := ccNewClient(t, dir)
	resp, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{})
	if err != nil {
		t.Fatalf("expected success after one retry, got %v", err)
	}
	if resp.SessionID != "s2" {
		t.Fatalf("expected recovered response (session s2), got %q", resp.SessionID)
	}
	if got := ccRunCount(countPath); got != 2 {
		t.Fatalf("expected 2 invocations (1 retry), got %d", got)
	}
}

func TestClaudeCodeCLIChat_TransientFailureStopsAfterOneRetry(t *testing.T) {
	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	scriptPath := filepath.Join(dir, "claude")
	// Always fail: should attempt exactly twice (initial + one retry).
	script := "#!/bin/sh\nprintf x >> " + shellQuote(countPath) + "\necho boom >&2\nexit 1\n"
	ccWriteStub(t, scriptPath, script)
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client := ccNewClient(t, dir)
	if _, err := client.Chat(context.Background(), ccUserMsg(), ChatOptions{}); err == nil {
		t.Fatal("expected error after exhausting retry, got nil")
	}
	if got := ccRunCount(countPath); got != 2 {
		t.Fatalf("expected 2 invocations (initial + 1 retry), got %d", got)
	}
}

// --- test helpers ---

func ccWriteStub(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
}

func ccNewClient(t *testing.T, dir string) Client {
	t.Helper()
	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return client
}

func ccUserMsg() []ChatMessage {
	return []ChatMessage{{Role: "user", Content: "hi"}}
}

func ccRunCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "x")
}
