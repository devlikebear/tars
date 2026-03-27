package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
)

func NewProjectCreateTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_create",
		Description: "Create a new project workspace and project metadata document.",
	Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "name":{"type":"string"},
    "type":{"type":"string"},
    "git_repo":{"type":"string"},
    "objective":{"type":"string"},
    "workflow_profile":{"type":"string"},
    "workflow_rules":{
      "type":"array",
      "items":{
        "type":"object",
        "properties":{
          "name":{"type":"string"},
          "params":{"type":"object","additionalProperties":{"type":"string"}}
        },
        "required":["name"],
        "additionalProperties":false
      }
    },
    "instructions":{"type":"string"},
    "clone_repo":{"type":"boolean"}
  },
  "required":["name"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				Name            string                 `json:"name"`
				Type            string                 `json:"type,omitempty"`
				GitRepo         string                 `json:"git_repo,omitempty"`
				Objective       string                 `json:"objective,omitempty"`
				WorkflowProfile string                 `json:"workflow_profile,omitempty"`
				WorkflowRules   []project.WorkflowRule `json:"workflow_rules,omitempty"`
				Instructions    string                 `json:"instructions,omitempty"`
				CloneRepo       bool                   `json:"clone_repo,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			created, err := store.Create(project.CreateInput{
				Name:            input.Name,
				Type:            input.Type,
				GitRepo:         input.GitRepo,
				Objective:       input.Objective,
				WorkflowProfile: input.WorkflowProfile,
				WorkflowRules:   input.WorkflowRules,
				Instructions:    input.Instructions,
				CloneRepo:       input.CloneRepo,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(created, false), nil
		},
	}
}

func NewProjectListTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_list",
		Description: "List projects in workspace.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			items, err := store.List()
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(items, false), nil
		},
	}
}

func NewProjectGetTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_get",
		Description: "Get one project metadata and instructions by project id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"project_id":{"type":"string"}},
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.Get(strings.TrimSpace(input.ProjectID))
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(item, false), nil
		},
	}
}

func NewProjectUpdateTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_update",
		Description: "Update project metadata/policy/objective/instructions.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "name":{"type":"string"},
    "type":{"type":"string"},
    "status":{"type":"string"},
    "git_repo":{"type":"string"},
    "objective":{"type":"string"},
    "instructions":{"type":"string"},
    "tools_allow":{"type":"array","items":{"type":"string"}},
    "tools_allow_groups":{"type":"array","items":{"type":"string"}},
    "tools_allow_patterns":{"type":"array","items":{"type":"string"}},
    "tools_deny":{"type":"array","items":{"type":"string"}},
    "tools_risk_max":{"type":"string"},
    "skills_allow":{"type":"array","items":{"type":"string"}},
    "workflow_profile":{"type":"string"},
    "workflow_rules":{
      "type":"array",
      "items":{
        "type":"object",
        "properties":{
          "name":{"type":"string"},
          "params":{"type":"object","additionalProperties":{"type":"string"}}
        },
        "required":["name"],
        "additionalProperties":false
      }
    },
    "mcp_servers":{"type":"array","items":{"type":"string"}},
    "secrets_refs":{"type":"array","items":{"type":"string"}}
  },
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input project.UpdatePayload
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			updated, err := store.Update(strings.TrimSpace(input.ProjectID), input.ToUpdateInput())
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(updated, false), nil
		},
	}
}

func NewProjectDeleteTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_delete",
		Description: "Archive a project by setting status=archived.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"project_id":{"type":"string"}},
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.Archive(strings.TrimSpace(input.ProjectID))
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(item, false), nil
		},
	}
}

func NewProjectActivateTool(store *project.Store, sessionStore *session.Store, mainSessionID string) Tool {
	return Tool{
		Name:        "project_activate",
		Description: "Bind active project to a session context.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "session_id":{"type":"string"}
  },
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil || sessionStore == nil {
				return jsonTextResult(map[string]any{"message": "project/session store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
				SessionID string `json:"session_id,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.Get(strings.TrimSpace(input.ProjectID))
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			targetSession := strings.TrimSpace(input.SessionID)
			if targetSession == "" {
				targetSession = strings.TrimSpace(mainSessionID)
			}
			if targetSession == "" {
				latest, err := sessionStore.Latest()
				if err == nil {
					targetSession = strings.TrimSpace(latest.ID)
				}
			}
			if targetSession == "" {
				return jsonTextResult(map[string]any{"message": "session_id is required"}, true), nil
			}
			if _, err := sessionStore.Get(targetSession); err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			if err := sessionStore.SetProjectID(targetSession, item.ID); err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(map[string]any{"project_id": item.ID, "session_id": targetSession, "activated": true}, false), nil
		},
	}
}
