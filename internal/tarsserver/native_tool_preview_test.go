package tarsserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

func TestNativeToolPreviewKeepsArgumentsAndErrorVisible(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"command": "missing-command " + strings.Repeat("argument ", 100), "background": false})
	registry := tool.NewRegistry()
	registry.Register(tool.NewExecTool(t.TempDir()))
	client := &mockLLMClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "native-exec", Name: "exec", Arguments: string(args)}}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
	}}
	var previews, results []string
	loop, records := setupAgentLoop(client, registry, "sess", 0, nil, zerolog.Nop(), func(phase, _, _, _, args, result string, _ ...bool) {
		if phase == "before_tool_call" || phase == "after_tool_call" {
			previews = append(previews, args)
		}
		if phase == "after_tool_call" {
			results = append(results, result)
		}
	}, nil)
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "run"}}, agent.RunOptions{Tools: registry.Schemas()}); err != nil {
		t.Fatal(err)
	}
	if len(previews) != 2 || len(results) != 1 || len(*records) != 1 {
		t.Fatalf("missing tool evidence: %d previews, %d results, %d records", len(previews), len(results), len(*records))
	}
	for _, p := range append(previews, (*records)[0].ToolArgs) {
		obj := decodePreviewObject(t, p)
		if command, _ := obj["command"].(string); !strings.HasPrefix(command, "missing-command") {
			t.Fatalf("lost command label: %s", p)
		}
	}
	for _, p := range append(results, (*records)[0].ToolResult) {
		if !strings.Contains(p, "executable file not found") && !strings.Contains(p, "not found") {
			t.Fatalf("error hidden by long command: %s", p)
		}
	}
}

func TestExecResultPreviewPrioritizesFailureAndRedacts(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"command": strings.Repeat("long command ", 100), "exit_code": 1, "message": "exit status 1", "stderr": "unknown argument; password=hidden-value", "stdout": strings.Repeat("noise", 100)})
	got := statusPreviewForTool("exec", string(input), 180)
	if !strings.Contains(got, "unknown argument") || !strings.Contains(got, "exit status 1") || strings.Contains(got, "hidden-value") || utf8.RuneCountInString(got) > 180 {
		t.Fatalf("unhelpful or unsafe error preview: %s", got)
	}
}
