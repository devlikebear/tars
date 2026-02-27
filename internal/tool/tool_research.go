package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devlikebear/tarsncase/internal/research"
)

func NewResearchReportTool(service *research.Service) Tool {
	return Tool{
		Name:        "research_report",
		Description: "Generate research report markdown and append summary log for a project.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "project_id":{"type":"string"},
    "topic":{"type":"string"},
    "summary":{"type":"string"},
    "body":{"type":"string"}
  },
  "required":["project_id","topic"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			if service == nil {
				return jsonTextResult(map[string]any{"message": "research service is not configured"}, true), nil
			}
			var input struct {
				ProjectID string `json:"project_id"`
				Topic     string `json:"topic"`
				Summary   string `json:"summary,omitempty"`
				Body      string `json:"body,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			report, err := service.Run(research.RunInput{
				ProjectID: input.ProjectID,
				Topic:     input.Topic,
				Summary:   input.Summary,
				Body:      input.Body,
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(report, false), nil
		},
	}
}
