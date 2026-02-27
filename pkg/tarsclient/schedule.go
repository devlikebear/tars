package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) ListSchedules(ctx context.Context) ([]ScheduleItem, error) {
	var out []ScheduleItem
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/schedules", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []ScheduleItem{}, nil
	}
	return out, nil
}

func (c *Client) CreateSchedule(ctx context.Context, req ScheduleCreateRequest) (ScheduleItem, error) {
	if strings.TrimSpace(req.Natural) == "" && strings.TrimSpace(req.Schedule) == "" {
		return ScheduleItem{}, fmt.Errorf("natural or schedule is required")
	}
	var out ScheduleItem
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/schedules", req, false, &out); err != nil {
		return ScheduleItem{}, err
	}
	return out, nil
}

func (c *Client) UpdateSchedule(ctx context.Context, id string, req ScheduleUpdateRequest) (ScheduleItem, error) {
	scheduleID := strings.TrimSpace(id)
	if scheduleID == "" {
		return ScheduleItem{}, fmt.Errorf("schedule id is required")
	}
	var out ScheduleItem
	if _, err := c.doJSON(ctx, http.MethodPatch, "/v1/schedules/"+url.PathEscape(scheduleID), req, false, &out); err != nil {
		return ScheduleItem{}, err
	}
	return out, nil
}

func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	scheduleID := strings.TrimSpace(id)
	if scheduleID == "" {
		return fmt.Errorf("schedule id is required")
	}
	_, err := c.doText(ctx, http.MethodDelete, "/v1/schedules/"+url.PathEscape(scheduleID), nil, false)
	return err
}
