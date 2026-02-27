package tarsclient

import "context"

func (c runtimeClient) listSchedules(ctx context.Context) ([]scheduleItem, error) {
	return c.client().ListSchedules(ctx)
}

func (c runtimeClient) createSchedule(ctx context.Context, req scheduleCreateRequest) (scheduleItem, error) {
	return c.client().CreateSchedule(ctx, req)
}

func (c runtimeClient) updateSchedule(ctx context.Context, id string, req scheduleUpdateRequest) (scheduleItem, error) {
	return c.client().UpdateSchedule(ctx, id, req)
}

func (c runtimeClient) deleteSchedule(ctx context.Context, id string) error {
	return c.client().DeleteSchedule(ctx, id)
}
