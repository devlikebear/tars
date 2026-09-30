package tools

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

func NewWriteFileToolWithPolicy(policy PathPolicy) Tool {
	return newWriteToolWithPolicy("write_file", policy)
}

func newWriteToolWithName(name, workspaceDir string) Tool {
	return newWriteToolWithPolicy(name, SingleDirPolicy(workspaceDir))
}

func newWriteToolWithPolicy(name string, policy PathPolicy) Tool {
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
			input, msg := parseWriteFileInput(params)
			if msg != "" {
				return writeFileError(msg), nil
			}
			absPath, err := resolveWritePathWithPolicy(policy, input.Path)
			if err != nil {
				return writeFileError(err.Error()), nil
			}

			if input.createDirs() {
				if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
					return writeFileError(fmt.Sprintf("create parent directories failed: %v", err)), nil
				}
			}

			info, statErr := os.Stat(absPath)
			target, msg := classifyWriteTarget(info, statErr)
			if msg != "" {
				return writeFileError(msg), nil
			}
			before := snapshotBeforeWrite(absPath, target.created)
			if err := writeTextFileAtomic(absPath, input.Content, target.mode); err != nil {
				return writeFileError(fmt.Sprintf("write file failed: %v", err)), nil
			}
			relPath := policyRelativePath(policy, absPath)
			result := JSONTextResult(writeFileResponse{
				Path:    relPath,
				Bytes:   len(input.Content),
				Created: target.created,
			}, false)
			return withFileChange(result, relPath, before, snapshotWritten([]byte(input.Content))), nil
		},
	}
}

type writeFileInput struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	CreateDirs *bool  `json:"create_dirs,omitempty"`
}

func writeFileError(message string) Result {
	return JSONTextResult(writeFileResponse{Message: message}, true)
}

func parseWriteFileInput(params json.RawMessage) (writeFileInput, string) {
	var input writeFileInput
	if err := json.Unmarshal(params, &input); err != nil {
		return input, fmt.Sprintf("invalid arguments: %v", err)
	}
	if input.Path == "" {
		return input, "path is required"
	}
	return input, ""
}

// createDirs reports whether missing parent directories are created; they
// are unless the call turns it off.
func (in writeFileInput) createDirs() bool {
	return in.CreateDirs == nil || *in.CreateDirs
}

// writeTarget is what a write replaces: its mode and whether it is new.
type writeTarget struct {
	mode    fs.FileMode
	created bool
}

// classifyWriteTarget reads the target's stat result, or says why the
// write cannot go ahead.
func classifyWriteTarget(info fs.FileInfo, statErr error) (writeTarget, string) {
	if os.IsNotExist(statErr) {
		return writeTarget{mode: 0o644, created: true}, ""
	}
	if statErr != nil {
		return writeTarget{}, fmt.Sprintf("stat file failed: %v", statErr)
	}
	if info.IsDir() {
		return writeTarget{}, "path is a directory"
	}
	return writeTarget{mode: info.Mode().Perm()}, ""
}

// snapshotBeforeWrite is what an existing file held, for the change report.
func snapshotBeforeWrite(absPath string, created bool) fileSnapshot {
	if created {
		return fileSnapshot{}
	}
	return snapshotFile(absPath)
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
