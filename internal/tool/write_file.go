package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
		Description: "Write UTF-8 text content to a workspace file using safe, atomic writes.",
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
			absPath, err := resolveWorkspaceWritePath(workspaceDir, input.Path)
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

			info, statErr := os.Stat(absPath)
			created := os.IsNotExist(statErr)
			if statErr == nil && info.IsDir() {
				return jsonTextResult(writeFileResponse{Message: "path is a directory"}, true), nil
			}
			if statErr != nil && !created {
				return jsonTextResult(writeFileResponse{Message: fmt.Sprintf("stat file failed: %v", statErr)}, true), nil
			}

			mode := fs.FileMode(0o644)
			if statErr == nil {
				mode = info.Mode().Perm()
			}
			if err := writeTextFileAtomic(absPath, input.Content, mode); err != nil {
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

func writeTextFileAtomic(absPath, content string, mode fs.FileMode) error {
	dir := filepath.Dir(absPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(absPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, absPath); err != nil {
		if runtime.GOOS == "windows" {
			if removeErr := os.Remove(absPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("replace existing file: %w", err)
			}
			if retryErr := os.Rename(tmpPath, absPath); retryErr != nil {
				return fmt.Errorf("rename temp file: %w", retryErr)
			}
		} else {
			return fmt.Errorf("rename temp file: %w", err)
		}
	}
	cleanup = false
	return nil
}
