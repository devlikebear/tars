package tool

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/session"
)

// NewSessionTool creates a single "session" tool that dispatches to status,
// list, history, send, spawn, runs, and agents sub-actions. Replaces
// session_status, sessions_list/history/send/spawn/runs, and agents_list.
func NewSessionTool(
	store *session.Store,
	runtime *gateway.Runtime,
	getStatus func(ctx context.Context) (SessionStatus, error),
) Tool {
	statusTool := NewSessionStatusTool(getStatus)
	listTool := NewSessionsListTool(store)
	historyTool := NewSessionsHistoryTool(store)
	sendTool := NewSessionsSendTool(runtime)
	spawnTool := NewSessionsSpawnTool(runtime)
	runsTool := NewSessionsRunsTool(runtime)
	agentsTool := NewAgentsListTool(runtime)
	return Tool{
		Name:        "session",
		Description: "Manage chat sessions. Actions: status (current session info), list (all sessions), history (read chat history), send (send message and wait), spawn (async agent run), runs (list/get/cancel async runs), agents (list available agents).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","list","history","send","spawn","runs","agents"]}
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
			case "status":
				return statusTool.Execute(ctx, json.RawMessage(`{}`))
			case "list":
				return listTool.Execute(ctx, json.RawMessage(`{}`))
			case "history":
				return historyTool.Execute(ctx, payload)
			case "send":
				return sendTool.Execute(ctx, payload)
			case "spawn":
				return spawnTool.Execute(ctx, payload)
			case "runs":
				return runsTool.Execute(ctx, payload)
			case "agents":
				return agentsTool.Execute(ctx, json.RawMessage(`{}`))
			default:
				return aggregatorError("action must be one of: status, list, history, send, spawn, runs, agents"), nil
			}
		},
	}
}
