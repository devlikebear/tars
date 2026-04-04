package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

// NewTasksTool creates a "tasks" aggregator tool for managing per-session
// plan and tasks. Actions: plan_set, plan_get, add, update, remove, list, clear.
func NewTasksTool(store *session.Store, workspaceDir string, getSessionID func() string) Tool {
	return Tool{
		Name: "tasks",
		Description: "Manage session-scoped plan and tasks. Actions: " +
			"plan_set (set session goal — archives previous plan), " +
			"plan_get (read current plan), " +
			"add (create a task), " +
			"update (change task status/title/description), " +
			"remove (delete a task), " +
			"list (show plan + all tasks with summary), " +
			"clear (reset plan and tasks). " +
			"Use for complex tasks with 3+ steps. Only ONE task in_progress at a time.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["plan_set","plan_get","add","update","remove","list","clear"]}
  },
  "required":["action"],
  "additionalProperties":true
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			payload, action, err := dispatchAction(params)
			if err != nil {
				return aggregatorError(err.Error()), nil
			}
			if store == nil || getSessionID == nil {
				return aggregatorError("session store is not configured"), nil
			}
			sessionID := getSessionID()
			if sessionID == "" {
				return aggregatorError("no active session"), nil
			}

			switch action {
			case "plan_set":
				return tasksPlanSet(store, workspaceDir, sessionID, payload)
			case "plan_get":
				return tasksPlanGet(store, sessionID)
			case "add":
				return tasksAdd(store, sessionID, payload)
			case "update":
				return tasksUpdate(store, sessionID, payload)
			case "remove":
				return tasksRemove(store, sessionID, payload)
			case "list":
				return tasksList(store, sessionID)
			case "clear":
				return tasksClear(store, workspaceDir, sessionID)
			default:
				return aggregatorError("action must be one of: plan_set, plan_get, add, update, remove, list, clear"), nil
			}
		},
	}
}

func tasksPlanSet(store *session.Store, workspaceDir string, sessionID string, params json.RawMessage) (Result, error) {
	var input struct {
		Goal        string `json:"goal"`
		Constraints string `json:"constraints,omitempty"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		return aggregatorError("goal is required"), nil
	}

	current, _ := store.GetTasks(sessionID)

	// Archive previous plan+tasks to memory if non-empty
	if current.Plan != nil || len(current.Tasks) > 0 {
		summary := session.ArchiveSummary(current)
		if summary != "" && workspaceDir != "" {
			_ = memory.AppendMemoryNote(workspaceDir, parseTimeOrNow(current.Plan), "[archived plan] "+summary)
		}
	}

	newTasks := session.SessionTasks{
		Plan: &session.Plan{
			Goal:        goal,
			Constraints: strings.TrimSpace(input.Constraints),
			CreatedAt:   session.NowRFC3339(),
		},
	}
	if err := store.SaveTasks(sessionID, newTasks); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"plan":     newTasks.Plan,
		"archived": current.Plan != nil,
	}, false), nil
}

func tasksPlanGet(store *session.Store, sessionID string) (Result, error) {
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	if st.Plan == nil {
		return JSONTextResult(map[string]any{"message": "no plan set for this session"}, false), nil
	}
	return JSONTextResult(map[string]any{
		"plan":    st.Plan,
		"summary": session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksAdd(store *session.Store, sessionID string, params json.RawMessage) (Result, error) {
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
		Status      string `json:"status,omitempty"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return aggregatorError("title is required"), nil
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "pending"
	}
	if !session.ValidTaskStatus(status) {
		return aggregatorError("status must be one of: pending, in_progress, completed, cancelled"), nil
	}

	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	task := session.Task{
		ID:          session.NextTaskID(st.Tasks),
		Title:       title,
		Status:      status,
		Description: strings.TrimSpace(input.Description),
	}
	st.Tasks = append(st.Tasks, task)
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"task":    task,
		"summary": session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksUpdate(store *session.Store, sessionID string, params json.RawMessage) (Result, error) {
	var input struct {
		ID          string  `json:"id"`
		Title       *string `json:"title,omitempty"`
		Status      *string `json:"status,omitempty"`
		Description *string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return aggregatorError("id is required"), nil
	}

	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	found := false
	for i := range st.Tasks {
		if st.Tasks[i].ID == id {
			if input.Title != nil {
				st.Tasks[i].Title = strings.TrimSpace(*input.Title)
			}
			if input.Status != nil {
				status := strings.ToLower(strings.TrimSpace(*input.Status))
				if !session.ValidTaskStatus(status) {
					return aggregatorError("status must be one of: pending, in_progress, completed, cancelled"), nil
				}
				st.Tasks[i].Status = status
			}
			if input.Description != nil {
				st.Tasks[i].Description = strings.TrimSpace(*input.Description)
			}
			found = true
			break
		}
	}
	if !found {
		return aggregatorError(fmt.Sprintf("task %q not found", id)), nil
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"updated": true,
		"summary": session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksRemove(store *session.Store, sessionID string, params json.RawMessage) (Result, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return aggregatorError("id is required"), nil
	}

	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	filtered := make([]session.Task, 0, len(st.Tasks))
	found := false
	for _, t := range st.Tasks {
		if t.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return aggregatorError(fmt.Sprintf("task %q not found", id)), nil
	}
	st.Tasks = filtered
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"removed": true,
		"summary": session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksList(store *session.Store, sessionID string) (Result, error) {
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"plan":    st.Plan,
		"tasks":   st.Tasks,
		"summary": session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksClear(store *session.Store, workspaceDir string, sessionID string) (Result, error) {
	current, _ := store.GetTasks(sessionID)

	// Archive to memory before clearing
	if current.Plan != nil || len(current.Tasks) > 0 {
		summary := session.ArchiveSummary(current)
		if summary != "" && workspaceDir != "" {
			_ = memory.AppendMemoryNote(workspaceDir, parseTimeOrNow(current.Plan), "[archived plan] "+summary)
		}
	}

	if err := store.SaveTasks(sessionID, session.SessionTasks{}); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"cleared": true,
	}, false), nil
}

func parseTimeOrNow(plan *session.Plan) time.Time {
	if plan != nil && plan.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, plan.CreatedAt); err == nil {
			return t
		}
	}
	return time.Now().UTC()
}
