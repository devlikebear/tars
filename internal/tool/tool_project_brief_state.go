package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func NewProjectBriefGetTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_brief_get",
		Description: "Read the resumable project brief for the current or specified session.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"brief_id":{"type":"string"}},
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				BriefID string `json:"brief_id,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			briefID := resolveBriefIDFromContext(ctx, input.BriefID)
			if briefID == "" {
				return jsonTextResult(map[string]any{"message": "brief_id is required"}, true), nil
			}
			item, err := store.GetBrief(briefID)
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(item, false), nil
		},
	}
}

func NewProjectBriefUpdateTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_brief_update",
		Description: "Create or update the resumable project brief for the current or specified session.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "brief_id":{"type":"string"},
    "title":{"type":"string"},
    "goal":{"type":"string"},
    "kind":{"type":"string"},
    "genre":{"type":"string"},
    "target_length":{"type":"string"},
    "cadence":{"type":"string"},
    "target_installments":{"type":"string"},
    "premise":{"type":"string"},
    "plot_seed":{"type":"string"},
    "style_preferences":{"type":"string"},
    "constraints":{"type":"array","items":{"type":"string"}},
    "must_have":{"type":"array","items":{"type":"string"}},
    "must_avoid":{"type":"array","items":{"type":"string"}},
    "open_questions":{"type":"array","items":{"type":"string"}},
    "decisions":{"type":"array","items":{"type":"string"}},
    "status":{"type":"string"},
    "summary":{"type":"string"},
    "body":{"type":"string"}
  },
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				BriefID            string   `json:"brief_id,omitempty"`
				Title              *string  `json:"title,omitempty"`
				Goal               *string  `json:"goal,omitempty"`
				Kind               *string  `json:"kind,omitempty"`
				Genre              *string  `json:"genre,omitempty"`
				TargetLength       *string  `json:"target_length,omitempty"`
				Cadence            *string  `json:"cadence,omitempty"`
				TargetInstallments *string  `json:"target_installments,omitempty"`
				Premise            *string  `json:"premise,omitempty"`
				PlotSeed           *string  `json:"plot_seed,omitempty"`
				StylePreferences   *string  `json:"style_preferences,omitempty"`
				Constraints        []string `json:"constraints,omitempty"`
				MustHave           []string `json:"must_have,omitempty"`
				MustAvoid          []string `json:"must_avoid,omitempty"`
				OpenQuestions      []string `json:"open_questions,omitempty"`
				Decisions          []string `json:"decisions,omitempty"`
				Status             *string  `json:"status,omitempty"`
				Summary            *string  `json:"summary,omitempty"`
				Body               *string  `json:"body,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			briefID := resolveBriefIDFromContext(ctx, input.BriefID)
			if briefID == "" {
				return jsonTextResult(map[string]any{"message": "brief_id is required"}, true), nil
			}
			item, err := store.UpdateBrief(briefID, project.BriefUpdateInput{
				Title:              input.Title,
				Goal:               input.Goal,
				Kind:               input.Kind,
				Genre:              input.Genre,
				TargetLength:       input.TargetLength,
				Cadence:            input.Cadence,
				TargetInstallments: input.TargetInstallments,
				Premise:            input.Premise,
				PlotSeed:           input.PlotSeed,
				StylePreferences:   input.StylePreferences,
				Constraints:        input.Constraints,
				MustHave:           input.MustHave,
				MustAvoid:          input.MustAvoid,
				OpenQuestions:      input.OpenQuestions,
				Decisions:          input.Decisions,
				Status:             input.Status,
				Summary:            input.Summary,
				Body:               input.Body,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(item, false), nil
		},
	}
}

func NewProjectBriefFinalizeTool(store *project.Store, sessionStore *session.Store) Tool {
	return Tool{
		Name:        "project_brief_finalize",
		Description: "Finalize a brief into a project, seed starter docs, and activate the project for the session.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"brief_id":{"type":"string"}},
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				BriefID string `json:"brief_id,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			briefID := resolveBriefIDFromContext(ctx, input.BriefID)
			if briefID == "" {
				return jsonTextResult(map[string]any{"message": "brief_id is required"}, true), nil
			}
			created, brief, err := store.FinalizeBrief(briefID, sessionStore)
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(map[string]any{
				"project":  created,
				"brief":    brief,
				"seeded":   true,
				"brief_id": briefID,
			}, false), nil
		},
	}
}

func NewProjectStateGetTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_state_get",
		Description: "Read STATE.md for a project.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"project_id":{"type":"string"}},
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.GetState(strings.TrimSpace(input.ProjectID))
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(item, false), nil
		},
	}
}

func NewProjectStateUpdateTool(store *project.Store) Tool {
	return Tool{
		Name:        "project_state_update",
		Description: "Create or update STATE.md for a project.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "goal":{"type":"string"},
    "phase":{"type":"string"},
    "status":{"type":"string"},
    "next_action":{"type":"string"},
    "remaining_tasks":{"type":"array","items":{"type":"string"}},
    "completion_summary":{"type":"string"},
    "last_run_summary":{"type":"string"},
    "last_run_at":{"type":"string"},
    "stop_reason":{"type":"string"},
    "body":{"type":"string"}
  },
  "required":["project_id"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if store == nil {
				return jsonTextResult(map[string]any{"message": "project store is not configured"}, true), nil
			}
			var input struct {
				ProjectID         string   `json:"project_id"`
				Goal              *string  `json:"goal,omitempty"`
				Phase             *string  `json:"phase,omitempty"`
				Status            *string  `json:"status,omitempty"`
				NextAction        *string  `json:"next_action,omitempty"`
				RemainingTasks    []string `json:"remaining_tasks,omitempty"`
				CompletionSummary *string  `json:"completion_summary,omitempty"`
				LastRunSummary    *string  `json:"last_run_summary,omitempty"`
				LastRunAt         *string  `json:"last_run_at,omitempty"`
				StopReason        *string  `json:"stop_reason,omitempty"`
				Body              *string  `json:"body,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := store.UpdateState(strings.TrimSpace(input.ProjectID), project.ProjectStateUpdateInput{
				Goal:              input.Goal,
				Phase:             input.Phase,
				Status:            input.Status,
				NextAction:        input.NextAction,
				RemainingTasks:    input.RemainingTasks,
				CompletionSummary: input.CompletionSummary,
				LastRunSummary:    input.LastRunSummary,
				LastRunAt:         input.LastRunAt,
				StopReason:        input.StopReason,
				Body:              input.Body,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(item, false), nil
		},
	}
}

func resolveBriefIDFromContext(ctx context.Context, provided string) string {
	meta := usage.CallMetaFromContext(ctx)
	sessionID := strings.TrimSpace(meta.SessionID)
	briefID := strings.TrimSpace(provided)
	if briefID == "" {
		return sessionID
	}
	if strings.EqualFold(briefID, "current") {
		return sessionID
	}
	return briefID
}
