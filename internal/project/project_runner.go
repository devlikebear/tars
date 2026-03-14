package project

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type AutopilotRunStatus string

const (
	AutopilotStatusRunning AutopilotRunStatus = "running"
	AutopilotStatusDone    AutopilotRunStatus = "done"
	AutopilotStatusBlocked AutopilotRunStatus = "blocked"
	AutopilotStatusFailed  AutopilotRunStatus = "failed"
)

type AutopilotRun struct {
	ProjectID  string             `json:"project_id"`
	RunID      string             `json:"run_id"`
	Status     AutopilotRunStatus `json:"status"`
	Message    string             `json:"message,omitempty"`
	Iterations int                `json:"iterations"`
	StartedAt  string             `json:"started_at,omitempty"`
	UpdatedAt  string             `json:"updated_at,omitempty"`
	FinishedAt string             `json:"finished_at,omitempty"`
}

type AutopilotManager struct {
	store             *Store
	runner            TaskRunner
	githubAuthChecker GitHubAuthChecker
	notify            func(projectID string, kind string)
	maxIterations     int

	mu   sync.RWMutex
	runs map[string]AutopilotRun
}

func NewAutopilotManager(
	store *Store,
	runner TaskRunner,
	checker GitHubAuthChecker,
	notify func(projectID string, kind string),
) *AutopilotManager {
	if checker == nil {
		checker = defaultGitHubAuthChecker
	}
	return &AutopilotManager{
		store:             store,
		runner:            runner,
		githubAuthChecker: checker,
		notify:            notify,
		maxIterations:     16,
		runs:              map[string]AutopilotRun{},
	}
}

func (m *AutopilotManager) Start(_ context.Context, projectID string) (AutopilotRun, error) {
	if m == nil || m.store == nil {
		return AutopilotRun{}, fmt.Errorf("autopilot manager store is not configured")
	}
	if m.runner == nil {
		return AutopilotRun{}, fmt.Errorf("autopilot manager runner is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return AutopilotRun{}, fmt.Errorf("project id is required")
	}
	if _, err := m.store.Get(projectID); err != nil {
		return AutopilotRun{}, err
	}

	now := m.store.nowFn().UTC()
	run := AutopilotRun{
		ProjectID: projectID,
		RunID:     fmt.Sprintf("autopilot-%s", now.Format("20060102T150405.000000000")),
		Status:    AutopilotStatusRunning,
		Message:   "Autopilot started",
		StartedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}

	m.mu.Lock()
	if current, ok := m.runs[projectID]; ok && current.Status == AutopilotStatusRunning {
		m.mu.Unlock()
		return current, nil
	}
	m.runs[projectID] = run
	m.mu.Unlock()

	m.publish(projectID, "autopilot")
	go m.run(projectID, run.RunID)
	return run, nil
}

func (m *AutopilotManager) Status(projectID string) (AutopilotRun, bool) {
	if m == nil {
		return AutopilotRun{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.runs[strings.TrimSpace(projectID)]
	return item, ok
}

func (m *AutopilotManager) run(projectID, runID string) {
	orch := NewOrchestratorWithGitHubAuthChecker(m.store, m.runner, m.githubAuthChecker)
	for iteration := 1; iteration <= m.maxIterations; iteration++ {
		board, err := m.store.GetBoard(projectID)
		if err != nil {
			m.fail(projectID, runID, iteration, err.Error())
			return
		}
		switch {
		case len(board.Tasks) == 0:
			if err := m.seedBacklog(projectID); err != nil {
				message := "Autopilot blocked: no tasks on the board"
				if strings.TrimSpace(err.Error()) != "" {
					message += ": " + strings.TrimSpace(err.Error())
				}
				m.block(projectID, runID, iteration, message)
				m.updateState(projectID, "blocked", "blocked", "Seed backlog and continue", message, message)
				return
			}
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusRunning
				item.Message = "PM seeded backlog and resumed autopilot"
				item.Iterations = iteration
			})
			m.updateState(projectID, "planning", "active", "Dispatch seeded backlog", "PM seeded MVP backlog", "")
			m.publish(projectID)
			continue
		case boardHasStatus(board, "todo"):
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusRunning
				item.Message = "Dispatching todo tasks"
				item.Iterations = iteration
			})
			m.updateState(projectID, "executing", "active", "Dispatch todo tasks", "Autopilot dispatching todo tasks", "")
			if _, err := orch.DispatchTodo(context.Background(), projectID); err != nil {
				m.block(projectID, runID, iteration, "Autopilot blocked after todo dispatch: "+strings.TrimSpace(err.Error()))
				return
			}
			m.publish(projectID)
		case boardHasStatus(board, "review"):
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusRunning
				item.Message = "Dispatching review tasks"
				item.Iterations = iteration
			})
			m.updateState(projectID, "reviewing", "active", "Dispatch review tasks", "Autopilot dispatching review tasks", "")
			if _, err := orch.DispatchReview(context.Background(), projectID); err != nil {
				m.block(projectID, runID, iteration, "Autopilot blocked after review dispatch: "+strings.TrimSpace(err.Error()))
				return
			}
			m.publish(projectID)
		case boardHasStatus(board, "in_progress"):
			task := firstTaskByStatus(board, "in_progress")
			message := "Autopilot blocked waiting on in-progress task"
			nextAction := "Answer the blocking question and rerun autopilot"
			if strings.TrimSpace(task.Title) != "" {
				message = "Autopilot blocked on task: " + strings.TrimSpace(task.Title)
				nextAction = "Resolve task blocker: " + strings.TrimSpace(task.Title)
			}
			_ = m.appendPMActivity(projectID, ActivityKindDecision, "needed", nextAction, map[string]string{
				"task_id": task.ID,
			})
			_ = m.appendPMActivity(projectID, ActivityKindReplan, "proposed", "Split or restage the blocked work before resuming autopilot", map[string]string{
				"task_id": task.ID,
			})
			m.block(projectID, runID, iteration, message)
			m.updateState(projectID, "blocked", "blocked", nextAction, message, message)
			return
		case allBoardTasksDone(board):
			message := "Autopilot completed all project tasks"
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusDone
				item.Message = message
				item.Iterations = iteration
				item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
			})
			m.updateState(projectID, "done", "done", "Project complete", message, "")
			m.publish(projectID)
			return
		default:
			m.block(projectID, runID, iteration, "Autopilot found no actionable todo or review tasks")
			return
		}
	}
	m.block(projectID, runID, m.maxIterations, "Autopilot reached the iteration limit")
}

func (m *AutopilotManager) fail(projectID, runID string, iteration int, message string) {
	m.setRun(projectID, func(item *AutopilotRun) {
		item.RunID = runID
		item.Status = AutopilotStatusFailed
		item.Message = strings.TrimSpace(message)
		item.Iterations = iteration
		item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
	})
	m.updateState(projectID, "blocked", "blocked", "Inspect autopilot failure", strings.TrimSpace(message), strings.TrimSpace(message))
	m.publish(projectID)
}

func (m *AutopilotManager) block(projectID, runID string, iteration int, message string) {
	m.setRun(projectID, func(item *AutopilotRun) {
		item.RunID = runID
		item.Status = AutopilotStatusBlocked
		item.Message = strings.TrimSpace(message)
		item.Iterations = iteration
		item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
	})
	_ = m.appendPMActivity(projectID, ActivityKindBlocker, "blocked", strings.TrimSpace(message), nil)
	m.updateState(projectID, "blocked", "blocked", "Review blocker and continue", strings.TrimSpace(message), strings.TrimSpace(message))
	m.publish(projectID)
}

func (m *AutopilotManager) setRun(projectID string, mutate func(*AutopilotRun)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.runs[strings.TrimSpace(projectID)]
	mutate(&item)
	item.ProjectID = strings.TrimSpace(projectID)
	item.UpdatedAt = m.store.nowFn().UTC().Format(time.RFC3339)
	m.runs[strings.TrimSpace(projectID)] = item
}

func (m *AutopilotManager) updateState(projectID, phase, status, nextAction, summary, stopReason string) {
	if m == nil || m.store == nil {
		return
	}
	phaseValue := phase
	statusValue := status
	next := nextAction
	lastRunSummary := summary
	stop := stopReason
	lastRunAt := m.store.nowFn().UTC().Format(time.RFC3339)
	input := ProjectStateUpdateInput{
		Phase:          &phaseValue,
		Status:         &statusValue,
		NextAction:     &next,
		LastRunSummary: &lastRunSummary,
		LastRunAt:      &lastRunAt,
	}
	if strings.TrimSpace(stopReason) != "" {
		input.StopReason = &stop
	}
	if strings.EqualFold(status, "done") {
		completionSummary := summary
		input.CompletionSummary = &completionSummary
	}
	_, _ = m.store.UpdateState(projectID, input)
}

func (m *AutopilotManager) publish(projectID string, kinds ...string) {
	if m == nil || m.notify == nil {
		return
	}
	if len(kinds) == 0 {
		kinds = []string{"board", "activity", "autopilot"}
	}
	for _, kind := range kinds {
		if strings.TrimSpace(kind) == "" {
			continue
		}
		m.notify(projectID, kind)
	}
}

func boardHasStatus(board Board, status string) bool {
	for _, task := range board.Tasks {
		if task.Status == strings.TrimSpace(status) {
			return true
		}
	}
	return false
}

func firstTaskByStatus(board Board, status string) BoardTask {
	for _, task := range board.Tasks {
		if task.Status == strings.TrimSpace(status) {
			return task
		}
	}
	return BoardTask{}
}

func allBoardTasksDone(board Board) bool {
	if len(board.Tasks) == 0 {
		return true
	}
	for _, task := range board.Tasks {
		if task.Status != "done" {
			return false
		}
	}
	return true
}

func (m *AutopilotManager) seedBacklog(projectID string) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("autopilot manager store is not configured")
	}
	item, err := m.store.Get(projectID)
	if err != nil {
		return err
	}
	board, err := m.store.GetBoard(projectID)
	if err != nil {
		return err
	}
	if len(board.Tasks) > 0 {
		return nil
	}

	projectLabel := firstNonEmpty(strings.TrimSpace(item.Name), strings.TrimSpace(item.Objective), "project")
	tasks := []BoardTask{
		{
			ID:             "pm-seed-bootstrap",
			Title:          "Bootstrap MVP for " + projectLabel,
			Status:         "todo",
			Assignee:       "dev-1",
			Role:           "developer",
			WorkerKind:     WorkerKindCodexCLI,
			ReviewRequired: true,
		},
		{
			ID:             "pm-seed-vertical-slice",
			Title:          "Implement first vertical slice for " + projectLabel,
			Status:         "todo",
			Assignee:       "dev-2",
			Role:           "developer",
			WorkerKind:     WorkerKindCodexCLI,
			ReviewRequired: true,
		},
	}
	if _, err := m.store.UpdateBoard(projectID, BoardUpdateInput{
		Columns: board.Columns,
		Tasks:   tasks,
	}); err != nil {
		return err
	}
	return m.appendPMActivity(projectID, ActivityKindReplan, "seeded", "PM seeded MVP backlog from the current project objective", map[string]string{
		"seed_tasks": fmt.Sprintf("%d", len(tasks)),
	})
}

func (m *AutopilotManager) appendPMActivity(projectID, kind, status, message string, meta map[string]string) error {
	if m == nil || m.store == nil {
		return nil
	}
	_, err := m.store.AppendActivity(projectID, ActivityAppendInput{
		Source:  ActivitySourcePM,
		Kind:    kind,
		Status:  status,
		Message: message,
		Meta:    meta,
	})
	return err
}
