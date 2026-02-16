package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

const (
	defaultReadFileMaxBytes = 8192
	maxReadFileMaxBytes     = 65536
)

type readFileResponse struct {
	Path      string `json:"path"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
	Content   string `json:"content,omitempty"`
	Message   string `json:"message,omitempty"`
}

func NewReadTool(workspaceDir string) Tool {
	return newReadToolWithName("read", workspaceDir)
}

func NewReadFileTool(workspaceDir string) Tool {
	return newReadToolWithName("read_file", workspaceDir)
}

func newReadToolWithName(name, workspaceDir string) Tool {
	return Tool{
		Name:        name,
		Description: "Read a UTF-8 text file from the workspace and return contents.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "path":{"type":"string","description":"Workspace-relative file path to read."},
    "max_bytes":{"type":"integer","minimum":1,"maximum":65536,"default":8192}
  },
  "required":["path"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Path     string `json:"path"`
				MaxBytes *int   `json:"max_bytes,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return readFileErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if input.Path == "" {
				return readFileErrorResult("path is required"), nil
			}

			maxBytes := defaultReadFileMaxBytes
			if input.MaxBytes != nil {
				maxBytes = *input.MaxBytes
			}
			if maxBytes <= 0 {
				maxBytes = defaultReadFileMaxBytes
			}
			if maxBytes > maxReadFileMaxBytes {
				maxBytes = maxReadFileMaxBytes
			}

			absPath, err := resolveWorkspacePath(workspaceDir, input.Path)
			if err != nil {
				return readFileErrorResult(err.Error()), nil
			}

			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return readFileErrorResult("file not found"), nil
				}
				return readFileErrorResult(fmt.Sprintf("stat file failed: %v", err)), nil
			}
			if info.IsDir() {
				return readFileErrorResult("path is a directory"), nil
			}

			raw, err := os.ReadFile(absPath)
			if err != nil {
				return readFileErrorResult(fmt.Sprintf("read file failed: %v", err)), nil
			}

			payload := readFileResponse{
				Path:  workspaceRelativePath(workspaceDir, absPath),
				Bytes: len(raw),
			}
			if len(raw) > maxBytes {
				payload.Truncated = true
				payload.Content = string(raw[:maxBytes])
				payload.Message = fmt.Sprintf("content truncated to %d bytes", maxBytes)
			} else {
				payload.Content = string(raw)
			}
			return jsonTextResult(payload, false), nil
		},
	}
}

func jsonTextResult(value any, isError bool) Result {
	raw, err := json.Marshal(value)
	if err != nil {
		return Result{
			Content: []ContentBlock{
				{Type: "text", Text: fmt.Sprintf("marshal result failed: %v", err)},
			},
			IsError: true,
		}
	}
	return Result{
		Content: []ContentBlock{
			{Type: "text", Text: string(raw)},
		},
		IsError: isError,
	}
}

func readFileErrorResult(message string) Result {
	return jsonTextResult(readFileResponse{Message: message}, true)
}
