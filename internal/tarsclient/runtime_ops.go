package tarsclient

import "context"

func (c runtimeClient) opsStatus(ctx context.Context) (opsStatus, error) {
	return c.client().OpsStatus(ctx)
}

func (c runtimeClient) opsCleanupPlan(ctx context.Context) (cleanupPlan, error) {
	return c.client().CreateCleanupPlan(ctx)
}

func (c runtimeClient) opsCleanupApply(ctx context.Context, approvalID string) (cleanupApplyResult, error) {
	return c.client().ApplyCleanup(ctx, approvalID)
}

func (c runtimeClient) listApprovals(ctx context.Context) ([]approvalItem, error) {
	return c.client().ListApprovals(ctx)
}

func (c runtimeClient) approveCleanup(ctx context.Context, approvalID string) error {
	return c.client().ApproveCleanup(ctx, approvalID)
}

func (c runtimeClient) rejectCleanup(ctx context.Context, approvalID string) error {
	return c.client().RejectCleanup(ctx, approvalID)
}
