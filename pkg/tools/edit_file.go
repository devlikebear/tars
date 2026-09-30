package tools

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
			input, msg := parseEditFileInput(params)
			if msg != "" {
				return editFileError(msg), nil
			}

			absPath, err := resolvePathWithPolicy(policy, input.Path)
			if err != nil {
				return editFileError(err.Error()), nil
			}
			body, err := os.ReadFile(absPath)
			if err != nil {
				return editFileError(readFileFailure(err)), nil
			}
			updated, replacements, msg := replaceEditText(string(body), input)
			if msg != "" {
				return editFileError(msg), nil
			}
			mode := fs.FileMode(0o644)
			if info, err := os.Stat(absPath); err == nil {
				mode = info.Mode().Perm()
			}
			if err := writeTextFileAtomic(absPath, updated, mode); err != nil {
				return editFileError(fmt.Sprintf("write file failed: %v", err)), nil
			}
			relPath := policyRelativePath(policy, absPath)
			result := JSONTextResult(editFileResponse{Path: relPath, Replacements: replacements}, false)
			return withFileChange(result, relPath, snapshotWritten(body), snapshotWritten([]byte(updated))), nil
		},
	}
}

type editFileInput struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func editFileError(message string) Result {
	return JSONTextResult(editFileResponse{Message: message}, true)
}

func parseEditFileInput(params json.RawMessage) (editFileInput, string) {
	var input editFileInput
	if err := json.Unmarshal(params, &input); err != nil {
		return input, fmt.Sprintf("invalid arguments: %v", err)
	}
	if input.Path == "" {
		return input, "path is required"
	}
	if input.OldText == "" {
		return input, "old_text is required"
	}
	return input, ""
}

// readFileFailure is the message for a failed read of the file to edit.
func readFileFailure(err error) string {
	if os.IsNotExist(err) {
		return "file not found"
	}
	return fmt.Sprintf("read file failed: %v", err)
}

// replaceEditText applies the edit to text, returning the new text and the
// number of replacements, or a message when the edit cannot apply.
func replaceEditText(text string, input editFileInput) (string, int, string) {
	count := strings.Count(text, input.OldText)
	if count == 0 {
		return "", 0, "old_text not found"
	}
	if !input.ReplaceAll {
		if count > 1 {
			return "", 0, "old_text is not unique; set replace_all=true"
		}
		return strings.Replace(text, input.OldText, input.NewText, 1), 1, ""
	}
	return strings.ReplaceAll(text, input.OldText, input.NewText), count, ""
}
