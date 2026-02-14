package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

type SessionStatus struct {
	SessionID       string `json:"session_id"`
	HistoryMessages int    `json:"history_messages"`
}

func NewSessionStatusTool(getStatus func(ctx context.Context) (SessionStatus, error)) Tool {
	return Tool{
		Name:        "session_status",
		Description: "Return current session metadata such as session id and history message count.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Execute: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			status, err := getStatus(ctx)
			if err != nil {
				return Result{}, err
			}
			raw, err := json.Marshal(status)
			if err != nil {
				return Result{}, fmt.Errorf("marshal session status: %w", err)
			}
			return Result{
				Content: []ContentBlock{
					{Type: "text", Text: string(raw)},
				},
			}, nil
		},
	}
}
