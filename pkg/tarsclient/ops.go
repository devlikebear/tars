package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) OpsStatus(ctx context.Context) (OpsStatus, error) {
	var out OpsStatus
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/ops/status", nil, false, &out); err != nil {
		return OpsStatus{}, err
	}
	return out, nil
}

func (c *Client) CreateCleanupPlan(ctx context.Context) (CleanupPlan, error) {
	var out CleanupPlan
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/ops/cleanup/plan", map[string]any{}, false, &out); err != nil {
		return CleanupPlan{}, err
	}
	return out, nil
}

func (c *Client) ApplyCleanup(ctx context.Context, approvalID string) (CleanupApplyResult, error) {
	id := strings.TrimSpace(approvalID)
	if id == "" {
		return CleanupApplyResult{}, fmt.Errorf("approval id is required")
	}
	var out CleanupApplyResult
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/ops/cleanup/apply", map[string]string{"approval_id": id}, false, &out); err != nil {
		return CleanupApplyResult{}, err
	}
	return out, nil
}

func (c *Client) ListApprovals(ctx context.Context) ([]Approval, error) {
	var out []Approval
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/ops/approvals", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []Approval{}, nil
	}
	return out, nil
}

func (c *Client) ApproveCleanup(ctx context.Context, approvalID string) error {
	id := strings.TrimSpace(approvalID)
	if id == "" {
		return fmt.Errorf("approval id is required")
	}
	_, err := c.doText(ctx, http.MethodPost, "/v1/ops/approvals/"+url.PathEscape(id)+"/approve", map[string]any{}, false)
	return err
}

func (c *Client) RejectCleanup(ctx context.Context, approvalID string) error {
	id := strings.TrimSpace(approvalID)
	if id == "" {
		return fmt.Errorf("approval id is required")
	}
	_, err := c.doText(ctx, http.MethodPost, "/v1/ops/approvals/"+url.PathEscape(id)+"/reject", map[string]any{}, false)
	return err
}
