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

func decodePreviewObject(t *testing.T, preview string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(preview), &out); err != nil {
		t.Fatalf("preview is not valid JSON: %v\n%s", err, preview)
	}
	return out
}

func TestProviderToolArgsPreview_KeepsLabelFieldsWhenArgsAreLong(t *testing.T) {
	// claude-code-cli encodes tool input with sorted keys, so the file path
	// of a Write comes after its whole content.
	args, _ := json.Marshal(map[string]any{
		"content":   "package llm\n\nimport (\n\t\"context\"\n)\n" + strings.Repeat("// filler line\n", 200),
		"file_path": "/home/dev/tars/pkg/llm/claude_code_cli.go",
	})
	got := providerToolArgsPreview("Write", string(args), 180)
	if n := utf8.RuneCountInString(got); n > 180 {
		t.Fatalf("preview has %d runes, want <= 180: %s", n, got)
	}
	obj := decodePreviewObject(t, got)
	if obj["file_path"] != "/home/dev/tars/pkg/llm/claude_code_cli.go" {
		t.Fatalf("file_path = %v, want the full path", obj["file_path"])
	}
	if !strings.HasPrefix(got, `{"file_path":`) {
		t.Fatalf("label field should come first: %s", got)
	}
	content, _ := obj["content"].(string)
	if !strings.HasPrefix(content, "package llm") || !strings.HasSuffix(content, "…") {
		t.Fatalf("content should be a shortened prefix, got %q", content)
	}
}

func TestProviderToolArgsPreview_BashKeepsCommandAndDescription(t *testing.T) {
	command := `rg -n "MCP 자동 주입" CLAUDE.md; ` + strings.Repeat("rg -n foo internal/tarsserver; ", 20)
	args, _ := json.Marshal(map[string]any{"command": command, "description": "Search the docs", "timeout": 120000})
	got := providerToolArgsPreview("Bash", string(args), 180)
	if n := utf8.RuneCountInString(got); n > 180 {
		t.Fatalf("preview has %d runes, want <= 180", n)
	}
	obj := decodePreviewObject(t, got)
	if obj["description"] != "Search the docs" {
		t.Fatalf("description = %v", obj["description"])
	}
	cmd, _ := obj["command"].(string)
	if !strings.HasPrefix(cmd, `rg -n "MCP 자동 주입" CLAUDE.md;`) {
		t.Fatalf("command should keep its start, got %q", cmd)
	}
	if !strings.HasPrefix(got, `{"command":`) {
		t.Fatalf("command should come first: %s", got)
	}
}

func TestProviderToolArgsPreview_ShortArgsStayWhole(t *testing.T) {
	got := providerToolArgsPreview("Read", `{"file_path":"/tmp/a <b>.txt","limit":20}`, 180)
	obj := decodePreviewObject(t, got)
	if obj["file_path"] != "/tmp/a <b>.txt" || obj["limit"] != float64(20) {
		t.Fatalf("unexpected preview %s", got)
	}
	if strings.Contains(got, `\u003c`) {
		t.Fatalf("preview should not HTML-escape: %s", got)
	}
}

func TestProviderToolArgsPreview_NestedValuesAndManyKeysStayValid(t *testing.T) {
	edits := make([]map[string]string, 0, 30)
	for range 30 {
		edits = append(edits, map[string]string{"old_string": strings.Repeat("a", 40), "new_string": strings.Repeat("b", 40)})
	}
	args, _ := json.Marshal(map[string]any{"edits": edits, "file_path": "/repo/main.go"})
	got := providerToolArgsPreview("MultiEdit", string(args), 180)
	if n := utf8.RuneCountInString(got); n > 180 {
		t.Fatalf("preview has %d runes", n)
	}
	obj := decodePreviewObject(t, got)
	if obj["file_path"] != "/repo/main.go" {
		t.Fatalf("file_path = %v", obj["file_path"])
	}

	many := map[string]any{"file_path": "/repo/x.go"}
	for i := range 60 {
		many["key_"+strings.Repeat("z", 3)+string(rune('a'+i%26))+string(rune('a'+i/26))] = "value"
	}
	manyArgs, _ := json.Marshal(many)
	got = providerToolArgsPreview("Edit", string(manyArgs), 180)
	if n := utf8.RuneCountInString(got); n > 180 {
		t.Fatalf("preview has %d runes", n)
	}
	if decodePreviewObject(t, got)["file_path"] != "/repo/x.go" {
		t.Fatalf("file_path lost: %s", got)
	}
}

func TestProviderToolArgsPreview_RedactsSecretsAndFallsBackForNonObjects(t *testing.T) {
	// Built at run time so the fixture is not a key-shaped literal in the repo.
	fakeKey := strings.Join([]string{"sk", "live", "1234567890"}, "-")
	got := providerToolArgsPreview("WebFetch", `{"url":"https://example.com","api_key":"`+fakeKey+`"}`, 180)
	if strings.Contains(got, fakeKey) {
		t.Fatalf("secret leaked: %s", got)
	}
	decodePreviewObject(t, got)

	if got := providerToolArgsPreview("Bash", "not json at all", 180); got != "not json at all" {
		t.Fatalf("non-JSON args should use the plain preview, got %q", got)
	}
	if got := providerToolArgsPreview("Bash", "", 180); got != "" {
		t.Fatalf("empty args should stay empty, got %q", got)
	}
}

func TestSetupAgentLoop_ProviderToolArgsStayParseable(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"content":   strings.Repeat("x", 2000),
		"file_path": "/repo/big.txt",
	})
	client := &providerToolStubClient{resp: llm.ChatResponse{
		Message:               llm.ChatMessage{Role: "assistant", Content: "done"},
		ProviderExecutedTools: []llm.ToolCall{{ID: "toolu_1", Name: "Write", Arguments: string(args)}},
	}}
	var previews []string
	sendStatus := func(name, _, _, _, argsPreview, _ string, _ ...bool) {
		if strings.HasPrefix(name, "provider_tool") {
			previews = append(previews, argsPreview)
		}
	}
	loop, toolCalls := setupAgentLoop(client, tool.NewRegistry(), "sess", 0, nil, zerolog.Nop(), sendStatus, nil)
	if _, err := loop.Run(context.Background(), []llm.ChatMessage{{Role: "user", Content: "go"}}, agent.RunOptions{}); err != nil {
		t.Fatalf("loop run: %v", err)
	}
	if len(previews) == 0 {
		t.Fatal("no provider_tool status sent")
	}
	for _, p := range previews {
		if decodePreviewObject(t, p)["file_path"] != "/repo/big.txt" {
			t.Fatalf("stream preview lost file_path: %s", p)
		}
	}
	if len(*toolCalls) != 1 {
		t.Fatalf("toolCalls = %d", len(*toolCalls))
	}
	rec := (*toolCalls)[0]
	if utf8.RuneCountInString(rec.ToolArgs) > 500 {
		t.Fatalf("transcript args too long: %d", utf8.RuneCountInString(rec.ToolArgs))
	}
	if decodePreviewObject(t, rec.ToolArgs)["file_path"] != "/repo/big.txt" {
		t.Fatalf("transcript args lost file_path: %s", rec.ToolArgs)
	}
}
