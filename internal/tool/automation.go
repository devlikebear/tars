package tool

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/tars/internal/cron"
)

func NewCronTool(store *cron.Store, runJob func(ctx context.Context, job cron.Job) (string, error)) Tool {
	listTool := NewCronListTool(store)
	getTool := NewCronGetTool(store)
	runsTool := NewCronRunsTool(store)
	createTool := NewCronCreateTool(store)
	updateTool := NewCronUpdateTool(store)
	deleteTool := NewCronDeleteTool(store)
	runTool := NewCronRunTool(store, runJob)
	return Tool{
		Name:        "cron",
		Description: "Manage cron jobs with actions: list, get, runs, create, update, delete, run.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","get","runs","create","update","delete","run"]}
  },
  "required":["action"],
  "additionalProperties":true
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			payload, action, err := normalizeAutomationActionInput(params)
			if err != nil {
				return automationErrorResult(err.Error()), nil
			}
			switch action {
			case "list":
				return listTool.Execute(ctx, json.RawMessage(`{}`))
			case "get":
				return getTool.Execute(ctx, payload)
			case "runs":
				return runsTool.Execute(ctx, payload)
			case "create":
				return createTool.Execute(ctx, payload)
			case "update":
				return updateTool.Execute(ctx, payload)
			case "delete":
				return deleteTool.Execute(ctx, payload)
			case "run":
				return runTool.Execute(ctx, payload)
			default:
				return automationErrorResult("action must be one of: list,get,runs,create,update,delete,run"), nil
			}
		},
	}
}

func normalizeAutomationActionInput(params json.RawMessage) (json.RawMessage, string, error) {
	return dispatchAction(params, aliasAutomationJobID)
}

// aliasAutomationJobID promotes the legacy `id` field to `job_id` when
// callers passed only the alias. The original `id` is dropped so downstream
// schemas see exactly one identifier.
func aliasAutomationJobID(payload map[string]json.RawMessage) {
	if _, ok := payload["job_id"]; !ok {
		if id, ok := payload["id"]; ok {
			payload["job_id"] = id
		}
	}
	delete(payload, "id")
}

func automationErrorResult(message string) Result {
	return JSONTextResult(map[string]string{
		"error": strings.TrimSpace(message),
	}, true)
}
