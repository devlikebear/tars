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
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/agent/agents", nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Agents == nil {
		return []AgentDescriptor{}, nil
	}
	return payload.Agents, nil
}

func (c *Client) SpawnRun(ctx context.Context, req SpawnRequest) (AgentRun, error) {
	var run AgentRun
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/agent/runs", req, false, &run); err != nil {
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
	path := fmt.Sprintf("/v1/agent/runs?limit=%d", limit)
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
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/agent/runs/"+url.PathEscape(id), nil, false, &run); err != nil {
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
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/agent/runs/"+url.PathEscape(id)+"/cancel", nil, false, &run); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func (c *Client) GatewayStatus(ctx context.Context) (GatewayStatus, error) {
	var status GatewayStatus
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/gateway/status", nil, false, &status); err != nil {
		return GatewayStatus{}, err
	}
	return status, nil
}

func (c *Client) GatewayReload(ctx context.Context) (GatewayStatus, error) {
	var status GatewayStatus
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/gateway/reload", nil, true, &status); err != nil {
		return GatewayStatus{}, err
	}
	return status, nil
}

func (c *Client) GatewayRestart(ctx context.Context) (GatewayStatus, error) {
	var status GatewayStatus
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/gateway/restart", nil, true, &status); err != nil {
		return GatewayStatus{}, err
	}
	return status, nil
}

func (c *Client) GatewayReportSummary(ctx context.Context) (GatewayReportSummary, error) {
	var out GatewayReportSummary
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/gateway/reports/summary", nil, false, &out); err != nil {
		return GatewayReportSummary{}, err
	}
	return out, nil
}

func (c *Client) GatewayReportRuns(ctx context.Context, limit int) (GatewayReportRuns, error) {
	if limit <= 0 {
		limit = 50
	}
	var out GatewayReportRuns
	path := fmt.Sprintf("/v1/gateway/reports/runs?limit=%d", limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return GatewayReportRuns{}, err
	}
	if out.Runs == nil {
		out.Runs = []AgentRun{}
	}
	return out, nil
}

func (c *Client) GatewayReportChannels(ctx context.Context, limit int) (GatewayReportChannels, error) {
	if limit <= 0 {
		limit = 50
	}
	var out GatewayReportChannels
	path := fmt.Sprintf("/v1/gateway/reports/channels?limit=%d", limit)
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return GatewayReportChannels{}, err
	}
	if out.Messages == nil {
		out.Messages = map[string][]ChannelReportMessage{}
	}
	return out, nil
}
