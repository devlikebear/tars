package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeFileResponse struct {
	Path    string `json:"path,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Created bool   `json:"created,omitempty"`
	Message string `json:"message,omitempty"`
}

func NewWriteTool(workspaceDir string) Tool {
	return newWriteToolWithName("write", workspaceDir)
}

func NewWriteFileTool(workspaceDir string) Tool {
	return newWriteToolWithName("write_file", workspaceDir)
}

func newWriteToolWithName(name, workspaceDir string) Tool {
	return Tool{
		Name:        name,
		Description: "Write UTF-8 text content to a workspace file.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "path":{"type":"string","description":"Workspace-relative file path to write."},
    "content":{"type":"string","description":"Text content to write."},
    "create_dirs":{"type":"boolean","default":true}
  },
  "required":["path","content"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Path       string `json:"path"`
				Content    string `json:"content"`
				CreateDirs *bool  `json:"create_dirs,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(writeFileResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if input.Path == "" {
				return jsonTextResult(writeFileResponse{Message: "path is required"}, true), nil
			}
			absPath, err := resolveWorkspacePath(workspaceDir, input.Path)
			if err != nil {
				return jsonTextResult(writeFileResponse{Message: err.Error()}, true), nil
			}

			createDirs := true
			if input.CreateDirs != nil {
				createDirs = *input.CreateDirs
			}
			if createDirs {
				if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
					return jsonTextResult(writeFileResponse{Message: fmt.Sprintf("create parent directories failed: %v", err)}, true), nil
				}
			}

			_, statErr := os.Stat(absPath)
			created := os.IsNotExist(statErr)
			if err := os.WriteFile(absPath, []byte(input.Content), 0o644); err != nil {
				return jsonTextResult(writeFileResponse{Message: fmt.Sprintf("write file failed: %v", err)}, true), nil
			}
			return jsonTextResult(writeFileResponse{
				Path:    workspaceRelativePath(workspaceDir, absPath),
				Bytes:   len(input.Content),
				Created: created,
			}, false), nil
		},
	}
}
