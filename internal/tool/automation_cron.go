package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cron"
)

func validateAutonomousProjectSchedule(_, _ string) error {
	// Project validation removed along with project system.
	return nil
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
			return JSONTextResult(map[string]any{
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
    "session_id":{"type":"string"},
    "session_target":{"type":"string"},
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
				Title          string          `json:"title,omitempty"`
				Prompt         string          `json:"prompt"`
				Message        string          `json:"message,omitempty"`
				TaskType       string          `json:"task_type,omitempty"`
				Schedule       string          `json:"schedule"`
				Enabled        *bool           `json:"enabled,omitempty"`
				SessionID      string          `json:"session_id,omitempty"`
				SessionTarget  string          `json:"session_target,omitempty"`
				WakeMode       string          `json:"wake_mode,omitempty"`
				DeliveryMode   string          `json:"delivery_mode,omitempty"`
				Payload        json.RawMessage `json:"payload,omitempty"`
				DeleteAfterRun *bool           `json:"delete_after_run,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return automationErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			input.Name = resolveCronJobName(input.Name, input.Title, input.Message, input.Prompt)
			input.Prompt = resolveCronJobPrompt(input.Prompt, input.Message, input.Title, input.TaskType)
			if err := validateNaturalTaskPrompt(input.Prompt); err != nil {
				return automationErrorResult(err.Error()), nil
			}
			sessionID, sessionTarget, err := resolveCronScopeFromContext(ctx, input.SessionID, input.SessionTarget)
			if err != nil {
				return automationErrorResult(err.Error()), nil
			}
			payload, err := augmentCronPayloadFromContext(ctx, input.Payload, input.TaskType, input.Name, input.Prompt, input.Message)
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
				SessionID:         sessionID,
				SessionTarget:     sessionTarget,
				WakeMode:          input.WakeMode,
				DeliveryMode:      input.DeliveryMode,
				Payload:           payload,
				DeleteAfterRun:    input.DeleteAfterRun != nil && *input.DeleteAfterRun,
				HasDeleteAfterRun: input.DeleteAfterRun != nil,
			})
			if err != nil {
				return automationErrorResult(fmt.Sprintf("create cron job failed: %v", err)), nil
			}
			return JSONTextResult(job, false), nil
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
    "session_id":{"type":"string"},
    "session_target":{"type":"string"},
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
				Title          *string          `json:"title,omitempty"`
				Prompt         *string          `json:"prompt,omitempty"`
				Message        *string          `json:"message,omitempty"`
				TaskType       *string          `json:"task_type,omitempty"`
				Schedule       *string          `json:"schedule,omitempty"`
				Enabled        *bool            `json:"enabled,omitempty"`
				SessionID      *string          `json:"session_id,omitempty"`
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
			currentJob, err := store.Get(input.JobID)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("get cron job failed: %v", err)), nil
			}
			if input.Name == nil && input.Title != nil {
				input.Name = input.Title
			}
			if input.Prompt == nil && input.Message != nil {
				prompt := resolveCronJobPrompt("", *input.Message, derefString(input.Title), derefString(input.TaskType))
				input.Prompt = &prompt
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
			if input.SessionID != nil {
				resolved, err := resolveCronSessionBindingFromContext(ctx, *input.SessionID)
				if err != nil {
					return automationErrorResult(err.Error()), nil
				}
				input.SessionID = &resolved
			}
			payload, err := augmentCronPayloadFromContext(
				ctx,
				derefRawMessage(input.Payload, currentJob.Payload),
				derefString(input.TaskType),
				derefString(input.Name),
				derefString(input.Prompt),
				derefString(input.Message),
			)
			if err != nil {
				return automationErrorResult(err.Error()), nil
			}
			input.Payload = &payload
			job, err := store.Update(input.JobID, cron.UpdateInput{
				Name:           input.Name,
				Prompt:         input.Prompt,
				Schedule:       input.Schedule,
				Enabled:        input.Enabled,
				SessionID:      input.SessionID,
				SessionTarget:  input.SessionTarget,
				WakeMode:       input.WakeMode,
				DeliveryMode:   input.DeliveryMode,
				Payload:        input.Payload,
				DeleteAfterRun: input.DeleteAfterRun,
			})
			if err != nil {
				return automationErrorResult(fmt.Sprintf("update cron job failed: %v", err)), nil
			}
			return JSONTextResult(job, false), nil
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

func resolveCronSessionBindingFromContext(ctx context.Context, provided string) (string, error) {
	sessionID := strings.TrimSpace(provided)
	if sessionID == "" || strings.EqualFold(sessionID, "global") || strings.EqualFold(sessionID, "isolated") || strings.EqualFold(sessionID, "none") {
		return "", nil
	}
	if !strings.EqualFold(sessionID, "current") {
		return sessionID, nil
	}
	if strings.EqualFold(currentSessionKindFromContext(ctx), "main") {
		return "", nil
	}
	current := currentSessionIDFromContext(ctx)
	if current == "" {
		return "", fmt.Errorf("current session is not available in this context")
	}
	return current, nil
}

func resolveCronScopeFromContext(ctx context.Context, providedSessionID string, providedSessionTarget string) (string, string, error) {
	sessionID, err := resolveCronSessionBindingFromContext(ctx, providedSessionID)
	if err != nil {
		return "", "", err
	}
	sessionTarget, err := resolveCronSessionTargetFromContext(ctx, providedSessionTarget)
	if err != nil {
		return "", "", err
	}
	currentSessionID := currentSessionIDFromContext(ctx)
	currentSessionKind := strings.ToLower(strings.TrimSpace(currentSessionKindFromContext(ctx)))
	if currentSessionKind == "" {
		return sessionID, sessionTarget, nil
	}

	switch currentSessionKind {
	case "main":
		switch strings.ToLower(strings.TrimSpace(providedSessionID)) {
		case "", "current", "global", "isolated", "none":
			sessionID = ""
			if strings.TrimSpace(sessionTarget) == "" || strings.EqualFold(sessionTarget, "isolated") {
				sessionTarget = "main"
			}
		}
	default:
		if currentSessionID == "" {
			return sessionID, sessionTarget, nil
		}
		switch strings.ToLower(strings.TrimSpace(providedSessionID)) {
		case "", "current", "global", "isolated", "none":
			sessionID = currentSessionID
			if strings.TrimSpace(sessionTarget) == "" ||
				strings.EqualFold(sessionTarget, "isolated") ||
				strings.EqualFold(sessionTarget, "current") ||
				strings.EqualFold(sessionTarget, "main") {
				sessionTarget = ""
			}
		}
	}
	return sessionID, sessionTarget, nil
}

func resolveCronJobName(name string, title string, message string, prompt string) string {
	for _, candidate := range []string{name, title, message, prompt} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resolveCronJobPrompt(prompt string, message string, title string, taskType string) string {
	if trimmed := strings.TrimSpace(prompt); trimmed != "" {
		return trimmed
	}
	if strings.EqualFold(strings.TrimSpace(taskType), "reminder") {
		reminderMessage := inferReminderMessage("", message, title, "")
		if reminderMessage != "" {
			return "다음 알림을 보내기: " + reminderMessage
		}
	}
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(title)
}

func augmentCronPayloadFromContext(
	ctx context.Context,
	raw json.RawMessage,
	taskType string,
	name string,
	prompt string,
	message string,
) (json.RawMessage, error) {
	resolvedTaskType := inferCronTaskType(taskType, name, prompt, message)
	meta := cron.PayloadMeta{
		TaskType:          resolvedTaskType,
		ReminderMessage:   inferReminderMessage(resolvedTaskType, message, name, prompt),
		SourceSessionKind: currentSessionKindFromContext(ctx),
	}
	target := currentTelegramTargetFromContext(ctx)
	meta.TelegramChatID = target.ChatID
	meta.TelegramThreadID = target.ThreadID
	meta.TelegramBotID = target.BotID
	return cron.MergePayloadMeta(raw, meta)
}

func inferCronTaskType(taskType string, name string, prompt string, message string) string {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case "reminder", "alert", "notify", "notification":
		return "reminder"
	}
	combined := strings.ToLower(strings.Join([]string{name, prompt, message}, " "))
	if strings.Contains(combined, "알림") || strings.Contains(combined, "remind") || strings.Contains(combined, "notification") {
		return "reminder"
	}
	return ""
}

func inferReminderMessage(taskType string, message string, title string, prompt string) string {
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		return trimmed
	}
	if !strings.EqualFold(strings.TrimSpace(taskType), "reminder") {
		return ""
	}
	trimmed := strings.TrimSpace(prompt)
	trimmed = strings.TrimPrefix(trimmed, "다음 알림을 보내기:")
	trimmed = strings.TrimPrefix(trimmed, "Send this reminder:")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.TrimSuffix(trimmed, "알림 보내기")
	trimmed = strings.TrimSuffix(trimmed, "알림 전송하기")
	return strings.TrimSpace(trimmed)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func derefRawMessage(value *json.RawMessage, fallback json.RawMessage) json.RawMessage {
	if value == nil {
		return fallback
	}
	return *value
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
			return JSONTextResult(map[string]any{
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
			return JSONTextResult(map[string]any{
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
			return JSONTextResult(job, false), nil
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
			return JSONTextResult(map[string]any{
				"job_id": input.JobID,
				"count":  len(runs),
				"runs":   runs,
			}, false), nil
		},
	}
}
