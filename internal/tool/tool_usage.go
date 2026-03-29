package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/usage"
)

func NewUsageReportTool(tracker *usage.Tracker) Tool {
	return Tool{
		Name:        "usage_report",
		Description: "Report token and cost usage summary by period and grouping.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "period":{"type":"string","enum":["today","week","month"],"default":"today"},
    "group_by":{"type":"string","enum":["provider","model","source","project"],"default":"provider"}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if tracker == nil {
				return JSONTextResult(map[string]any{"message": "usage tracker is not configured"}, true), nil
			}
			var input struct {
				Period  string `json:"period,omitempty"`
				GroupBy string `json:"group_by,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			summary, err := tracker.Summary(strings.TrimSpace(input.Period), strings.TrimSpace(input.GroupBy))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			limits := tracker.Limits()
			return JSONTextResult(map[string]any{
				"summary": summary,
				"limits":  limits,
			}, false), nil
		},
	}
}
