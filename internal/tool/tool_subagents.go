package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
)

func NewSubagentsRunTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "subagents_run",
		Description: "Run multiple read-only subagents in parallel and return compact summaries.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "agent":{"type":"string","description":"Optional safe prompt agent. Defaults to explorer."},
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":300000,"default":60000},
    "tasks":{
      "type":"array",
      "minItems":1,
      "maxItems":8,
      "items":{
        "type":"object",
        "properties":{
          "title":{"type":"string"},
          "prompt":{"type":"string"}
        },
        "required":["prompt"],
        "additionalProperties":false
      }
    }
  },
  "required":["tasks"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			var input struct {
				Agent     string `json:"agent,omitempty"`
				TimeoutMS int    `json:"timeout_ms,omitempty"`
				Tasks     []struct {
					Title  string `json:"title,omitempty"`
					Prompt string `json:"prompt"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if len(input.Tasks) == 0 {
				return JSONTextResult(map[string]any{"message": "tasks must contain at least one item"}, true), nil
			}

			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			meta := usage.CallMetaFromContext(ctx)
			maxThreads, maxDepth := runtime.SubagentLimits()
			if maxThreads > 0 && len(input.Tasks) > maxThreads {
				return JSONTextResult(map[string]any{
					"message": fmt.Sprintf("requested %d tasks exceeds gateway_subagents_max_threads=%d", len(input.Tasks), maxThreads),
				}, true), nil
			}

			agentName := strings.TrimSpace(input.Agent)
			if agentName == "" {
				agentName = "explorer"
			}
			info, ok := runtime.LookupAgent(agentName)
			if !ok {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("subagent %q is not available", agentName)}, true), nil
			}
			if msg := validateSafeSubagent(info); msg != "" {
				return JSONTextResult(map[string]any{"message": msg}, true), nil
			}

			parentRunID := strings.TrimSpace(meta.RunID)
			rootRunID := ""
			nextDepth := 1
			if parentRunID != "" {
				parentRun, found := runtime.GetByWorkspace(workspaceID, parentRunID)
				if !found {
					return JSONTextResult(map[string]any{"message": fmt.Sprintf("parent run not found: %s", parentRunID)}, true), nil
				}
				rootRunID = strings.TrimSpace(parentRun.RootRunID)
				if rootRunID == "" {
					rootRunID = strings.TrimSpace(parentRun.ID)
				}
				nextDepth = parentRun.Depth + 1
			}
			if maxDepth > 0 && nextDepth > maxDepth {
				return JSONTextResult(map[string]any{
					"message": fmt.Sprintf("subagent depth %d exceeds gateway_subagents_max_depth=%d", nextDepth, maxDepth),
				}, true), nil
			}

			timeout := input.TimeoutMS
			if timeout <= 0 {
				timeout = 60000
			}
			waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
			defer cancel()

			type subagentRequest struct {
				title  string
				prompt string
				run    gateway.Run
			}
			requests := make([]subagentRequest, 0, len(input.Tasks))
			spawnedRuns := make([]gateway.Run, 0, len(input.Tasks))
			for _, task := range input.Tasks {
				prompt := strings.TrimSpace(task.Prompt)
				if prompt == "" {
					cancelSubagentRuns(runtime, workspaceID, spawnedRuns)
					return JSONTextResult(map[string]any{"message": "each task prompt is required"}, true), nil
				}
				title := strings.TrimSpace(task.Title)
				if title == "" {
					title = "subagent"
				}
				run, err := runtime.Spawn(waitCtx, gateway.SpawnRequest{
					WorkspaceID:     workspaceID,
					ProjectID:       strings.TrimSpace(meta.ProjectID),
					Title:           title,
					Prompt:          prompt,
					Agent:           agentName,
					ParentRunID:     parentRunID,
					RootRunID:       rootRunID,
					ParentSessionID: strings.TrimSpace(meta.SessionID),
					Depth:           nextDepth,
					SessionKind:     "subagent",
					SessionHidden:   true,
				})
				if err != nil {
					cancelSubagentRuns(runtime, workspaceID, spawnedRuns)
					return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				spawnedRuns = append(spawnedRuns, run)
				requests = append(requests, subagentRequest{title: title, prompt: prompt, run: run})
			}

			type subagentResult struct {
				RunID           string `json:"run_id"`
				SessionID       string `json:"session_id"`
				Agent           string `json:"agent"`
				Title           string `json:"title"`
				Status          string `json:"status"`
				ParentRunID     string `json:"parent_run_id,omitempty"`
				ParentSessionID string `json:"parent_session_id,omitempty"`
				Depth           int    `json:"depth,omitempty"`
				Summary         string `json:"summary,omitempty"`
				Error           string `json:"error,omitempty"`
			}
			results := make([]subagentResult, 0, len(requests))
			hadFailure := false
			for _, item := range requests {
				final, err := runtime.Wait(waitCtx, item.run.ID)
				if err != nil {
					cancelSubagentRuns(runtime, workspaceID, spawnedRuns)
					return JSONTextResult(map[string]any{"message": fmt.Sprintf("wait subagent %s failed: %v", item.run.ID, err)}, true), nil
				}
				summary := trimSubagentSummary(final.Response, 220)
				if summary == "" {
					summary = trimSubagentSummary(final.Error, 220)
				}
				if final.Status != gateway.RunStatusCompleted {
					hadFailure = true
				}
				results = append(results, subagentResult{
					RunID:           final.ID,
					SessionID:       final.SessionID,
					Agent:           final.Agent,
					Title:           item.title,
					Status:          string(final.Status),
					ParentRunID:     final.ParentRunID,
					ParentSessionID: final.ParentSessionID,
					Depth:           final.Depth,
					Summary:         summary,
					Error:           strings.TrimSpace(final.Error),
				})
			}

			return JSONTextResult(map[string]any{
				"count":     len(results),
				"agent":     agentName,
				"subagents": results,
			}, hadFailure), nil
		},
	}
}

func cancelSubagentRuns(runtime *gateway.Runtime, workspaceID string, runs []gateway.Run) {
	if runtime == nil {
		return
	}
	for _, run := range runs {
		_, _ = runtime.CancelByWorkspace(workspaceID, run.ID)
	}
}

func validateSafeSubagent(info gateway.AgentInfo) string {
	if strings.TrimSpace(info.Kind) != "prompt" {
		return fmt.Sprintf("subagent %q must be a prompt-based agent", strings.TrimSpace(info.Name))
	}
	if strings.TrimSpace(strings.ToLower(info.PolicyMode)) != "allowlist" {
		return fmt.Sprintf("subagent %q must use an allowlist tool policy", strings.TrimSpace(info.Name))
	}
	if len(info.ToolsAllow) == 0 {
		return fmt.Sprintf("subagent %q must define a read-only tools_allow list", strings.TrimSpace(info.Name))
	}
	for _, name := range info.ToolsAllow {
		if isHighRiskSubagentTool(name) {
			return fmt.Sprintf("subagent %q allows high-risk tool %q", strings.TrimSpace(info.Name), strings.TrimSpace(name))
		}
	}
	return ""
}

func isHighRiskSubagentTool(name string) bool {
	canonical := CanonicalToolName(name)
	switch canonical {
	case "exec", "process", "write_file", "edit_file", "apply_patch", "workspace":
		return true
	}
	return strings.HasPrefix(canonical, "write_") || strings.HasPrefix(canonical, "edit_")
}

func trimSubagentSummary(text string, max int) string {
	value := strings.TrimSpace(text)
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
