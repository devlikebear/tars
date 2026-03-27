package tarsclient

import "context"

func (c runtimeClient) listProjects(ctx context.Context) ([]projectInfo, error) {
	return c.client().ListProjects(ctx)
}

func (c runtimeClient) createProject(ctx context.Context, req projectCreateRequest) (projectInfo, error) {
	return c.client().CreateProject(ctx, req)
}

func (c runtimeClient) getProject(ctx context.Context, projectID string) (projectInfo, error) {
	return c.client().GetProject(ctx, projectID)
}

func (c runtimeClient) updateProject(ctx context.Context, projectID string, req projectUpdateRequest) (projectInfo, error) {
	return c.client().UpdateProject(ctx, projectID, req)
}

func (c runtimeClient) deleteProject(ctx context.Context, projectID string) error {
	return c.client().DeleteProject(ctx, projectID)
}

func (c runtimeClient) activateProject(ctx context.Context, projectID, sessionID string) error {
	return c.client().ActivateProject(ctx, projectID, sessionID)
}

func (c runtimeClient) getProjectBoard(ctx context.Context, projectID string) (projectBoard, error) {
	return c.client().GetProjectBoard(ctx, projectID)
}

func (c runtimeClient) listProjectActivity(ctx context.Context, projectID string, limit int) ([]projectActivity, error) {
	return c.client().ListProjectActivity(ctx, projectID, limit)
}

func (c runtimeClient) dispatchProject(ctx context.Context, projectID, stage string) (projectDispatchReport, error) {
	return c.client().DispatchProject(ctx, projectID, stage)
}

func (c runtimeClient) startProjectAutopilot(ctx context.Context, projectID string) (projectAutopilotRun, error) {
	return c.client().StartProjectAutopilot(ctx, projectID)
}

func (c runtimeClient) advanceProjectAutopilot(ctx context.Context, projectID string) (projectPhaseSnapshot, error) {
	return c.client().AdvanceProjectAutopilot(ctx, projectID)
}

func (c runtimeClient) getProjectAutopilot(ctx context.Context, projectID string) (projectAutopilotRun, error) {
	return c.client().GetProjectAutopilot(ctx, projectID)
}

func (c runtimeClient) usageSummary(ctx context.Context, period, groupBy string) (usageSummary, usageLimits, usageLimitStatus, error) {
	return c.client().GetUsageSummary(ctx, period, groupBy)
}

func (c runtimeClient) usageLimits(ctx context.Context) (usageLimits, error) {
	return c.client().GetUsageLimits(ctx)
}

func (c runtimeClient) updateUsageLimits(ctx context.Context, req usageLimits) (usageLimits, error) {
	return c.client().UpdateUsageLimits(ctx, req)
}
