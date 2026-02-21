package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

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

func (c runtimeClient) whoami(ctx context.Context) (whoamiInfo, error) {
	var out whoamiInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/auth/whoami", nil, false, &out); err != nil {
		return whoamiInfo{}, err
	}
	return out, nil
}

func (c runtimeClient) healthz(ctx context.Context) (healthInfo, error) {
	var status healthInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/healthz", nil, false, &status); err != nil {
		return healthInfo{}, err
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
