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

const (
	defaultPlanningBlockTimeout  = 24 * time.Hour
	defaultAutopilotRunRetention = 30 * 24 * time.Hour
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
	store                *Store
	runner               TaskRunner
	githubAuthChecker    GitHubAuthChecker
	notify               func(projectID string, kind string)
	maxIterations        int
	loopInterval         time.Duration
	planningBlockTimeout time.Duration
	runRetention         time.Duration

	mu    sync.RWMutex
	runs  map[string]AutopilotRun
	loops map[string]context.CancelFunc
	steps map[string]bool
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
		store:                store,
		runner:               runner,
		githubAuthChecker:    checker,
		notify:               notify,
		maxIterations:        16,
		loopInterval:         time.Minute,
		planningBlockTimeout: defaultPlanningBlockTimeout,
		runRetention:         defaultAutopilotRunRetention,
		runs:                 map[string]AutopilotRun{},
		loops:                map[string]context.CancelFunc{},
		steps:                map[string]bool{},
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
			continue
		}
		if m.runExpired(run) {
			m.removeRun(item.ID)
			continue
		}
		// Fix stale "running" status for runs that were actually blocked/failed.
		if run.Status == AutopilotStatusRunning {
			msg := strings.ToLower(run.Message)
			if strings.Contains(msg, "blocked") {
				run.Status = AutopilotStatusBlocked
			} else if strings.Contains(msg, "failed") {
				run.Status = AutopilotStatusFailed
			}
		}
		m.cacheRun(item.ID, run)
	}
	// Do NOT call EnsureActiveRuns here; autopilot loops will resume
	// naturally on the next heartbeat via EnsureActiveRuns callback.
	return nil
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
		if m.runExpired(item) {
			m.removeRun(projectID)
			return AutopilotRun{}, false
		}
		return item, true
	}
	item, err := m.loadRun(projectID)
	if err != nil {
		return AutopilotRun{}, false
	}
	if m.runExpired(item) {
		m.removeRun(projectID)
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
	if m.steps[projectID] {
		return true
	}
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
		if ctx.Err() != nil {
			return
		}
		step := m.runIteration(ctx, orch, projectID, runID, iteration, true)
		if step.Stop {
			return
		}
		if step.Immediate {
			immediateStreak++
		} else {
			immediateStreak = 0
		}
		if !m.applyImmediateThrottle(ctx, &immediateStreak) {
			return
		}
	}
}

type autopilotStepResult struct {
	Immediate bool
	Stop      bool
}

func (m *AutopilotManager) runIteration(
	ctx context.Context,
	orch *Orchestrator,
	projectID string,
	runID string,
	iteration int,
	isBackgroundRun bool,
) autopilotStepResult {
	board, err := m.store.GetBoard(projectID)
	if err != nil {
		m.fail(projectID, runID, iteration, err.Error())
		return autopilotStepResult{Stop: true}
	}
	switch {
	case len(board.Tasks) == 0:
		return m.handleEmptyBoard(ctx, orch, projectID, runID, iteration, isBackgroundRun)
	case boardHasStatus(board, "todo"):
		return m.handleDispatchStage(ctx, orch, projectID, runID, iteration, "todo", isBackgroundRun)
	case boardHasStatus(board, "review"):
		return m.handleDispatchStage(ctx, orch, projectID, runID, iteration, "review", isBackgroundRun)
	case boardHasStatus(board, "in_progress"):
		return m.handleInProgressBoard(ctx, projectID, runID, iteration, board, isBackgroundRun)
	case allBoardTasksDone(board):
		m.completeRun(projectID, iteration)
		return autopilotStepResult{Stop: true}
	default:
		return m.handleIdleBoard(ctx, projectID, runID, iteration, isBackgroundRun)
	}
}

func (m *AutopilotManager) handleEmptyBoard(
	ctx context.Context,
	orch *Orchestrator,
	projectID string,
	runID string,
	iteration int,
	isBackgroundRun bool,
) autopilotStepResult {
	// Try auto-planning via LLM before blocking
	m.setRunningRun(projectID, "Planning next phase via LLM", iteration)
	m.publish(projectID, "autopilot")
	_ = m.appendPMActivity(projectID, "autopilot", "planning", "Auto-planning: generating backlog from project brief", nil)

	tasks, err := orch.PlanTasks(ctx, projectID)
	if err != nil {
		// Planning failed — fall back to manual block
		_ = m.appendPMActivity(projectID, "autopilot", "blocked", fmt.Sprintf("Auto-planning failed: %v", err), nil)
		if isBackgroundRun && m.planningBlockExpired(projectID) {
			message := "Planning approval timed out: backlog is still empty"
			nextAction := "Update or approve the backlog, then run /project autopilot advance to continue"
			m.planningTimedOut(projectID, runID, iteration, message, nextAction)
			return autopilotStepResult{Stop: true}
		}
		message := fmt.Sprintf("Auto-planning failed: %v", err)
		nextAction := "Create or approve the next phase backlog manually"
		m.planningRequired(projectID, runID, iteration, message, nextAction)
		return autopilotStepResult{Stop: true}
	}

	// Write a clean board with planned tasks (overwrite any corrupted KANBAN.md)
	cleanBoard := Board{
		ProjectID: projectID,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Columns:   append([]string(nil), defaultBoardColumns...),
		Tasks:     tasks,
	}
	if writeErr := m.store.writeBoard(cleanBoard); writeErr != nil {
		message := fmt.Sprintf("Failed to write board with planned tasks: %v", writeErr)
		m.planningRequired(projectID, runID, iteration, message, "Retry or manually update the board")
		return autopilotStepResult{Stop: true}
	}

	taskTitles := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskTitles = append(taskTitles, t.Title)
	}
	_ = m.appendPMActivity(projectID, "autopilot", "planned",
		fmt.Sprintf("Auto-planned %d tasks: %s", len(tasks), strings.Join(taskTitles, "; ")), nil)
	m.publish(projectID, "autopilot", "board")
	return autopilotStepResult{Immediate: true} // continue to dispatch
}

func (m *AutopilotManager) handleDispatchStage(
	ctx context.Context,
	orch *Orchestrator,
	projectID string,
	runID string,
	iteration int,
	stage string,
	isBackgroundRun bool,
) autopilotStepResult {
	m.setRunningRun(projectID, "Dispatching "+stage+" tasks", iteration)
	if update, ok := DefaultWorkflowPolicy.AutopilotDispatchState(stage); ok {
		m.updateState(projectID, update)
	}
	var err error
	switch stage {
	case "review":
		_, err = orch.DispatchReview(context.Background(), projectID)
	default:
		_, err = orch.DispatchTodo(context.Background(), projectID)
	}
	if err == nil {
		m.publish(projectID)
		return autopilotStepResult{Immediate: true}
	}
	recovered, recoverErr := m.autoRecover(projectID, iteration, errorSource(stage+" dispatch"), err)
	if recoverErr != nil {
		m.fail(projectID, runID, iteration, recoverErr.Error())
		return autopilotStepResult{Stop: true}
	}
	if recovered {
		m.publish(projectID)
		return autopilotStepResult{Immediate: true}
	}
	return m.waitBlockedLoop(
		ctx,
		projectID,
		runID,
		iteration,
		"Autopilot blocked after "+stage+" dispatch: "+strings.TrimSpace(err.Error()),
		"Autopilot will retry after the loop interval",
		isBackgroundRun,
	)
}

func (m *AutopilotManager) handleInProgressBoard(
	ctx context.Context,
	projectID string,
	runID string,
	iteration int,
	board Board,
	isBackgroundRun bool,
) autopilotStepResult {
	recovered, recoverErr := m.autoRecover(projectID, iteration, "stalled in-progress task", nil)
	if recoverErr != nil {
		m.fail(projectID, runID, iteration, recoverErr.Error())
		return autopilotStepResult{Stop: true}
	}
	if recovered {
		m.publish(projectID)
		return autopilotStepResult{Immediate: true}
	}
	task := firstTaskByStatus(board, "in_progress")
	message := "Autopilot waiting on in-progress task"
	if strings.TrimSpace(task.Title) != "" {
		message = "Autopilot waiting on task: " + strings.TrimSpace(task.Title)
	}
	return m.waitBlockedLoop(ctx, projectID, runID, iteration, message, "Autopilot will retry after the loop interval", isBackgroundRun)
}

func (m *AutopilotManager) handleIdleBoard(
	ctx context.Context,
	projectID string,
	runID string,
	iteration int,
	isBackgroundRun bool,
) autopilotStepResult {
	return m.waitBlockedLoop(
		ctx,
		projectID,
		runID,
		iteration,
		"Autopilot found no actionable todo or review tasks",
		"Autopilot will retry after the loop interval",
		isBackgroundRun,
	)
}

func (m *AutopilotManager) waitBlockedLoop(
	ctx context.Context,
	projectID string,
	runID string,
	iteration int,
	message string,
	nextAction string,
	isBackgroundRun bool,
) autopilotStepResult {
	if !isBackgroundRun {
		m.blockWithNextAction(projectID, runID, iteration, message, nextAction)
		return autopilotStepResult{Stop: true}
	}
	m.noteBlocked(projectID, runID, iteration, message, nextAction)
	if !m.waitForNextTick(ctx) {
		return autopilotStepResult{Stop: true}
	}
	return autopilotStepResult{}
}

func (m *AutopilotManager) setRunningRun(projectID string, message string, iteration int) {
	m.setRun(projectID, func(item *AutopilotRun) {
		item.Status = AutopilotStatusRunning
		item.Message = strings.TrimSpace(message)
		item.Iterations = iteration
		item.FinishedAt = ""
	})
}

func (m *AutopilotManager) completeRun(projectID string, iteration int) {
	message := "Autopilot completed all project tasks"
	m.transition(projectID, autopilotTransition{
		runStatus:  AutopilotStatusDone,
		message:    message,
		iteration:  iteration,
		terminal:   true,
		stateUpdate: DefaultWorkflowPolicy.AutopilotCompletedState(message),
	})
}

func (m *AutopilotManager) applyImmediateThrottle(ctx context.Context, immediateStreak *int) bool {
	if m.maxIterations <= 0 || *immediateStreak < m.maxIterations {
		return true
	}
	if !m.waitForNextTick(ctx) {
		return false
	}
	*immediateStreak = 0
	return true
}

func (m *AutopilotManager) fail(projectID, runID string, iteration int, message string) {
	m.transition(projectID, autopilotTransition{
		runID:       runID,
		runStatus:   AutopilotStatusFailed,
		message:     message,
		iteration:   iteration,
		terminal:    true,
		stateUpdate: DefaultWorkflowPolicy.AutopilotFailedState(message),
	})
}

func (m *AutopilotManager) block(projectID, runID string, iteration int, message string) {
	m.blockWithNextAction(projectID, runID, iteration, message, "Review blocker and continue")
}

func (m *AutopilotManager) blockWithNextAction(projectID, runID string, iteration int, message string, nextAction string) {
	_ = m.appendPMActivity(projectID, ActivityKindBlocker, "blocked", strings.TrimSpace(message), nil)
	m.transition(projectID, autopilotTransition{
		runID:       runID,
		runStatus:   AutopilotStatusBlocked,
		message:     message,
		iteration:   iteration,
		terminal:    true,
		stateUpdate: DefaultWorkflowPolicy.AutopilotBlockedState(message, nextAction),
	})
}

func (m *AutopilotManager) planningRequired(projectID, runID string, iteration int, message string, nextAction string) {
	_ = m.appendPMActivity(projectID, ActivityKindDecision, "needed", strings.TrimSpace(message), nil)
	m.transition(projectID, autopilotTransition{
		runID:       runID,
		runStatus:   AutopilotStatusBlocked,
		message:     message,
		iteration:   iteration,
		terminal:    true,
		stateUpdate: DefaultWorkflowPolicy.AutopilotPlanningRequiredState(message, nextAction),
	})
}

func (m *AutopilotManager) planningTimedOut(projectID, runID string, iteration int, message string, nextAction string) {
	_ = m.appendPMActivity(projectID, ActivityKindDecision, "expired", strings.TrimSpace(message), nil)
	m.transition(projectID, autopilotTransition{
		runID:       runID,
		runStatus:   AutopilotStatusBlocked,
		message:     message,
		iteration:   iteration,
		terminal:    true,
		stateUpdate: DefaultWorkflowPolicy.AutopilotBlockedState(message, nextAction),
	})
}

func (m *AutopilotManager) noteBlocked(projectID, runID string, iteration int, message string, nextAction string) {
	m.transition(projectID, autopilotTransition{
		runID:       runID,
		runStatus:   AutopilotStatusRunning,
		message:     message,
		iteration:   iteration,
		terminal:    false,
		stateUpdate: DefaultWorkflowPolicy.AutopilotBlockedState(message, nextAction),
	})
}

// autopilotTransition describes a state change for an autopilot run.
// All transition methods delegate to transition() to avoid repeating
// the updateState → setRun → publish sequence.
type autopilotTransition struct {
	runID       string
	runStatus   AutopilotRunStatus
	message     string
	iteration   int
	terminal    bool // true → set FinishedAt; false → clear it
	stateUpdate workflowStateUpdate
}

func (m *AutopilotManager) transition(projectID string, t autopilotTransition) {
	m.updateState(projectID, t.stateUpdate)
	msg := strings.TrimSpace(t.message)
	m.setRun(projectID, func(item *AutopilotRun) {
		if t.runID != "" {
			item.RunID = t.runID
		}
		item.Status = t.runStatus
		item.Message = msg
		item.Iterations = t.iteration
		if t.terminal {
			item.FinishedAt = m.store.nowFn().UTC().Format(time.RFC3339)
		} else {
			item.FinishedAt = ""
		}
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

func (m *AutopilotManager) updateState(projectID string, update workflowStateUpdate) {
	if m == nil || m.store == nil {
		return
	}
	lastRunAt := m.store.nowFn().UTC().Format(time.RFC3339)
	input := update.stateInput()
	input.LastRunAt = &lastRunAt
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
	return DefaultWorkflowPolicy.BoardHasStatus(board, status)
}

func firstTaskByStatus(board Board, status string) BoardTask {
	task, _ := DefaultWorkflowPolicy.FirstTaskByStatus(board, status)
	return task
}

func allBoardTasksDone(board Board) bool {
	return DefaultWorkflowPolicy.AllBoardTasksDone(board)
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
	tasks, recoveredIDs, ok := DefaultWorkflowPolicy.RecoverStalledTasks(board.Tasks)
	if !ok {
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
	m.updateState(projectID, DefaultWorkflowPolicy.AutopilotRecoveredState(message))
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
	board, err := m.store.GetBoard(projectID)
	if err != nil {
		return false, err
	}
	state, err := m.store.GetState(projectID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "project state not found") {
			return DefaultWorkflowPolicy.ShouldAutopilotRun(item, board, nil), nil
		}
		return false, err
	}
	if DefaultWorkflowPolicy.ShouldAutopilotRun(item, board, &state) {
		return true, nil
	}
	if len(board.Tasks) == 0 && m.planningBlockExpired(projectID) {
		return true, nil
	}
	return false, nil
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

func (m *AutopilotManager) planningBlockExpired(projectID string) bool {
	if m == nil || m.store == nil {
		return false
	}
	state, err := m.store.GetState(projectID)
	if err != nil {
		return false
	}
	if DefaultWorkflowPolicy.NormalizeProjectStatePhase(state.Phase) != string(PhasePlanning) {
		return false
	}
	if DefaultWorkflowPolicy.NormalizeProjectStateStatus(state.Status) != string(PhaseStatusBlocked) {
		return false
	}
	lastRunAt := parseTime(state.LastRunAt)
	if lastRunAt.IsZero() {
		return false
	}
	timeout := m.runtimePolicyForProject(projectID).PlanningBlockTimeout
	if timeout <= 0 {
		return false
	}
	return !m.store.nowFn().UTC().Before(lastRunAt.Add(timeout))
}

func (m *AutopilotManager) runExpired(item AutopilotRun) bool {
	if m == nil {
		return false
	}
	if !isTerminalAutopilotRunStatus(item.Status) {
		return false
	}
	lastTouched := parseTime(item.FinishedAt)
	if lastTouched.IsZero() {
		lastTouched = parseTime(item.UpdatedAt)
	}
	if lastTouched.IsZero() {
		lastTouched = parseTime(item.StartedAt)
	}
	if lastTouched.IsZero() {
		return false
	}
	retention := m.runtimePolicyForProject(item.ProjectID).RunRetention
	if retention <= 0 {
		return false
	}
	return lastTouched.Before(m.store.nowFn().UTC().Add(-retention))
}

func (m *AutopilotManager) runtimePolicyForProject(projectID string) WorkflowRuntimePolicy {
	policy := WorkflowRuntimePolicy{
		PlanningBlockTimeout: m.planningBlockTimeout,
		RunRetention:         m.runRetention,
	}
	if policy.PlanningBlockTimeout <= 0 {
		policy.PlanningBlockTimeout = defaultPlanningBlockTimeout
	}
	if policy.RunRetention <= 0 {
		policy.RunRetention = defaultAutopilotRunRetention
	}
	if m == nil || m.store == nil {
		return policy
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return policy
	}
	project, err := m.store.Get(projectID)
	if err != nil {
		return policy
	}
	applyWorkflowRuntimeRuleOverrides(&policy, project.WorkflowRules)
	return policy
}

func isTerminalAutopilotRunStatus(status AutopilotRunStatus) bool {
	switch status {
	case AutopilotStatusDone, AutopilotStatusBlocked, AutopilotStatusFailed:
		return true
	default:
		return false
	}
}

func (m *AutopilotManager) removeRun(projectID string) {
	if m == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	m.mu.Lock()
	delete(m.runs, projectID)
	m.mu.Unlock()
	path := m.autopilotRunPath(projectID)
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
}

// Resume re-starts a blocked or failed autopilot run.
func (m *AutopilotManager) Resume(ctx context.Context, projectID string) (AutopilotRun, error) {
	if m == nil {
		return AutopilotRun{}, fmt.Errorf("autopilot manager not configured")
	}
	projectID = strings.TrimSpace(projectID)
	run, ok := m.Status(projectID)
	if !ok {
		return AutopilotRun{}, fmt.Errorf("no autopilot run for project %s", projectID)
	}
	if run.Status != AutopilotStatusBlocked && run.Status != AutopilotStatusFailed {
		return run, nil // already running or done
	}
	// Reset status and restart the loop
	m.stopLoop(projectID)
	m.setRun(projectID, func(r *AutopilotRun) {
		r.Status = AutopilotStatusRunning
		r.Message = "resumed by user"
		r.FinishedAt = ""
		r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	})
	_ = m.persistCurrentRun(projectID)
	runID := run.RunID
	go m.run(ctx, projectID, runID)
	updated, _ := m.Status(projectID)
	return updated, nil
}

// Reset removes the current autopilot run, allowing a fresh start.
func (m *AutopilotManager) Reset(projectID string) error {
	if m == nil {
		return fmt.Errorf("autopilot manager not configured")
	}
	m.stopLoop(strings.TrimSpace(projectID))
	m.removeRun(strings.TrimSpace(projectID))
	return nil
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
