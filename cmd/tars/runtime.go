package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type runtimeClient struct {
	serverURL     string
	apiToken      string
	adminAPIToken string
	workspaceID   string
	httpClient    *http.Client
}

type agentDescriptor struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	Enabled            bool     `json:"enabled,omitempty"`
	Kind               string   `json:"kind,omitempty"`
	Source             string   `json:"source,omitempty"`
	Entry              string   `json:"entry,omitempty"`
	Default            bool     `json:"default,omitempty"`
	PolicyMode         string   `json:"policy_mode,omitempty"`
	ToolsAllow         []string `json:"tools_allow,omitempty"`
	ToolsAllowCount    int      `json:"tools_allow_count,omitempty"`
	ToolsAllowGroups   []string `json:"tools_allow_groups,omitempty"`
	ToolsAllowPatterns []string `json:"tools_allow_patterns,omitempty"`
	SessionRoutingMode string   `json:"session_routing_mode,omitempty"`
	SessionFixedID     string   `json:"session_fixed_id,omitempty"`
}

type agentRun struct {
	RunID       string `json:"run_id"`
	SessionID   string `json:"session_id,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Status      string `json:"status"`
	Accepted    bool   `json:"accepted"`
	Response    string `json:"response,omitempty"`
	Error       string `json:"error,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type gatewayStatus struct {
	Enabled bool `json:"enabled"`
	Version int  `json:"version"`
}

type spawnRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Message   string `json:"message"`
	Agent     string `json:"agent,omitempty"`
}

type spawnCommand struct {
	SessionID string
	Title     string
	Agent     string
	Wait      bool
	Message   string
}

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

func (c runtimeClient) requestJSON(ctx context.Context, method, path string, body any, admin bool, out any) error {
	endpoint, err := c.resolve(path)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token := strings.TrimSpace(c.apiToken)
	if admin {
		token = strings.TrimSpace(c.adminAPIToken)
		if token == "" {
			token = strings.TrimSpace(c.apiToken)
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ws := strings.TrimSpace(c.workspaceID); ws != "" {
		req.Header.Set("Tars-Workspace-Id", ws)
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s status %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(text)))
	}
	if out == nil || len(text) == 0 {
		return nil
	}
	if err := json.Unmarshal(text, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c runtimeClient) resolve(path string) (string, error) {
	base := strings.TrimSpace(c.serverURL)
	if base == "" {
		base = "http://127.0.0.1:43180"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid server url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}

func parseSpawnCommand(raw string) (spawnCommand, error) {
	args := strings.Fields(strings.TrimSpace(raw))
	cmd := spawnCommand{}
	message := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--wait":
			cmd.Wait = true
		case "--agent", "--title", "--session":
			if i+1 >= len(args) {
				return spawnCommand{}, fmt.Errorf("%s requires a value", a)
			}
			v := args[i+1]
			i++
			switch a {
			case "--agent":
				cmd.Agent = v
			case "--title":
				cmd.Title = v
			case "--session":
				cmd.SessionID = v
			}
		default:
			if strings.HasPrefix(a, "--") {
				return spawnCommand{}, fmt.Errorf("unknown option: %s", a)
			}
			message = append(message, a)
		}
	}
	cmd.Message = strings.TrimSpace(strings.Join(message, " "))
	if cmd.Message == "" {
		return spawnCommand{}, fmt.Errorf("spawn message is required")
	}
	return cmd, nil
}

func isRunTerminal(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "completed", "failed", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func waitRun(ctx context.Context, api runtimeClient, runID string, interval time.Duration) (agentRun, error) {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		run, err := api.getRun(ctx, runID)
		if err != nil {
			return agentRun{}, err
		}
		if isRunTerminal(run.Status) {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return agentRun{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func parseOptionalLimit(v string, fallback int) (int, error) {
	val := strings.TrimSpace(v)
	if val == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	return n, nil
}
