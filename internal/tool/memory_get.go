package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
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
    "target":{"type":"string","enum":["daily","memory","experiences"],"default":"daily"},
    "query":{"type":"string","description":"optional text query for experiences target"},
    "category":{"type":"string","description":"optional category filter for experiences target"},
    "project_id":{"type":"string","description":"optional project id filter for experiences target"},
    "limit":{"type":"integer","minimum":1,"maximum":100,"default":8}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Date      string `json:"date,omitempty"`
				Target    string `json:"target,omitempty"`
				Query     string `json:"query,omitempty"`
				Category  string `json:"category,omitempty"`
				ProjectID string `json:"project_id,omitempty"`
				Limit     int    `json:"limit,omitempty"`
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
			case "experiences":
				rows, err := memory.SearchExperiences(workspaceDir, memory.SearchOptions{
					Query:     strings.TrimSpace(input.Query),
					Category:  strings.TrimSpace(input.Category),
					ProjectID: strings.TrimSpace(input.ProjectID),
					Limit:     input.Limit,
				})
				if err != nil {
					return memoryGetErrorResult(fmt.Sprintf("search experiences failed: %v", err)), nil
				}
				payload := map[string]any{
					"target":  "experiences",
					"query":   strings.TrimSpace(input.Query),
					"results": rows,
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					return memoryGetErrorResult(fmt.Sprintf("marshal experiences failed: %v", err)), nil
				}
				return Result{
					Content: []ContentBlock{
						{Type: "text", Text: string(encoded)},
					},
				}, nil
			default:
				return memoryGetErrorResult("target must be one of: daily, memory, experiences"), nil
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
