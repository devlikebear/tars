package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/sysprompt"
)

func NewWorkspaceSyspromptGetTool(workspaceDir string) Tool {
	return newSyspromptGetTool(
		"workspace_sysprompt_get",
		workspaceDir,
		sysprompt.ScopeWorkspace,
		"Inspect workspace system-prompt files that define user identity, TARS persona, and related workspace prompt context. USER.md is about the user; IDENTITY.md is about TARS.",
	)
}

func NewWorkspaceSyspromptSetTool(workspaceDir string) Tool {
	return newSyspromptSetTool(
		"workspace_sysprompt_set",
		workspaceDir,
		sysprompt.ScopeWorkspace,
		"Update one workspace system-prompt file. Use this for user identity, TARS persona, and related workspace prompt context instead of generic file-edit tools.",
	)
}

func NewAgentSyspromptGetTool(workspaceDir string) Tool {
	return newSyspromptGetTool(
		"agent_sysprompt_get",
		workspaceDir,
		sysprompt.ScopeAgent,
		"Inspect agent system-prompt files. AGENTS.md defines agent operating rules; TOOLS.md defines tool environment guidance and usage expectations.",
	)
}

func NewAgentSyspromptSetTool(workspaceDir string) Tool {
	return newSyspromptSetTool(
		"agent_sysprompt_set",
		workspaceDir,
		sysprompt.ScopeAgent,
		"Update one agent system-prompt file. Use this for AGENTS.md and TOOLS.md instead of generic file-edit tools.",
	)
}

func newSyspromptGetTool(name string, workspaceDir string, scope sysprompt.Scope, description string) Tool {
	return Tool{
		Name:        name,
		Description: description,
		Parameters:  syspromptGetSchema(scope),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				File string `json:"file,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			file := strings.TrimSpace(input.File)
			if file == "" {
				items, err := sysprompt.List(workspaceDir, scope)
				if err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return JSONTextResult(map[string]any{
					"scope": scope,
					"count": len(items),
					"items": items,
				}, false), nil
			}
			item, err := sysprompt.Get(workspaceDir, scope, file)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func newSyspromptSetTool(name string, workspaceDir string, scope sysprompt.Scope, description string) Tool {
	return Tool{
		Name:        name,
		Description: description,
		Parameters:  syspromptSetSchema(scope),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				File    string `json:"file"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			file := strings.TrimSpace(input.File)
			if file == "" {
				return JSONTextResult(map[string]any{"message": "file is required"}, true), nil
			}
			item, err := sysprompt.Save(workspaceDir, scope, file, input.Content)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func syspromptGetSchema(scope sysprompt.Scope) json.RawMessage {
	return syspromptSchema(scope, false)
}

func syspromptSetSchema(scope sysprompt.Scope) json.RawMessage {
	return syspromptSchema(scope, true)
}

func syspromptSchema(scope sysprompt.Scope, includeContent bool) json.RawMessage {
	paths := make([]string, 0, len(sysprompt.Specs(scope)))
	for _, spec := range sysprompt.Specs(scope) {
		paths = append(paths, spec.Path)
	}
	properties := map[string]any{
		"file": map[string]any{
			"type":        "string",
			"enum":        paths,
			"description": "System-prompt file to inspect or update.",
		},
	}
	required := []string{}
	if includeContent {
		properties["content"] = map[string]any{
			"type":        "string",
			"description": "Full new file content to save.",
		}
		required = []string{"file", "content"}
	}
	raw, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	})
	return raw
}
