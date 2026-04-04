package tool

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/tars/internal/project"
)

// NewProjectWorkTool creates a single "project_work" tool that dispatches to
// board, activity, dispatch, and state sub-actions. Replaces project_board_get/
// update, project_activity_get/append, project_dispatch, project_state_get/update.
func NewProjectWorkTool(store *project.Store, runner project.TaskRunner, checker project.GitHubAuthChecker) Tool {
	boardGet := NewProjectBoardGetTool(store)
	boardUpdate := NewProjectBoardUpdateTool(store)
	activityGet := NewProjectActivityGetTool(store)
	activityAppend := NewProjectActivityAppendTool(store)
	dispatch := NewProjectDispatchTool(store, runner, checker)
	stateGet := NewProjectStateGetTool(store)
	stateUpdate := NewProjectStateUpdateTool(store)
	return Tool{
		Name:        "project_work",
		Description: "Manage project runtime. Actions: board_get, board_update, activity_get, activity_append, dispatch, state_get, state_update.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["board_get","board_update","activity_get","activity_append","dispatch","state_get","state_update"]}
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
			case "board_get":
				return boardGet.Execute(ctx, payload)
			case "board_update":
				return boardUpdate.Execute(ctx, payload)
			case "activity_get":
				return activityGet.Execute(ctx, payload)
			case "activity_append":
				return activityAppend.Execute(ctx, payload)
			case "dispatch":
				return dispatch.Execute(ctx, payload)
			case "state_get":
				return stateGet.Execute(ctx, payload)
			case "state_update":
				return stateUpdate.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: board_get, board_update, activity_get, activity_append, dispatch, state_get, state_update"), nil
			}
		},
	}
}
