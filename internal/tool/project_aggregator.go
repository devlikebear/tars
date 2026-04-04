package tool

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
)

// NewProjectTool creates a single "project" tool that dispatches to CRUD and
// activate sub-actions. Replaces the individual project_create/list/get/update/
// delete/activate tools.
func NewProjectTool(store *project.Store, sessionStore *session.Store, mainSessionID string) Tool {
	createTool := NewProjectCreateTool(store)
	listTool := NewProjectListTool(store)
	getTool := NewProjectGetTool(store)
	updateTool := NewProjectUpdateTool(store)
	deleteTool := NewProjectDeleteTool(store)
	activateTool := NewProjectActivateTool(store, sessionStore, mainSessionID)
	return Tool{
		Name:        "project",
		Description: "Manage projects. Actions: create, list, get, update, delete, activate.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["create","list","get","update","delete","activate"]}
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
			case "create":
				return createTool.Execute(ctx, payload)
			case "list":
				return listTool.Execute(ctx, json.RawMessage(`{}`))
			case "get":
				return getTool.Execute(ctx, payload)
			case "update":
				return updateTool.Execute(ctx, payload)
			case "delete":
				return deleteTool.Execute(ctx, payload)
			case "activate":
				return activateTool.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: create, list, get, update, delete, activate"), nil
			}
		},
	}
}
