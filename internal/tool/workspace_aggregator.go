package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/sysprompt"
)

// NewWorkspaceTool creates a single "workspace" tool that manages workspace and
// agent system-prompt files. Replaces workspace_sysprompt_get/set and
// agent_sysprompt_get/set.
func NewWorkspaceTool(workspaceDir string) Tool {
	wsGet := NewWorkspaceSyspromptGetTool(workspaceDir)
	wsSet := NewWorkspaceSyspromptSetTool(workspaceDir)
	agGet := NewAgentSyspromptGetTool(workspaceDir)
	agSet := NewAgentSyspromptSetTool(workspaceDir)

	// Build combined file enum for description.
	var wsFiles, agFiles []string
	for _, spec := range sysprompt.Specs(sysprompt.ScopeWorkspace) {
		wsFiles = append(wsFiles, spec.Path)
	}
	for _, spec := range sysprompt.Specs(sysprompt.ScopeAgent) {
		agFiles = append(agFiles, spec.Path)
	}

	desc := fmt.Sprintf(
		"Manage workspace system-prompt files. scope=workspace covers %s; scope=agent covers %s. "+
			"Actions: list (show all files in scope), get (read one file), set (update one file). "+
			"USER.md stores user identity (name, language, preferences) — use this when the user introduces themselves or asks to remember personal info. "+
			"IDENTITY.md stores TARS persona. AGENTS.md/TOOLS.md store agent operating rules.",
		strings.Join(wsFiles, ", "), strings.Join(agFiles, ", "),
	)

	return Tool{
		Name:        "workspace",
		Description: desc,
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","get","set"]},
    "scope":{"type":"string","enum":["workspace","agent"],"default":"workspace"}
  },
  "required":["action"],
  "additionalProperties":true
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			payload, action, err := dispatchAction(params)
			if err != nil {
				return aggregatorError(err.Error()), nil
			}
			// Extract scope from remaining payload.
			var meta struct {
				Scope string `json:"scope"`
			}
			_ = json.Unmarshal(payload, &meta)
			scope := strings.ToLower(strings.TrimSpace(meta.Scope))
			if scope == "" {
				scope = "workspace"
			}
			switch action {
			case "list", "get":
				if scope == "agent" {
					return agGet.Execute(ctx, payload)
				}
				return wsGet.Execute(ctx, payload)
			case "set":
				if scope == "agent" {
					return agSet.Execute(ctx, payload)
				}
				return wsSet.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: list, get, set"), nil
			}
		},
	}
}
