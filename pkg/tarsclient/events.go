package tarsclient

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) GetEventHistory(ctx context.Context, limit int) (EventsHistoryInfo, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var out EventsHistoryInfo
	path := fmt.Sprintf("/v1/events/history?limit=%d", limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return EventsHistoryInfo{}, err
	}
	if out.Items == nil {
		out.Items = []NotificationMessage{}
	}
	return out, nil
}

func (c *Client) MarkEventsRead(ctx context.Context, lastID int64) (EventsReadInfo, error) {
	if lastID < 0 {
		lastID = 0
	}
	req := map[string]any{
		"last_id": lastID,
	}
	var out EventsReadInfo
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/events/read", req, false, &out); err != nil {
		return EventsReadInfo{}, err
	}
	return out, nil
}
