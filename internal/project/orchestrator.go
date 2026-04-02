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
	skillResolver     SkillResolver
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

// SetSkillResolver sets the resolver used to inject project skill content
// into task prompts. May be nil.
func (o *Orchestrator) SetSkillResolver(r SkillResolver) {
	o.skillResolver = r
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
	tasks := DefaultWorkflowPolicy.TasksForDispatchStage(board, status)
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
	projectItem, err := o.store.Get(projectID)
	if err != nil {
		return TaskRun{}, err
	}
	workflow := ResolveWorkflowExecutionPolicy(projectItem)

	profile, err := o.prepareTaskDispatch(ctx, projectID, projectItem, workflow, task)
	if err != nil {
		return TaskRun{}, err
	}

	skills := o.resolveProjectSkills(projectItem)
	run, err := o.startTaskRun(ctx, projectID, task, profile, skills)
	if err != nil {
		return TaskRun{}, err
	}

	finished, err := o.waitTaskRun(ctx, projectID, task, run, profile)
	if err != nil {
		return finished, err
	}

	report := ParseTaskReport(finished.Response)
	if finished.Status == TaskRunStatusFailed || finished.Status == TaskRunStatusCanceled {
		return o.handleFailedTaskRun(projectID, task, run, finished, profile, report)
	}

	resolution := o.newDispatchTaskResolution(task, report)
	o.recordDispatchVerificationActivities(projectID, task, resolution)
	gateErrs := o.verifyDispatchGates(workflow, task, resolution)
	resolution.FinalStatus = o.resolveTaskFinalStatus(task, finished, gateErrs)
	if err := o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
		item.Status = resolution.FinalStatus
		item.WorkerKind = firstNonEmpty(strings.TrimSpace(finished.WorkerKind), profile.Kind)
		item.Issue = resolution.IssueRef
		item.Branch = resolution.BranchRef
		item.PR = resolution.PRRef
	}); err != nil {
		return finished, err
	}
	if err := o.appendDispatchCompletionActivity(projectID, task, run.ID, finished, profile, report, resolution.FinalStatus, gateErrs); err != nil {
		return finished, err
	}
	_ = o.appendAgentReport(projectID, task, finished, report)
	if len(gateErrs) > 0 {
		return finished, fmt.Errorf("task gate failed: %s", strings.Join(gateErrs, ", "))
	}
	return finished, nil
}

type dispatchTaskResolution struct {
	FinalStatus string
	TestStatus  string
	BuildStatus string
	IssueRef    string
	BranchRef   string
	PRRef       string
}

func (o *Orchestrator) prepareTaskDispatch(ctx context.Context, projectID string, projectItem Project, workflow WorkflowExecutionPolicy, task BoardTask) (WorkerProfile, error) {
	profile, err := ResolveWorkerProfileForProject(projectItem, task)
	if err != nil {
		return WorkerProfile{}, err
	}
	if workflow.RequireGitHubAuth && o.githubAuthChecker != nil {
		if err := o.githubAuthChecker(ctx); err != nil {
			wrapped := fmt.Errorf("github auth precondition failed: %w", err)
			_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
				TaskID:  task.ID,
				Kind:    ActivityKindIssueStatus,
				Status:  "blocked",
				Message: strings.TrimSpace(wrapped.Error()),
			})
			return WorkerProfile{}, wrapped
		}
	}
	if err := o.updateBoardTask(projectID, task.ID, func(item *BoardTask) {
		item.Status = "in_progress"
		item.WorkerKind = profile.Kind
	}); err != nil {
		return WorkerProfile{}, err
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
		return WorkerProfile{}, err
	}
	return profile, nil
}

func (o *Orchestrator) resolveProjectSkills(p Project) []SkillContent {
	if o.skillResolver == nil || len(p.SkillsAllow) == 0 {
		return nil
	}
	return o.skillResolver.ResolveSkills(p.SkillsAllow)
}

func (o *Orchestrator) startTaskRun(ctx context.Context, projectID string, task BoardTask, profile WorkerProfile, skills []SkillContent) (TaskRun, error) {
	run, err := o.runner.Start(ctx, TaskRunRequest{
		ProjectID:  strings.TrimSpace(projectID),
		TaskID:     task.ID,
		Title:      task.Title,
		Prompt:     BuildTaskPrompt(task, projectID, profile, skills...),
		Agent:      task.Assignee,
		Role:       task.Role,
		WorkerKind: profile.Kind,
	})
	if err == nil {
		return run, nil
	}
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

func (o *Orchestrator) waitTaskRun(ctx context.Context, projectID string, task BoardTask, run TaskRun, profile WorkerProfile) (TaskRun, error) {
	finished, err := o.runner.Wait(ctx, run.ID)
	if err == nil {
		return finished, nil
	}
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

func (o *Orchestrator) handleFailedTaskRun(
	projectID string,
	task BoardTask,
	run TaskRun,
	finished TaskRun,
	profile WorkerProfile,
	report TaskReport,
) (TaskRun, error) {
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

func (o *Orchestrator) newDispatchTaskResolution(task BoardTask, report TaskReport) dispatchTaskResolution {
	return dispatchTaskResolution{
		TestStatus:  verificationStatus(report.Tests),
		BuildStatus: verificationStatus(report.Build),
		IssueRef:    firstNonEmpty(strings.TrimSpace(report.Issue), strings.TrimSpace(task.Issue)),
		BranchRef:   firstNonEmpty(strings.TrimSpace(report.Branch), strings.TrimSpace(task.Branch)),
		PRRef:       firstNonEmpty(strings.TrimSpace(report.PR), strings.TrimSpace(task.PR)),
	}
}

func (o *Orchestrator) verifyDispatchGates(workflow WorkflowExecutionPolicy, task BoardTask, resolution dispatchTaskResolution) []string {
	gateErrs := []string{}
	if workflow.RequireTests && strings.TrimSpace(task.TestCommand) != "" && resolution.TestStatus != "passed" {
		gateErrs = append(gateErrs, "tests not passed")
	}
	if workflow.RequireBuild && strings.TrimSpace(task.BuildCommand) != "" && resolution.BuildStatus != "passed" {
		gateErrs = append(gateErrs, "build not passed")
	}
	if workflow.RequireIssue && strings.TrimSpace(resolution.IssueRef) == "" {
		gateErrs = append(gateErrs, "issue missing")
	}
	if workflow.RequireBranch && strings.TrimSpace(resolution.BranchRef) == "" {
		gateErrs = append(gateErrs, "branch missing")
	}
	if workflow.RequirePR && strings.TrimSpace(resolution.PRRef) == "" {
		gateErrs = append(gateErrs, "pr missing")
	}
	return gateErrs
}

func (o *Orchestrator) resolveTaskFinalStatus(task BoardTask, finished TaskRun, gateErrs []string) string {
	if finished.Status == TaskRunStatusFailed || finished.Status == TaskRunStatusCanceled {
		return "todo"
	}
	if len(gateErrs) > 0 {
		return "in_progress"
	}
	if task.ReviewRequired {
		return "review"
	}
	return "done"
}

func (o *Orchestrator) recordDispatchVerificationActivities(projectID string, task BoardTask, resolution dispatchTaskResolution) {
	if strings.TrimSpace(task.TestCommand) != "" {
		_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindTestStatus,
			Status:  resolution.TestStatus,
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
			Status:  resolution.BuildStatus,
			Message: "Task build verification reported",
			Meta: map[string]string{
				"command": task.BuildCommand,
			},
		})
	}
	_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindIssueStatus,
		Status:  ternaryStatus(resolution.IssueRef != "", "ready", "blocked"),
		Message: "Task issue metadata recorded",
		Meta: map[string]string{
			"issue": resolution.IssueRef,
		},
	})
	_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindBranchStatus,
		Status:  ternaryStatus(resolution.BranchRef != "", "ready", "blocked"),
		Message: "Task branch metadata recorded",
		Meta: map[string]string{
			"branch": resolution.BranchRef,
		},
	})
	_ = o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindPRStatus,
		Status:  ternaryStatus(resolution.PRRef != "", "ready", "blocked"),
		Message: "Task pull request metadata recorded",
		Meta: map[string]string{
			"pr": resolution.PRRef,
		},
	})
}

func (o *Orchestrator) appendDispatchCompletionActivity(
	projectID string,
	task BoardTask,
	runID string,
	finished TaskRun,
	profile WorkerProfile,
	report TaskReport,
	finalStatus string,
	gateErrs []string,
) error {
	activityStatus := finalStatus
	message := "Task run completed"
	if len(gateErrs) > 0 {
		activityStatus = "in_progress"
		message = "Task run blocked by verification or GitHub Flow gate"
	}
	return o.store.appendSystemActivity(projectID, ActivityAppendInput{
		TaskID:  task.ID,
		Kind:    ActivityKindTaskStatus,
		Status:  activityStatus,
		Message: message,
		Meta: map[string]string{
			"run_id":      runID,
			"agent":       firstNonEmpty(strings.TrimSpace(finished.Agent), strings.TrimSpace(task.Assignee)),
			"assignee":    strings.TrimSpace(task.Assignee),
			"worker_kind": firstNonEmpty(strings.TrimSpace(finished.WorkerKind), profile.Kind),
			"report":      taskReportStatus(report, finished.Status),
		},
	})
}

func (o *Orchestrator) dispatchReviewTask(ctx context.Context, projectID string, task BoardTask) (TaskRun, error) {
	projectItem, err := o.store.Get(projectID)
	if err != nil {
		return TaskRun{}, err
	}
	reviewTask := task
	reviewTask.Role = "reviewer"
	reviewTask.WorkerKind = ""

	profile, err := ResolveWorkerProfileForProject(projectItem, reviewTask)
	if err != nil {
		return TaskRun{}, err
	}
	skills := o.resolveProjectSkills(projectItem)
	prompt := BuildTaskPrompt(reviewTask, projectID, profile, skills...) +
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
