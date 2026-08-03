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
			"contract_update (edit explicit goal/scope/done criteria/verification/artifacts), " +
			"contract_approve (mark the contract approved), " +
			"plan_get (read current plan + status), " +
			"add (create a task), " +
			"update (change task status/title/description; first in_progress auto-promotes proposed plans to executing; the plan flips to completed once every task is completed/cancelled), " +
			"remove (delete a task), " +
			"evidence_add (attach verification evidence to a task), " +
			"evidence_remove (remove evidence from a task), " +
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
    "action":{"type":"string","enum":["plan_set","contract_update","contract_approve","plan_get","add","update","remove","evidence_add","evidence_remove","list","clear","plan_propose","plan_approve","plan_pause","plan_resume","plan_abort"]}
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
			case "contract_update":
				return tasksContractUpdate(store, sessionID, payload)
			case "contract_approve":
				return tasksContractApprove(store, sessionID)
			case "plan_get":
				return tasksPlanGet(store, sessionID)
			case "add":
				return tasksAdd(store, sessionID, payload)
			case "update":
				return tasksUpdate(store, sessionID, payload)
			case "remove":
				return tasksRemove(store, sessionID, payload)
			case "evidence_add":
				return tasksEvidenceAdd(store, sessionID, payload)
			case "evidence_remove":
				return tasksEvidenceRemove(store, sessionID, payload)
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
				return aggregatorError("action must be one of: plan_set, contract_update, contract_approve, plan_get, add, update, remove, evidence_add, evidence_remove, list, clear, plan_propose, plan_approve, plan_pause, plan_resume, plan_abort"), nil
			}
		},
	}
}

type taskContractPayload struct {
	Goal                 string                   `json:"goal,omitempty"`
	Scope                string                   `json:"scope,omitempty"`
	DoneCriteria         []string                 `json:"done_criteria,omitempty"`
	VerificationCommands []string                 `json:"verification_commands,omitempty"`
	Artifacts            []string                 `json:"artifacts,omitempty"`
	ProofPolicy          *session.TaskProofPolicy `json:"proof_policy,omitempty"`
	Contract             *session.TaskContract    `json:"contract,omitempty"`
}

type taskEvidencePayload struct {
	TaskID     string                `json:"task_id,omitempty"`
	ID         string                `json:"id,omitempty"`
	EvidenceID string                `json:"evidence_id,omitempty"`
	Type       string                `json:"type,omitempty"`
	Title      string                `json:"title,omitempty"`
	Summary    string                `json:"summary,omitempty"`
	URL        string                `json:"url,omitempty"`
	Command    string                `json:"command,omitempty"`
	Path       string                `json:"path,omitempty"`
	Status     string                `json:"status,omitempty"`
	Evidence   *session.TaskEvidence `json:"evidence,omitempty"`
}

func tasksPlanSet(store *session.Store, workspaceDir string, sessionID string, params json.RawMessage) (Result, error) {
	var input struct {
		Goal        string `json:"goal"`
		Constraints string `json:"constraints,omitempty"`
		taskContractPayload
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
			_ = memory.AppendMemoryNote(workspaceDir, parseTimeOrNow(current.Plan), archivedPlanMemoryEntry(sessionID, summary))
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
		Contract: contractFromPayload(taskContractPayload(input.taskContractPayload), goal, input.Constraints, now),
	}
	if err := store.SaveTasks(sessionID, newTasks); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"plan":     newTasks.Plan,
		"contract": newTasks.Contract,
		"archived": current.Plan != nil,
	}, false), nil
}

func contractFromPayload(input taskContractPayload, fallbackGoal string, fallbackScope string, now string) *session.TaskContract {
	if input.Contract != nil {
		contract := *input.Contract
		if strings.TrimSpace(contract.Goal) == "" {
			contract.Goal = fallbackGoal
		}
		if strings.TrimSpace(contract.Scope) == "" {
			contract.Scope = fallbackScope
		}
		if strings.TrimSpace(contract.Status) == "" {
			contract.Status = session.ContractStatusDraft
		}
		if strings.TrimSpace(contract.CreatedAt) == "" {
			contract.CreatedAt = now
		}
		contract.UpdatedAt = now
		return &contract
	}
	scope := strings.TrimSpace(input.Scope)
	if scope == "" {
		scope = strings.TrimSpace(fallbackScope)
	}
	doneCriteria := input.DoneCriteria
	if len(doneCriteria) == 0 {
		doneCriteria = []string{"Planned tasks are completed or intentionally cancelled"}
	}
	return &session.TaskContract{
		Goal:                 strings.TrimSpace(firstNonEmpty(input.Goal, fallbackGoal)),
		Scope:                scope,
		DoneCriteria:         doneCriteria,
		VerificationCommands: input.VerificationCommands,
		Artifacts:            input.Artifacts,
		ProofPolicy:          input.ProofPolicy,
		Status:               session.ContractStatusDraft,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func tasksContractUpdate(store *session.Store, sessionID string, params json.RawMessage) (Result, error) {
	var input taskContractPayload
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	fallbackGoal := ""
	fallbackScope := ""
	if st.Plan != nil {
		fallbackGoal = st.Plan.Goal
		fallbackScope = st.Plan.Constraints
	}
	now := session.NowRFC3339()
	st.Contract = contractFromPayload(input, fallbackGoal, fallbackScope, now)
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"contract": st.Contract,
	}, false), nil
}

func tasksContractApprove(store *session.Store, sessionID string) (Result, error) {
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	if st.Contract == nil {
		fallbackGoal := ""
		fallbackScope := ""
		if st.Plan != nil {
			fallbackGoal = st.Plan.Goal
			fallbackScope = st.Plan.Constraints
		}
		st.Contract = contractFromPayload(taskContractPayload{}, fallbackGoal, fallbackScope, session.NowRFC3339())
	}
	st.Contract.Status = session.ContractStatusApproved
	st.Contract.UpdatedAt = session.NowRFC3339()
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"contract": st.Contract,
		"plan":     st.Plan,
		"summary":  session.TaskSummary(st.Tasks),
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
	if target == session.PlanStatusExecuting && st.Contract != nil && st.Contract.Status != session.ContractStatusApproved {
		st.Contract.Status = session.ContractStatusApproved
		st.Contract.UpdatedAt = st.Plan.UpdatedAt
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"plan":     st.Plan,
		"contract": st.Contract,
		"summary":  session.TaskSummary(st.Tasks),
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
		"plan":     st.Plan,
		"contract": st.Contract,
		"summary":  session.TaskSummary(st.Tasks),
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
		"task":     task,
		"plan":     st.Plan,
		"contract": st.Contract,
		"summary":  session.TaskSummary(st.Tasks),
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
		"updated":  true,
		"plan":     st.Plan,
		"contract": st.Contract,
		"summary":  session.TaskSummary(st.Tasks),
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
		"removed":  true,
		"plan":     st.Plan,
		"contract": st.Contract,
		"summary":  session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksEvidenceAdd(store *session.Store, sessionID string, params json.RawMessage) (Result, error) {
	var input taskEvidencePayload
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	taskID := strings.TrimSpace(firstNonEmpty(input.TaskID, input.ID))
	if taskID == "" {
		return aggregatorError("task_id is required"), nil
	}
	ev := taskEvidenceFromPayload(input)
	if ev.Type == "" {
		ev.Type = session.EvidenceTypeCommandOutputSummary
	}
	if !session.ValidEvidenceType(ev.Type) {
		return aggregatorError("evidence type must be one of: test_result, image, log_excerpt, pr_link, release_tag, command_output_summary"), nil
	}
	if evidenceEmpty(ev) {
		return aggregatorError("evidence requires title, summary, url, command, or path"), nil
	}

	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	ev.ID = session.NextEvidenceID(st.Tasks)
	if strings.TrimSpace(ev.CreatedAt) == "" {
		ev.CreatedAt = session.NowRFC3339()
	}
	for i := range st.Tasks {
		if st.Tasks[i].ID == taskID {
			st.Tasks[i].Evidence = append(st.Tasks[i].Evidence, ev)
			if err := store.SaveTasks(sessionID, st); err != nil {
				return aggregatorError(err.Error()), nil
			}
			return JSONTextResult(map[string]any{
				"task":     st.Tasks[i],
				"evidence": ev,
				"plan":     st.Plan,
				"contract": st.Contract,
				"summary":  session.TaskSummary(st.Tasks),
			}, false), nil
		}
	}
	return aggregatorError(fmt.Sprintf("task %q not found", taskID)), nil
}

func tasksEvidenceRemove(store *session.Store, sessionID string, params json.RawMessage) (Result, error) {
	var input taskEvidencePayload
	if err := json.Unmarshal(params, &input); err != nil {
		return aggregatorError(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	taskID := strings.TrimSpace(input.TaskID)
	evidenceID := strings.TrimSpace(firstNonEmpty(input.EvidenceID, input.ID))
	if evidenceID == "" {
		return aggregatorError("evidence_id is required"), nil
	}
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	for taskIdx := range st.Tasks {
		if taskID != "" && st.Tasks[taskIdx].ID != taskID {
			continue
		}
		filtered := make([]session.TaskEvidence, 0, len(st.Tasks[taskIdx].Evidence))
		var removed *session.TaskEvidence
		for _, ev := range st.Tasks[taskIdx].Evidence {
			if ev.ID == evidenceID {
				copy := ev
				removed = &copy
				continue
			}
			filtered = append(filtered, ev)
		}
		if removed == nil {
			continue
		}
		st.Tasks[taskIdx].Evidence = filtered
		if err := store.SaveTasks(sessionID, st); err != nil {
			return aggregatorError(err.Error()), nil
		}
		return JSONTextResult(map[string]any{
			"removed":  true,
			"task":     st.Tasks[taskIdx],
			"evidence": removed,
			"plan":     st.Plan,
			"contract": st.Contract,
			"summary":  session.TaskSummary(st.Tasks),
		}, false), nil
	}
	if taskID != "" {
		return aggregatorError(fmt.Sprintf("evidence %q not found on task %q", evidenceID, taskID)), nil
	}
	return aggregatorError(fmt.Sprintf("evidence %q not found", evidenceID)), nil
}

func taskEvidenceFromPayload(input taskEvidencePayload) session.TaskEvidence {
	ev := session.TaskEvidence{}
	if input.Evidence != nil {
		ev = *input.Evidence
	}
	if strings.TrimSpace(input.Type) != "" {
		ev.Type = input.Type
	}
	if strings.TrimSpace(input.Title) != "" {
		ev.Title = input.Title
	}
	if strings.TrimSpace(input.Summary) != "" {
		ev.Summary = input.Summary
	}
	if strings.TrimSpace(input.URL) != "" {
		ev.URL = input.URL
	}
	if strings.TrimSpace(input.Command) != "" {
		ev.Command = input.Command
	}
	if strings.TrimSpace(input.Path) != "" {
		ev.Path = input.Path
	}
	if strings.TrimSpace(input.Status) != "" {
		ev.Status = input.Status
	}
	ev.ID = strings.TrimSpace(ev.ID)
	ev.Type = strings.ToLower(strings.TrimSpace(ev.Type))
	ev.Title = strings.TrimSpace(ev.Title)
	ev.Summary = strings.TrimSpace(ev.Summary)
	ev.URL = strings.TrimSpace(ev.URL)
	ev.Command = strings.TrimSpace(ev.Command)
	ev.Path = strings.TrimSpace(ev.Path)
	ev.Status = strings.ToLower(strings.TrimSpace(ev.Status))
	ev.CreatedAt = strings.TrimSpace(ev.CreatedAt)
	return ev
}

func evidenceEmpty(ev session.TaskEvidence) bool {
	return strings.TrimSpace(ev.Title) == "" &&
		strings.TrimSpace(ev.Summary) == "" &&
		strings.TrimSpace(ev.URL) == "" &&
		strings.TrimSpace(ev.Command) == "" &&
		strings.TrimSpace(ev.Path) == ""
}

func tasksList(store *session.Store, sessionID string) (Result, error) {
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return aggregatorError(err.Error()), nil
	}
	return JSONTextResult(map[string]any{
		"plan":     st.Plan,
		"contract": st.Contract,
		"tasks":    st.Tasks,
		"summary":  session.TaskSummary(st.Tasks),
	}, false), nil
}

func tasksClear(store *session.Store, workspaceDir string, sessionID string) (Result, error) {
	current, _ := store.GetTasks(sessionID)

	// Archive to memory before clearing
	if current.Plan != nil || len(current.Tasks) > 0 {
		summary := session.ArchiveSummary(current)
		if summary != "" && workspaceDir != "" {
			_ = memory.AppendMemoryNote(workspaceDir, parseTimeOrNow(current.Plan), archivedPlanMemoryEntry(sessionID, summary))
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

func archivedPlanMemoryEntry(sessionID, summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "[archived plan] " + summary
	}
	return "[archived plan] session=" + sessionID + "\n" + summary
}
