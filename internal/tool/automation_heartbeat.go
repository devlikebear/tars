package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type HeartbeatStatus struct {
	Configured       bool   `json:"configured"`
	ActiveHours      string `json:"active_hours,omitempty"`
	Timezone         string `json:"timezone,omitempty"`
	ChatBusy         bool   `json:"chat_busy,omitempty"`
	LastRunAt        string `json:"last_run_at,omitempty"`
	LastSkipped      bool   `json:"last_skipped,omitempty"`
	LastSkipReason   string `json:"last_skip_reason,omitempty"`
	LastLogged       bool   `json:"last_logged,omitempty"`
	LastAcknowledged bool   `json:"last_acknowledged,omitempty"`
	LastResponse     string `json:"last_response,omitempty"`
	LastError        string `json:"last_error,omitempty"`
}

type HeartbeatRunResult struct {
	Response     string    `json:"response,omitempty"`
	Skipped      bool      `json:"skipped,omitempty"`
	SkipReason   string    `json:"skip_reason,omitempty"`
	Logged       bool      `json:"logged,omitempty"`
	Acknowledged bool      `json:"acknowledged,omitempty"`
	RanAt        time.Time `json:"ran_at,omitempty"`
}

func NewHeartbeatStatusTool(getStatus func(ctx context.Context) (HeartbeatStatus, error)) Tool {
	return Tool{
		Name:        "heartbeat_status",
		Description: "Return tarsd heartbeat runtime status and latest execution result.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			if getStatus == nil {
				return automationErrorResult("heartbeat status provider is not configured"), nil
			}
			status, err := getStatus(ctx)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("get heartbeat status failed: %v", err)), nil
			}
			return jsonTextResult(status, false), nil
		},
	}
}

func NewHeartbeatRunOnceTool(runOnce func(ctx context.Context) (HeartbeatRunResult, error)) Tool {
	return Tool{
		Name:        "heartbeat_run_once",
		Description: "Run heartbeat once immediately and return execution result.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			if runOnce == nil {
				return automationErrorResult("heartbeat runner is not configured"), nil
			}
			result, err := runOnce(ctx)
			if err != nil {
				return automationErrorResult(fmt.Sprintf("run heartbeat failed: %v", err)), nil
			}
			return jsonTextResult(result, false), nil
		},
	}
}
