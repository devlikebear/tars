package tool

import (
	"context"
	"encoding/json"
	"fmt"
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

func NewHeartbeatTool(
	getStatus func(ctx context.Context) (HeartbeatStatus, error),
	runOnce func(ctx context.Context) (HeartbeatRunResult, error),
) Tool {
	statusTool := NewHeartbeatStatusTool(getStatus)
	runTool := NewHeartbeatRunOnceTool(runOnce)
	return Tool{
		Name:        "heartbeat",
		Description: "Manage heartbeat with actions: status, run_once.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","run_once","run"]}
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
			case "status":
				return statusTool.Execute(ctx, json.RawMessage(`{}`))
			case "run_once", "run":
				return runTool.Execute(ctx, payload)
			default:
				return automationErrorResult("action must be one of: status,run_once"), nil
			}
		},
	}
}

func normalizeAutomationActionInput(params json.RawMessage) (json.RawMessage, string, error) {
	raw := strings.TrimSpace(string(params))
	if raw == "" || raw == "null" {
		return nil, "", fmt.Errorf("action is required")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, "", fmt.Errorf("invalid arguments: %v", err)
	}
	if payload == nil {
		payload = map[string]json.RawMessage{}
	}
	var action string
	if v, ok := payload["action"]; ok {
		if err := json.Unmarshal(v, &action); err != nil {
			return nil, "", fmt.Errorf("action must be string")
		}
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return nil, "", fmt.Errorf("action is required")
	}
	delete(payload, "action")
	if _, ok := payload["job_id"]; !ok {
		if id, ok := payload["id"]; ok {
			payload["job_id"] = id
		}
	}
	delete(payload, "id")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal normalized payload: %v", err)
	}
	return normalized, action, nil
}

func automationErrorResult(message string) Result {
	return JSONTextResult(map[string]string{
		"error": strings.TrimSpace(message),
	}, true)
}
