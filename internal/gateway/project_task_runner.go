package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/project"
)

type projectTaskRuntime interface {
	Spawn(ctx context.Context, req SpawnRequest) (Run, error)
	Wait(ctx context.Context, runID string) (Run, error)
}

type ProjectTaskRunner struct {
	runtime     projectTaskRuntime
	workspaceID string
	mu          sync.Mutex
	runKinds    map[string]string
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
	r.mu.Lock()
	if r.runKinds == nil {
		r.runKinds = map[string]string{}
	}
	r.runKinds[strings.TrimSpace(run.ID)] = workerKind
	r.mu.Unlock()
	return project.TaskRun{
		ID:         run.ID,
		TaskID:     strings.TrimSpace(req.TaskID),
		Agent:      run.Agent,
		WorkerKind: workerKind,
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
	requestedWorkerKind := ""
	r.mu.Lock()
	if r.runKinds != nil {
		requestedWorkerKind = strings.TrimSpace(r.runKinds[strings.TrimSpace(runID)])
		delete(r.runKinds, strings.TrimSpace(runID))
	}
	r.mu.Unlock()
	if requestedWorkerKind == "" {
		requestedWorkerKind = strings.ToLower(strings.TrimSpace(run.Agent))
	}
	return project.TaskRun{
		ID:         run.ID,
		Agent:      run.Agent,
		WorkerKind: requestedWorkerKind,
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
