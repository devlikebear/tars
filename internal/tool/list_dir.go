package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	defaultListDirMaxEntries = 100
	maxListDirMaxEntries     = 500
)

type listDirEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

type listDirResponse struct {
	Path      string         `json:"path"`
	Recursive bool           `json:"recursive"`
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated,omitempty"`
	Entries   []listDirEntry `json:"entries,omitempty"`
	Message   string         `json:"message,omitempty"`
}

func NewListDirTool(workspaceDir string) Tool {
	return Tool{
		Name:        "list_dir",
		Description: "List files and directories under a workspace path.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "path":{"type":"string","description":"Workspace-relative directory path.","default":"."},
    "recursive":{"type":"boolean","default":false},
    "max_entries":{"type":"integer","minimum":1,"maximum":500,"default":100}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Path       string `json:"path,omitempty"`
				Recursive  bool   `json:"recursive,omitempty"`
				MaxEntries *int   `json:"max_entries,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return listDirErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			maxEntries := resolvePositiveBoundedInt(defaultListDirMaxEntries, maxListDirMaxEntries, input.MaxEntries)

			absPath, err := resolveWorkspacePath(workspaceDir, input.Path)
			if err != nil {
				return listDirErrorResult(err.Error()), nil
			}

			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return listDirErrorResult("directory not found"), nil
				}
				return listDirErrorResult(fmt.Sprintf("stat directory failed: %v", err)), nil
			}
			if !info.IsDir() {
				return listDirErrorResult("path is not a directory"), nil
			}

			entries, truncated, err := collectDirEntries(workspaceDir, absPath, input.Recursive, maxEntries)
			if err != nil {
				return listDirErrorResult(fmt.Sprintf("list directory failed: %v", err)), nil
			}

			return JSONTextResult(listDirResponse{
				Path:      workspaceRelativePath(workspaceDir, absPath),
				Recursive: input.Recursive,
				Count:     len(entries),
				Truncated: truncated,
				Entries:   entries,
			}, false), nil
		},
	}
}

func collectDirEntries(workspaceDir, absPath string, recursive bool, maxEntries int) ([]listDirEntry, bool, error) {
	if !recursive {
		dirEntries, err := os.ReadDir(absPath)
		if err != nil {
			return nil, false, err
		}
		out := make([]listDirEntry, 0, minInt(len(dirEntries), maxEntries))
		truncated := false
		for _, item := range dirEntries {
			if len(out) >= maxEntries {
				truncated = true
				break
			}
			entry, err := buildListDirEntry(workspaceDir, filepath.Join(absPath, item.Name()), item.Type())
			if err != nil {
				continue
			}
			out = append(out, entry)
		}
		return out, truncated, nil
	}

	out := make([]listDirEntry, 0, maxEntries)
	truncated := false
	err := filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == absPath {
			return nil
		}
		if len(out) >= maxEntries {
			truncated = true
			return fs.SkipAll
		}
		entry, buildErr := buildListDirEntry(workspaceDir, path, d.Type())
		if buildErr != nil {
			return nil
		}
		out = append(out, entry)
		return nil
	})
	return out, truncated, err
}

func buildListDirEntry(workspaceDir, absPath string, mode fs.FileMode) (listDirEntry, error) {
	entryType := "file"
	if mode.IsDir() {
		entryType = "dir"
	} else if mode&os.ModeSymlink != 0 {
		entryType = "symlink"
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return listDirEntry{}, err
	}
	entry := listDirEntry{
		Path: workspaceRelativePath(workspaceDir, absPath),
		Type: entryType,
	}
	if !info.IsDir() {
		entry.Size = info.Size()
	}
	return entry, nil
}

func listDirErrorResult(message string) Result {
	return JSONTextResult(listDirResponse{Message: message}, true)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
