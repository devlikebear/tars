package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d: %+v", len(resp.Message.ToolCalls), resp.Message.ToolCalls)
	}
	call := resp.Message.ToolCalls[0]
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
		{"acceptEdits", "acceptEdits"},
		{"plan", "plan"},
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
