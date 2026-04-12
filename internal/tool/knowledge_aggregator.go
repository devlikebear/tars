package tool

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/tars/internal/memory"
)

// NewKnowledgeTool creates a single "knowledge" tool that dispatches to list,
// get, upsert, and delete sub-actions for wiki-style knowledge base notes.
// Replaces the individual memory_kb_list/get/upsert/delete tools.
func NewKnowledgeTool(backend memory.Backend) Tool {
	listTool := NewMemoryKBListTool(backend)
	getTool := NewMemoryKBGetTool(backend)
	upsertTool := NewMemoryKBUpsertTool(backend)
	deleteTool := NewMemoryKBDeleteTool(backend)
	return Tool{
		Name:        "knowledge",
		Description: "Manage wiki-style knowledge base notes. Actions: list (browse notes), get (read one note by slug), upsert (create or update a note), delete (remove a note).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","get","upsert","delete"]}
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
			case "list":
				return listTool.Execute(ctx, payload)
			case "get":
				return getTool.Execute(ctx, payload)
			case "upsert":
				return upsertTool.Execute(ctx, payload)
			case "delete":
				return deleteTool.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: list, get, upsert, delete"), nil
			}
		},
	}
}
