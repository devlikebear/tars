package tarsclient

import "context"

func (c runtimeClient) usageSummary(ctx context.Context, period, groupBy string) (usageSummary, usageLimits, usageLimitStatus, error) {
	return c.client().GetUsageSummary(ctx, period, groupBy)
}

func (c runtimeClient) usageLimits(ctx context.Context) (usageLimits, error) {
	return c.client().GetUsageLimits(ctx)
}

func (c runtimeClient) updateUsageLimits(ctx context.Context, req usageLimits) (usageLimits, error) {
	return c.client().UpdateUsageLimits(ctx, req)
}
