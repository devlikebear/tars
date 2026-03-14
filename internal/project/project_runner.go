package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const autopilotRunDocumentName = "AUTOPILOT.json"

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
	loopInterval      time.Duration

	mu    sync.RWMutex
	runs  map[string]AutopilotRun
	loops map[string]context.CancelFunc
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
		loopInterval:      time.Minute,
		runs:              map[string]AutopilotRun{},
		loops:             map[string]context.CancelFunc{},
	}
}

func (m *AutopilotManager) RestorePersistedRuns() error {
	if m == nil || m.store == nil {
		return nil
	}
	projects, err := m.store.List()
	if err != nil {
		return err
	}
	for _, item := range projects {
		run, err := m.loadRun(item.ID)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			m.cacheRun(item.ID, run)
		}
	}
	_, err = m.EnsureActiveRuns(context.Background())
	return err
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
	if current, ok := m.Status(projectID); ok && m.IsRunning(projectID) {
		return current, nil
	}

	now := m.store.nowFn().UTC()
	message := "Autopilot started"
	if current, ok := m.Status(projectID); ok && current.Status != AutopilotStatusDone && current.Status != AutopilotStatusFailed {
		message = "Autopilot resumed"
	}
	run := AutopilotRun{
		ProjectID: projectID,
		RunID:     fmt.Sprintf("autopilot-%s", now.Format("20060102T150405.000000000")),
		Status:    AutopilotStatusRunning,
		Message:   message,
		StartedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	if existing, ok := m.loops[projectID]; ok && existing != nil {
		current := m.runs[projectID]
		m.mu.Unlock()
		cancel()
		if current.ProjectID == "" {
			current = run
		}
		return current, nil
	}
	m.runs[projectID] = run
	m.loops[projectID] = cancel
	m.mu.Unlock()
	if err := m.persistRun(run); err != nil {
		m.clearLoop(projectID)
		cancel()
		return AutopilotRun{}, err
	}

	m.publish(projectID, "autopilot")
	go m.run(loopCtx, projectID, run.RunID)
	return run, nil
}

func (m *AutopilotManager) Status(projectID string) (AutopilotRun, bool) {
	if m == nil {
		return AutopilotRun{}, false
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.RLock()
	item, ok := m.runs[projectID]
	m.mu.RUnlock()
	if ok {
		return item, true
	}
	item, err := m.loadRun(projectID)
	if err != nil {
		return AutopilotRun{}, false
	}
	m.cacheRun(projectID, item)
	return item, true
}

func (m *AutopilotManager) IsRunning(projectID string) bool {
	if m == nil {
		return false
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	cancel, ok := m.loops[projectID]
	return ok && cancel != nil
}

func (m *AutopilotManager) EnsureActiveRuns(ctx context.Context) (int, error) {
	if m == nil || m.store == nil {
		return 0, nil
	}
	projects, err := m.store.List()
	if err != nil {
		return 0, err
	}
	started := 0
	for _, item := range projects {
		active, activeErr := m.shouldAutopilotProjectRun(item.ID)
		if activeErr != nil {
			return started, activeErr
		}
		current, ok := m.Status(item.ID)
		if ok && current.Status == AutopilotStatusDone {
			continue
		}
		if !active || m.IsRunning(item.ID) {
			continue
		}
		if _, startErr := m.Start(ctx, item.ID); startErr != nil {
			return started, startErr
		}
		started++
	}
	return started, nil
}

func (m *AutopilotManager) run(ctx context.Context, projectID, runID string) {
	defer func() {
		m.clearLoop(projectID)
		m.publish(projectID, "autopilot")
	}()
	orch := NewOrchestratorWithGitHubAuthChecker(m.store, m.runner, m.githubAuthChecker)
	immediateStreak := 0
	for iteration := 1; ; iteration++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
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
				m.noteBlocked(projectID, runID, iteration, message, "Autopilot will retry after the loop interval")
				if !m.waitForNextTick(ctx) {
					return
				}
				immediateStreak = 0
				continue
			}
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusRunning
				item.Message = "PM seeded backlog and resumed autopilot"
				item.Iterations = iteration
				item.FinishedAt = ""
			})
			m.updateState(projectID, "planning", "active", "Dispatch seeded backlog", "PM seeded MVP backlog", "")
			m.publish(projectID)
			immediateStreak++
			continue
		case boardHasStatus(board, "todo"):
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusRunning
				item.Message = "Dispatching todo tasks"
				item.Iterations = iteration
				item.FinishedAt = ""
			})
			m.updateState(projectID, "executing", "active", "Dispatch todo tasks", "Autopilot dispatching todo tasks", "")
			if _, err := orch.DispatchTodo(context.Background(), projectID); err != nil {
				recovered, recoverErr := m.autoRecover(projectID, iteration, "todo dispatch", err)
				if recoverErr != nil {
					m.fail(projectID, runID, iteration, recoverErr.Error())
					return
				}
				if recovered {
					immediateStreak++
					m.publish(projectID)
					continue
				}
				m.noteBlocked(projectID, runID, iteration, "Autopilot blocked after todo dispatch: "+strings.TrimSpace(err.Error()), "Autopilot will retry after the loop interval")
				if !m.waitForNextTick(ctx) {
					return
				}
				immediateStreak = 0
				continue
			}
			m.publish(projectID)
			immediateStreak++
		case boardHasStatus(board, "review"):
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusRunning
				item.Message = "Dispatching review tasks"
				item.Iterations = iteration
				item.FinishedAt = ""
			})
			m.updateState(projectID, "reviewing", "active", "Dispatch review tasks", "Autopilot dispatching review tasks", "")
			if _, err := orch.DispatchReview(context.Background(), projectID); err != nil {
				recovered, recoverErr := m.autoRecover(projectID, iteration, "review dispatch", err)
				if recoverErr != nil {
					m.fail(projectID, runID, iteration, recoverErr.Error())
					return
				}
				if recovered {
					immediateStreak++
					m.publish(projectID)
					continue
				}
				m.noteBlocked(projectID, runID, iteration, "Autopilot blocked after review dispatch: "+strings.TrimSpace(err.Error()), "Autopilot will retry after the loop interval")
				if !m.waitForNextTick(ctx) {
					return
				}
				immediateStreak = 0
				continue
			}
			m.publish(projectID)
			immediateStreak++
		case boardHasStatus(board, "in_progress"):
			task := firstTaskByStatus(board, "in_progress")
			recovered, recoverErr := m.autoRecover(projectID, iteration, "stalled in-progress task", nil)
			if recoverErr != nil {
				m.fail(projectID, runID, iteration, recoverErr.Error())
				return
			}
			if recovered {
				immediateStreak++
				m.publish(projectID)
				continue
			}
			message := "Autopilot waiting on in-progress task"
			nextAction := "Autopilot will retry after the loop interval"
			if strings.TrimSpace(task.Title) != "" {
				message = "Autopilot waiting on task: " + strings.TrimSpace(task.Title)
			}
			m.noteBlocked(projectID, runID, iteration, message, nextAction)
			if !m.waitForNextTick(ctx) {
				return
			}
			immediateStreak = 0
			continue
		case allBoardTasksDone(board):
			message := "Autopilot completed all project tasks"
			m.updateState(projectID, "done", "done", "Project complete", message, "")
			m.setRun(projectID, func(item *AutopilotRun) {
				item.Status = AutopilotStatusDone
				item.Message = message
				item.Iterations = iteration
				item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
			})
			m.publish(projectID)
			return
		default:
			m.noteBlocked(projectID, runID, iteration, "Autopilot found no actionable todo or review tasks", "Autopilot will retry after the loop interval")
			if !m.waitForNextTick(ctx) {
				return
			}
			immediateStreak = 0
			continue
		}
		if m.maxIterations > 0 && immediateStreak >= m.maxIterations {
			if !m.waitForNextTick(ctx) {
				return
			}
			immediateStreak = 0
		}
	}
}

func (m *AutopilotManager) fail(projectID, runID string, iteration int, message string) {
	m.updateState(projectID, "blocked", "blocked", "Inspect autopilot failure", strings.TrimSpace(message), strings.TrimSpace(message))
	m.setRun(projectID, func(item *AutopilotRun) {
		item.RunID = runID
		item.Status = AutopilotStatusFailed
		item.Message = strings.TrimSpace(message)
		item.Iterations = iteration
		item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
	})
	m.publish(projectID)
}

func (m *AutopilotManager) block(projectID, runID string, iteration int, message string) {
	_ = m.appendPMActivity(projectID, ActivityKindBlocker, "blocked", strings.TrimSpace(message), nil)
	m.updateState(projectID, "blocked", "blocked", "Review blocker and continue", strings.TrimSpace(message), strings.TrimSpace(message))
	m.setRun(projectID, func(item *AutopilotRun) {
		item.RunID = runID
		item.Status = AutopilotStatusBlocked
		item.Message = strings.TrimSpace(message)
		item.Iterations = iteration
		item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
	})
	m.publish(projectID)
}

func (m *AutopilotManager) noteBlocked(projectID, runID string, iteration int, message string, nextAction string) {
	message = strings.TrimSpace(message)
	nextAction = strings.TrimSpace(nextAction)
	if nextAction == "" {
		nextAction = "Autopilot will retry after the loop interval"
	}
	m.updateState(projectID, "blocked", "blocked", nextAction, message, message)
	m.setRun(projectID, func(item *AutopilotRun) {
		item.RunID = runID
		item.Status = AutopilotStatusRunning
		item.Message = message
		item.Iterations = iteration
		item.FinishedAt = ""
	})
	m.publish(projectID)
}

func (m *AutopilotManager) setRun(projectID string, mutate func(*AutopilotRun)) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.runs[projectID]
	mutate(&item)
	item.ProjectID = projectID
	item.UpdatedAt = m.store.nowFn().UTC().Format(time.RFC3339)
	_ = m.persistRun(item)
	m.runs[projectID] = item
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

func (m *AutopilotManager) autoRecover(projectID string, iteration int, reason errorSource, sourceErr error) (bool, error) {
	if m == nil || m.store == nil {
		return false, fmt.Errorf("autopilot manager store is not configured")
	}
	board, err := m.store.GetBoard(projectID)
	if err != nil {
		return false, err
	}
	if !boardHasStatus(board, "in_progress") {
		return false, nil
	}
	tasks := make([]BoardTask, 0, len(board.Tasks))
	recoveredIDs := make([]string, 0)
	for _, task := range board.Tasks {
		if task.Status == "in_progress" {
			task.Status = "todo"
			recoveredIDs = append(recoveredIDs, task.ID)
		}
		tasks = append(tasks, task)
	}
	if len(recoveredIDs) == 0 {
		return false, nil
	}
	if _, err := m.store.UpdateBoard(projectID, BoardUpdateInput{
		Columns: board.Columns,
		Tasks:   tasks,
	}); err != nil {
		return false, err
	}
	message := "PM auto-requeued stalled work"
	if len(recoveredIDs) == 1 {
		message = "PM auto-requeued stalled task: " + recoveredIDs[0]
	}
	meta := map[string]string{
		"reason": strings.TrimSpace(string(reason)),
		"tasks":  strings.Join(recoveredIDs, ","),
	}
	if sourceErr != nil && strings.TrimSpace(sourceErr.Error()) != "" {
		meta["error"] = strings.TrimSpace(sourceErr.Error())
	}
	_ = m.appendPMActivity(projectID, ActivityKindDecision, "auto_retry", "PM diagnosed the blocker and retried the implementation loop without user input", meta)
	_ = m.appendPMActivity(projectID, ActivityKindReplan, "applied", message, meta)
	m.updateState(projectID, "executing", "active", "Retry recovered tasks", message, "")
	return true, nil
}

func (m *AutopilotManager) cacheRun(projectID string, run AutopilotRun) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[projectID] = run
}

func (m *AutopilotManager) clearLoop(projectID string) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.loops, projectID)
}

func (m *AutopilotManager) loadRun(projectID string) (AutopilotRun, error) {
	if m == nil || m.store == nil {
		return AutopilotRun{}, fmt.Errorf("autopilot manager store is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return AutopilotRun{}, fmt.Errorf("project id is required")
	}
	raw, err := os.ReadFile(m.autopilotRunPath(projectID))
	if err != nil {
		return AutopilotRun{}, err
	}
	var item AutopilotRun
	if err := json.Unmarshal(raw, &item); err != nil {
		return AutopilotRun{}, err
	}
	if strings.TrimSpace(item.ProjectID) == "" {
		item.ProjectID = projectID
	}
	return item, nil
}

func (m *AutopilotManager) persistRun(item AutopilotRun) error {
	if m == nil || m.store == nil {
		return nil
	}
	projectID := strings.TrimSpace(item.ProjectID)
	if projectID == "" {
		return fmt.Errorf("autopilot project id is required")
	}
	if _, err := m.store.Get(projectID); err != nil {
		return err
	}
	return writeJSONAtomicWithTrailingNewline(m.autopilotRunPath(projectID), item)
}

func (m *AutopilotManager) autopilotRunPath(projectID string) string {
	if m == nil || m.store == nil {
		return ""
	}
	return m.store.ProjectFilePath(projectID, autopilotRunDocumentName)
}

func (m *AutopilotManager) shouldAutopilotProjectRun(projectID string) (bool, error) {
	if m == nil || m.store == nil {
		return false, nil
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false, nil
	}
	item, err := m.store.Get(projectID)
	if err != nil {
		return false, err
	}
	if item.Status == "archived" {
		return false, nil
	}
	board, err := m.store.GetBoard(projectID)
	if err != nil {
		return false, err
	}
	if len(board.Tasks) == 0 {
		return true, nil
	}
	if !allBoardTasksDone(board) {
		return true, nil
	}
	state, err := m.store.GetState(projectID)
	if err != nil {
		return false, err
	}
	return state.Status != "done", nil
}

func (m *AutopilotManager) waitForNextTick(ctx context.Context) bool {
	interval := m.loopInterval
	if interval <= 0 {
		interval = time.Minute
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type errorSource string

func writeJSONAtomicWithTrailingNewline(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
