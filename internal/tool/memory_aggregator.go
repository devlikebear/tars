package tool

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

// NewMemoryTool creates a single "memory" tool that dispatches to save, search,
// and get sub-actions. Replaces the individual memory_save, memory_search, and
// memory_get tools.
func NewMemoryTool(workspaceDir string, backend memory.Backend, nowFn func() time.Time) Tool {
	saveTool := NewMemorySaveTool(backend, nowFn)
	searchTool := NewMemorySearchTool(workspaceDir, backend)
	getTool := NewMemoryGetTool(workspaceDir, backend)
	return Tool{
		Name:        "memory",
		Description: "Manage long-term memory. Actions: save (store experiences/facts), search (find past context across memory and sessions), get (read daily logs, MEMORY.md, or experiences). For user profile info (name, language, preferences), use workspace tool instead.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["save","search","get"]}
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
			case "save":
				return saveTool.Execute(ctx, payload)
			case "search":
				return searchTool.Execute(ctx, payload)
			case "get":
				return getTool.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: save, search, get"), nil
			}
		},
	}
}
