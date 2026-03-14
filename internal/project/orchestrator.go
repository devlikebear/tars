package project

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type TaskRunStatus string

const (
	TaskRunStatusAccepted  TaskRunStatus = "accepted"
	TaskRunStatusRunning   TaskRunStatus = "running"
	TaskRunStatusCompleted TaskRunStatus = "completed"
	TaskRunStatusFailed    TaskRunStatus = "failed"
	TaskRunStatusCanceled  TaskRunStatus = "canceled"
)

type TaskRunRequest struct {
	ProjectID string
	TaskID    string
	Title     string
	Prompt    string
	Agent     string
	Role      string
}

type TaskRun struct {
	ID       string
	TaskID   string
	Agent    string
	Status   TaskRunStatus
	Response string
	Error    string
}

type TaskRunner interface {
	Start(ctx context.Context, req TaskRunRequest) (TaskRun, error)
	Wait(ctx context.Context, runID string) (TaskRun, error)
}

type DispatchReport struct {
	ProjectID string
	Runs      []TaskRun
}

type Orchestrator struct {
	store  *Store
	runner TaskRunner
	mu     sync.Mutex
}

func NewOrchestrator(store *Store, runner TaskRunner) *Orchestrator {
	return &Orchestrator{
		store:  store,
		runner: runner,
	}
}

func (o *Orchestrator) DispatchTodo(ctx context.Context, projectID string) (DispatchReport, error) {
	if o == nil || o.store == nil {
		return DispatchReport{}, fmt.Errorf("project orchestrator store is not configured")
	}
	if o.runner == nil {
		return DispatchReport{}, fmt.Errorf("project orchestrator runner is not configured")
	}
	board, err := o.store.GetBoard(projectID)
	if err != nil {
		return DispatchReport{}, err
	}
	tasks := make([]BoardTask, 0, len(board.Tasks))
	for _, task := range board.Tasks {
		if task.Status == "todo" {
			tasks = append(tasks, task)
		}
	}
	report := DispatchReport{
		ProjectID: strings.TrimSpace(projectID),
		Runs:      make([]TaskRun, len(tasks)),
	}
	if len(tasks) == 0 {
		return report, nil
	}

	var (
		wg       sync.WaitGroup
		firstErr error
		errMu    sync.Mutex
	)
	for i, task := range tasks {
		wg.Add(1)
		go func(index int, task BoardTask) {
			defer wg.Done()
			run, runErr := o.dispatchTask(ctx, projectID, task)
			report.Runs[index] = run
			if runErr != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = runErr
				}
				errMu.Unlock()
			}
		}(i, task)
	}
	wg.Wait()
	if firstErr != nil {
		return report, firstErr
	}
	return report, nil
}

func (o *Orchestrator) dispatchTask(ctx context.Context, projectID string, task BoardTask) (TaskRun, error) {
	if err := o.setBoardTaskStatus(projectID, task.ID, "in_progress"); err != nil {
		return TaskRun{}, err
	}
	if err := o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindTaskStatus,
		Status:  "in_progress",
		Message: "Task dispatched to worker",
		Meta: map[string]string{
			"assignee": task.Assignee,
			"role":     task.Role,
		},
	}); err != nil {
		return TaskRun{}, err
	}

	run, err := o.runner.Start(ctx, TaskRunRequest{
		ProjectID: strings.TrimSpace(projectID),
		TaskID:    task.ID,
		Title:     task.Title,
		Prompt:    buildTaskPrompt(task, projectID),
		Agent:     task.Assignee,
		Role:      task.Role,
	})
	if err != nil {
		_ = o.setBoardTaskStatus(projectID, task.ID, "todo")
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTaskStatus,
			Status:  "failed",
			Message: "Task dispatch failed",
		})
		return TaskRun{}, err
	}

	finished, err := o.runner.Wait(ctx, run.ID)
	if err != nil {
		_ = o.setBoardTaskStatus(projectID, task.ID, "todo")
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTaskStatus,
			Status:  "failed",
			Message: "Task run failed",
			Meta: map[string]string{
				"run_id": run.ID,
			},
		})
		return run, err
	}

	finalStatus := "done"
	if task.ReviewRequired {
		finalStatus = "review"
	}
	if finished.Status == TaskRunStatusFailed || finished.Status == TaskRunStatusCanceled {
		finalStatus = "todo"
	}
	if err := o.setBoardTaskStatus(projectID, task.ID, finalStatus); err != nil {
		return finished, err
	}

	activityStatus := finalStatus
	message := "Task run completed"
	if finished.Status == TaskRunStatusFailed || finished.Status == TaskRunStatusCanceled {
		activityStatus = "failed"
		message = "Task run failed"
	}
	if err := o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindTaskStatus,
		Status:  activityStatus,
		Message: message,
		Meta: map[string]string{
			"run_id": run.ID,
			"agent":  firstNonEmpty(strings.TrimSpace(finished.Agent), strings.TrimSpace(task.Assignee)),
		},
	}); err != nil {
		return finished, err
	}
	return finished, nil
}

func (o *Orchestrator) setBoardTaskStatus(projectID, taskID, status string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	board, err := o.store.GetBoard(projectID)
	if err != nil {
		return err
	}
	updated := false
	tasks := make([]BoardTask, 0, len(board.Tasks))
	for _, task := range board.Tasks {
		if task.ID == strings.TrimSpace(taskID) {
			task.Status = strings.TrimSpace(status)
			updated = true
		}
		tasks = append(tasks, task)
	}
	if !updated {
		return fmt.Errorf("task not found: %s", strings.TrimSpace(taskID))
	}
	_, err = o.store.UpdateBoard(projectID, BoardUpdateInput{
		Columns: board.Columns,
		Tasks:   tasks,
	})
	return err
}

func buildTaskPrompt(task BoardTask, projectID string) string {
	var builder strings.Builder
	builder.WriteString("Project ID: ")
	builder.WriteString(strings.TrimSpace(projectID))
	builder.WriteString("\nTask: ")
	builder.WriteString(strings.TrimSpace(task.Title))
	if role := strings.TrimSpace(task.Role); role != "" {
		builder.WriteString("\nRole: ")
		builder.WriteString(role)
	}
	if testCmd := strings.TrimSpace(task.TestCommand); testCmd != "" {
		builder.WriteString("\nTest command: ")
		builder.WriteString(testCmd)
	}
	if buildCmd := strings.TrimSpace(task.BuildCommand); buildCmd != "" {
		builder.WriteString("\nBuild command: ")
		builder.WriteString(buildCmd)
	}
	return builder.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
