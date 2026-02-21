package tarsclient

import "context"

func (c runtimeClient) listCronJobs(ctx context.Context) ([]cronJob, error) {
	return c.client().ListCronJobs(ctx)
}

func (c runtimeClient) createCronJob(ctx context.Context, schedule, prompt string) (cronJob, error) {
	return c.client().CreateCronJob(ctx, schedule, prompt)
}

func (c runtimeClient) getCronJob(ctx context.Context, jobID string) (cronJob, error) {
	return c.client().GetCronJob(ctx, jobID)
}

func (c runtimeClient) updateCronJobEnabled(ctx context.Context, jobID string, enabled bool) (cronJob, error) {
	return c.client().UpdateCronJobEnabled(ctx, jobID, enabled)
}

func (c runtimeClient) runCronJob(ctx context.Context, jobID string) (string, error) {
	return c.client().RunCronJob(ctx, jobID)
}

func (c runtimeClient) listCronRuns(ctx context.Context, jobID string, limit int) ([]cronRunRecord, error) {
	return c.client().ListCronRuns(ctx, jobID, limit)
}

func (c runtimeClient) deleteCronJob(ctx context.Context, jobID string) error {
	return c.client().DeleteCronJob(ctx, jobID)
}
