package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
)

func validateAutonomousProjectSchedule(prompt, projectID string) error {
	if !strings.Contains(prompt, "brief_id=") {
		return nil
	}
	if strings.TrimSpace(projectID) != "" {
		return nil
	}
	return fmt.Errorf("autonomous project work requires a finalized project: brief를 먼저 finalize하고 project_id로 예약하세요")
}

func NewCronListTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_list",
		Description: "List registered cron jobs managed by tars.",
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
    "project_id":{"type":"string"},
    "wake_mode":{"type":"string"},
    "delivery_mode":{"type":"string"},
    "payload":{"type":"object"},
    "delete_after_run":{"type":"boolean"}
  },
  "required":["prompt"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return automationErrorResult("cron store is not configured"), nil
			}
			var input struct {
				Name           string          `json:"name"`
				Prompt         string          `json:"prompt"`
				Schedule       string          `json:"schedule"`
				Enabled        *bool           `json:"enabled,omitempty"`
				SessionTarget  string          `json:"session_target,omitempty"`
				ProjectID      string          `json:"project_id,omitempty"`
				WakeMode       string          `json:"wake_mode,omitempty"`
				DeliveryMode   string          `json:"delivery_mode,omitempty"`
				Payload        json.RawMessage `json:"payload,omitempty"`
				DeleteAfterRun *bool           `json:"delete_after_run,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if err := validateNaturalTaskPrompt(input.Prompt); err != nil {
				return automationErrorResult(err.Error()), nil
			}
			if err := validateAutonomousProjectSchedule(input.Prompt, input.ProjectID); err != nil {
				return automationErrorResult(err.Error()), nil
			}
			sessionTarget, err := resolveCronSessionTargetFromContext(ctx, input.SessionTarget)
			if err != nil {
				return automationErrorResult(err.Error()), nil
			}
			hasEnable := input.Enabled != nil
			enabled := true
			if input.Enabled != nil {
				enabled = *input.Enabled
			}
			job, err := store.CreateWithOptions(cron.CreateInput{
				Name:              input.Name,
				Prompt:            input.Prompt,
				Schedule:          input.Schedule,
				Enabled:           enabled,
				HasEnable:         hasEnable,
				SessionTarget:     sessionTarget,
				ProjectID:         input.ProjectID,
				WakeMode:          input.WakeMode,
				DeliveryMode:      input.DeliveryMode,
				Payload:           input.Payload,
				DeleteAfterRun:    input.DeleteAfterRun != nil && *input.DeleteAfterRun,
				HasDeleteAfterRun: input.DeleteAfterRun != nil,
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
    "project_id":{"type":"string"},
    "wake_mode":{"type":"string"},
    "delivery_mode":{"type":"string"},
    "payload":{"type":"object"},
    "delete_after_run":{"type":"boolean"}
  },
  "required":["job_id"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
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
				ProjectID      *string          `json:"project_id,omitempty"`
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
			current, err := store.Get(input.JobID)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("get cron job failed: %v", err)), nil
			}
			if input.Prompt != nil {
				if err := validateNaturalTaskPrompt(*input.Prompt); err != nil {
					return automationErrorResult(err.Error()), nil
				}
			}
			if input.SessionTarget != nil {
				resolved, err := resolveCronSessionTargetFromContext(ctx, *input.SessionTarget)
				if err != nil {
					return automationErrorResult(err.Error()), nil
				}
				input.SessionTarget = &resolved
			}
			effectivePrompt := current.Prompt
			if input.Prompt != nil {
				effectivePrompt = *input.Prompt
			}
			effectiveProjectID := current.ProjectID
			if input.ProjectID != nil {
				effectiveProjectID = *input.ProjectID
			}
			if err := validateAutonomousProjectSchedule(effectivePrompt, effectiveProjectID); err != nil {
				return automationErrorResult(err.Error()), nil
			}
			job, err := store.Update(input.JobID, cron.UpdateInput{
				Name:           input.Name,
				Prompt:         input.Prompt,
				Schedule:       input.Schedule,
				Enabled:        input.Enabled,
				SessionTarget:  input.SessionTarget,
				ProjectID:      input.ProjectID,
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

func resolveCronSessionTargetFromContext(ctx context.Context, provided string) (string, error) {
	_ = ctx
	sessionTarget := strings.TrimSpace(provided)
	if !strings.EqualFold(sessionTarget, "current") {
		return sessionTarget, nil
	}
	return "main", nil
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

func NewCronGetTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_get",
		Description: "Get a cron job by id.",
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
			job, err := store.Get(input.JobID)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("get cron job failed: %v", err)), nil
			}
			return jsonTextResult(job, false), nil
		},
	}
}

func NewCronRunsTool(store *cron.Store) Tool {
	return Tool{
		Name:        "cron_runs",
		Description: "List cron run history by job id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "job_id":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":500,"default":50}
  },
  "required":["job_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return automationErrorResult("cron store is not configured"), nil
			}
			var input struct {
				JobID string `json:"job_id"`
				Limit int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			input.JobID = strings.TrimSpace(input.JobID)
			if input.JobID == "" {
				return automationErrorResult("job_id is required"), nil
			}
			if input.Limit <= 0 {
				input.Limit = 50
			}
			runs, err := store.ListRuns(input.JobID, input.Limit)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("list cron runs failed: %v", err)), nil
			}
			return jsonTextResult(map[string]any{
				"job_id": input.JobID,
				"count":  len(runs),
				"runs":   runs,
			}, false), nil
		},
	}
}
