package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/project"
)

func NewProjectBoardGetTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_board_get",
		Description: "Read the project board and task metadata for a project.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"project_id":{"type":"string"}},
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.GetBoard(strings.TrimSpace(input.ProjectID))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewProjectBoardUpdateTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_board_update",
		Description: "Update project board columns or tasks.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "columns":{"type":"array","items":{"type":"string"}},
    "tasks":{"type":"array","items":{"type":"object"}}
  },
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string              `json:"project_id"`
				Columns   []string            `json:"columns,omitempty"`
				Tasks     []project.BoardTask `json:"tasks,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.UpdateBoard(strings.TrimSpace(input.ProjectID), project.BoardUpdateInput{
				Columns: input.Columns,
				Tasks:   input.Tasks,
			})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewProjectActivityGetTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_activity_get",
		Description: "Read recent project activity entries.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "limit":{"type":"integer"}
  },
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			items, err := store.ListActivity(strings.TrimSpace(input.ProjectID), input.Limit)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(map[string]any{
				"count": len(items),
				"items": items,
			}, false), nil
		},
	}
}

func NewProjectActivityAppendTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_activity_append",
		Description: "Append a project activity entry.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "task_id":{"type":"string"},
    "source":{"type":"string"},
    "agent":{"type":"string"},
    "kind":{"type":"string"},
    "status":{"type":"string"},
    "message":{"type":"string"},
    "timestamp":{"type":"string"},
    "meta":{"type":"object","additionalProperties":{"type":"string"}}
  },
  "required":["project_id","source","kind"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string            `json:"project_id"`
				TaskID    string            `json:"task_id,omitempty"`
				Source    string            `json:"source"`
				Agent     string            `json:"agent,omitempty"`
				Kind      string            `json:"kind"`
				Status    string            `json:"status,omitempty"`
				Message   string            `json:"message,omitempty"`
				Timestamp string            `json:"timestamp,omitempty"`
				Meta      map[string]string `json:"meta,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.AppendActivity(strings.TrimSpace(input.ProjectID), project.ActivityAppendInput{
				TaskID:    input.TaskID,
				Source:    input.Source,
				Agent:     input.Agent,
				Kind:      input.Kind,
				Status:    input.Status,
				Message:   input.Message,
				Timestamp: input.Timestamp,
				Meta:      input.Meta,
			})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewProjectDispatchTool(store *project.Store, runner project.TaskRunner, checker project.GitHubAuthChecker) Tool {
	return Tool{
		Name:        "project_dispatch",
		Description: "Dispatch project tasks for the todo or review stage.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "stage":{"type":"string"}
  },
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			if runner == nil {
				return JSONTextResult(map[string]any{"message": "project task runner is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
				Stage     string `json:"stage,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			stage := strings.ToLower(strings.TrimSpace(input.Stage))
			if stage == "" {
				stage = "todo"
			}
			orchestrator := project.NewOrchestratorWithGitHubAuthChecker(store, runner, checker)
			var (
				report project.DispatchReport
				err    error
			)
			switch stage {
			case "todo":
				report, err = orchestrator.DispatchTodo(ctx, strings.TrimSpace(input.ProjectID))
			case "review":
				report, err = orchestrator.DispatchReview(ctx, strings.TrimSpace(input.ProjectID))
			default:
				return JSONTextResult(map[string]any{"message": "stage must be todo or review"}, true), nil
			}
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(report, false), nil
		},
	}
}

type projectAutopilotStarter interface {
	Start(context.Context, string) (project.AutopilotRun, error)
}

func NewProjectAutopilotStartTool(manager projectAutopilotStarter) Tool {
	return Tool{
		Name:        "project_autopilot_start",
		Description: "Start or resume autonomous execution for a project phase in the background.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"project_id":{"type":"string"}},
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if manager == nil {
				return JSONTextResult(map[string]any{"message": "project autopilot manager is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			run, err := manager.Start(ctx, strings.TrimSpace(input.ProjectID))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(run, false), nil
		},
	}
}

type projectAutopilotAdvancer interface {
	Advance(context.Context, string) (project.PhaseSnapshot, error)
}

func NewProjectAutopilotAdvanceTool(manager projectAutopilotAdvancer) Tool {
	return Tool{
		Name:        "project_autopilot_advance",
		Description: "Advance the project phase engine by one synchronous step and return the current phase snapshot.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"project_id":{"type":"string"}},
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if manager == nil {
				return JSONTextResult(map[string]any{"message": "project autopilot manager is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			snapshot, err := manager.Advance(ctx, strings.TrimSpace(input.ProjectID))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(snapshot, false), nil
		},
	}
}
