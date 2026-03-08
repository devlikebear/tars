package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

type captureRunnerToolsClient struct {
	toolNames []string
	messages  []llm.ChatMessage
}

func (c *captureRunnerToolsClient) Ask(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (c *captureRunnerToolsClient) Chat(_ context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.messages = append([]llm.ChatMessage(nil), messages...)
	names := make([]string, 0, len(opts.Tools))
	for _, schema := range opts.Tools {
		names = append(names, schema.Function.Name)
	}
	c.toolNames = names
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "ok",
		},
	}, nil
}

func TestAgentPromptRunnerWithTools_IncludesExtraTools(t *testing.T) {
	client := &captureRunnerToolsClient{}
	extra := tool.Tool{
		Name:        "telegram_send",
		Description: "send telegram message",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (tool.Result, error) {
			return tool.Result{}, nil
		},
	}
	runner := newAgentPromptRunnerWithTools(t.TempDir(), client, 2, zerolog.New(io.Discard), extra)
	if runner == nil {
		t.Fatalf("expected non-nil runner")
	}
	if _, err := runner(context.Background(), "cron:test", "hello", nil); err != nil {
		t.Fatalf("runner call failed: %v", err)
	}
	found := false
	for _, name := range client.toolNames {
		if name == "telegram_send" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected telegram_send in tool schemas, got %+v", client.toolNames)
	}
}

func TestAgentPromptRunnerWithTools_CronUsesMinimalToolsetAndPrompt(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"IDENTITY.md":  "identity block",
		"USER.md":      "user profile",
		"HEARTBEAT.md": "heartbeat check list",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	client := &captureRunnerToolsClient{}
	runner := newAgentPromptRunnerWithTools(root, client, 8, zerolog.New(io.Discard))
	if runner == nil {
		t.Fatalf("expected non-nil runner")
	}
	if _, err := runner(context.Background(), "cron:test", "hello", nil); err != nil {
		t.Fatalf("runner call failed: %v", err)
	}
	if len(client.messages) < 2 {
		t.Fatalf("expected system+user messages, got %+v", client.messages)
	}
	systemPrompt := client.messages[0].Content
	if strings.Contains(systemPrompt, "heartbeat check list") {
		t.Fatalf("did not expect heartbeat content in cron system prompt: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "memory_search") {
		t.Fatalf("did not expect memory tool rule in cron system prompt: %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "telegram_send") {
		t.Fatalf("expected cron system prompt to mention telegram_send fallback, got %q", systemPrompt)
	}

	got := strings.Join(client.toolNames, ",")
	if strings.Contains(got, "exec") || strings.Contains(got, "memory_search") || strings.Contains(got, "schedule_create") {
		t.Fatalf("unexpected broad cron toolset: %s", got)
	}
	for _, want := range []string{"read_file", "write_file", "edit_file", "list_dir", "glob", "project_get", "project_update", "project_state_get", "project_state_update"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected cron tool %q in %s", want, got)
		}
	}
}
