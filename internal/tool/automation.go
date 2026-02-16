package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
)

type HeartbeatStatus struct {
	Configured       bool   `json:"configured"`
	ActiveHours      string `json:"active_hours,omitempty"`
	Timezone         string `json:"timezone,omitempty"`
	ChatBusy         bool   `json:"chat_busy,omitempty"`
	LastRunAt        string `json:"last_run_at,omitempty"`
	LastSkipped      bool   `json:"last_skipped,omitempty"`
	LastSkipReason   string `json:"last_skip_reason,omitempty"`
	LastLogged       bool   `json:"last_logged,omitempty"`
	LastAcknowledged bool   `json:"last_acknowledged,omitempty"`
	LastResponse     string `json:"last_response,omitempty"`
	LastError        string `json:"last_error,omitempty"`
}

type HeartbeatRunResult struct {
	Response     string    `json:"response,omitempty"`
	Skipped      bool      `json:"skipped,omitempty"`
	SkipReason   string    `json:"skip_reason,omitempty"`
	Logged       bool      `json:"logged,omitempty"`
	Acknowledged bool      `json:"acknowledged,omitempty"`
	RanAt        time.Time `json:"ran_at,omitempty"`
}

func NewCronListTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_list",
		Description: "List registered cron jobs managed by tarsd.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if store == nil {
				return automationErrorResult("cron store is not configured"), nil
			}
			jobs, err := store.List()
			if err != nil {
				return automationErrorResult(fmt.Sprintf("list cron jobs failed: %v", err)), nil
			}
			return jsonTextResult(map[string]any{
				"count": len(jobs),
				"jobs":  jobs,
			}, false), nil
		},
	}
}

func NewCronCreateTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_create",
		Description: "Create a cron job. Prompt is required.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "name":{"type":"string"},
    "prompt":{"type":"string"},
    "schedule":{"type":"string"},
    "enabled":{"type":"boolean"},
    "session_target":{"type":"string"},
    "wake_mode":{"type":"string"},
    "delivery_mode":{"type":"string"},
    "payload":{"type":"object"},
    "delete_after_run":{"type":"boolean"}
  },
  "required":["prompt"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return automationErrorResult("cron store is not configured"), nil
			}
			var input struct {
				Name           string          `json:"name"`
				Prompt         string          `json:"prompt"`
				Schedule       string          `json:"schedule"`
				Enabled        *bool           `json:"enabled,omitempty"`
				SessionTarget  string          `json:"session_target,omitempty"`
				WakeMode       string          `json:"wake_mode,omitempty"`
				DeliveryMode   string          `json:"delivery_mode,omitempty"`
				Payload        json.RawMessage `json:"payload,omitempty"`
				DeleteAfterRun bool            `json:"delete_after_run,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			hasEnable := input.Enabled != nil
			enabled := true
			if input.Enabled != nil {
				enabled = *input.Enabled
			}
			job, err := store.CreateWithOptions(cron.CreateInput{
				Name:           input.Name,
				Prompt:         input.Prompt,
				Schedule:       input.Schedule,
				Enabled:        enabled,
				HasEnable:      hasEnable,
				SessionTarget:  input.SessionTarget,
				WakeMode:       input.WakeMode,
				DeliveryMode:   input.DeliveryMode,
				Payload:        input.Payload,
				DeleteAfterRun: input.DeleteAfterRun,
			})
			if err != nil {
				return automationErrorResult(fmt.Sprintf("create cron job failed: %v", err)), nil
			}
			return jsonTextResult(job, false), nil
		},
	}
}

func NewCronUpdateTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_update",
		Description: "Update a cron job by id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "job_id":{"type":"string"},
    "name":{"type":"string"},
    "prompt":{"type":"string"},
    "schedule":{"type":"string"},
    "enabled":{"type":"boolean"},
    "session_target":{"type":"string"},
    "wake_mode":{"type":"string"},
    "delivery_mode":{"type":"string"},
    "payload":{"type":"object"},
    "delete_after_run":{"type":"boolean"}
  },
  "required":["job_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return automationErrorResult("cron store is not configured"), nil
			}
			var input struct {
				JobID          string           `json:"job_id"`
				Name           *string          `json:"name,omitempty"`
				Prompt         *string          `json:"prompt,omitempty"`
				Schedule       *string          `json:"schedule,omitempty"`
				Enabled        *bool            `json:"enabled,omitempty"`
				SessionTarget  *string          `json:"session_target,omitempty"`
				WakeMode       *string          `json:"wake_mode,omitempty"`
				DeliveryMode   *string          `json:"delivery_mode,omitempty"`
				Payload        *json.RawMessage `json:"payload,omitempty"`
				DeleteAfterRun *bool            `json:"delete_after_run,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			input.JobID = strings.TrimSpace(input.JobID)
			if input.JobID == "" {
				return automationErrorResult("job_id is required"), nil
			}
			job, err := store.Update(input.JobID, cron.UpdateInput{
				Name:           input.Name,
				Prompt:         input.Prompt,
				Schedule:       input.Schedule,
				Enabled:        input.Enabled,
				SessionTarget:  input.SessionTarget,
				WakeMode:       input.WakeMode,
				DeliveryMode:   input.DeliveryMode,
				Payload:        input.Payload,
				DeleteAfterRun: input.DeleteAfterRun,
			})
			if err != nil {
				return automationErrorResult(fmt.Sprintf("update cron job failed: %v", err)), nil
			}
			return jsonTextResult(job, false), nil
		},
	}
}

func NewCronDeleteTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_delete",
		Description: "Delete a cron job by id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"job_id":{"type":"string"}},
  "required":["job_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return automationErrorResult("cron store is not configured"), nil
			}
			var input struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			input.JobID = strings.TrimSpace(input.JobID)
			if input.JobID == "" {
				return automationErrorResult("job_id is required"), nil
			}
			if err := store.Delete(input.JobID); err != nil {
				return automationErrorResult(fmt.Sprintf("delete cron job failed: %v", err)), nil
			}
			return jsonTextResult(map[string]any{
				"job_id":  input.JobID,
				"deleted": true,
			}, false), nil
		},
	}
}

func NewCronRunTool(store *cron.Store, runJob func(ctx context.Context, job cron.Job) (string, error)) Tool {
	return Tool{
		Name:        "cron_run",
		Description: "Run a cron job immediately by id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"job_id":{"type":"string"}},
  "required":["job_id"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if store == nil || runJob == nil {
				return automationErrorResult("cron runner is not configured"), nil
			}
			var input struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			input.JobID = strings.TrimSpace(input.JobID)
			if input.JobID == "" {
				return automationErrorResult("job_id is required"), nil
			}
			job, err := store.Get(input.JobID)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("get cron job failed: %v", err)), nil
			}
			if !store.TryStartRun(job.ID) {
				return automationErrorResult("job is already running"), nil
			}
			defer store.FinishRun(job.ID)

			response, runErr := runJob(ctx, job)
			_, _ = store.MarkRunResult(job.ID, time.Now().UTC(), response, runErr)
			if runErr != nil {
				return automationErrorResult(fmt.Sprintf("run cron job failed: %v", runErr)), nil
			}
			return jsonTextResult(map[string]any{
				"job_id":     job.ID,
				"job_name":   job.Name,
				"job_prompt": job.Prompt,
				"response":   response,
			}, false), nil
		},
	}
}

func NewHeartbeatStatusTool(getStatus func(ctx context.Context) (HeartbeatStatus, error)) Tool {
	return Tool{
		Name:        "heartbeat_status",
		Description: "Return tarsd heartbeat runtime status and latest execution result.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			if getStatus == nil {
				return automationErrorResult("heartbeat status provider is not configured"), nil
			}
			status, err := getStatus(ctx)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("get heartbeat status failed: %v", err)), nil
			}
			return jsonTextResult(status, false), nil
		},
	}
}

func NewHeartbeatRunOnceTool(runOnce func(ctx context.Context) (HeartbeatRunResult, error)) Tool {
	return Tool{
		Name:        "heartbeat_run_once",
		Description: "Run heartbeat once immediately and return execution result.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			if runOnce == nil {
				return automationErrorResult("heartbeat runner is not configured"), nil
			}
			result, err := runOnce(ctx)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("run heartbeat failed: %v", err)), nil
			}
			return jsonTextResult(result, false), nil
		},
	}
}

func NewCronTool(store *cron.Store, runJob func(ctx context.Context, job cron.Job) (string, error)) Tool {
	listTool := NewCronListTool(store)
	createTool := NewCronCreateTool(store)
	updateTool := NewCronUpdateTool(store)
	deleteTool := NewCronDeleteTool(store)
	runTool := NewCronRunTool(store, runJob)
	return Tool{
		Name:        "cron",
		Description: "Manage cron jobs with actions: list, create, update, delete, run.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["list","create","update","delete","run"]}
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
			case "create":
				return createTool.Execute(ctx, payload)
			case "update":
				return updateTool.Execute(ctx, payload)
			case "delete":
				return deleteTool.Execute(ctx, payload)
			case "run":
				return runTool.Execute(ctx, payload)
			default:
				return automationErrorResult("action must be one of: list,create,update,delete,run"), nil
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
	return jsonTextResult(map[string]string{
		"error": strings.TrimSpace(message),
	}, true)
}
