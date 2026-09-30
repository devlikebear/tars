package tools

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
				return JSONTextResult(applyPatchResponse{Message: "apply_patch is disabled"}, true), nil
			}
			var input struct {
				Patch  string `json:"patch"`
				DryRun bool   `json:"dry_run,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(applyPatchResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			patchText := strings.TrimSpace(input.Patch)
			if patchText == "" {
				return JSONTextResult(applyPatchResponse{Message: "patch is required"}, true), nil
			}
			files, err := parsePatchFiles(patchText)
			if err != nil {
				return JSONTextResult(applyPatchResponse{Message: err.Error()}, true), nil
			}
			if len(files) > maxApplyPatchFiles {
				return JSONTextResult(applyPatchResponse{Message: fmt.Sprintf("too many files in patch (max=%d)", maxApplyPatchFiles)}, true), nil
			}
			for _, f := range files {
				if filepath.IsAbs(f) {
					return JSONTextResult(applyPatchResponse{Message: fmt.Sprintf("absolute path is not allowed: %s", f)}, true), nil
				}
				clean := filepath.Clean(strings.TrimSpace(f))
				if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
					return JSONTextResult(applyPatchResponse{Message: fmt.Sprintf("patch path escapes workspace: %s", f)}, true), nil
				}
			}

			patchPath, err := patchExecutable()
			if err != nil {
				return JSONTextResult(applyPatchResponse{Message: err.Error()}, true), nil
			}

			args := []string{"-p0", "-u", "--forward", "--batch"}
			if input.DryRun {
				args = append(args, "--dry-run")
			}
			var before []fileSnapshot
			if !input.DryRun {
				before = snapshotPatchFiles(workspaceDir, files)
			}
			cmd := exec.CommandContext(ctx, patchPath, args...)
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
			}
			// A failed patch may still have changed some files (--batch
			// applies the hunks it can), so report whatever is on disk.
			return reportPatchChanges(JSONTextResult(resp, err != nil), workspaceDir, files, before), nil
		},
	}
}

// snapshotPatchFile reads a file the patch names, for its change report. The
// name has passed the lexical checks above, but a symlink inside the
// workspace could still lead outside it, so the path is resolved the way the
// write tools resolve theirs; a path that escapes reports nothing rather
// than reading a file the tools may not touch.
func snapshotPatchFile(workspaceDir, name string) fileSnapshot {
	absPath, err := resolveWorkspaceWritePath(workspaceDir, filepath.FromSlash(name))
	if err != nil {
		return fileSnapshot{}
	}
	return snapshotFile(absPath)
}

// snapshotPatchFiles reads every file the patch names before it runs.
func snapshotPatchFiles(workspaceDir string, files []string) []fileSnapshot {
	before := make([]fileSnapshot, len(files))
	for i, f := range files {
		before[i] = snapshotPatchFile(workspaceDir, f)
	}
	return before
}

// reportPatchChanges attaches what the patch did to each file snapshotted
// in before; a dry run has no snapshots and reports nothing.
func reportPatchChanges(result Result, workspaceDir string, files []string, before []fileSnapshot) Result {
	for i, f := range before {
		result = withFileChange(result, filepath.Clean(files[i]), f, snapshotPatchFile(workspaceDir, files[i]))
	}
	return result
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
