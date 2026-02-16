package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type applyPatchResponse struct {
	Applied bool     `json:"applied"`
	DryRun  bool     `json:"dry_run,omitempty"`
	Files   []string `json:"files,omitempty"`
	Stdout  string   `json:"stdout,omitempty"`
	Stderr  string   `json:"stderr,omitempty"`
	Message string   `json:"message,omitempty"`
}

const maxApplyPatchFiles = 20

func NewApplyPatchTool(workspaceDir string, enabled bool) Tool {
	return Tool{
		Name:        "apply_patch",
		Description: "Apply a unified diff patch in the workspace (MVP).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "patch":{"type":"string","description":"Unified diff text."},
    "dry_run":{"type":"boolean","default":false}
  },
  "required":["patch"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(applyPatchResponse{Message: "apply_patch is disabled"}, true), nil
			}
			var input struct {
				Patch  string `json:"patch"`
				DryRun bool   `json:"dry_run,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(applyPatchResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			patchText := strings.TrimSpace(input.Patch)
			if patchText == "" {
				return jsonTextResult(applyPatchResponse{Message: "patch is required"}, true), nil
			}
			files, err := parsePatchFiles(patchText)
			if err != nil {
				return jsonTextResult(applyPatchResponse{Message: err.Error()}, true), nil
			}
			if len(files) > maxApplyPatchFiles {
				return jsonTextResult(applyPatchResponse{Message: fmt.Sprintf("too many files in patch (max=%d)", maxApplyPatchFiles)}, true), nil
			}
			for _, f := range files {
				if filepath.IsAbs(f) {
					return jsonTextResult(applyPatchResponse{Message: fmt.Sprintf("absolute path is not allowed: %s", f)}, true), nil
				}
				clean := filepath.Clean(strings.TrimSpace(f))
				if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
					return jsonTextResult(applyPatchResponse{Message: fmt.Sprintf("patch path escapes workspace: %s", f)}, true), nil
				}
			}

			args := []string{"-p0", "-u", "--forward", "--batch"}
			if input.DryRun {
				args = append(args, "--dry-run")
			}
			cmd := exec.CommandContext(ctx, "patch", args...)
			cmd.Dir = workspaceDir
			cmd.Stdin = strings.NewReader(input.Patch)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()
			resp := applyPatchResponse{
				Applied: err == nil,
				DryRun:  input.DryRun,
				Files:   files,
				Stdout:  trimOutput(stdout.String(), maxExecOutputBytes),
				Stderr:  trimOutput(stderr.String(), maxExecOutputBytes),
			}
			if err != nil {
				resp.Message = fmt.Sprintf("patch apply failed: %v", err)
				return jsonTextResult(resp, true), nil
			}
			return jsonTextResult(resp, false), nil
		},
	}
}

func parsePatchFiles(patch string) ([]string, error) {
	lines := strings.Split(patch, "\n")
	seen := map[string]struct{}{}
	out := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			path := strings.TrimSpace(parts[1])
			if path == "/dev/null" || path == "null" {
				continue
			}
			path = strings.TrimPrefix(path, "a/")
			path = strings.TrimPrefix(path, "b/")
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("patch does not include any file headers")
	}
	return out, nil
}
