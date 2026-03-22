package tarsclient

import "context"

func (c runtimeClient) listSessions(ctx context.Context) ([]sessionSummary, error) {
	return c.client().ListSessions(ctx)
}

func (c runtimeClient) createSession(ctx context.Context, title string) (sessionSummary, error) {
	return c.client().CreateSession(ctx, title)
}

func (c runtimeClient) getHistory(ctx context.Context, sessionID string) ([]sessionMessage, error) {
	return c.client().GetHistory(ctx, sessionID)
}

func (c runtimeClient) exportSession(ctx context.Context, sessionID string) (string, error) {
	return c.client().ExportSession(ctx, sessionID)
}

func (c runtimeClient) searchSessions(ctx context.Context, keyword string) ([]sessionSummary, error) {
	return c.client().SearchSessions(ctx, keyword)
}

func (c runtimeClient) status(ctx context.Context) (statusInfo, error) {
	return c.client().Status(ctx)
}

func (c runtimeClient) providers(ctx context.Context) (providersInfo, error) {
	return c.client().Providers(ctx)
}

func (c runtimeClient) models(ctx context.Context) (modelsInfo, error) {
	return c.client().Models(ctx)
}

func (c runtimeClient) whoami(ctx context.Context) (whoamiInfo, error) {
	return c.client().Whoami(ctx)
}

func (c runtimeClient) healthz(ctx context.Context) (healthInfo, error) {
	return c.client().Healthz(ctx)
}

func (c runtimeClient) compact(ctx context.Context, req compactRequest) (compactInfo, error) {
	return c.client().CompactWithOptions(ctx, req)
}

func (c runtimeClient) heartbeatRunOnce(ctx context.Context) (heartbeatInfo, error) {
	return c.client().HeartbeatRunOnce(ctx)
}

func (c runtimeClient) listSkills(ctx context.Context) ([]skillDef, error) {
	return c.client().ListSkills(ctx)
}

func (c runtimeClient) listPlugins(ctx context.Context) ([]pluginDef, error) {
	return c.client().ListPlugins(ctx)
}

func (c runtimeClient) listMCPServers(ctx context.Context) ([]mcpServerInfo, error) {
	return c.client().ListMCPServers(ctx)
}

func (c runtimeClient) listMCPTools(ctx context.Context) ([]mcpToolInfo, error) {
	return c.client().ListMCPTools(ctx)
}

func (c runtimeClient) reloadExtensions(ctx context.Context) (extensionsReloadInfo, error) {
	return c.client().ReloadExtensions(ctx)
}
