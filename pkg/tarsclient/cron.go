package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) ListCronJobs(ctx context.Context) ([]CronJob, error) {
	var out []CronJob
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/cron/jobs", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []CronJob{}, nil
	}
	return out, nil
}

func (c *Client) CreateCronJob(ctx context.Context, schedule, prompt string) (CronJob, error) {
	s := strings.TrimSpace(schedule)
	p := strings.TrimSpace(prompt)
	if s == "" || p == "" {
		return CronJob{}, fmt.Errorf("schedule and prompt are required")
	}
	req := map[string]any{
		"schedule": s,
		"prompt":   p,
	}
	var out CronJob
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/cron/jobs", req, false, &out); err != nil {
		return CronJob{}, err
	}
	return out, nil
}

func (c *Client) GetCronJob(ctx context.Context, jobID string) (CronJob, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return CronJob{}, fmt.Errorf("job id is required")
	}
	var out CronJob
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/cron/jobs/"+url.PathEscape(id), nil, false, &out); err != nil {
		return CronJob{}, err
	}
	return out, nil
}

func (c *Client) UpdateCronJobEnabled(ctx context.Context, jobID string, enabled bool) (CronJob, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return CronJob{}, fmt.Errorf("job id is required")
	}
	req := map[string]any{"enabled": enabled}
	var out CronJob
	if _, err := c.doJSON(ctx, http.MethodPut, "/v1/cron/jobs/"+url.PathEscape(id), req, false, &out); err != nil {
		return CronJob{}, err
	}
	return out, nil
}

func (c *Client) RunCronJob(ctx context.Context, jobID string) (string, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return "", fmt.Errorf("job id is required")
	}
	var out struct {
		Response string `json:"response"`
	}
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/cron/jobs/"+url.PathEscape(id)+"/run", nil, false, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Response), nil
}

func (c *Client) ListCronRuns(ctx context.Context, jobID string, limit int) ([]CronRunRecord, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return nil, fmt.Errorf("job id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	var out []CronRunRecord
	path := fmt.Sprintf("/v1/cron/jobs/%s/runs?limit=%d", url.PathEscape(id), limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []CronRunRecord{}, nil
	}
	return out, nil
}

func (c *Client) DeleteCronJob(ctx context.Context, jobID string) error {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return fmt.Errorf("job id is required")
	}
	_, err := c.doText(ctx, http.MethodDelete, "/v1/cron/jobs/"+url.PathEscape(id), nil, false)
	return err
}
