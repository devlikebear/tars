package agentruntime

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxFileAttentionSparklineBuckets = 12

type runtimeFileOperation struct {
	ToolName    string
	ToolCallID  string
	Path        string
	Action      string
	Kind        string
	ToolIsError bool
}

func (r *Runtime) recordRuntimeToolCall(runID string, call RuntimeToolCall) {
	op, ok := runtimeFileOperationFromToolCall(call)
	if !ok {
		return
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	r.mu.Lock()
	state := r.runs[strings.TrimSpace(runID)]
	if state == nil {
		r.mu.Unlock()
		return
	}
	applyFileAttentionSummary(&state.run, op, now)
	state.run.UpdatedAt = now
	r.stateVersion++
	r.mu.Unlock()

	r.publishRunEvent(runID, RunEvent{
		Type:        "tool.call",
		Timestamp:   now,
		ToolName:    op.ToolName,
		ToolCallID:  op.ToolCallID,
		Path:        op.Path,
		Action:      op.Action,
		ToolIsError: op.ToolIsError,
		Message:     op.Action + " " + op.Path,
	})
	r.persistSnapshot()
}

func runtimeFileOperationFromToolCall(call RuntimeToolCall) (runtimeFileOperation, bool) {
	toolName := strings.TrimSpace(call.ToolName)
	action, kind, ok := classifyFileToolAction(toolName)
	if !ok {
		return runtimeFileOperation{}, false
	}
	path, ok := fileToolPathFromArgs(call.ToolArgs, kind)
	if !ok {
		return runtimeFileOperation{}, false
	}
	return runtimeFileOperation{
		ToolName:    toolName,
		ToolCallID:  strings.TrimSpace(call.ToolCallID),
		Path:        path,
		Action:      action,
		Kind:        kind,
		ToolIsError: call.ToolIsError,
	}, true
}

func classifyFileToolAction(toolName string) (action string, kind string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read", "read_file":
		return "read", "read", true
	case "list_dir":
		return "read", "list", true
	case "write", "write_file":
		return "edit", "write", true
	case "edit", "edit_file":
		return "edit", "edit", true
	default:
		return "", "", false
	}
}

func fileToolPathFromArgs(rawArgs string, kind string) (string, bool) {
	var args map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawArgs)), &args); err != nil {
		return "", false
	}
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" && kind == "list" {
		path = "."
	}
	path = normalizeFileAttentionPath(path)
	if path == "" {
		return "", false
	}
	return path, true
}

func normalizeFileAttentionPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "" {
		return ""
	}
	return cleaned
}

func applyFileAttentionSummary(run *Run, op runtimeFileOperation, timestamp string) {
	if run == nil || strings.TrimSpace(op.Path) == "" {
		return
	}
	idx := -1
	for i := range run.FileAttention {
		if run.FileAttention[i].Path == op.Path {
			idx = i
			break
		}
	}
	if idx == -1 {
		run.FileAttention = append(run.FileAttention, FileAttentionSummary{
			Path:    op.Path,
			FirstAt: timestamp,
		})
		idx = len(run.FileAttention) - 1
	}
	summary := &run.FileAttention[idx]
	summary.Total++
	summary.LastAt = timestamp
	if summary.FirstAt == "" {
		summary.FirstAt = timestamp
	}
	switch op.Kind {
	case "list":
		summary.Reads++
		summary.Lists++
	case "write":
		summary.Edits++
		summary.Writes++
	case "edit":
		summary.Edits++
	default:
		summary.Reads++
	}
	summary.Sparkline = appendSparklineAccess(summary.Sparkline)
	run.FileOpsTotal++
	sortFileAttention(run.FileAttention)
}

func appendSparklineAccess(values []int) []int {
	next := append([]int(nil), values...)
	if len(next) < maxFileAttentionSparklineBuckets {
		return append(next, 1)
	}
	next[len(next)-1]++
	return next
}

func sortFileAttention(rows []FileAttentionSummary) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		return rows[i].Path < rows[j].Path
	})
}
