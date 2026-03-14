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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
