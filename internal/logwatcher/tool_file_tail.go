package logwatcher

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/devlikebear/tars/internal/tool"
)

const fileTailMaxTail = 5000

type fileTailInput struct {
	Path string `json:"path"`
	Tail int    `json:"tail,omitempty"`
	Grep string `json:"grep,omitempty"`
}

type fileTailOutput struct {
	Path      string   `json:"path"`
	Tail      int      `json:"tail"`
	Grep      string   `json:"grep,omitempty"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

func newFileTailTool() tool.Tool {
	return tool.Tool{
		Name: "file_tail",
		Description: "Read the last N lines of a file, optionally filtered by substring. " +
			"Intended for local log files that Docker doesn't own.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type":"string","description":"Absolute or workspace-relative file path."},
    "tail": {"type":"integer","description":"Max lines to return (1..5000).","default":200},
    "grep": {"type":"string","description":"Optional substring filter applied before truncation."}
  },
  "required": ["path"],
  "additionalProperties": false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (tool.Result, error) {
			var input fileTailInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.Path = strings.TrimSpace(input.Path)
			input.Grep = strings.TrimSpace(input.Grep)
			if input.Path == "" {
				return tool.JSONTextResult(map[string]any{"message": "path is required"}, true), nil
			}
			tail := input.Tail
			if tail <= 0 {
				tail = 200
			}
			if tail > fileTailMaxTail {
				tail = fileTailMaxTail
			}

			file, err := os.Open(input.Path)
			if err != nil {
				switch {
				case errors.Is(err, fs.ErrNotExist):
					return tool.JSONTextResult(map[string]any{"message": "file not found", "path": input.Path}, true), nil
				case errors.Is(err, fs.ErrPermission):
					return tool.JSONTextResult(map[string]any{"message": "permission denied", "path": input.Path}, true), nil
				default:
					return tool.JSONTextResult(map[string]any{"message": "open failed", "detail": err.Error()}, true), nil
				}
			}
			defer file.Close()

			lines, truncated, err := tailFile(file, tail, input.Grep)
			if err != nil {
				return tool.JSONTextResult(map[string]any{"message": "read failed", "detail": err.Error()}, true), nil
			}

			out := fileTailOutput{
				Path:      input.Path,
				Tail:      tail,
				Grep:      input.Grep,
				Lines:     lines,
				Truncated: truncated,
			}
			return tool.JSONTextResult(out, false), nil
		},
	}
}

func tailFile(r *os.File, tail int, grep string) ([]string, bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	ring := make([]string, 0, tail)
	total := 0
	matchAll := grep == ""
	for scanner.Scan() {
		line := scanner.Text()
		if !matchAll && !strings.Contains(line, grep) {
			continue
		}
		total++
		if len(ring) == tail {
			copy(ring, ring[1:])
			ring[len(ring)-1] = line
		} else {
			ring = append(ring, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return ring, total > tail, nil
}
