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

func (c runtimeClient) gatewayStatus(ctx context.Context) (gatewayStatus, error) {
	return c.client().GatewayStatus(ctx)
}

func (c runtimeClient) gatewayReload(ctx context.Context) (gatewayStatus, error) {
	return c.client().GatewayReload(ctx)
}

func (c runtimeClient) gatewayRestart(ctx context.Context) (gatewayStatus, error) {
	return c.client().GatewayRestart(ctx)
}

func (c runtimeClient) gatewayReportSummary(ctx context.Context) (gatewayReportSummary, error) {
	return c.client().GatewayReportSummary(ctx)
}

func (c runtimeClient) gatewayReportRuns(ctx context.Context, limit int) (gatewayReportRuns, error) {
	return c.client().GatewayReportRuns(ctx, limit)
}

func (c runtimeClient) gatewayReportChannels(ctx context.Context, limit int) (gatewayReportChannels, error) {
	return c.client().GatewayReportChannels(ctx, limit)
}

func (c runtimeClient) telegramPairings(ctx context.Context) (telegramPairingsInfo, error) {
	return c.client().TelegramPairings(ctx)
}

func (c runtimeClient) approveTelegramPairing(ctx context.Context, code string) (telegramPairingAllowed, error) {
	return c.client().ApproveTelegramPairing(ctx, code)
}
