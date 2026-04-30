package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) ListAgents(ctx context.Context) ([]AgentDescriptor, error) {
	var payload struct {
		Agents []AgentDescriptor `json:"agents"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/agentruntime/agents", nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Agents == nil {
		return []AgentDescriptor{}, nil
	}
	return payload.Agents, nil
}

func (c *Client) SpawnRun(ctx context.Context, req SpawnRequest) (AgentRun, error) {
	var run AgentRun
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/agentruntime/runs", req, false, &run); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func (c *Client) ListRuns(ctx context.Context, limit int) ([]AgentRun, error) {
	if limit <= 0 {
		limit = 30
	}
	var payload struct {
		Runs []AgentRun `json:"runs"`
	}
	path := fmt.Sprintf("/v1/agentruntime/runs?limit=%d", limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Runs == nil {
		return []AgentRun{}, nil
	}
	return payload.Runs, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (AgentRun, error) {
	id := strings.TrimSpace(runID)
	if id == "" {
		return AgentRun{}, fmt.Errorf("run id is required")
	}
	var run AgentRun
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/agentruntime/runs/"+url.PathEscape(id), nil, false, &run); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func (c *Client) CancelRun(ctx context.Context, runID string) (AgentRun, error) {
	id := strings.TrimSpace(runID)
	if id == "" {
		return AgentRun{}, fmt.Errorf("run id is required")
	}
	var run AgentRun
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/agentruntime/runs/"+url.PathEscape(id)+"/cancel", nil, false, &run); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func (c *Client) AgentRuntimeStatus(ctx context.Context) (AgentRuntimeStatus, error) {
	var status AgentRuntimeStatus
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/agentruntime/status", nil, false, &status); err != nil {
		return AgentRuntimeStatus{}, err
	}
	return status, nil
}

func (c *Client) AgentRuntimeReload(ctx context.Context) (AgentRuntimeStatus, error) {
	var status AgentRuntimeStatus
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/agentruntime/reload", nil, true, &status); err != nil {
		return AgentRuntimeStatus{}, err
	}
	return status, nil
}

func (c *Client) AgentRuntimeRestart(ctx context.Context) (AgentRuntimeStatus, error) {
	var status AgentRuntimeStatus
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/agentruntime/restart", nil, true, &status); err != nil {
		return AgentRuntimeStatus{}, err
	}
	return status, nil
}

func (c *Client) AgentRuntimeReportSummary(ctx context.Context) (AgentRuntimeReportSummary, error) {
	var out AgentRuntimeReportSummary
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/agentruntime/reports/summary", nil, false, &out); err != nil {
		return AgentRuntimeReportSummary{}, err
	}
	return out, nil
}

func (c *Client) AgentRuntimeReportRuns(ctx context.Context, limit int) (AgentRuntimeReportRuns, error) {
	if limit <= 0 {
		limit = 50
	}
	var out AgentRuntimeReportRuns
	path := fmt.Sprintf("/v1/agentruntime/reports/runs?limit=%d", limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return AgentRuntimeReportRuns{}, err
	}
	if out.Runs == nil {
		out.Runs = []AgentRun{}
	}
	return out, nil
}

func (c *Client) AgentRuntimeReportChannels(ctx context.Context, limit int) (AgentRuntimeReportChannels, error) {
	if limit <= 0 {
		limit = 50
	}
	var out AgentRuntimeReportChannels
	path := fmt.Sprintf("/v1/agentruntime/reports/channels?limit=%d", limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return AgentRuntimeReportChannels{}, err
	}
	if out.Messages == nil {
		out.Messages = map[string][]ChannelReportMessage{}
	}
	return out, nil
}
