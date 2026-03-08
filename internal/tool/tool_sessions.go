package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
)

func NewSessionsListTool(store *session.Store) Tool {
	return Tool{
		Name:        "sessions_list",
		Description: "List chat sessions.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{},
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "session store is not configured"}, true), nil
			}
			sessions, err := store.List()
			if err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("list sessions failed: %v", err)}, true), nil
			}
			return jsonTextResult(map[string]any{"count": len(sessions), "sessions": sessions}, false), nil
		},
	}
}

func NewSessionsHistoryTool(store *session.Store) Tool {
	return Tool{
		Name:        "sessions_history",
		Description: "Read chat history for a session.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "session_id":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":500,"default":50}
  },
  "required":["session_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "session store is not configured"}, true), nil
			}
			var input struct {
				SessionID string `json:"session_id"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.SessionID = strings.TrimSpace(input.SessionID)
			if input.SessionID == "" {
				return jsonTextResult(map[string]any{"message": "session_id is required"}, true), nil
			}
			if _, err := store.Get(input.SessionID); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("get session failed: %v", err)}, true), nil
			}
			messages, err := session.ReadMessages(store.TranscriptPath(input.SessionID))
			if err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("read history failed: %v", err)}, true), nil
			}
			limit := input.Limit
			if limit <= 0 {
				limit = 50
			}
			if len(messages) > limit {
				messages = messages[len(messages)-limit:]
			}
			return jsonTextResult(map[string]any{
				"session_id": input.SessionID,
				"count":      len(messages),
				"messages":   messages,
			}, false), nil
		},
	}
}

func NewSessionsSendTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "sessions_send",
		Description: "Send a prompt to a session and wait for completion.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "session_id":{"type":"string"},
    "title":{"type":"string"},
    "message":{"type":"string"},
    "agent":{"type":"string"},
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":120000,"default":30000}
  },
  "required":["message"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				SessionID string `json:"session_id"`
				Title     string `json:"title"`
				Message   string `json:"message"`
				Agent     string `json:"agent"`
				TimeoutMS int    `json:"timeout_ms,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			run, err := runtime.Spawn(ctx, gateway.SpawnRequest{
				WorkspaceID: workspaceID,
				SessionID:   input.SessionID,
				Title:       input.Title,
				Prompt:      input.Message,
				Agent:       input.Agent,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			timeout := input.TimeoutMS
			if timeout <= 0 {
				timeout = 30000
			}
			waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
			defer cancel()
			final, err := runtime.Wait(waitCtx, run.ID)
			if err != nil {
				return jsonTextResult(map[string]any{
					"accepted":   true,
					"run_id":     run.ID,
					"session_id": run.SessionID,
					"status":     run.Status,
					"message":    fmt.Sprintf("wait run failed: %v", err),
				}, true), nil
			}
			isError := final.Status != gateway.RunStatusCompleted
			return jsonTextResult(map[string]any{
				"accepted":     true,
				"run_id":       final.ID,
				"session_id":   final.SessionID,
				"status":       final.Status,
				"response":     final.Response,
				"error":        final.Error,
				"completed_at": final.CompletedAt,
			}, isError), nil
		},
	}
}

func NewSessionsSpawnTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "sessions_spawn",
		Description: "Spawn an async agent run and return accepted + run_id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "session_id":{"type":"string"},
    "title":{"type":"string"},
    "message":{"type":"string"},
    "agent":{"type":"string"}
  },
  "required":["message"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				SessionID string `json:"session_id"`
				Title     string `json:"title"`
				Message   string `json:"message"`
				Agent     string `json:"agent"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			run, err := runtime.Spawn(ctx, gateway.SpawnRequest{
				WorkspaceID: workspaceID,
				SessionID:   input.SessionID,
				Title:       input.Title,
				Prompt:      input.Message,
				Agent:       input.Agent,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(map[string]any{
				"accepted":   true,
				"run_id":     run.ID,
				"session_id": run.SessionID,
				"status":     run.Status,
			}, false), nil
		},
	}
}

func NewSessionsRunsTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "sessions_runs",
		Description: "List/get/cancel async runs.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","get","cancel"]},
    "run_id":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":200,"default":50}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Action string `json:"action"`
				RunID  string `json:"run_id,omitempty"`
				Limit  int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			switch strings.TrimSpace(input.Action) {
			case "list":
				runs := runtime.ListByWorkspace(workspaceID, input.Limit)
				return jsonTextResult(map[string]any{"count": len(runs), "runs": runs}, false), nil
			case "get":
				runID := strings.TrimSpace(input.RunID)
				if runID == "" {
					return jsonTextResult(map[string]any{"message": "run_id is required"}, true), nil
				}
				run, ok := runtime.GetByWorkspace(workspaceID, runID)
				if !ok {
					return jsonTextResult(map[string]any{"message": "run not found"}, true), nil
				}
				return jsonTextResult(run, false), nil
			case "cancel":
				runID := strings.TrimSpace(input.RunID)
				if runID == "" {
					return jsonTextResult(map[string]any{"message": "run_id is required"}, true), nil
				}
				run, err := runtime.CancelByWorkspace(workspaceID, runID)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				return jsonTextResult(run, false), nil
			default:
				return jsonTextResult(map[string]any{"message": "action must be one of: list|get|cancel"}, true), nil
			}
		},
	}
}

func NewAgentsListTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "agents_list",
		Description: "List available agents for sessions_spawn/sessions_send.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if runtime == nil {
				return jsonTextResult(map[string]any{"count": 0, "agents": []map[string]any{}}, false), nil
			}
			agents := runtime.Agents()
			return jsonTextResult(map[string]any{"count": len(agents), "agents": agents}, false), nil
		},
	}
}
