package tarsclient

import "context"

func (c runtimeClient) eventHistory(ctx context.Context, limit int) (eventsHistoryInfo, error) {
	return c.client().GetEventHistory(ctx, limit)
}

func (c runtimeClient) markEventsRead(ctx context.Context, lastID int64) (eventsReadInfo, error) {
	return c.client().MarkEventsRead(ctx, lastID)
}
