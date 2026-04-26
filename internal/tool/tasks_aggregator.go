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
// plan and tasks. The plan follows a small state machine
// (drafting → proposed → executing ↔ paused, terminal: completed/aborted)
// so the chat UI can offer review/approve and runtime intervention without
// extra round-trips.
func NewTasksTool(store *session.Store, workspaceDir string, getSessionID func() string) Tool {
	return Tool{
		Name: "tasks",
		Description: "Manage session-scoped plan and tasks. Actions: " +
			"plan_set (set session goal — archives previous plan, status=drafting), " +
			"plan_get (read current plan + status), " +
			"add (create a task), " +
			"update (change task status/title/description; first in_progress auto-promotes proposed plans to executing; the plan flips to completed once every task is completed/cancelled), " +
			"remove (delete a task), " +
			"list (show plan + all tasks with summary), " +
			"clear (reset plan and tasks), " +
			"plan_propose (drafting → proposed; signal plan is ready for user review), " +
			"plan_approve (proposed → executing; user has approved, start running), " +
			"plan_pause (executing → paused; halt execution), " +
			"plan_resume (paused → executing; continue execution), " +
			"plan_abort (any except completed/aborted → aborted). " +
			"Use for complex tasks with 3+ steps. Only ONE task in_progress at a time.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["plan_set","plan_get","add","update","remove","list","clear","plan_propose","plan_approve","plan_pause","plan_resume","plan_abort"]}
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
			case "plan_propose":
				return tasksPlanTransition(store, sessionID, session.PlanStatusProposed,
					[]string{session.PlanStatusDrafting})
			case "plan_approve":
				return tasksPlanTransition(store, sessionID, session.PlanStatusExecuting,
					[]string{session.PlanStatusProposed})
			case "plan_pause":
				return tasksPlanTransition(store, sessionID, session.PlanStatusPaused,
					[]string{session.PlanStatusExecuting})
			case "plan_resume":
				return tasksPlanTransition(store, sessionID, session.PlanStatusExecuting,
					[]string{session.PlanStatusPaused})
			case "plan_abort":
				return tasksPlanTransition(store, sessionID, session.PlanStatusAborted,
					[]string{
						session.PlanStatusDrafting,
						session.PlanStatusProposed,
						session.PlanStatusExecuting,
						session.PlanStatusPaused,
					})
			default:
				return aggregatorError("action must be one of: plan_set, plan_get, add, update, remove, list, clear, plan_propose, plan_approve, plan_pause, plan_resume, plan_abort"), nil
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

	now := session.NowRFC3339()
	newTasks := session.SessionTasks{
		Plan: &session.Plan{
			Goal:        goal,
			Constraints: strings.TrimSpace(input.Constraints),
			CreatedAt:   now,
			Status:      session.PlanStatusDrafting,
			UpdatedAt:   now,
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

// tasksPlanTransition is the shared implementation for plan_propose /
// plan_approve / plan_pause / plan_resume / plan_abort. allowedFrom lists
// the statuses from which the transition is valid; anything else returns
// an error so the LLM (or test) sees an explicit reason rather than a
// silent no-op.
func tasksPlanTransition(store *session.Store, sessionID, target string, allowedFrom []string) (Result, error) {
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	if st.Plan == nil {
		return aggregatorError("no plan exists; call plan_set first"), nil
	}
	current := strings.TrimSpace(st.Plan.Status)
	if current == "" {
		current = session.PlanStatusExecuting // legacy default
	}
	allowed := false
	for _, from := range allowedFrom {
		if current == from {
			allowed = true
			break
		}
	}
	if !allowed {
		return aggregatorError(fmt.Sprintf(
			"cannot transition plan from %q to %q (allowed from: %s)",
			current, target, strings.Join(allowedFrom, ", "),
		)), nil
	}
	st.Plan.Status = target
	st.Plan.UpdatedAt = session.NowRFC3339()
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"plan":    st.Plan,
		"summary": session.TaskSummary(st.Tasks),
	}, false), nil
}

// applyAutoTransitions reflects task-driven plan-status changes. Two rules:
//
//  1. If the plan is still "proposed" but a task has just become
//     in_progress, the LLM clearly skipped plan_approve — treat the work
//     as live and flip the plan to executing.
//  2. If at least one task exists and every task is either completed or
//     cancelled, mark the plan completed. (Aborted/completed plans are
//     terminal and untouched.)
//
// Returns true when Status changed so callers can refresh UpdatedAt.
func applyAutoTransitions(plan *session.Plan, tasks []session.Task) bool {
	if plan == nil {
		return false
	}
	current := strings.TrimSpace(plan.Status)
	if current == session.PlanStatusCompleted || current == session.PlanStatusAborted {
		return false
	}

	changed := false

	// Rule 1: any task in_progress while plan == proposed → executing.
	if current == session.PlanStatusProposed {
		for _, t := range tasks {
			if t.Status == "in_progress" {
				plan.Status = session.PlanStatusExecuting
				current = session.PlanStatusExecuting
				changed = true
				break
			}
		}
	}

	// Rule 2: all tasks finished → completed.
	if len(tasks) > 0 && current != session.PlanStatusCompleted {
		allDone := true
		for _, t := range tasks {
			if t.Status != "completed" && t.Status != "cancelled" {
				allDone = false
				break
			}
		}
		if allDone {
			plan.Status = session.PlanStatusCompleted
			changed = true
		}
	}

	return changed
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
	if applyAutoTransitions(st.Plan, st.Tasks) {
		st.Plan.UpdatedAt = session.NowRFC3339()
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"task":    task,
		"plan":    st.Plan,
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
	if applyAutoTransitions(st.Plan, st.Tasks) {
		st.Plan.UpdatedAt = session.NowRFC3339()
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"updated": true,
		"plan":    st.Plan,
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
	if applyAutoTransitions(st.Plan, st.Tasks) {
		st.Plan.UpdatedAt = session.NowRFC3339()
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"removed": true,
		"plan":    st.Plan,
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
