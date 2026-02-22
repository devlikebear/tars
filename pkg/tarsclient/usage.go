package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) GetUsageSummary(ctx context.Context, period string, groupBy string) (UsageSummary, UsageLimits, UsageLimitStatus, error) {
	values := url.Values{}
	if v := strings.TrimSpace(period); v != "" {
		values.Set("period", v)
	}
	if v := strings.TrimSpace(groupBy); v != "" {
		values.Set("group_by", v)
	}
	path := "/v1/usage/summary"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out struct {
		Summary     UsageSummary     `json:"summary"`
		Limits      UsageLimits      `json:"limits"`
		LimitStatus UsageLimitStatus `json:"limit_status"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return UsageSummary{}, UsageLimits{}, UsageLimitStatus{}, err
	}
	return out.Summary, out.Limits, out.LimitStatus, nil
}

func (c *Client) GetUsageLimits(ctx context.Context) (UsageLimits, error) {
	var out UsageLimits
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/usage/limits", nil, false, &out); err != nil {
		return UsageLimits{}, err
	}
	return out, nil
}

func (c *Client) UpdateUsageLimits(ctx context.Context, req UsageLimits) (UsageLimits, error) {
	if v := strings.TrimSpace(strings.ToLower(req.Mode)); v != "" {
		if v != "soft" && v != "hard" {
			return UsageLimits{}, fmt.Errorf("mode must be one of: soft|hard")
		}
		req.Mode = v
	}
	var out UsageLimits
	if _, err := c.doJSON(ctx, http.MethodPatch, "/v1/usage/limits", req, true, &out); err != nil {
		return UsageLimits{}, err
	}
	return out, nil
}
