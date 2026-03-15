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
	ProjectID  string
	TaskID     string
	Title      string
	Prompt     string
	Agent      string
	Role       string
	WorkerKind string
}

type TaskRun struct {
	ID         string
	TaskID     string
	Agent      string
	WorkerKind string
	Status     TaskRunStatus
	Response   string
	Error      string
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
	store             *Store
	runner            TaskRunner
	githubAuthChecker GitHubAuthChecker
	mu                sync.Mutex
}

func NewOrchestrator(store *Store, runner TaskRunner) *Orchestrator {
	return NewOrchestratorWithGitHubAuthChecker(store, runner, defaultGitHubAuthChecker)
}

func NewOrchestratorWithGitHubAuthChecker(store *Store, runner TaskRunner, checker GitHubAuthChecker) *Orchestrator {
	if checker == nil {
		checker = defaultGitHubAuthChecker
	}
	return &Orchestrator{
		store:             store,
		runner:            runner,
		githubAuthChecker: checker,
	}
}

func (o *Orchestrator) DispatchTodo(ctx context.Context, projectID string) (DispatchReport, error) {
	return o.dispatchTasksByStatus(ctx, projectID, "todo", o.dispatchTask)
}

func (o *Orchestrator) DispatchReview(ctx context.Context, projectID string) (DispatchReport, error) {
	return o.dispatchTasksByStatus(ctx, projectID, "review", o.dispatchReviewTask)
}

func (o *Orchestrator) dispatchTasksByStatus(
	ctx context.Context,
	projectID string,
	status string,
	dispatchFn func(context.Context, string, BoardTask) (TaskRun, error),
) (DispatchReport, error) {
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
		if task.Status == strings.TrimSpace(status) {
			tasks = append(tasks, task)
		}
	}
	tasks = filterDispatchableTasksByStatus(strings.TrimSpace(status), tasks)
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
			run, runErr := dispatchFn(ctx, projectID, task)
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
	profile, err := ResolveWorkerProfile(task)
	if err != nil {
		return TaskRun{}, err
	}
	if o.githubAuthChecker != nil {
		if err := o.githubAuthChecker(ctx); err != nil {
			wrapped := fmt.Errorf("github auth precondition failed: %w", err)
			_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
				TaskID:  task.ID,
				Kind:    ActivityKindIssueStatus,
				Status:  "blocked",
				Message: strings.TrimSpace(wrapped.Error()),
			})
			return TaskRun{}, wrapped
		}
	}
	if err := o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
		item.Status = "in_progress"
		item.WorkerKind = profile.Kind
	}); err != nil {
		return TaskRun{}, err
	}
	if err := o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindTaskStatus,
		Status:  "in_progress",
		Message: "Task dispatched to worker",
		Meta: map[string]string{
			"assignee":    task.Assignee,
			"role":        task.Role,
			"worker_kind": profile.Kind,
		},
	}); err != nil {
		return TaskRun{}, err
	}

	run, err := o.runner.Start(ctx, TaskRunRequest{
		ProjectID:  strings.TrimSpace(projectID),
		TaskID:     task.ID,
		Title:      task.Title,
		Prompt:     BuildTaskPrompt(task, projectID, profile),
		Agent:      task.Assignee,
		Role:       task.Role,
		WorkerKind: profile.Kind,
	})
	if err != nil {
		_ = o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
			item.Status = "todo"
			item.WorkerKind = profile.Kind
		})
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTaskStatus,
			Status:  "failed",
			Message: firstNonEmpty("Task dispatch failed: "+strings.TrimSpace(err.Error()), "Task dispatch failed"),
			Meta: map[string]string{
				"worker_kind": profile.Kind,
			},
		})
		return TaskRun{}, err
	}

	finished, err := o.runner.Wait(ctx, run.ID)
	if err != nil {
		_ = o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
			item.Status = "todo"
			item.WorkerKind = profile.Kind
		})
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTaskStatus,
			Status:  "failed",
			Message: "Task run failed",
			Meta: map[string]string{
				"run_id":      run.ID,
				"worker_kind": profile.Kind,
			},
		})
		return run, err
	}

	report := ParseTaskReport(finished.Response)
	if finished.Status == TaskRunStatusFailed || finished.Status == TaskRunStatusCanceled {
		if err := o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
			item.Status = "todo"
			item.WorkerKind = firstNonEmpty(strings.TrimSpace(finished.WorkerKind), profile.Kind)
		}); err != nil {
			return finished, err
		}
		_ = o.appendAgentReport(projectID, task, finished, report)
		message := firstNonEmpty(strings.TrimSpace(finished.Error), taskReportSummary(report, finished), "task run failed")
		if err := o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTaskStatus,
			Status:  "failed",
			Message: "Task run failed: " + strings.TrimSpace(message),
			Meta: map[string]string{
				"run_id":                run.ID,
				"agent":                 firstNonEmpty(strings.TrimSpace(finished.Agent), strings.TrimSpace(task.Assignee)),
				"assignee":              strings.TrimSpace(task.Assignee),
				"worker_kind":           firstNonEmpty(strings.TrimSpace(finished.WorkerKind), profile.Kind),
				"worker_executor_agent": strings.TrimSpace(finished.Agent),
				"error":                 strings.TrimSpace(finished.Error),
			},
		}); err != nil {
			return finished, err
		}
		return finished, fmt.Errorf("task run failed: %s", strings.TrimSpace(message))
	}

	finalStatus := "done"
	if task.ReviewRequired {
		finalStatus = "review"
	}
	testStatus := verificationStatus(report.Tests)
	buildStatus := verificationStatus(report.Build)
	issueRef := firstNonEmpty(strings.TrimSpace(report.Issue), strings.TrimSpace(task.Issue))
	branchRef := firstNonEmpty(strings.TrimSpace(report.Branch), strings.TrimSpace(task.Branch))
	prRef := firstNonEmpty(strings.TrimSpace(report.PR), strings.TrimSpace(task.PR))

	if strings.TrimSpace(task.TestCommand) != "" {
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTestStatus,
			Status:  testStatus,
			Message: "Task test verification reported",
			Meta: map[string]string{
				"command": task.TestCommand,
			},
		})
	}
	if strings.TrimSpace(task.BuildCommand) != "" {
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindBuildStatus,
			Status:  buildStatus,
			Message: "Task build verification reported",
			Meta: map[string]string{
				"command": task.BuildCommand,
			},
		})
	}
	_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindIssueStatus,
		Status:  ternaryStatus(issueRef != "", "ready", "blocked"),
		Message: "Task issue metadata recorded",
		Meta: map[string]string{
			"issue": issueRef,
		},
	})
	_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindBranchStatus,
		Status:  ternaryStatus(branchRef != "", "ready", "blocked"),
		Message: "Task branch metadata recorded",
		Meta: map[string]string{
			"branch": branchRef,
		},
	})
	_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindPRStatus,
		Status:  ternaryStatus(prRef != "", "ready", "blocked"),
		Message: "Task pull request metadata recorded",
		Meta: map[string]string{
			"pr": prRef,
		},
	})

	gateErrs := []string{}
	if strings.TrimSpace(task.TestCommand) != "" && testStatus != "passed" {
		gateErrs = append(gateErrs, "tests not passed")
	}
	if strings.TrimSpace(task.BuildCommand) != "" && buildStatus != "passed" {
		gateErrs = append(gateErrs, "build not passed")
	}
	if strings.TrimSpace(issueRef) == "" {
		gateErrs = append(gateErrs, "issue missing")
	}
	if strings.TrimSpace(branchRef) == "" {
		gateErrs = append(gateErrs, "branch missing")
	}
	if strings.TrimSpace(prRef) == "" {
		gateErrs = append(gateErrs, "pr missing")
	}
	if len(gateErrs) > 0 && finished.Status != TaskRunStatusFailed && finished.Status != TaskRunStatusCanceled {
		finalStatus = "in_progress"
	}
	if err := o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
		item.Status = finalStatus
		item.WorkerKind = firstNonEmpty(strings.TrimSpace(finished.WorkerKind), profile.Kind)
		item.Issue = issueRef
		item.Branch = branchRef
		item.PR = prRef
	}); err != nil {
		return finished, err
	}

	activityStatus := finalStatus
	message := "Task run completed"
	if finished.Status == TaskRunStatusFailed || finished.Status == TaskRunStatusCanceled {
		activityStatus = "failed"
		message = "Task run failed"
	} else if len(gateErrs) > 0 {
		activityStatus = "in_progress"
		message = "Task run blocked by verification or GitHub Flow gate"
	}
	if err := o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindTaskStatus,
		Status:  activityStatus,
		Message: message,
		Meta: map[string]string{
			"run_id":      run.ID,
			"agent":       firstNonEmpty(strings.TrimSpace(finished.Agent), strings.TrimSpace(task.Assignee)),
			"assignee":    strings.TrimSpace(task.Assignee),
			"worker_kind": firstNonEmpty(strings.TrimSpace(finished.WorkerKind), profile.Kind),
			"report":      taskReportStatus(report, finished.Status),
		},
	}); err != nil {
		return finished, err
	}
	_ = o.appendAgentReport(projectID, task, finished, report)
	if len(gateErrs) > 0 && finished.Status != TaskRunStatusFailed && finished.Status != TaskRunStatusCanceled {
		return finished, fmt.Errorf("task gate failed: %s", strings.Join(gateErrs, ", "))
	}
	return finished, nil
}

func (o *Orchestrator) dispatchReviewTask(ctx context.Context, projectID string, task BoardTask) (TaskRun, error) {
	reviewTask := task
	reviewTask.Role = "reviewer"
	reviewTask.WorkerKind = ""

	profile, err := ResolveWorkerProfile(reviewTask)
	if err != nil {
		return TaskRun{}, err
	}
	prompt := BuildTaskPrompt(reviewTask, projectID, profile) +
		"\n\nCurrent task status: review" +
		"\nImplementation worker_kind: " + strings.TrimSpace(task.WorkerKind)

	run, err := o.runner.Start(ctx, TaskRunRequest{
		ProjectID:  strings.TrimSpace(projectID),
		TaskID:     task.ID,
		Title:      task.Title,
		Prompt:     prompt,
		Agent:      firstNonEmpty(strings.TrimSpace(task.Assignee), "reviewer"),
		Role:       "reviewer",
		WorkerKind: profile.Kind,
	})
	if err != nil {
		return TaskRun{}, err
	}

	finished, err := o.runner.Wait(ctx, run.ID)
	if err != nil {
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindReviewStatus,
			Status:  "blocked",
			Message: "Review run failed",
			Meta: map[string]string{
				"run_id":      run.ID,
				"worker_kind": profile.Kind,
			},
		})
		return run, err
	}

	report := ParseTaskReport(finished.Response)
	reviewStatus := strings.ToLower(strings.TrimSpace(report.Status))
	nextStatus := "review"
	approvedBy := ""
	message := "Review blocked"
	switch reviewStatus {
	case "approved":
		nextStatus = "done"
		approvedBy = firstNonEmpty(strings.TrimSpace(finished.Agent), profile.Kind)
		message = "Review approved"
	case "rejected":
		nextStatus = "in_progress"
		message = "Review rejected"
	default:
		reviewStatus = "blocked"
	}

	if err := o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
		item.Status = nextStatus
		item.ReviewApprovedBy = approvedBy
	}); err != nil {
		return finished, err
	}

	if err := o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindReviewStatus,
		Status:  reviewStatus,
		Message: message,
		Meta: map[string]string{
			"run_id":      run.ID,
			"worker_kind": profile.Kind,
			"reviewer":    firstNonEmpty(strings.TrimSpace(finished.Agent), profile.Kind),
		},
	}); err != nil {
		return finished, err
	}
	_ = o.appendAgentReport(projectID, task, finished, report)
	return finished, nil
}

func (o *Orchestrator) updateBoardTask(projectID, taskID string, mutate func(*BoardTask)) error {
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
			mutate(&task)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func filterDispatchableTasksByStatus(status string, tasks []BoardTask) []BoardTask {
	return DefaultWorkflowPolicy.FilterDispatchableTasks(status, tasks)
}

func ternaryStatus(ok bool, whenTrue, whenFalse string) string {
	if ok {
		return whenTrue
	}
	return whenFalse
}

func (o *Orchestrator) appendAgentReport(projectID string, task BoardTask, run TaskRun, report TaskReport) error {
	if o == nil || o.store == nil {
		return nil
	}
	status := taskReportStatus(report, run.Status)
	message := taskReportSummary(report, run)
	if strings.TrimSpace(message) == "" {
		message = "Worker reported task progress"
	}
	meta := map[string]string{
		"run_id":                strings.TrimSpace(run.ID),
		"summary":               strings.TrimSpace(report.Summary),
		"notes":                 strings.TrimSpace(report.Notes),
		"tests":                 strings.TrimSpace(report.Tests),
		"build":                 strings.TrimSpace(report.Build),
		"issue":                 strings.TrimSpace(report.Issue),
		"branch":                strings.TrimSpace(report.Branch),
		"pr":                    strings.TrimSpace(report.PR),
		"error":                 strings.TrimSpace(run.Error),
		"worker_kind":           strings.TrimSpace(run.WorkerKind),
		"worker_executor_agent": strings.TrimSpace(run.Agent),
	}
	_, err := o.store.AppendActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Source:  ActivitySourceAgent,
		Agent:   firstNonEmpty(strings.TrimSpace(run.Agent), strings.TrimSpace(task.Assignee)),
		Kind:    ActivityKindAgentReport,
		Status:  status,
		Message: message,
		Meta:    meta,
	})
	return err
}
