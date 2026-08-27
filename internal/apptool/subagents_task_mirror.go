package apptool

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

// SubagentsTaskMirrorConfig wires subagent planning/orchestration progress into
// the same per-session tasks file used by the Console Tasks panel.
type SubagentsTaskMirrorConfig struct {
	Store        *session.Store
	WorkspaceDir string
	GetSessionID func() string
}

type subagentsTaskMirror struct {
	store        *session.Store
	workspaceDir string
	getSessionID func() string
}

func newSubagentsTaskMirror(configs []SubagentsTaskMirrorConfig) subagentsTaskMirror {
	if len(configs) == 0 {
		return subagentsTaskMirror{}
	}
	cfg := configs[0]
	return subagentsTaskMirror{
		store:        cfg.Store,
		workspaceDir: cfg.WorkspaceDir,
		getSessionID: cfg.GetSessionID,
	}
}

func (m subagentsTaskMirror) sessionID() string {
	if m.store == nil || m.getSessionID == nil {
		return ""
	}
	return strings.TrimSpace(m.getSessionID())
}

func (m subagentsTaskMirror) mirrorPlan(goal, flowID string, steps []subagentFlowStepInput) error {
	sessionID := m.sessionID()
	if sessionID == "" {
		return nil
	}

	current, err := m.store.GetTasks(sessionID)
	if err != nil {
		return err
	}
	if current.Plan != nil || len(current.Tasks) > 0 {
		summary := session.ArchiveSummary(current)
		if summary != "" && m.workspaceDir != "" {
			_ = memory.AppendMemoryNote(m.workspaceDir, parseTimeOrNow(current.Plan), archivedPlanMemoryEntry(sessionID, summary))
		}
	}

	now := session.NowRFC3339()
	next := session.SessionTasks{
		Plan: &session.Plan{
			Goal:        strings.TrimSpace(goal),
			Constraints: subagentPlanConstraints(flowID),
			CreatedAt:   now,
			Status:      session.PlanStatusDrafting,
			UpdatedAt:   now,
		},
		Tasks: []session.Task{},
	}
	if next.Plan.Goal == "" {
		next.Plan.Goal = subagentFlowGoal(flowID)
	}
	appendSubagentTasks(&next, flowID, steps)
	return m.store.SaveTasks(sessionID, next)
}

func (m subagentsTaskMirror) ensurePlan(flowID string, steps []subagentFlowStepInput) error {
	sessionID := m.sessionID()
	if sessionID == "" {
		return nil
	}

	current, err := m.store.GetTasks(sessionID)
	if err != nil {
		return err
	}
	if current.Plan == nil || !sessionTasksContainFlow(current.Tasks, flowID) {
		return m.mirrorPlan(subagentFlowGoal(flowID), flowID, steps)
	}

	changed := false
	for _, step := range steps {
		stepID := normalizedSubagentStepID(step)
		for _, task := range step.Tasks {
			taskID := strings.TrimSpace(task.ID)
			if taskID == "" || findSubagentTaskIndex(current.Tasks, flowID, taskID) >= 0 {
				continue
			}
			current.Tasks = append(current.Tasks, session.Task{
				ID:          session.NextTaskID(current.Tasks),
				Title:       subagentMirrorTaskTitle(taskID, task.Title),
				Status:      "pending",
				Description: buildSubagentTaskDescription(flowID, stepID, taskID, "", "", "", ""),
			})
			changed = true
		}
	}
	if changed && current.Plan != nil {
		current.Plan.UpdatedAt = session.NowRFC3339()
	}
	if !changed {
		return nil
	}
	return m.store.SaveTasks(sessionID, current)
}

func (m subagentsTaskMirror) markTaskInProgress(flowID, stepID, taskID, title string, run agentruntime.Run) error {
	return m.updateTask(flowID, stepID, taskID, title, func(st *session.SessionTasks, task *session.Task) {
		task.Status = "in_progress"
		task.Description = buildSubagentTaskDescription(flowID, stepID, taskID, strings.TrimSpace(run.ID), "", "", "")
		if st.Plan != nil && st.Plan.Status != session.PlanStatusCompleted && st.Plan.Status != session.PlanStatusAborted {
			st.Plan.Status = session.PlanStatusExecuting
			st.Plan.UpdatedAt = session.NowRFC3339()
		}
	})
}

func (m subagentsTaskMirror) markTaskFinal(flowID, stepID, taskID, title string, final agentruntime.Run) error {
	status := "completed"
	errText := ""
	summary := trimSubagentSummary(final.Response, 220)
	if final.Status != agentruntime.RunStatusCompleted {
		status = "cancelled"
		errText = strings.TrimSpace(final.Error)
		if errText == "" {
			errText = string(final.Status)
		}
		if summary == "" {
			summary = trimSubagentSummary(final.Error, 220)
		}
	}
	runID := strings.TrimSpace(final.ID)
	return m.updateTask(flowID, stepID, taskID, title, func(st *session.SessionTasks, task *session.Task) {
		task.Status = status
		task.Description = buildSubagentTaskDescription(flowID, stepID, taskID, runID, summary, errText, "")
		if applyAutoTransitions(st.Plan, st.Tasks) && st.Plan != nil {
			st.Plan.UpdatedAt = session.NowRFC3339()
		}
	})
}

func (m subagentsTaskMirror) markTaskCancelled(flowID, stepID, taskID, title, errText string) error {
	return m.updateTask(flowID, stepID, taskID, title, func(st *session.SessionTasks, task *session.Task) {
		task.Status = "cancelled"
		task.Description = buildSubagentTaskDescription(flowID, stepID, taskID, "", "", strings.TrimSpace(errText), "")
		if applyAutoTransitions(st.Plan, st.Tasks) && st.Plan != nil {
			st.Plan.UpdatedAt = session.NowRFC3339()
		}
	})
}

func (m subagentsTaskMirror) markStepTasksCancelled(flowID, stepID string, tasks []subagentFlowTaskInput, reason string) {
	for _, task := range tasks {
		_ = m.markTaskCancelled(flowID, stepID, strings.TrimSpace(task.ID), subagentMirrorTaskTitle(task.ID, task.Title), reason)
	}
}

func (m subagentsTaskMirror) markUnfinishedTasksCancelled(flowID, reason string) {
	sessionID := m.sessionID()
	if sessionID == "" {
		return
	}
	st, err := m.store.GetTasks(sessionID)
	if err != nil {
		return
	}
	changed := false
	for i := range st.Tasks {
		if subagentMarkerValue(st.Tasks[i].Description, "flow") != strings.TrimSpace(flowID) {
			continue
		}
		if st.Tasks[i].Status == "completed" || st.Tasks[i].Status == "cancelled" {
			continue
		}
		stepID := subagentMarkerValue(st.Tasks[i].Description, "step")
		taskID := subagentMarkerValue(st.Tasks[i].Description, "task")
		st.Tasks[i].Status = "cancelled"
		st.Tasks[i].Description = buildSubagentTaskDescription(flowID, stepID, taskID, "", "", reason, "")
		changed = true
	}
	if changed {
		if applyAutoTransitions(st.Plan, st.Tasks) && st.Plan != nil {
			st.Plan.UpdatedAt = session.NowRFC3339()
		}
		_ = m.store.SaveTasks(sessionID, st)
	}
}

func (m subagentsTaskMirror) updateTask(flowID, stepID, taskID, title string, mutate func(*session.SessionTasks, *session.Task)) error {
	sessionID := m.sessionID()
	if sessionID == "" || strings.TrimSpace(taskID) == "" {
		return nil
	}
	st, err := m.store.GetTasks(sessionID)
	if err != nil {
		return err
	}
	idx := findSubagentTaskIndex(st.Tasks, flowID, taskID)
	if idx < 0 {
		st.Tasks = append(st.Tasks, session.Task{
			ID:          session.NextTaskID(st.Tasks),
			Title:       subagentMirrorTaskTitle(taskID, title),
			Status:      "pending",
			Description: buildSubagentTaskDescription(flowID, stepID, taskID, "", "", "", ""),
		})
		idx = len(st.Tasks) - 1
	}
	if strings.TrimSpace(title) != "" {
		st.Tasks[idx].Title = strings.TrimSpace(title)
	}
	mutate(&st, &st.Tasks[idx])
	return m.store.SaveTasks(sessionID, st)
}

func appendSubagentTasks(st *session.SessionTasks, flowID string, steps []subagentFlowStepInput) {
	for _, step := range steps {
		stepID := normalizedSubagentStepID(step)
		for _, task := range step.Tasks {
			taskID := strings.TrimSpace(task.ID)
			if taskID == "" {
				continue
			}
			st.Tasks = append(st.Tasks, session.Task{
				ID:          session.NextTaskID(st.Tasks),
				Title:       subagentMirrorTaskTitle(taskID, task.Title),
				Status:      "pending",
				Description: buildSubagentTaskDescription(flowID, stepID, taskID, "", "", "", ""),
			})
		}
	}
}

func sessionTasksContainFlow(tasks []session.Task, flowID string) bool {
	for _, task := range tasks {
		if subagentMarkerValue(task.Description, "flow") == strings.TrimSpace(flowID) {
			return true
		}
	}
	return false
}

func findSubagentTaskIndex(tasks []session.Task, flowID, taskID string) int {
	for i, task := range tasks {
		if subagentMarkerValue(task.Description, "flow") == strings.TrimSpace(flowID) &&
			subagentMarkerValue(task.Description, "task") == strings.TrimSpace(taskID) {
			return i
		}
	}
	return -1
}

func subagentMarkerValue(description, key string) string {
	firstLine, _, _ := strings.Cut(strings.TrimSpace(description), "\n")
	firstLine = strings.TrimPrefix(firstLine, "[")
	firstLine = strings.TrimSuffix(firstLine, "]")
	prefix := strings.TrimSpace(key) + "="
	for _, field := range strings.Fields(firstLine) {
		if strings.HasPrefix(field, prefix) {
			return strings.TrimPrefix(field, prefix)
		}
	}
	return ""
}

func subagentTaskMarker(flowID, stepID, taskID string) string {
	parts := []string{"subagent", "flow=" + strings.TrimSpace(flowID)}
	if strings.TrimSpace(stepID) != "" {
		parts = append(parts, "step="+strings.TrimSpace(stepID))
	}
	parts = append(parts, "task="+strings.TrimSpace(taskID))
	return strings.Join(parts, " ")
}

func buildSubagentTaskDescription(flowID, stepID, taskID, runID, summary, errText, note string) string {
	lines := []string{"[" + subagentTaskMarker(flowID, stepID, taskID) + "]"}
	if strings.TrimSpace(runID) != "" {
		lines = append(lines, "Run: "+strings.TrimSpace(runID))
	}
	if strings.TrimSpace(summary) != "" {
		lines = append(lines, "Summary: "+strings.TrimSpace(summary))
	}
	if strings.TrimSpace(errText) != "" {
		lines = append(lines, "Error: "+strings.TrimSpace(errText))
	}
	if strings.TrimSpace(note) != "" {
		lines = append(lines, strings.TrimSpace(note))
	}
	return strings.Join(lines, "\n")
}

func normalizedSubagentStepID(step subagentFlowStepInput) string {
	stepID := strings.TrimSpace(step.ID)
	if stepID != "" {
		return stepID
	}
	mode := strings.TrimSpace(step.Mode)
	if mode == "" {
		return "step"
	}
	return mode
}

func subagentMirrorTaskTitle(taskID, title string) string {
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(taskID); trimmed != "" {
		return trimmed
	}
	return "subagent"
}

func subagentFlowGoal(flowID string) string {
	if strings.TrimSpace(flowID) == "" {
		return "Subagent flow"
	}
	return fmt.Sprintf("Subagent flow %s", strings.TrimSpace(flowID))
}

func subagentPlanConstraints(flowID string) string {
	if strings.TrimSpace(flowID) == "" {
		return "Mirrored from subagents_plan/subagents_orchestrate."
	}
	return fmt.Sprintf("Mirrored from subagents_plan/subagents_orchestrate flow %s.", strings.TrimSpace(flowID))
}
