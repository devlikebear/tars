package tarsclient

import "context"

func (c runtimeClient) browserStatus(ctx context.Context) (browserState, error) {
	return c.client().BrowserStatus(ctx)
}

func (c runtimeClient) browserProfiles(ctx context.Context) ([]browserProfile, error) {
	return c.client().BrowserProfiles(ctx)
}

func (c runtimeClient) browserRelay(ctx context.Context) (browserRelayInfo, error) {
	return c.client().BrowserRelay(ctx)
}

func (c runtimeClient) browserLogin(ctx context.Context, siteID string, profile string) (browserLoginResult, error) {
	return c.client().BrowserLogin(ctx, siteID, profile)
}

func (c runtimeClient) browserCheck(ctx context.Context, siteID string, profile string) (browserCheckResult, error) {
	return c.client().BrowserCheck(ctx, siteID, profile)
}

func (c runtimeClient) browserRun(ctx context.Context, siteID string, flowAction string, profile string) (browserRunResult, error) {
	return c.client().BrowserRun(ctx, siteID, flowAction, profile)
}

func (c runtimeClient) vaultStatus(ctx context.Context) (vaultStatusInfo, error) {
	return c.client().VaultStatus(ctx)
}
