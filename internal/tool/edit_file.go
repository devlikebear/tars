package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type editFileResponse struct {
	Path         string `json:"path,omitempty"`
	Replacements int    `json:"replacements,omitempty"`
	Message      string `json:"message,omitempty"`
}

func NewEditTool(workspaceDir string) Tool {
	return newEditToolWithName("edit", workspaceDir)
}

func NewEditFileTool(workspaceDir string) Tool {
	return newEditToolWithName("edit_file", workspaceDir)
}

func NewEditFileToolWithPolicy(policy PathPolicy) Tool {
	return newEditToolWithPolicy("edit_file", policy)
}

func newEditToolWithName(name, workspaceDir string) Tool {
	return newEditToolWithPolicy(name, SingleDirPolicy(workspaceDir))
}

func newEditToolWithPolicy(name string, policy PathPolicy) Tool {
	return Tool{
		Name:        name,
		Description: "Edit a workspace file by replacing exact text.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "path":{"type":"string","description":"Workspace-relative file path."},
    "old_text":{"type":"string","description":"Text to replace."},
    "new_text":{"type":"string","description":"Replacement text."},
    "replace_all":{"type":"boolean","default":false}
  },
  "required":["path","old_text","new_text"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Path       string `json:"path"`
				OldText    string `json:"old_text"`
				NewText    string `json:"new_text"`
				ReplaceAll bool   `json:"replace_all,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(editFileResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if input.Path == "" {
				return JSONTextResult(editFileResponse{Message: "path is required"}, true), nil
			}
			if input.OldText == "" {
				return JSONTextResult(editFileResponse{Message: "old_text is required"}, true), nil
			}

			absPath, err := resolvePathWithPolicy(policy, input.Path)
			if err != nil {
				return JSONTextResult(editFileResponse{Message: err.Error()}, true), nil
			}
			body, err := os.ReadFile(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return JSONTextResult(editFileResponse{Message: "file not found"}, true), nil
				}
				return JSONTextResult(editFileResponse{Message: fmt.Sprintf("read file failed: %v", err)}, true), nil
			}
			text := string(body)
			count := strings.Count(text, input.OldText)
			if count == 0 {
				return JSONTextResult(editFileResponse{Message: "old_text not found"}, true), nil
			}
			if !input.ReplaceAll && count > 1 {
				return JSONTextResult(editFileResponse{Message: "old_text is not unique; set replace_all=true"}, true), nil
			}
			n := 1
			if input.ReplaceAll {
				n = -1
			}
			updated := strings.Replace(text, input.OldText, input.NewText, n)
			mode := fs.FileMode(0o644)
			if info, err := os.Stat(absPath); err == nil {
				mode = info.Mode().Perm()
			}
			if err := writeTextFileAtomic(absPath, updated, mode); err != nil {
				return JSONTextResult(editFileResponse{Message: fmt.Sprintf("write file failed: %v", err)}, true), nil
			}
			replacements := 1
			if input.ReplaceAll {
				replacements = count
			}
			return JSONTextResult(editFileResponse{
				Path:         policyRelativePath(policy, absPath),
				Replacements: replacements,
			}, false), nil
		},
	}
}
