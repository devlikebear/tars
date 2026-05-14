package tarsserver

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

func TestInsertSystemMessageBeforeUser_InsertsBeforeLastUser(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "second"},
	}
	out := insertSystemMessageBeforeUser(msgs, "queued feedback")
	if len(out) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(out))
	}
	if out[3].Role != "system" || out[3].Content != "queued feedback" {
		t.Fatalf("expected system feedback at index 3, got %+v", out[3])
	}
	if out[4].Role != "user" || out[4].Content != "second" {
		t.Fatalf("expected last user preserved at index 4, got %+v", out[4])
	}
}

func TestInsertSystemMessageBeforeUser_EmptyContentNoOp(t *testing.T) {
	msgs := []llm.ChatMessage{{Role: "user", Content: "hi"}}
	out := insertSystemMessageBeforeUser(msgs, "")
	if len(out) != 1 {
		t.Fatalf("expected no-op, got %+v", out)
	}
}

func TestInsertSystemMessageBeforeUser_NoUserAppendsToEnd(t *testing.T) {
	msgs := []llm.ChatMessage{{Role: "system", Content: "sys"}}
	out := insertSystemMessageBeforeUser(msgs, "feedback")
	if len(out) != 2 || out[1].Role != "system" {
		t.Fatalf("expected append at end, got %+v", out)
	}
}

func TestBuildLLMMessagesWithBlocks_PropagatesToolCallMetadataFromHistory(t *testing.T) {
	msgs := buildLLMMessagesWithBlocks("system prompt", []session.Message{
		{
			Role:    "user",
			Content: "read the file",
		},
		{
			Role:       "tool",
			Content:    `{"count":1}`,
			ToolCallID: "call_123",
			ToolName:   "read_file",
			ToolArgs:   `{"path":"README.md"}`,
		},
		{
			Role:    "assistant",
			Content: "I reviewed that file",
		},
	}, "what now", nil)

	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	if msgs[2].Role != "assistant" {
		t.Fatalf("expected assistant at index 2, got %q", msgs[2].Role)
	}
	if len(msgs[2].ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", msgs[2].ToolCalls)
	}
	if msgs[2].ToolCalls[0].ID != "call_123" {
		t.Fatalf("expected tool call id call_123, got %q", msgs[2].ToolCalls[0].ID)
	}
	if msgs[2].ToolCalls[0].Name != "read_file" {
		t.Fatalf("expected tool name read_file, got %q", msgs[2].ToolCalls[0].Name)
	}
	if msgs[3].Role != "tool" {
		t.Fatalf("expected tool at index 3, got %q", msgs[3].Role)
	}
	if msgs[3].ToolCallID != "call_123" {
		t.Fatalf("expected tool_call_id call_123, got %q", msgs[3].ToolCallID)
	}
}

func TestBuildLLMMessagesWithBlocks_SkipsToolMessagesWithoutToolCallID(t *testing.T) {
	msgs := buildLLMMessagesWithBlocks("system prompt", []session.Message{
		{
			Role:    "tool",
			Content: `{"count":1}`,
		},
	}, "what now", nil)

	if len(msgs) != 2 { // system + current user
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "user" {
		t.Fatalf("expected user message to be second, got %q", msgs[1].Role)
	}
}

func TestBuildLLMMessagesWithBlocks_DropsTrailingToolWithoutMatchingAssistant(t *testing.T) {
	msgs := buildLLMMessagesWithBlocks("system prompt", []session.Message{
		{
			Role:       "tool",
			Content:    `{"count":1}`,
			ToolName:   "read_file",
			ToolArgs:   `{"path":"README.md"}`,
			ToolCallID: "call_123",
		},
	}, "what now", nil)

	if len(msgs) != 2 { // system + current user
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "user" {
		t.Fatalf("expected user message to be second, got %q", msgs[1].Role)
	}
}

func TestStatusPreview_RedactsSensitiveFields(t *testing.T) {
	input := `{"password":"p@ss","token":"abc123","path":"README.md"}`
	preview := statusPreview(input, 240)
	if strings.Contains(preview, "p@ss") || strings.Contains(preview, "abc123") {
		t.Fatalf("expected redaction in preview, got %q", preview)
	}
	if !strings.Contains(preview, `"password":"***"`) {
		t.Fatalf("expected password redaction, got %q", preview)
	}
	if !strings.Contains(preview, `"path":"README.md"`) {
		t.Fatalf("expected non-sensitive fields preserved, got %q", preview)
	}
}

func TestStatusPreview_RedactsBearerToken(t *testing.T) {
	preview := statusPreview("authorization=Bearer tok_abcdef123", 240)
	if strings.Contains(preview, "tok_abcdef123") {
		t.Fatalf("expected bearer token redaction, got %q", preview)
	}
	if !strings.Contains(strings.ToLower(preview), "authorization=***") {
		t.Fatalf("expected authorization redaction, got %q", preview)
	}
}

func TestStatusPreviewForTool_CompactsSubagentsRunArgs(t *testing.T) {
	input := `{"agent":"explorer","mode":"parallel","tasks":[{"title":"Check API","prompt":"Inspect the API carefully and report findings","tier":"light"},{"prompt":"Review frontend behavior"}]}`
	preview := statusPreviewForTool("subagents_run", input, 40)
	if !strings.Contains(preview, `"count":2`) {
		t.Fatalf("expected compact count, got %q", preview)
	}
	if !strings.Contains(preview, `"title":"Check API"`) {
		t.Fatalf("expected task title, got %q", preview)
	}
	if strings.Contains(preview, "Inspect the API carefully") {
		t.Fatalf("expected prompt body to be omitted, got %q", preview)
	}
	if strings.Contains(preview, "...") {
		t.Fatalf("expected compact subagent preview to avoid generic truncation, got %q", preview)
	}
}

func TestStatusPreviewForTool_CompactsSubagentsRunResult(t *testing.T) {
	input := `{"count":2,"agent":"explorer","subagents":[{"run_id":"run_1234567890","session_id":"session_a","agent":"explorer","title":"Check API","status":"completed","tier":"light","summary":"done"},{"run_id":"run_failed","title":"Review UI","status":"failed","error":"model failed"}]}`
	preview := statusPreviewForTool("subagents_run", input, 40)
	if !strings.Contains(preview, `"run_id":"run_1234567890"`) {
		t.Fatalf("expected run id, got %q", preview)
	}
	if !strings.Contains(preview, `"status":"failed"`) {
		t.Fatalf("expected failed status, got %q", preview)
	}
	if !strings.Contains(preview, `"error":"model failed"`) {
		t.Fatalf("expected compact error, got %q", preview)
	}
}
