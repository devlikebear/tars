package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/tars/pkg/tools"
)

func TestRegistrySchemasUsePublicTypes(t *testing.T) {
	registry := tools.NewRegistryWithScope(tools.RegistryScopeUser)
	registry.Register(tools.Tool{
		Name:        "answer",
		Description: "Return an answer.",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
			return tools.JSONTextResult(map[string]int{"answer": 42}, false), nil
		},
	})

	schemas := registry.Schemas()
	if len(schemas) != 1 {
		t.Fatalf("len(Schemas()) = %d, want 1", len(schemas))
	}
	if schemas[0].Function.Name != "answer" {
		t.Fatalf("schema name = %q", schemas[0].Function.Name)
	}
	result, err := registry.All()[0].Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("result IsError = true")
	}
}

func TestPublicConstructorsAndHelpers(t *testing.T) {
	root := t.TempDir()
	singleDirPolicy := tools.SingleDirPolicy(root)
	if len(singleDirPolicy.AllowedDirs) != 1 {
		t.Fatalf("SingleDirPolicy() = %+v", singleDirPolicy)
	}
	policy := tools.NewPathPolicy(root, []string{root}, root)
	manager := tools.NewProcessManager()

	constructors := []tools.Tool{
		tools.NewReadTool(root),
		tools.NewReadFileTool(root),
		tools.NewReadFileToolWithPolicy(policy),
		tools.NewWriteTool(root),
		tools.NewWriteFileTool(root),
		tools.NewWriteFileToolWithPolicy(policy),
		tools.NewEditTool(root),
		tools.NewEditFileTool(root),
		tools.NewEditFileToolWithPolicy(policy),
		tools.NewListDirTool(root),
		tools.NewListDirToolWithPolicy(policy),
		tools.NewGlobTool(root),
		tools.NewGlobToolWithPolicy(policy),
		tools.NewProjectSkillToolWithPolicy(policy),
		tools.NewApplyPatchTool(root, false),
		tools.NewExecTool(root),
		tools.NewExecToolWithManager(root, manager),
		tools.NewExecToolWithPolicy(policy, manager),
		tools.NewExecToolWithOptions(policy, manager, tools.ExecToolOptions{MaxTimeoutMS: 1000}),
		tools.NewProcessTool(manager),
		tools.NewWebFetchTool(false),
		tools.NewWebFetchToolWithOptions(tools.WebFetchOptions{Enabled: false}),
		tools.NewWebSearchTool(false, ""),
		tools.NewWebSearchToolWithOptions(tools.WebSearchOptions{Enabled: false}),
		tools.NewMemoryTool(root, nil, nil),
		tools.NewMemorySaveTool(nil, nil),
		tools.NewMemorySearchTool(root, nil),
		tools.NewMemoryGetTool(root, nil),
	}
	for _, tool := range constructors {
		if tool.Name == "" {
			t.Fatalf("constructor returned unnamed tool: %+v", tool)
		}
	}

	if got := tools.CanonicalToolName("shell_exec"); got != "exec" {
		t.Fatalf("CanonicalToolName() = %q", got)
	}
	if len(tools.ToolNameAliases()) == 0 {
		t.Fatalf("ToolNameAliases() is empty")
	}
	if !tools.IsExecToolName("run_command") {
		t.Fatalf("IsExecToolName() = false")
	}
	if !tools.IsHighRiskToolName("write_file") {
		t.Fatalf("IsHighRiskToolName() = false")
	}
	if len(tools.KnownToolGroupNames()) == 0 {
		t.Fatalf("KnownToolGroupNames() is empty")
	}
	if group := tools.NormalizeToolGroupName("terminal"); group != "shell" {
		t.Fatalf("NormalizeToolGroupName() = %q", group)
	}
	if group := tools.ToolGroupForName("read_file"); group != "files" {
		t.Fatalf("ToolGroupForName() = %q", group)
	}
	known := map[string]struct{}{"read_file": {}, "exec": {}}
	if groups := tools.KnownToolGroups(known); len(groups["files"]) != 1 {
		t.Fatalf("KnownToolGroups() = %+v", groups)
	}
	if valid, expanded, unknown := tools.ExpandToolGroups([]string{"files", "missing"}, known); len(valid) != 1 || len(expanded) != 1 || len(unknown) != 1 {
		t.Fatalf("ExpandToolGroups() = %v %v %v", valid, expanded, unknown)
	}
	if valid, matched, invalid := tools.ExpandToolPatterns([]string{"read_*", "["}, known); len(valid) != 1 || len(matched) != 1 || len(invalid) != 1 {
		t.Fatalf("ExpandToolPatterns() = %v %v %v", valid, matched, invalid)
	}

	policyResult := tools.Policy{AllowTools: []string{"read_file"}, UseAllowTools: true}.Resolve([]string{"read_file", "exec"}, "test")
	if len(policyResult.Allowed) != 1 || len(policyResult.Blocked) != 1 {
		t.Fatalf("Policy.Resolve() = %+v", policyResult)
	}
	if blocked, ok := tools.ParseBlockedToolError(errors.New("plain")); ok || blocked.Tool != "" {
		t.Fatalf("ParseBlockedToolError() plain = %+v, %v", blocked, ok)
	}
	if blocked, ok := tools.ParseBlockedToolError(tools.BlockedToolError{Tool: "exec", Rule: "tool_allow", Source: "test"}); !ok || blocked.Tool != "exec" {
		t.Fatalf("ParseBlockedToolError() = %+v, %v", blocked, ok)
	}

	raw := tools.MustJSON(map[string]string{"ok": "yes"})
	if len(raw) == 0 {
		t.Fatalf("MustJSON() returned empty")
	}
	result := tools.JSONTextResult(map[string]string{"ok": "yes"}, false)
	if result.IsError || result.Text() == "" {
		t.Fatalf("JSONTextResult() = %+v", result)
	}

	emitter := &recordingEmitter{}
	ctx := tools.WithLineEmitter(context.Background(), emitter)
	if tools.LineEmitterFromContext(ctx) == nil {
		t.Fatalf("LineEmitterFromContext() = nil")
	}
	streamer := tools.BindLineEmitter(emitter, "call-1")
	ctx = tools.WithToolOutputStreamer(ctx, streamer)
	if tools.ToolOutputStreamerFromContext(ctx) == nil {
		t.Fatalf("ToolOutputStreamerFromContext() = nil")
	}
	streamer.EmitLine(tools.StreamStdout, "hello")
	if emitter.last != "call-1:stdout:hello" {
		t.Fatalf("emitter.last = %q", emitter.last)
	}
}

type recordingEmitter struct {
	last string
}

func (e *recordingEmitter) EmitToolLine(toolCallID, stream, text string) {
	e.last = toolCallID + ":" + stream + ":" + text
}
