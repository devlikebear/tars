package tool

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/tars/internal/ops"
)

// NewOpsTool creates a single "ops" tool that dispatches to status,
// cleanup_plan, and cleanup_apply sub-actions. Replaces ops_status,
// ops_cleanup_plan, and ops_cleanup_apply.
func NewOpsTool(manager *ops.Manager) Tool {
	statusTool := NewOpsStatusTool(manager)
	planTool := NewOpsCleanupPlanTool(manager)
	applyTool := NewOpsCleanupApplyTool(manager)
	return Tool{
		Name:        "ops",
		Description: "Workstation operations. Actions: status (disk/process info), cleanup_plan (create safe cleanup plan), cleanup_apply (apply approved cleanup by approval_id).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["status","cleanup_plan","cleanup_apply"]}
  },
  "required":["action"],
  "additionalProperties":true
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			payload, action, err := dispatchAction(params)
			if err != nil {
				return aggregatorError(err.Error()), nil
			}
			switch action {
			case "status":
				return statusTool.Execute(ctx, json.RawMessage(`{}`))
			case "cleanup_plan":
				return planTool.Execute(ctx, json.RawMessage(`{}`))
			case "cleanup_apply":
				return applyTool.Execute(ctx, payload)
			default:
				return aggregatorError("action must be one of: status, cleanup_plan, cleanup_apply"), nil
			}
		},
	}
}
