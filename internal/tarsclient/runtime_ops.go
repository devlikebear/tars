package tarsclient

import "context"

func (c runtimeClient) opsStatus(ctx context.Context) (opsStatus, error) {
	return c.client().OpsStatus(ctx)
}

func (c runtimeClient) opsCleanupPlan(ctx context.Context) (cleanupPlan, error) {
	return c.client().CreateCleanupPlan(ctx)
}
