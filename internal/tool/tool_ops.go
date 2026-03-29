package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/ops"
)

func NewOpsStatusTool(manager *ops.Manager) Tool {
	return Tool{
		Name:        "ops_status",
		Description: "Read current workstation ops status (disk/process).",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			if manager == nil {
				return JSONTextResult(map[string]any{"message": "ops manager is not configured"}, true), nil
			}
			status, err := manager.Status(ctx)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(status, false), nil
		},
	}
}

func NewOpsCleanupPlanTool(manager *ops.Manager) Tool {
	return Tool{
		Name:        "ops_cleanup_plan",
		Description: "Create a safe cleanup plan and issue approval request.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			if manager == nil {
				return JSONTextResult(map[string]any{"message": "ops manager is not configured"}, true), nil
			}
			plan, err := manager.CreateCleanupPlan(ctx)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(plan, false), nil
		},
	}
}

func NewOpsCleanupApplyTool(manager *ops.Manager) Tool {
	return Tool{
		Name:        "ops_cleanup_apply",
		Description: "Apply an approved cleanup plan by approval id.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"approval_id":{"type":"string"}},
  "required":["approval_id"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if manager == nil {
				return JSONTextResult(map[string]any{"message": "ops manager is not configured"}, true), nil
			}
			var input struct {
				ApprovalID string `json:"approval_id"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			result, err := manager.ApplyCleanup(ctx, strings.TrimSpace(input.ApprovalID))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(result, false), nil
		},
	}
}
