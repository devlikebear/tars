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

type sessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type sessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

type statusInfo struct {
	WorkspaceDir string `json:"workspace_dir"`
	SessionCount int    `json:"session_count"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	AuthRole     string `json:"auth_role,omitempty"`
}

type compactInfo struct {
	Message string `json:"message"`
}

type heartbeatInfo struct {
	Response     string `json:"response"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skip_reason,omitempty"`
	Acknowledged bool   `json:"acknowledged"`
	Logged       bool   `json:"logged"`
}

type skillDef struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	UserInvocable bool   `json:"user_invocable"`
	Source        string `json:"source,omitempty"`
	RuntimePath   string `json:"runtime_path,omitempty"`
}

type pluginDef struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
	RootDir string `json:"root_dir,omitempty"`
}

type mcpServerInfo struct {
	Name      string `json:"name"`
	Command   string `json:"command,omitempty"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type mcpToolInfo struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type extensionsReloadInfo struct {
	Reloaded         bool  `json:"reloaded"`
	Version          int64 `json:"version,omitempty"`
	Skills           int   `json:"skills,omitempty"`
	Plugins          int   `json:"plugins,omitempty"`
	MCPCount         int   `json:"mcp_count,omitempty"`
	GatewayRefreshed bool  `json:"gateway_refreshed,omitempty"`
	GatewayAgents    int   `json:"gateway_agents,omitempty"`
}

type cronJob struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	Schedule       string `json:"schedule"`
	Enabled        bool   `json:"enabled"`
	DeleteAfterRun bool   `json:"delete_after_run,omitempty"`
	SessionTarget  string `json:"session_target,omitempty"`
	WakeMode       string `json:"wake_mode,omitempty"`
	DeliveryMode   string `json:"delivery_mode,omitempty"`
	LastRunAt      string `json:"last_run_at,omitempty"`
	LastRunError   string `json:"last_run_error,omitempty"`
}

type cronRunRecord struct {
	JobID    string `json:"job_id"`
	RanAt    string `json:"ran_at"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
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
	Enabled                 bool `json:"enabled"`
	Version                 int  `json:"version"`
	ChannelsLocalEnabled    bool `json:"channels_local_enabled"`
	ChannelsWebhookEnabled  bool `json:"channels_webhook_enabled"`
	ChannelsTelegramEnabled bool `json:"channels_telegram_enabled"`
}

type gatewayReportSummary struct {
	GeneratedAt      string         `json:"generated_at"`
	SummaryEnabled   bool           `json:"summary_enabled"`
	ArchiveEnabled   bool           `json:"archive_enabled"`
	RunsTotal        int            `json:"runs_total"`
	RunsActive       int            `json:"runs_active"`
	RunsByStatus     map[string]int `json:"runs_by_status"`
	ChannelsTotal    int            `json:"channels_total"`
	MessagesTotal    int            `json:"messages_total"`
	MessagesBySource map[string]int `json:"messages_by_source"`
}

type gatewayReportRuns struct {
	GeneratedAt    string     `json:"generated_at"`
	ArchiveEnabled bool       `json:"archive_enabled"`
	Count          int        `json:"count"`
	Runs           []agentRun `json:"runs"`
}

type channelReportMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Source    string `json:"source"`
	Direction string `json:"direction"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type gatewayReportChannels struct {
	GeneratedAt    string                            `json:"generated_at"`
	ArchiveEnabled bool                              `json:"archive_enabled"`
	Count          int                               `json:"count"`
	Messages       map[string][]channelReportMessage `json:"messages"`
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

func (c runtimeClient) listSessions(ctx context.Context) ([]sessionSummary, error) {
	var sessions []sessionSummary
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/sessions", nil, false, &sessions); err != nil {
		return nil, err
	}
	if sessions == nil {
		return []sessionSummary{}, nil
	}
	return sessions, nil
}

func (c runtimeClient) createSession(ctx context.Context, title string) (sessionSummary, error) {
	var session sessionSummary
	req := map[string]string{"title": strings.TrimSpace(title)}
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/sessions", req, false, &session); err != nil {
		return sessionSummary{}, err
	}
	return session, nil
}

func (c runtimeClient) getHistory(ctx context.Context, sessionID string) ([]sessionMessage, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	var messages []sessionMessage
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(id)+"/history", nil, false, &messages); err != nil {
		return nil, err
	}
	if messages == nil {
		return []sessionMessage{}, nil
	}
	return messages, nil
}

func (c runtimeClient) exportSession(ctx context.Context, sessionID string) (string, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return "", fmt.Errorf("session id is required")
	}
	text, err := c.requestText(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/export", nil, false)
	if err != nil {
		return "", err
	}
	return text, nil
}

func (c runtimeClient) searchSessions(ctx context.Context, keyword string) ([]sessionSummary, error) {
	query := strings.TrimSpace(keyword)
	if query == "" {
		return nil, fmt.Errorf("search keyword is required")
	}
	path := "/v1/sessions/search?q=" + url.QueryEscape(query)
	var sessions []sessionSummary
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, false, &sessions); err != nil {
		return nil, err
	}
	if sessions == nil {
		return []sessionSummary{}, nil
	}
	return sessions, nil
}

func (c runtimeClient) status(ctx context.Context) (statusInfo, error) {
	var status statusInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/status", nil, false, &status); err != nil {
		return statusInfo{}, err
	}
	return status, nil
}

func (c runtimeClient) compact(ctx context.Context, sessionID string) (compactInfo, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return compactInfo{}, fmt.Errorf("session id is required")
	}
	var out compactInfo
	req := map[string]string{"session_id": id}
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/compact", req, false, &out); err != nil {
		return compactInfo{}, err
	}
	return out, nil
}

func (c runtimeClient) heartbeatRunOnce(ctx context.Context) (heartbeatInfo, error) {
	var out heartbeatInfo
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/heartbeat/run-once", nil, false, &out); err != nil {
		return heartbeatInfo{}, err
	}
	return out, nil
}

func (c runtimeClient) listSkills(ctx context.Context) ([]skillDef, error) {
	var out []skillDef
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/skills", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []skillDef{}, nil
	}
	return out, nil
}

func (c runtimeClient) listPlugins(ctx context.Context) ([]pluginDef, error) {
	var out []pluginDef
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/plugins", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []pluginDef{}, nil
	}
	return out, nil
}

func (c runtimeClient) listMCPServers(ctx context.Context) ([]mcpServerInfo, error) {
	var out []mcpServerInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/mcp/servers", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []mcpServerInfo{}, nil
	}
	return out, nil
}

func (c runtimeClient) listMCPTools(ctx context.Context) ([]mcpToolInfo, error) {
	var out []mcpToolInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/mcp/tools", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []mcpToolInfo{}, nil
	}
	return out, nil
}

func (c runtimeClient) reloadExtensions(ctx context.Context) (extensionsReloadInfo, error) {
	var out extensionsReloadInfo
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/runtime/extensions/reload", nil, true, &out); err != nil {
		return extensionsReloadInfo{}, err
	}
	return out, nil
}

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

func (c runtimeClient) requestJSON(ctx context.Context, method, path string, body any, admin bool, out any) error {
	text, err := c.requestText(ctx, method, path, body, admin)
	if err != nil {
		return err
	}
	if out == nil || len(text) == 0 {
		return nil
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c runtimeClient) requestText(ctx context.Context, method, path string, body any, admin bool) (string, error) {
	endpoint, err := c.resolve(path)
	if err != nil {
		return "", err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return "", err
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
		return "", err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s %s status %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(text)))
	}
	return string(text), nil
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
	rawPath := strings.TrimSpace(path)
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	ref, err := url.Parse(rawPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + ref.Path
	u.RawQuery = ref.RawQuery
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
