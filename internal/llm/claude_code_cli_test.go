package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClaudeCodeCLIClientChat_ParsesStreamJSON(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"hello from claude"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","duration_ms":12,"duration_api_ms":10,"is_error":false,"num_turns":1,"session_id":"sess-1","stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7},"result":"hello from claude"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "You are a local coding assistant."},
		{Role: "user", Content: "Say hello."},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Message.Content != "hello from claude" {
		t.Fatalf("expected assistant content, got %q", resp.Message.Content)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("expected stop reason end_turn, got %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	if resp.SessionID != "sess-1" {
		t.Fatalf("expected session id sess-1, got %q", resp.SessionID)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{
		"-p",
		"--output-format",
		"stream-json",
		"--model",
		"sonnet",
		"--permission-mode",
		"auto",
		"--system-prompt",
		"You are a local coding assistant.",
		"Say hello.",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("expected args to contain %q, got:\n%s", want, args)
		}
	}
}

func TestClaudeCodeCLIClientChat_AppliesHarnessControlsAndParsesCost(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	envPath := filepath.Join(dir, "claude-env.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
env > `+shellQuote(envPath)+`
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"implemented"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","duration_ms":12,"is_error":false,"num_turns":3,"session_id":"sess-harness","stop_reason":"end_turn","total_cost_usd":0.42,"usage":{"input_tokens":21,"output_tokens":8},"result":"implemented"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	t.Setenv("ANTHROPIC_API_KEY", "must-not-reach-harness")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/must-not-reach-harness.sock")

	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "implement it"}}, ChatOptions{
		ClaudeCodePermissionMode: "dontAsk",
		ClaudeCodeHarness: &ClaudeCodeHarnessOptions{
			SafeMode: true, StrictMCP: true, DisableChrome: true, IsolateEnvironment: true,
			Tools:        []string{"Read", "Edit", "Bash"},
			AllowedTools: []string{"Read", "Edit", "Bash(go test *)"},
			MaxTurns:     12, MaxBudgetUSD: 2.5,
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Turns != 3 || resp.Usage.CostUSD != 0.42 {
		t.Fatalf("normalized harness usage = turns:%d usage:%+v", resp.Turns, resp.Usage)
	}
	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)
	for _, flag := range []string{"--safe-mode", "--strict-mcp-config", "--no-chrome"} {
		if !strings.Contains(args, flag+"\n") {
			t.Fatalf("expected %s in args, got:\n%s", flag, args)
		}
	}
	wantValues := map[string]string{
		"--permission-mode": "dontAsk",
		"--tools":           "Read,Edit,Bash",
		"--allowedTools":    "Read,Edit,Bash(go test *)",
		"--max-turns":       "12",
		"--max-budget-usd":  "2.5",
	}
	for flag, want := range wantValues {
		if got := extractFlagValue(args, flag); got != want {
			t.Fatalf("%s = %q, want %q; args:\n%s", flag, got, want, args)
		}
	}
	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read harness env: %v", err)
	}
	environment := string(envData)
	if strings.Contains(environment, "must-not-reach-harness") || strings.Contains(environment, "ANTHROPIC_API_KEY=") || strings.Contains(environment, "SSH_AUTH_SOCK=") {
		t.Fatalf("credential-bearing environment reached harness:\n%s", environment)
	}
	if !strings.Contains(environment, "HOME=") || !strings.Contains(environment, "PATH=") {
		t.Fatalf("minimal runtime environment missing:\n%s", environment)
	}
}

func TestClaudeCodeCLIChat_CallerCancellationKillsProcessTree(t *testing.T) {
	dir := t.TempDir()
	startedPath := filepath.Join(dir, "started")
	scriptPath := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf started > " + shellQuote(startedPath) + "\nsleep 30\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client, err := NewClaudeCodeCLIClient(dir, "sonnet")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Chat(ctx, []ChatMessage{{Role: "user", Content: "wait"}}, ChatOptions{})
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("Claude Code stub did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("caller cancellation did not kill the CLI process tree")
	}
}

// TestClaudeCodeCLIClientChat_CapturesSessionInit verifies session_id is
// captured from system.init events (before any result event arrives), which
// is the canonical Agent SDK / stream-json source of truth.
func TestClaudeCodeCLIClientChat_CapturesSessionInit(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-init-xyz","data":{"model":"sonnet"}}'
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1},"result":"ok"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "ping"},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.SessionID != "sess-init-xyz" {
		t.Fatalf("expected session id from init event, got %q", resp.SessionID)
	}
}

// TestClaudeCodeCLIClientChat_ParsesToolUse verifies tool_use content blocks
// inside assistant messages are mapped into ChatMessage.ToolCalls so the rest
// of TARS can observe what tools the upstream agent invoked.
func TestClaudeCodeCLIClientChat_ParsesToolUse(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-tool","data":{}}'
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"reading file"},{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/tmp/a.txt"}}]}}'
printf '%s\n' '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_01","content":"file contents"}]}}'
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"done"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":2},"result":"done"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "read it"},
	}, ChatOptions{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	// Claude Code self-executed the tool inside its subprocess; the audit
	// trail lives on ProviderExecutedTools, NOT on Message.ToolCalls.
	// Message.ToolCalls is reserved for "model wants TARS to execute this"
	// semantics, which agent.Loop dispatches to its tool registry — surfacing
	// claude's tools there would cause double-execution / blocked-tool errors.
	if len(resp.Message.ToolCalls) != 0 {
		t.Fatalf("expected Message.ToolCalls to be empty for claude-code-cli, got %+v", resp.Message.ToolCalls)
	}
	if len(resp.ProviderExecutedTools) != 1 {
		t.Fatalf("expected 1 provider-executed tool, got %d: %+v", len(resp.ProviderExecutedTools), resp.ProviderExecutedTools)
	}
	call := resp.ProviderExecutedTools[0]
	if call.ID != "toolu_01" {
		t.Fatalf("tool call id: %q", call.ID)
	}
	if call.Name != "Read" {
		t.Fatalf("tool call name: %q", call.Name)
	}
	if !strings.Contains(call.Arguments, "/tmp/a.txt") {
		t.Fatalf("tool call arguments should be JSON containing /tmp/a.txt, got %q", call.Arguments)
	}
	// Final assistant text comes from the last assistant turn.
	if !strings.Contains(resp.Message.Content, "done") {
		t.Fatalf("expected final assistant text 'done', got %q", resp.Message.Content)
	}
	if resp.SessionID != "sess-tool" {
		t.Fatalf("session id: %q", resp.SessionID)
	}
}

// TestClaudeCodeCLIClientChat_ResumeSessionPassesFlagAndSlimsPrompt verifies
// that when ChatOptions.ResumeSessionID is set the provider:
//   - passes --resume <session_id>
//   - drops --no-session-persistence (otherwise --resume can't load the saved transcript)
//   - sends only the latest user message, not the full transcript text builder
func TestClaudeCodeCLIClientChat_ResumeSessionPassesFlagAndSlimsPrompt(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-resumed","data":{}}'
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"continuing"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1},"result":"continuing","session_id":"sess-resumed"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "old turn 1"},
		{Role: "assistant", Content: "old reply 1"},
		{Role: "user", Content: "follow-up please"},
	}, ChatOptions{ResumeSessionID: "sess-abc"})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.SessionID != "sess-resumed" {
		t.Fatalf("expected session id sess-resumed, got %q", resp.SessionID)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)

	for _, want := range []string{"--resume", "sess-abc", "follow-up please"} {
		if !strings.Contains(args, want) {
			t.Fatalf("expected args to contain %q, got:\n%s", want, args)
		}
	}
	if strings.Contains(args, "--no-session-persistence") {
		t.Fatalf("--no-session-persistence must be dropped in resume mode, got:\n%s", args)
	}
	// Slim-prompt assertions: the historical transcript text builder output
	// (uppercase ROLE labels, "Continue the conversation below…") must not be
	// re-sent, since the upstream session already has it.
	for _, forbidden := range []string{"Continue the conversation below", "USER:", "ASSISTANT:", "old turn 1", "old reply 1"} {
		if strings.Contains(args, forbidden) {
			t.Fatalf("resume mode should not re-send transcript fragment %q, got:\n%s", forbidden, args)
		}
	}
}

// TestClaudeCodeCLIClientChat_FreshSessionKeepsNoSessionPersistence verifies
// that when ResumeSessionID is empty (default), the provider still passes
// --no-session-persistence to avoid littering ~/.claude with throwaway
// transcript files.
func TestClaudeCodeCLIClientChat_FreshSessionKeepsNoSessionPersistence(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"hi"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"hi"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if !strings.Contains(string(args), "--no-session-persistence") {
		t.Fatalf("expected --no-session-persistence in fresh-session args, got:\n%s", args)
	}
	if strings.Contains(string(args), "--resume") {
		t.Fatalf("expected no --resume in fresh-session args, got:\n%s", args)
	}
}

// TestClaudeCodeCLIClientChat_MCPConfigPathPassedWhenServersProvided verifies
// that ChatOptions.ClaudeCodeMCPServers triggers --mcp-config <path> with a
// materialized JSON file containing the Claude Code-shaped mcpServers map.
// Both stdio and remote (http) shapes are covered. The temp file lives only
// for the duration of one Chat call and is cleaned up afterwards.
func TestClaudeCodeCLIClientChat_MCPConfigPathPassedWhenServersProvided(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	// The stub copies the mcp config file content into a side file so the
	// test can inspect it after the cli process exits and the deferred
	// cleanup removes the temp file.
	mcpCapturePath := filepath.Join(dir, "captured-mcp.json")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
mcp_path=""
i=0
for arg in "$@"; do
  i=$((i+1))
  if [ "$arg" = "--mcp-config" ]; then
    eval "mcp_path=\${$((i+1))}"
    break
  fi
done
if [ -n "$mcp_path" ] && [ -f "$mcp_path" ]; then
  cat "$mcp_path" > `+shellQuote(mcpCapturePath)+`
fi
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"ok"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	_, err = client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{
		ClaudeCodeMCPServers: []ClaudeCodeMCPServer{
			{
				Name:    "fs",
				Command: "/usr/bin/mcp-fs",
				Args:    []string{"--root", "/tmp"},
				Env:     map[string]string{"DEBUG": "1"},
			},
			{
				Name:      "remote",
				Transport: "http",
				URL:       "https://mcp.example.com/sse",
				Headers:   map[string]string{"Authorization": "Bearer x"},
			},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)
	if !strings.Contains(args, "--mcp-config") {
		t.Fatalf("expected --mcp-config in args, got:\n%s", args)
	}

	// Verify the file content was a well-formed Claude Code mcp config with
	// our two servers.
	captured, err := os.ReadFile(mcpCapturePath)
	if err != nil {
		t.Fatalf("read captured mcp config: %v", err)
	}
	var payload struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("decode mcp config: %v\npayload=%s", err, captured)
	}
	fs, ok := payload.MCPServers["fs"]
	if !ok {
		t.Fatalf("expected fs server in mcp config, got: %s", captured)
	}
	if fs["type"] != "stdio" {
		t.Fatalf("fs server should be stdio, got: %v", fs["type"])
	}
	if fs["command"] != "/usr/bin/mcp-fs" {
		t.Fatalf("fs command: %v", fs["command"])
	}
	rem, ok := payload.MCPServers["remote"]
	if !ok {
		t.Fatalf("expected remote server in mcp config, got: %s", captured)
	}
	if rem["type"] != "http" {
		t.Fatalf("remote server should be http, got: %v", rem["type"])
	}
	if rem["url"] != "https://mcp.example.com/sse" {
		t.Fatalf("remote url: %v", rem["url"])
	}

	// Temp file should be cleaned up after Chat returns.
	mcpPathFromArgs := extractFlagValue(args, "--mcp-config")
	if mcpPathFromArgs == "" {
		t.Fatalf("could not find --mcp-config value in args:\n%s", args)
	}
	if _, err := os.Stat(mcpPathFromArgs); !os.IsNotExist(err) {
		t.Fatalf("mcp config temp file %q should be removed after Chat, got err=%v", mcpPathFromArgs, err)
	}
}

// TestClaudeCodeCLIClientChat_MCPConfigSkippedWhenEmpty verifies that an empty
// or all-skipped MCPServers slice produces NO --mcp-config flag so we don't
// pass an empty config file to claude.
func TestClaudeCodeCLIClientChat_MCPConfigSkippedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"ok"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	// Case 1: nil slice — no flag.
	if _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	args, _ := os.ReadFile(argsPath)
	if strings.Contains(string(args), "--mcp-config") {
		t.Fatalf("expected no --mcp-config flag when servers nil, got:\n%s", args)
	}

	// Case 2: all entries have empty Name — should be skipped, no flag.
	if _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{
		ClaudeCodeMCPServers: []ClaudeCodeMCPServer{{Name: "", Command: "x"}, {Name: "   ", URL: "y"}},
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	args, _ = os.ReadFile(argsPath)
	if strings.Contains(string(args), "--mcp-config") {
		t.Fatalf("expected no --mcp-config flag when all entries are empty-named, got:\n%s", args)
	}
}

// extractFlagValue scans newline-separated argv (as written by the stub) for
// the value immediately following the given flag. Returns "" if absent.
func extractFlagValue(args, flag string) string {
	lines := strings.Split(args, "\n")
	for i, ln := range lines {
		if ln == flag && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	return ""
}

// TestResolveClaudeCodePermissionMode verifies the recognized-values whitelist
// and the auto fallback for empty/unknown input.
func TestResolveClaudeCodePermissionMode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "auto"},
		{"auto", "auto"},
		{"default", "default"},
		{"acceptEdits", "acceptEdits"},
		{"plan", "plan"},
		{"dontAsk", "dontAsk"},
		{"bypassPermissions", "bypassPermissions"},
		{"  plan  ", "plan"}, // whitespace trimmed
		{"unknown", "auto"},  // unknown → fallback
		{"PLAN", "auto"},     // case-sensitive: unknown
	}
	for _, tc := range cases {
		got := resolveClaudeCodePermissionMode(tc.in)
		if got != tc.want {
			t.Errorf("resolveClaudeCodePermissionMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestClaudeCodeCLIClientChat_PermissionModePropagatesAndFallsBack verifies
// that ChatOptions.ClaudeCodePermissionMode is reflected in --permission-mode
// and that an unknown value silently degrades to auto.
func TestClaudeCodeCLIClientChat_PermissionModePropagatesAndFallsBack(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty fallback", "", "auto"},
		{"valid acceptEdits", "acceptEdits", "acceptEdits"},
		{"valid plan", "plan", "plan"},
		{"valid bypass", "bypassPermissions", "bypassPermissions"},
		{"unknown fallback", "Aggressive", "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			argsPath := filepath.Join(dir, "claude-args.txt")
			scriptPath := filepath.Join(dir, "claude")
			script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"ok"}'
`) + "\n"
			if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
				t.Fatalf("write cli stub: %v", err)
			}
			t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

			client, err := NewProvider(ProviderOptions{
				Provider: "claude-code-cli",
				Model:    "sonnet",
				WorkDir:  dir,
			})
			if err != nil {
				t.Fatalf("new provider: %v", err)
			}
			if _, err := client.Chat(context.Background(), []ChatMessage{
				{Role: "user", Content: "go"},
			}, ChatOptions{ClaudeCodePermissionMode: tc.in}); err != nil {
				t.Fatalf("chat: %v", err)
			}
			args, _ := os.ReadFile(argsPath)
			got := extractFlagValue(string(args), "--permission-mode")
			if got != tc.want {
				t.Fatalf("permission-mode argv: got %q, want %q\nargs:\n%s", got, tc.want, args)
			}
		})
	}
}

func TestClaudeCodeSkillDirName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"github-flow", "github-flow"},
		{"GitHub Flow", "github-flow"},
		{"  tars_github.flow  ", "tars-github-flow"},
		{"a---b", "a-b"},
		{"메모", ""},
		{"-- -- ", ""},
		{"v2.config!!", "v2-config"},
	}
	for _, c := range cases {
		if got := claudeCodeSkillDirName(c.in); got != c.want {
			t.Errorf("claudeCodeSkillDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWriteClaudeCodeSkillsPluginDir_StructureAndCleanup(t *testing.T) {
	dir, cleanup, err := writeClaudeCodeSkillsPluginDir([]ClaudeCodeSkill{
		{Name: "github-flow", Description: "release flow\nwith newline", Content: "Do the flow."},
		{Name: "Bad Name", Description: "second", Content: "body two"},
		{Name: "", Description: "skipped-empty-name"},
		{Name: "메모", Description: "skipped-nonascii"},
		{Name: "github-flow", Description: "dup dir, skipped"},
	})
	if err != nil {
		t.Fatalf("write plugin dir: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty dir for 2 usable skills")
	}
	defer cleanup()

	manifestRaw, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("manifest not valid json: %v", err)
	}
	if manifest.Name != "tars-skills" {
		t.Fatalf("manifest name: got %q want tars-skills", manifest.Name)
	}

	gf, err := os.ReadFile(filepath.Join(dir, "skills", "github-flow", "SKILL.md"))
	if err != nil {
		t.Fatalf("read github-flow SKILL.md: %v", err)
	}
	gfStr := string(gf)
	if !strings.Contains(gfStr, "name: github-flow") {
		t.Fatalf("missing name frontmatter:\n%s", gfStr)
	}
	if !strings.Contains(gfStr, "description: release flow with newline") {
		t.Fatalf("description newline not collapsed:\n%s", gfStr)
	}
	if !strings.Contains(gfStr, "Do the flow.") {
		t.Fatalf("body missing:\n%s", gfStr)
	}

	if _, err := os.Stat(filepath.Join(dir, "skills", "bad-name", "SKILL.md")); err != nil {
		t.Fatalf("expected bad-name skill dir: %v", err)
	}

	skillEntries, err := os.ReadDir(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	if len(skillEntries) != 2 {
		names := []string{}
		for _, e := range skillEntries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected 2 skill dirs, got %d: %v", len(skillEntries), names)
	}

	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove the plugin dir, stat err=%v", err)
	}
}

func TestWriteClaudeCodeSkillsPluginDir_EmptyReturnsNoFlag(t *testing.T) {
	dir, cleanup, err := writeClaudeCodeSkillsPluginDir(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cleanup()
	if dir != "" {
		t.Fatalf("expected empty dir for nil skills, got %q", dir)
	}
	dir2, cleanup2, err := writeClaudeCodeSkillsPluginDir([]ClaudeCodeSkill{{Name: ""}, {Name: "  "}, {Name: "메모"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cleanup2()
	if dir2 != "" {
		t.Fatalf("expected empty dir when all skills unusable, got %q", dir2)
	}
}

func TestClaudeCodeCLIClientChat_SkillsPluginDirPassed(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	skillCapture := filepath.Join(dir, "skill-capture.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
pdir=""
i=0
for arg in "$@"; do
  i=$((i+1))
  if [ "$arg" = "--plugin-dir" ]; then
    eval "pdir=\${$((i+1))}"
    break
  fi
done
if [ -n "$pdir" ] && [ -f "$pdir/skills/probe/SKILL.md" ]; then
  cat "$pdir/skills/probe/SKILL.md" > `+shellQuote(skillCapture)+`
fi
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"ok"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		ClaudeCodeSkills: []ClaudeCodeSkill{{Name: "probe", Description: "probe skill", Content: "probe body"}},
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	args, _ := os.ReadFile(argsPath)
	if !strings.Contains(string(args), "--plugin-dir") {
		t.Fatalf("expected --plugin-dir in args:\n%s", args)
	}
	captured, err := os.ReadFile(skillCapture)
	if err != nil {
		t.Fatalf("skill file not materialized / not found by stub: %v", err)
	}
	if !strings.Contains(string(captured), "name: probe") || !strings.Contains(string(captured), "probe body") {
		t.Fatalf("SKILL.md content unexpected:\n%s", captured)
	}

	pdir := extractFlagValue(string(args), "--plugin-dir")
	if pdir == "" {
		t.Fatalf("could not find --plugin-dir value in args:\n%s", args)
	}
	if _, err := os.Stat(pdir); !os.IsNotExist(err) {
		t.Fatalf("plugin dir %q should be removed after Chat, stat err=%v", pdir, err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// TestWriteClaudeCodeSettingsFile asserts the converter's behavioral contract:
// empty input yields no file, blank/duplicate rules are normalized away, and —
// critically — the emitted JSON has *exactly* the two keys
// `permissions` → `deny` and nothing else. The "adversarial" subtest feeds
// rule strings that look like attempts to smuggle other settings keys and
// proves they land inside the deny array as opaque strings, never as
// top-level settings keys, because the function has no code path to emit
// anything but a deny list.
func TestWriteClaudeCodeSettingsFile(t *testing.T) {
	t.Run("empty yields no file", func(t *testing.T) {
		for _, in := range [][]string{nil, {}, {"", "   ", "\t"}} {
			path, cleanup, err := writeClaudeCodeSettingsFile(in)
			if err != nil {
				t.Fatalf("unexpected error for %v: %v", in, err)
			}
			cleanup()
			if path != "" {
				t.Fatalf("expected empty path for %v, got %q", in, path)
			}
		}
	})

	t.Run("trims dedups and preserves order", func(t *testing.T) {
		path, cleanup, err := writeClaudeCodeSettingsFile([]string{
			"  Bash(rm:*)  ", "WebFetch", "Bash(rm:*)", "", "WebFetch", "Bash(git push:*)",
		})
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		defer cleanup()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read settings file: %v", err)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode settings: %v\n%s", err, raw)
		}
		if len(doc) != 1 {
			t.Fatalf("settings doc must have exactly one top-level key, got %d: %s", len(doc), raw)
		}
		perms, ok := doc["permissions"]
		if !ok {
			t.Fatalf("missing permissions key: %s", raw)
		}
		var permObj map[string]json.RawMessage
		if err := json.Unmarshal(perms, &permObj); err != nil {
			t.Fatalf("decode permissions: %v", err)
		}
		if len(permObj) != 1 {
			t.Fatalf("permissions must have exactly one key (deny), got %d: %s", len(permObj), perms)
		}
		var deny []string
		if err := json.Unmarshal(permObj["deny"], &deny); err != nil {
			t.Fatalf("decode deny: %v", err)
		}
		want := []string{"Bash(rm:*)", "WebFetch", "Bash(git push:*)"}
		if !reflect.DeepEqual(deny, want) {
			t.Fatalf("deny mismatch:\n got %v\nwant %v", deny, want)
		}
	})

	t.Run("adversarial keys stay inside deny array", func(t *testing.T) {
		path, cleanup, err := writeClaudeCodeSettingsFile([]string{
			`env`, `apiKeyHelper`, `hooks`, `{"env":{"ANTHROPIC_API_KEY":"x"}}`,
		})
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		defer cleanup()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, leaked := doc["env"]; leaked {
			t.Fatalf("env must never be a top-level settings key: %s", raw)
		}
		if _, leaked := doc["hooks"]; leaked {
			t.Fatalf("hooks must never be a top-level settings key: %s", raw)
		}
		if _, leaked := doc["apiKeyHelper"]; leaked {
			t.Fatalf("apiKeyHelper must never be a top-level settings key: %s", raw)
		}
		if len(doc) != 1 {
			t.Fatalf("only permissions allowed at top level, got: %s", raw)
		}
	})
}

// TestClaudeCodeCLIClientChat_SettingsPassedWhenDenyProvided verifies that a
// non-empty ClaudeCodePermissionDeny triggers `--settings <path>` with a
// materialized {"permissions":{"deny":[...]}} file, and that the temp file is
// cleaned up after the Chat call returns.
func TestClaudeCodeCLIClientChat_SettingsPassedWhenDenyProvided(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	settingsCapturePath := filepath.Join(dir, "captured-settings.json")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
settings_path=""
i=0
for arg in "$@"; do
  i=$((i+1))
  if [ "$arg" = "--settings" ]; then
    eval "settings_path=\${$((i+1))}"
    break
  fi
done
if [ -n "$settings_path" ] && [ -f "$settings_path" ]; then
  cat "$settings_path" > `+shellQuote(settingsCapturePath)+`
fi
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"ok"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	if _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{
		ClaudeCodePermissionDeny: []string{"Bash(rm:*)", "WebFetch"},
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsData)
	if !strings.Contains(args, "--settings") {
		t.Fatalf("expected --settings in args, got:\n%s", args)
	}

	captured, err := os.ReadFile(settingsCapturePath)
	if err != nil {
		t.Fatalf("read captured settings: %v", err)
	}
	var payload struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("decode settings: %v\n%s", err, captured)
	}
	if !reflect.DeepEqual(payload.Permissions.Deny, []string{"Bash(rm:*)", "WebFetch"}) {
		t.Fatalf("deny mismatch: %v\n%s", payload.Permissions.Deny, captured)
	}

	settingsPathFromArgs := extractFlagValue(args, "--settings")
	if settingsPathFromArgs == "" {
		t.Fatalf("could not find --settings value in args:\n%s", args)
	}
	if _, err := os.Stat(settingsPathFromArgs); !os.IsNotExist(err) {
		t.Fatalf("settings temp file %q should be removed after Chat, got err=%v", settingsPathFromArgs, err)
	}
}

// TestClaudeCodeCLIClientChat_SettingsSkippedWhenEmpty verifies that a nil or
// all-blank deny slice produces NO --settings flag.
func TestClaudeCodeCLIClientChat_SettingsSkippedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"ok"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"ok"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)

	client, err := NewProvider(ProviderOptions{
		Provider: "claude-code-cli",
		Model:    "sonnet",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	if _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{ClaudeCodePermissionDeny: []string{"", "   "}}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	args, _ := os.ReadFile(argsPath)
	if strings.Contains(string(args), "--settings") {
		t.Fatalf("expected no --settings flag when deny all-blank, got:\n%s", args)
	}
}
