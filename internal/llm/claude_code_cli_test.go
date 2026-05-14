package llm

import (
	"context"
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
