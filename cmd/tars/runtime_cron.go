package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c runtimeClient) listCronJobs(ctx context.Context) ([]cronJob, error) {
	var out []cronJob
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/cron/jobs", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []cronJob{}, nil
	}
	return out, nil
}

func (c runtimeClient) createCronJob(ctx context.Context, schedule, prompt string) (cronJob, error) {
	s := strings.TrimSpace(schedule)
	p := strings.TrimSpace(prompt)
	if s == "" || p == "" {
		return cronJob{}, fmt.Errorf("schedule and prompt are required")
	}
	req := map[string]any{
		"schedule": s,
		"prompt":   p,
	}
	var out cronJob
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/cron/jobs", req, false, &out); err != nil {
		return cronJob{}, err
	}
	return out, nil
}

func (c runtimeClient) getCronJob(ctx context.Context, jobID string) (cronJob, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return cronJob{}, fmt.Errorf("job id is required")
	}
	var out cronJob
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/cron/jobs/"+url.PathEscape(id), nil, false, &out); err != nil {
		return cronJob{}, err
	}
	return out, nil
}

func (c runtimeClient) updateCronJobEnabled(ctx context.Context, jobID string, enabled bool) (cronJob, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return cronJob{}, fmt.Errorf("job id is required")
	}
	req := map[string]any{"enabled": enabled}
	var out cronJob
	if err := c.requestJSON(ctx, http.MethodPut, "/v1/cron/jobs/"+url.PathEscape(id), req, false, &out); err != nil {
		return cronJob{}, err
	}
	return out, nil
}

func (c runtimeClient) runCronJob(ctx context.Context, jobID string) (string, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return "", fmt.Errorf("job id is required")
	}
	var out struct {
		Response string `json:"response"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/cron/jobs/"+url.PathEscape(id)+"/run", nil, false, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Response), nil
}

func (c runtimeClient) listCronRuns(ctx context.Context, jobID string, limit int) ([]cronRunRecord, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return nil, fmt.Errorf("job id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	var out []cronRunRecord
	path := fmt.Sprintf("/v1/cron/jobs/%s/runs?limit=%d", url.PathEscape(id), limit)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []cronRunRecord{}, nil
	}
	return out, nil
}

func (c runtimeClient) deleteCronJob(ctx context.Context, jobID string) error {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return fmt.Errorf("job id is required")
	}
	_, err := c.requestText(ctx, http.MethodDelete, "/v1/cron/jobs/"+url.PathEscape(id), nil, false)
	return err
}
