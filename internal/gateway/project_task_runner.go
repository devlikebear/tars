package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/project"
)

type projectTaskRuntime interface {
	Spawn(ctx context.Context, req SpawnRequest) (Run, error)
	Wait(ctx context.Context, runID string) (Run, error)
}

type ProjectTaskRunner struct {
	runtime     projectTaskRuntime
	workspaceID string
}

func NewProjectTaskRunner(runtime projectTaskRuntime, workspaceID string) *ProjectTaskRunner {
	return &ProjectTaskRunner{
		runtime:     runtime,
		workspaceID: strings.TrimSpace(workspaceID),
	}
}

func (r *ProjectTaskRunner) Start(ctx context.Context, req project.TaskRunRequest) (project.TaskRun, error) {
	if r == nil || r.runtime == nil {
		return project.TaskRun{}, fmt.Errorf("gateway project task runner is not configured")
	}
	workerKind := strings.ToLower(strings.TrimSpace(req.WorkerKind))
	if workerKind == "" {
		return project.TaskRun{}, fmt.Errorf("worker kind is required")
	}

	run, err := r.runtime.Spawn(ctx, SpawnRequest{
		WorkspaceID: r.workspaceID,
		ProjectID:   strings.TrimSpace(req.ProjectID),
		Title:       strings.TrimSpace(req.Title),
		Prompt:      strings.TrimSpace(req.Prompt),
		Agent:       workerKind,
	})
	if err != nil && strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "unknown agent") {
		run, err = r.runtime.Spawn(ctx, SpawnRequest{
			WorkspaceID: r.workspaceID,
			ProjectID:   strings.TrimSpace(req.ProjectID),
			Title:       strings.TrimSpace(req.Title),
			Prompt:      strings.TrimSpace(req.Prompt),
			Agent:       "",
		})
	}
	if err != nil {
		return project.TaskRun{}, err
	}
	resolvedWorkerKind := workerKind
	if actualAgent := strings.ToLower(strings.TrimSpace(run.Agent)); actualAgent != "" {
		resolvedWorkerKind = actualAgent
	}
	return project.TaskRun{
		ID:         run.ID,
		TaskID:     strings.TrimSpace(req.TaskID),
		Agent:      run.Agent,
		WorkerKind: resolvedWorkerKind,
		Status:     mapProjectTaskRunStatus(run.Status),
		Response:   run.Response,
		Error:      run.Error,
	}, nil
}

func (r *ProjectTaskRunner) Wait(ctx context.Context, runID string) (project.TaskRun, error) {
	if r == nil || r.runtime == nil {
		return project.TaskRun{}, fmt.Errorf("gateway project task runner is not configured")
	}
	run, err := r.runtime.Wait(ctx, runID)
	if err != nil {
		return project.TaskRun{}, err
	}
	return project.TaskRun{
		ID:         run.ID,
		Agent:      run.Agent,
		WorkerKind: strings.ToLower(strings.TrimSpace(run.Agent)),
		Status:     mapProjectTaskRunStatus(run.Status),
		Response:   run.Response,
		Error:      run.Error,
	}, nil
}

func mapProjectTaskRunStatus(status RunStatus) project.TaskRunStatus {
	switch status {
	case RunStatusAccepted:
		return project.TaskRunStatusAccepted
	case RunStatusRunning:
		return project.TaskRunStatusRunning
	case RunStatusFailed:
		return project.TaskRunStatusFailed
	case RunStatusCanceled:
		return project.TaskRunStatusCanceled
	default:
		return project.TaskRunStatusCompleted
	}
}
