package project

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type PhaseName string

const (
	PhasePlanning  PhaseName = "planning"
	PhaseDrafting  PhaseName = "drafting"
	PhaseExecuting PhaseName = "executing"
	PhaseReviewing PhaseName = "reviewing"
	PhaseBlocked   PhaseName = "blocked"
	PhaseDone      PhaseName = "done"
)

type PhaseStatus string

const (
	PhaseStatusActive  PhaseStatus = "active"
	PhaseStatusPaused  PhaseStatus = "paused"
	PhaseStatusBlocked PhaseStatus = "blocked"
	PhaseStatusDone    PhaseStatus = "done"
)

type PhaseSnapshot struct {
	ProjectID  string             `json:"project_id"`
	Name       PhaseName          `json:"name"`
	Status     PhaseStatus        `json:"status"`
	NextAction string             `json:"next_action,omitempty"`
	Summary    string             `json:"summary,omitempty"`
	Message    string             `json:"message,omitempty"`
	RunStatus  AutopilotRunStatus `json:"run_status,omitempty"`
	UpdatedAt  string             `json:"updated_at,omitempty"`
}

type PhaseEngine interface {
	Start(context.Context, string) (AutopilotRun, error)
	Status(string) (AutopilotRun, bool)
	Current(string) (PhaseSnapshot, bool)
	Advance(context.Context, string) (PhaseSnapshot, error)
	EnsureActiveRuns(context.Context) (int, error)
	Escalate(string, string) error
	Resume(context.Context, string) (AutopilotRun, error)
	Reset(string) error
	RunScheduled(context.Context, string) (string, error)
	Stop(string) error
	CronInterval() time.Duration
}

func normalizePhaseName(raw string) PhaseName {
	switch DefaultWorkflowPolicy.NormalizeProjectStatePhase(raw) {
	case string(PhaseDrafting):
		return PhaseDrafting
	case string(PhaseExecuting):
		return PhaseExecuting
	case string(PhaseReviewing):
		return PhaseReviewing
	case string(PhaseBlocked):
		return PhaseBlocked
	case string(PhaseDone):
		return PhaseDone
	default:
		return PhasePlanning
	}
}

func normalizePhaseStatus(raw string) PhaseStatus {
	switch DefaultWorkflowPolicy.NormalizeProjectStateStatus(raw) {
	case string(PhaseStatusPaused):
		return PhaseStatusPaused
	case string(PhaseStatusBlocked):
		return PhaseStatusBlocked
	case string(PhaseStatusDone):
		return PhaseStatusDone
	default:
		return PhaseStatusActive
	}
}

func (m *AutopilotManager) Current(projectID string) (PhaseSnapshot, bool) {
	if m == nil || m.store == nil {
		return PhaseSnapshot{}, false
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return PhaseSnapshot{}, false
	}
	if _, err := m.store.Get(projectID); err != nil {
		return PhaseSnapshot{}, false
	}

	run, runOK := m.Status(projectID)
	state, err := m.store.GetState(projectID)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "project state not found") {
			return PhaseSnapshot{}, false
		}
		if !runOK {
			return PhaseSnapshot{}, false
		}
		state = DefaultWorkflowPolicy.DefaultProjectState(projectID)
	}

	current := PhaseSnapshot{
		ProjectID:  projectID,
		Name:       normalizePhaseName(state.Phase),
		Status:     normalizePhaseStatus(state.Status),
		NextAction: strings.TrimSpace(state.NextAction),
		Summary: firstNonEmpty(
			strings.TrimSpace(state.LastRunSummary),
			strings.TrimSpace(state.CompletionSummary),
			strings.TrimSpace(state.StopReason),
		),
		UpdatedAt: strings.TrimSpace(state.LastRunAt),
	}
	if runOK {
		current.RunStatus = run.Status
		current.Message = strings.TrimSpace(run.Message)
		if updatedAt := strings.TrimSpace(run.UpdatedAt); updatedAt != "" {
			current.UpdatedAt = updatedAt
		}
	}
	return current, true
}

func (m *AutopilotManager) Advance(ctx context.Context, projectID string) (PhaseSnapshot, error) {
	if m == nil || m.store == nil {
		return PhaseSnapshot{}, fmt.Errorf("autopilot manager store is not configured")
	}
	if m.runner == nil {
		return PhaseSnapshot{}, fmt.Errorf("autopilot manager runner is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return PhaseSnapshot{}, fmt.Errorf("project id is required")
	}
	if _, err := m.store.Get(projectID); err != nil {
		return PhaseSnapshot{}, err
	}
	if !m.beginSynchronousStep(projectID) {
		current, ok := m.Current(projectID)
		if !ok {
			return PhaseSnapshot{}, fmt.Errorf("phase snapshot unavailable for project %s", projectID)
		}
		return current, nil
	}
	defer m.endSynchronousStep(projectID)

	run, iteration := m.prepareAdvanceRun(projectID)
	orch := NewOrchestratorWithGitHubAuthChecker(m.store, m.runner, m.githubAuthChecker)
	m.runIteration(ctx, orch, projectID, run.RunID, iteration, false)
	if err := m.persistCurrentRun(projectID); err != nil {
		return PhaseSnapshot{}, err
	}

	current, ok := m.Current(projectID)
	if !ok {
		return PhaseSnapshot{}, fmt.Errorf("phase snapshot unavailable for project %s", projectID)
	}
	if current.RunStatus == AutopilotStatusFailed {
		return current, fmt.Errorf("%s", strings.TrimSpace(current.Message))
	}
	return current, nil
}

func (m *AutopilotManager) beginSynchronousStep(projectID string) bool {
	if m == nil {
		return false
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.loops[projectID]; ok && cancel != nil {
		return false
	}
	if m.steps[projectID] {
		return false
	}
	m.steps[projectID] = true
	return true
}

func (m *AutopilotManager) endSynchronousStep(projectID string) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.Lock()
	delete(m.steps, projectID)
	m.mu.Unlock()
}

func (m *AutopilotManager) persistCurrentRun(projectID string) error {
	if m == nil {
		return nil
	}
	current, ok := m.Status(projectID)
	if !ok {
		return nil
	}
	return m.persistRun(current)
}

func (m *AutopilotManager) prepareAdvanceRun(projectID string) (AutopilotRun, int) {
	now := m.store.nowFn().UTC()
	current, ok := m.Status(projectID)
	if !ok {
		runID := fmt.Sprintf("autopilot-%s", now.Format("20060102T150405.000000000"))
		m.setRun(projectID, func(item *AutopilotRun) {
			item.ProjectID = projectID
			item.RunID = runID
			item.Status = AutopilotStatusRunning
			item.Message = "Autopilot advanced"
			item.StartedAt = now.Format(time.RFC3339)
			item.FinishedAt = ""
			item.Iterations = 0
		})
		persisted, _ := m.Status(projectID)
		return persisted, 1
	}

	if strings.TrimSpace(current.RunID) == "" {
		current.RunID = fmt.Sprintf("autopilot-%s", now.Format("20060102T150405.000000000"))
	}
	if strings.TrimSpace(current.StartedAt) == "" {
		current.StartedAt = now.Format(time.RFC3339)
	}
	nextIteration := current.Iterations + 1
	if nextIteration <= 0 {
		nextIteration = 1
	}
	m.setRun(projectID, func(item *AutopilotRun) {
		item.ProjectID = projectID
		item.RunID = current.RunID
		item.StartedAt = current.StartedAt
		item.FinishedAt = ""
		if item.Status == "" {
			item.Status = AutopilotStatusRunning
		}
	})
	persisted, _ := m.Status(projectID)
	return persisted, nextIteration
}

func (m *AutopilotManager) Escalate(projectID string, reason string) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("autopilot manager store is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project id is required")
	}
	if _, err := m.store.Get(projectID); err != nil {
		return err
	}

	message := strings.TrimSpace(reason)
	if message == "" {
		message = "Autopilot escalation requested"
	}
	run, ok := m.Status(projectID)
	if !ok || strings.TrimSpace(run.RunID) == "" {
		now := m.store.nowFn().UTC()
		run = AutopilotRun{
			ProjectID: projectID,
			RunID:     fmt.Sprintf("autopilot-escalated-%s", now.Format("20060102T150405.000000000")),
			StartedAt: now.Format(time.RFC3339),
		}
	}

	m.stopLoop(projectID)
	iteration := run.Iterations
	if iteration <= 0 {
		iteration = 1
	}
	m.block(projectID, run.RunID, iteration, message)
	return nil
}

func (m *AutopilotManager) stopLoop(projectID string) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.Lock()
	cancel := m.loops[projectID]
	delete(m.loops, projectID)
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
