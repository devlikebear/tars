package main

import (
	"context"
	"net/http"
	"strings"
)

func (c runtimeClient) browserStatus(ctx context.Context) (browserState, error) {
	var out browserState
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/browser/status", nil, false, &out); err != nil {
		return browserState{}, err
	}
	return out, nil
}

func (c runtimeClient) browserProfiles(ctx context.Context) ([]browserProfile, error) {
	var payload struct {
		Profiles []browserProfile `json:"profiles"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/browser/profiles", nil, false, &payload); err != nil {
		return nil, err
	}
	if payload.Profiles == nil {
		return []browserProfile{}, nil
	}
	return payload.Profiles, nil
}

func (c runtimeClient) browserLogin(ctx context.Context, siteID string, profile string) (browserLoginResult, error) {
	payload := map[string]string{
		"site_id": strings.TrimSpace(siteID),
	}
	if strings.TrimSpace(profile) != "" {
		payload["profile"] = strings.TrimSpace(profile)
	}
	var out browserLoginResult
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/browser/login", payload, false, &out); err != nil {
		return browserLoginResult{}, err
	}
	return out, nil
}

func (c runtimeClient) browserCheck(ctx context.Context, siteID string, profile string) (browserCheckResult, error) {
	payload := map[string]string{
		"site_id": strings.TrimSpace(siteID),
	}
	if strings.TrimSpace(profile) != "" {
		payload["profile"] = strings.TrimSpace(profile)
	}
	var out browserCheckResult
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/browser/check", payload, false, &out); err != nil {
		return browserCheckResult{}, err
	}
	return out, nil
}

func (c runtimeClient) browserRun(ctx context.Context, siteID string, flowAction string, profile string) (browserRunResult, error) {
	payload := map[string]string{
		"site_id":     strings.TrimSpace(siteID),
		"flow_action": strings.TrimSpace(flowAction),
	}
	if strings.TrimSpace(profile) != "" {
		payload["profile"] = strings.TrimSpace(profile)
	}
	var out browserRunResult
	if err := c.requestJSON(ctx, http.MethodPost, "/v1/browser/run", payload, false, &out); err != nil {
		return browserRunResult{}, err
	}
	return out, nil
}

func (c runtimeClient) vaultStatus(ctx context.Context) (vaultStatusInfo, error) {
	var out vaultStatusInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/v1/vault/status", nil, false, &out); err != nil {
		return vaultStatusInfo{}, err
	}
	return out, nil
}
