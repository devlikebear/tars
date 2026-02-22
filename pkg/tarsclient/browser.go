package tarsclient

import (
	"context"
	"net/http"
	"strings"
)

func (c *Client) BrowserStatus(ctx context.Context) (BrowserState, error) {
	var out BrowserState
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/browser/status", nil, false, &out); err != nil {
		return BrowserState{}, err
	}
	return out, nil
}

func (c *Client) BrowserProfiles(ctx context.Context) ([]BrowserProfile, error) {
	var payload struct {
		Profiles []BrowserProfile `json:"profiles"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/browser/profiles", nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Profiles == nil {
		return []BrowserProfile{}, nil
	}
	return payload.Profiles, nil
}

func (c *Client) BrowserRelay(ctx context.Context) (BrowserRelayInfo, error) {
	var out BrowserRelayInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/browser/relay", nil, false, &out); err != nil {
		return BrowserRelayInfo{}, err
	}
	if out.OriginAllowlist == nil {
		out.OriginAllowlist = []string{}
	}
	return out, nil
}

func (c *Client) BrowserLogin(ctx context.Context, siteID string, profile string) (BrowserLoginResult, error) {
	payload := map[string]string{
		"site_id": strings.TrimSpace(siteID),
	}
	if strings.TrimSpace(profile) != "" {
		payload["profile"] = strings.TrimSpace(profile)
	}
	var out BrowserLoginResult
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/browser/login", payload, false, &out); err != nil {
		return BrowserLoginResult{}, err
	}
	return out, nil
}

func (c *Client) BrowserCheck(ctx context.Context, siteID string, profile string) (BrowserCheckResult, error) {
	payload := map[string]string{
		"site_id": strings.TrimSpace(siteID),
	}
	if strings.TrimSpace(profile) != "" {
		payload["profile"] = strings.TrimSpace(profile)
	}
	var out BrowserCheckResult
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/browser/check", payload, false, &out); err != nil {
		return BrowserCheckResult{}, err
	}
	return out, nil
}

func (c *Client) BrowserRun(ctx context.Context, siteID string, flowAction string, profile string) (BrowserRunResult, error) {
	payload := map[string]string{
		"site_id":     strings.TrimSpace(siteID),
		"flow_action": strings.TrimSpace(flowAction),
	}
	if strings.TrimSpace(profile) != "" {
		payload["profile"] = strings.TrimSpace(profile)
	}
	var out BrowserRunResult
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/browser/run", payload, false, &out); err != nil {
		return BrowserRunResult{}, err
	}
	return out, nil
}

func (c *Client) VaultStatus(ctx context.Context) (VaultStatusInfo, error) {
	var out VaultStatusInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/vault/status", nil, false, &out); err != nil {
		return VaultStatusInfo{}, err
	}
	return out, nil
}
