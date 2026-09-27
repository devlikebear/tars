package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A caller that resumes the session next time must get a saved one: with
// --no-session-persistence the next --resume finds nothing.
func TestClaudeCodeCLIClientChat_PersistSessionSavesTheFreshSession(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","session_id":"s-new","usage":{"input_tokens":1,"output_tokens":1},"result":"hi"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{PersistSession: true}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if strings.Contains(string(args), "--no-session-persistence") || strings.Contains(string(args), "--resume") {
		t.Fatalf("a persisted fresh session must carry neither flag, got:\n%s", args)
	}
}

// The CLI works in the caller's directory (a chat session's cwd) and keeps
// the configured workspace reachable through --add-dir.
func TestClaudeCodeCLIClientChat_WorkDirSetsTheProcessDirectory(t *testing.T) {
	workspace := t.TempDir()
	project := t.TempDir()
	argsPath := filepath.Join(workspace, "claude-args.txt")
	pwdPath := filepath.Join(workspace, "claude-pwd.txt")
	scriptPath := filepath.Join(workspace, "claude")
	script := strings.TrimSpace(`#!/bin/sh
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
pwd -P > `+shellQuote(pwdPath)+`
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"result":"hi"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: workspace})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, ChatOptions{WorkDir: project}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	pwd, err := os.ReadFile(pwdPath)
	if err != nil {
		t.Fatalf("read pwd: %v", err)
	}
	wantProject, _ := filepath.EvalSymlinks(project)
	if got := strings.TrimSpace(string(pwd)); got != wantProject {
		t.Fatalf("cli ran in %q, want the session directory %q", got, wantProject)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if got := extractFlagValue(string(args), "--add-dir"); got != workspace {
		t.Fatalf("--add-dir = %q, want the configured workspace %q", got, workspace)
	}
}

// A --resume of a session the CLI never saved fails the same way every time.
// Chat does not retry it; it starts a fresh session with the whole
// transcript so the turn still succeeds, and reports the new session ID.
func TestClaudeCodeCLIClientChat_MissingSessionFallsBackToAFreshSession(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "claude-args.txt")
	runsPath := filepath.Join(dir, "claude-runs.txt")
	scriptPath := filepath.Join(dir, "claude")
	script := strings.TrimSpace(`#!/bin/sh
echo run >> `+shellQuote(runsPath)+`
for arg in "$@"; do
  if [ "$arg" = "--resume" ]; then
    echo "No conversation found with session ID: s-old" >&2
    exit 1
  fi
done
printf '%s\n' "$@" > `+shellQuote(argsPath)+`
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s-new","model":"sonnet"}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","session_id":"s-new","usage":{"input_tokens":1,"output_tokens":1},"result":"second answer"}'
`) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write cli stub: %v", err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", scriptPath)
	client, err := NewProvider(ProviderOptions{Provider: "claude-code-cli", Model: "sonnet", WorkDir: dir})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	resp, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
	}, ChatOptions{ResumeSessionID: "s-old", PersistSession: true})
	if err != nil {
		t.Fatalf("chat should fall back to a fresh session: %v", err)
	}
	if resp.SessionID != "s-new" {
		t.Fatalf("session id = %q, want the fresh session", resp.SessionID)
	}
	runs, _ := os.ReadFile(runsPath)
	if n := strings.Count(string(runs), "run"); n != 2 {
		t.Fatalf("cli ran %d times, want 2 (the failed resume once, then a fresh session)", n)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	for _, flag := range []string{"--resume", "--no-session-persistence"} {
		if strings.Contains(string(args), flag) {
			t.Fatalf("fresh fallback must not pass %s, got:\n%s", flag, args)
		}
	}
	if !strings.Contains(string(args), "first question") {
		t.Fatalf("fresh fallback must carry the whole transcript, got:\n%s", args)
	}
}
