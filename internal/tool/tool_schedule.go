package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/schedule"
)

func NewScheduleCreateTool(store *schedule.Store) Tool {
	return Tool{
		Name:        "schedule_create",
		Description: "Create a schedule item from natural language or explicit schedule.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "natural":{"type":"string"},
    "title":{"type":"string"},
    "prompt":{"type":"string"},
    "schedule":{"type":"string"},
    "project_id":{"type":"string"},
    "timezone":{"type":"string"}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "schedule store is not configured"}, true), nil
			}
			var input struct {
				Natural   string `json:"natural"`
				Title     string `json:"title,omitempty"`
				Prompt    string `json:"prompt,omitempty"`
				Schedule  string `json:"schedule,omitempty"`
				ProjectID string `json:"project_id,omitempty"`
				Timezone  string `json:"timezone,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if strings.TrimSpace(input.Prompt) != "" {
				if err := validateNaturalTaskPrompt(input.Prompt); err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				if err := validateAutonomousProjectSchedule(input.Prompt, input.ProjectID); err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
			}
			item, err := store.Create(schedule.CreateInput{
				Natural:   input.Natural,
				Title:     input.Title,
				Prompt:    input.Prompt,
				Schedule:  input.Schedule,
				ProjectID: input.ProjectID,
				Timezone:  input.Timezone,
			})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewScheduleListTool(store *schedule.Store) Tool {
	return Tool{
		Name:        "schedule_list",
		Description: "List schedule items.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(_ context.Context, _ json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "schedule store is not configured"}, true), nil
			}
			items, err := store.List()
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(items, false), nil
		},
	}
}

func NewScheduleUpdateTool(store *schedule.Store) Tool {
	return Tool{
		Name:        "schedule_update",
		Description: "Update a schedule item.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "schedule_id":{"type":"string"},
    "title":{"type":"string"},
    "prompt":{"type":"string"},
    "schedule":{"type":"string"},
    "status":{"type":"string"},
    "project_id":{"type":"string"},
    "timezone":{"type":"string"}
  },
  "required":["schedule_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "schedule store is not configured"}, true), nil
			}
			var input struct {
				ScheduleID string  `json:"schedule_id"`
				Title      *string `json:"title,omitempty"`
				Prompt     *string `json:"prompt,omitempty"`
				Schedule   *string `json:"schedule,omitempty"`
				Status     *string `json:"status,omitempty"`
				ProjectID  *string `json:"project_id,omitempty"`
				Timezone   *string `json:"timezone,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if input.Prompt != nil {
				if err := validateNaturalTaskPrompt(*input.Prompt); err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
			}
			current, err := store.Get(strings.TrimSpace(input.ScheduleID))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			effectivePrompt := current.Prompt
			if input.Prompt != nil {
				effectivePrompt = *input.Prompt
			}
			effectiveProjectID := current.ProjectID
			if input.ProjectID != nil {
				effectiveProjectID = *input.ProjectID
			}
			if strings.TrimSpace(effectivePrompt) != "" {
				if err := validateAutonomousProjectSchedule(effectivePrompt, effectiveProjectID); err != nil {
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
			}
			item, err := store.Update(strings.TrimSpace(input.ScheduleID), schedule.UpdateInput{
				Title:     input.Title,
				Prompt:    input.Prompt,
				Schedule:  input.Schedule,
				Status:    input.Status,
				ProjectID: input.ProjectID,
				Timezone:  input.Timezone,
			})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewScheduleDeleteTool(store *schedule.Store) Tool {
	return Tool{
		Name:        "schedule_delete",
		Description: "Delete a schedule item.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"schedule_id":{"type":"string"}},
  "required":["schedule_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "schedule store is not configured"}, true), nil
			}
			var input struct {
				ScheduleID string `json:"schedule_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if err := store.Delete(strings.TrimSpace(input.ScheduleID)); err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(map[string]any{"schedule_id": strings.TrimSpace(input.ScheduleID), "deleted": true}, false), nil
		},
	}
}

func NewScheduleCompleteTool(store *schedule.Store) Tool {
	return Tool{
		Name:        "schedule_complete",
		Description: "Mark a schedule item as completed.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"schedule_id":{"type":"string"}},
  "required":["schedule_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return JSONTextResult(map[string]any{"message": "schedule store is not configured"}, true), nil
			}
			var input struct {
				ScheduleID string `json:"schedule_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.Complete(strings.TrimSpace(input.ScheduleID))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}
