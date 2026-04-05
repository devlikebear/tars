package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	var sessions []SessionSummary
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/sessions", nil, false, &sessions); err != nil {
		return nil, err
	}
	if sessions == nil {
		return []SessionSummary{}, nil
	}
	return sessions, nil
}

func (c *Client) CreateSession(ctx context.Context, title string) (SessionSummary, error) {
	var session SessionSummary
	req := map[string]string{"title": strings.TrimSpace(title)}
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/sessions", req, false, &session); err != nil {
		return SessionSummary{}, err
	}
	return session, nil
}

func (c *Client) GetHistory(ctx context.Context, sessionID string) ([]SessionMessage, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	var messages []SessionMessage
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(id)+"/history", nil, false, &messages); err != nil {
		return nil, err
	}
	if messages == nil {
		return []SessionMessage{}, nil
	}
	return messages, nil
}

func (c *Client) ExportSession(ctx context.Context, sessionID string) (string, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return "", fmt.Errorf("session id is required")
	}
	return c.doText(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/export", nil, false)
}

func (c *Client) SearchSessions(ctx context.Context, keyword string) ([]SessionSummary, error) {
	query := strings.TrimSpace(keyword)
	if query == "" {
		return nil, fmt.Errorf("search keyword is required")
	}
	path := "/v1/sessions/search?q=" + url.QueryEscape(query)
	var sessions []SessionSummary
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &sessions); err != nil {
		return nil, err
	}
	if sessions == nil {
		return []SessionSummary{}, nil
	}
	return sessions, nil
}

func (c *Client) Status(ctx context.Context) (StatusInfo, error) {
	var status StatusInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/status", nil, false, &status); err != nil {
		return StatusInfo{}, err
	}
	return status, nil
}

func (c *Client) Providers(ctx context.Context) (ProvidersInfo, error) {
	var out ProvidersInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/providers", nil, false, &out); err != nil {
		return ProvidersInfo{}, err
	}
	if out.Providers == nil {
		out.Providers = []ProviderInfo{}
	}
	return out, nil
}

func (c *Client) Models(ctx context.Context) (ModelsInfo, error) {
	var out ModelsInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/models", nil, false, &out); err != nil {
		return ModelsInfo{}, err
	}
	if out.Models == nil {
		out.Models = []string{}
	}
	return out, nil
}

func (c *Client) Whoami(ctx context.Context) (WhoamiInfo, error) {
	var out WhoamiInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/auth/whoami", nil, false, &out); err != nil {
		return WhoamiInfo{}, err
	}
	return out, nil
}

func (c *Client) Healthz(ctx context.Context) (HealthInfo, error) {
	var status HealthInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/healthz", nil, false, &status); err != nil {
		return HealthInfo{}, err
	}
	return status, nil
}

func (c *Client) Compact(ctx context.Context, sessionID string) (CompactInfo, error) {
	return c.CompactWithOptions(ctx, CompactRequest{SessionID: sessionID})
}

func (c *Client) CompactWithOptions(ctx context.Context, req CompactRequest) (CompactInfo, error) {
	id := strings.TrimSpace(req.SessionID)
	if id == "" {
		return CompactInfo{}, fmt.Errorf("session id is required")
	}
	req.SessionID = id
	req.Instructions = strings.TrimSpace(req.Instructions)
	var out CompactInfo
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/compact", req, false, &out); err != nil {
		return CompactInfo{}, err
	}
	return out, nil
}

// PulseRunOnce triggers a single pulse watchdog tick on the server and
// returns the resulting tick outcome. Replaces the previous
// HeartbeatRunOnce call — heartbeat has been removed.
func (c *Client) PulseRunOnce(ctx context.Context) (PulseInfo, error) {
	var out PulseInfo
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/pulse/run-once", nil, false, &out); err != nil {
		return PulseInfo{}, err
	}
	return out, nil
}

func (c *Client) ListSkills(ctx context.Context) ([]SkillDef, error) {
	var out []SkillDef
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/skills", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []SkillDef{}, nil
	}
	return out, nil
}

func (c *Client) ListPlugins(ctx context.Context) ([]PluginDef, error) {
	var out []PluginDef
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/plugins", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []PluginDef{}, nil
	}
	return out, nil
}

func (c *Client) ListMCPServers(ctx context.Context) ([]MCPServerInfo, error) {
	var out []MCPServerInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/mcp/servers", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []MCPServerInfo{}, nil
	}
	return out, nil
}

func (c *Client) ListMCPTools(ctx context.Context) ([]MCPToolInfo, error) {
	var out []MCPToolInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/mcp/tools", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []MCPToolInfo{}, nil
	}
	return out, nil
}

func (c *Client) ReloadExtensions(ctx context.Context) (ExtensionsReloadInfo, error) {
	var out ExtensionsReloadInfo
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/runtime/extensions/reload", nil, true, &out); err != nil {
		return ExtensionsReloadInfo{}, err
	}
	return out, nil
}
