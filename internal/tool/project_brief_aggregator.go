package tool

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
)

// NewProjectBriefTool creates a single "project_brief" tool that dispatches to
// get, update, and finalize sub-actions. Replaces project_brief_get/update/
// finalize.
func NewProjectBriefTool(store *project.Store, sessionStore *session.Store) Tool {
	getTool := NewProjectBriefGetTool(store)
	updateTool := NewProjectBriefUpdateTool(store)
	finalizeTool := NewProjectBriefFinalizeTool(store, sessionStore)
	return Tool{
		Name:        "project_brief",
		Description: "Manage project briefs (planning/ideation). Actions: get (read brief), update (create or update brief), finalize (convert brief into a project).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["get","update","finalize"]}
  },
  "required":["action"],
  "additionalProperties":true
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			payload, action, err := dispatchAction(params)
			if err != nil {
				return aggregatorError(err.Error()), nil
			}
			switch action {
			case "get":
				return getTool.Execute(ctx, payload)
			case "update":
				return updateTool.Execute(ctx, payload)
			case "finalize":
				return finalizeTool.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: get, update, finalize"), nil
			}
		},
	}
}
