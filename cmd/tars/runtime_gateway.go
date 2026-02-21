package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c runtimeClient) listAgents(ctx context.Context) ([]agentDescriptor, error) {
	var payload struct {
		Agents []agentDescriptor `json:"agents"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/agent/agents", nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Agents == nil {
		return []agentDescriptor{}, nil
	}
	return payload.Agents, nil
}

func (c runtimeClient) spawnRun(ctx context.Context, req spawnRequest) (agentRun, error) {
	var run agentRun
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/agent/runs", req, false, &run); err != nil {
		return agentRun{}, err
	}
	return run, nil
}

func (c runtimeClient) listRuns(ctx context.Context, limit int) ([]agentRun, error) {
	if limit <= 0 {
		limit = 30
	}
	var payload struct {
		Runs []agentRun `json:"runs"`
	}
	path := fmt.Sprintf("/v1/agent/runs?limit=%d", limit)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Runs == nil {
		return []agentRun{}, nil
	}
	return payload.Runs, nil
}

func (c runtimeClient) getRun(ctx context.Context, runID string) (agentRun, error) {
	id := strings.TrimSpace(runID)
	if id == "" {
		return agentRun{}, fmt.Errorf("run id is required")
	}
	var run agentRun
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/agent/runs/"+url.PathEscape(id), nil, false, &run); err != nil {
		return agentRun{}, err
	}
	return run, nil
}

func (c runtimeClient) cancelRun(ctx context.Context, runID string) (agentRun, error) {
	id := strings.TrimSpace(runID)
	if id == "" {
		return agentRun{}, fmt.Errorf("run id is required")
	}
	var run agentRun
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/agent/runs/"+url.PathEscape(id)+"/cancel", nil, false, &run); err != nil {
		return agentRun{}, err
	}
	return run, nil
}

func (c runtimeClient) gatewayStatus(ctx context.Context) (gatewayStatus, error) {
	var status gatewayStatus
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/gateway/status", nil, false, &status); err != nil {
		return gatewayStatus{}, err
	}
	return status, nil
}

func (c runtimeClient) gatewayReload(ctx context.Context) (gatewayStatus, error) {
	var status gatewayStatus
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/gateway/reload", nil, true, &status); err != nil {
		return gatewayStatus{}, err
	}
	return status, nil
}

func (c runtimeClient) gatewayRestart(ctx context.Context) (gatewayStatus, error) {
	var status gatewayStatus
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/gateway/restart", nil, true, &status); err != nil {
		return gatewayStatus{}, err
	}
	return status, nil
}

func (c runtimeClient) gatewayReportSummary(ctx context.Context) (gatewayReportSummary, error) {
	var out gatewayReportSummary
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/gateway/reports/summary", nil, false, &out); err != nil {
		return gatewayReportSummary{}, err
	}
	return out, nil
}

func (c runtimeClient) gatewayReportRuns(ctx context.Context, limit int) (gatewayReportRuns, error) {
	if limit <= 0 {
		limit = 50
	}
	var out gatewayReportRuns
	path := fmt.Sprintf("/v1/gateway/reports/runs?limit=%d", limit)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return gatewayReportRuns{}, err
	}
	if out.Runs == nil {
		out.Runs = []agentRun{}
	}
	return out, nil
}

func (c runtimeClient) gatewayReportChannels(ctx context.Context, limit int) (gatewayReportChannels, error) {
	if limit <= 0 {
		limit = 50
	}
	var out gatewayReportChannels
	path := fmt.Sprintf("/v1/gateway/reports/channels?limit=%d", limit)
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return gatewayReportChannels{}, err
	}
	if out.Messages == nil {
		out.Messages = map[string][]channelReportMessage{}
	}
	return out, nil
}
