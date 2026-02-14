package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func NewMemoryGetTool(workspaceDir string) Tool {
	return newMemoryGetTool(workspaceDir, func() time.Time {
		return time.Now().UTC()
	})
}

func newMemoryGetTool(workspaceDir string, nowFn func() time.Time) Tool {
	return Tool{
		Name:        "memory_get",
		Description: "Get daily memory log by date or full long-term MEMORY.md content.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "date":{"type":"string","description":"Date for daily memory in YYYY-MM-DD format."},
    "target":{"type":"string","enum":["daily","memory"],"default":"daily"}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Date   string `json:"date,omitempty"`
				Target string `json:"target,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return memoryGetErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			target := strings.ToLower(strings.TrimSpace(input.Target))
			if target == "" {
				target = "daily"
			}

			switch target {
			case "daily":
				date := strings.TrimSpace(input.Date)
				if date == "" {
					date = nowFn().UTC().Format("2006-01-02")
				}
				if _, err := time.Parse("2006-01-02", date); err != nil {
					return memoryGetErrorResult("invalid date format: expected YYYY-MM-DD"), nil
				}
				path := filepath.Join(workspaceDir, "memory", date+".md")
				return readMemoryGetFile(path, fmt.Sprintf("no daily memory found for %s", date)), nil
			case "memory":
				path := filepath.Join(workspaceDir, "MEMORY.md")
				return readMemoryGetFile(path, "MEMORY.md not found"), nil
			default:
				return memoryGetErrorResult("target must be one of: daily, memory"), nil
			}
		},
	}
}

func readMemoryGetFile(path string, notFoundMessage string) Result {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{
				Content: []ContentBlock{
					{Type: "text", Text: notFoundMessage},
				},
			}
		}
		return memoryGetErrorResult(fmt.Sprintf("read memory file failed: %v", err))
	}
	return Result{
		Content: []ContentBlock{
			{Type: "text", Text: string(raw)},
		},
	}
}

func memoryGetErrorResult(message string) Result {
	return Result{
		Content: []ContentBlock{
			{Type: "text", Text: message},
		},
		IsError: true,
	}
}
