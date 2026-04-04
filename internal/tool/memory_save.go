package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

func NewMemorySaveTool(workspaceDir string, semantic *memory.Service, nowFn func() time.Time) Tool {
	if nowFn == nil {
		nowFn = time.Now
	}
	return Tool{
		Name:        "memory_save",
		Description: "Save durable conversation/task experiences to structured long-term memory.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "category":{"type":"string","description":"conversation|task_completed|error_resolved|preference|fact"},
    "summary":{"type":"string"},
    "tags":{"type":"array","items":{"type":"string"}},
    "source_session":{"type":"string"},
    "project_id":{"type":"string"},
    "importance":{"type":"integer","minimum":1,"maximum":10},
    "auto":{"type":"boolean"}
  },
  "required":["summary"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Category      string   `json:"category,omitempty"`
				Summary       string   `json:"summary"`
				Tags          []string `json:"tags,omitempty"`
				SourceSession string   `json:"source_session,omitempty"`
				ProjectID     string   `json:"project_id,omitempty"`
				Importance    int      `json:"importance,omitempty"`
				Auto          bool     `json:"auto,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return memoryGetErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			summary := strings.TrimSpace(input.Summary)
			if summary == "" {
				return memoryGetErrorResult("summary is required"), nil
			}
			exp := memory.Experience{
				Timestamp:     nowFn().UTC(),
				Category:      strings.TrimSpace(input.Category),
				Summary:       summary,
				Tags:          input.Tags,
				SourceSession: strings.TrimSpace(input.SourceSession),
				ProjectID:     strings.TrimSpace(input.ProjectID),
				Importance:    input.Importance,
				Auto:          input.Auto,
			}
			err := memory.AppendExperience(workspaceDir, exp)
			if err != nil {
				return memoryGetErrorResult(err.Error()), nil
			}
			if err := memory.AppendMemoryNote(workspaceDir, exp.Timestamp, summary); err != nil {
				return memoryGetErrorResult(err.Error()), nil
			}
			if semantic != nil {
				_ = semantic.IndexExperience(context.Background(), exp)
			}
			out := map[string]any{
				"saved":   true,
				"summary": summary,
			}
			if strings.TrimSpace(input.ProjectID) != "" {
				out["project_id"] = strings.TrimSpace(input.ProjectID)
			}
			encoded, err := json.Marshal(out)
			if err != nil {
				return memoryGetErrorResult(fmt.Sprintf("marshal result failed: %v", err)), nil
			}
			return Result{Content: []ContentBlock{{Type: "text", Text: string(encoded)}}}, nil
		},
	}
}
