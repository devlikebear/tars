package tarsclient

import "context"

func (c runtimeClient) listAgents(ctx context.Context) ([]agentDescriptor, error) {
	return c.client().ListAgents(ctx)
}

func (c runtimeClient) spawnRun(ctx context.Context, req agentSpawnRequest) (agentRun, error) {
	return c.client().SpawnRun(ctx, req)
}

func (c runtimeClient) listRuns(ctx context.Context, limit int) ([]agentRun, error) {
	return c.client().ListRuns(ctx, limit)
}

func (c runtimeClient) getRun(ctx context.Context, runID string) (agentRun, error) {
	return c.client().GetRun(ctx, runID)
}

func (c runtimeClient) cancelRun(ctx context.Context, runID string) (agentRun, error) {
	return c.client().CancelRun(ctx, runID)
}

func (c runtimeClient) agentRuntimeStatus(ctx context.Context) (agentRuntimeStatus, error) {
	return c.client().AgentRuntimeStatus(ctx)
}

func (c runtimeClient) agentRuntimeReload(ctx context.Context) (agentRuntimeStatus, error) {
	return c.client().AgentRuntimeReload(ctx)
}

func (c runtimeClient) agentRuntimeRestart(ctx context.Context) (agentRuntimeStatus, error) {
	return c.client().AgentRuntimeRestart(ctx)
}

func (c runtimeClient) agentRuntimeReportSummary(ctx context.Context) (agentRuntimeReportSummary, error) {
	return c.client().AgentRuntimeReportSummary(ctx)
}

func (c runtimeClient) agentRuntimeReportRuns(ctx context.Context, limit int) (agentRuntimeReportRuns, error) {
	return c.client().AgentRuntimeReportRuns(ctx, limit)
}

func (c runtimeClient) agentRuntimeReportChannels(ctx context.Context, limit int) (agentRuntimeReportChannels, error) {
	return c.client().AgentRuntimeReportChannels(ctx, limit)
}

func (c runtimeClient) telegramPairings(ctx context.Context) (telegramPairingsInfo, error) {
	return c.client().TelegramPairings(ctx)
}

func (c runtimeClient) approveTelegramPairing(ctx context.Context, code string) (telegramPairingAllowed, error) {
	return c.client().ApproveTelegramPairing(ctx, code)
}
