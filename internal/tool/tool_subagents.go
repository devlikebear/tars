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
		Description: "Run multiple independent read-only subagents in parallel and return compact summaries.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
	    "agent":{"type":"string","description":"Optional safe prompt agent. Defaults to explorer."},
	    "mode":{"type":"string","enum":["parallel","consensus"],"description":"Execution mode. Defaults to parallel."},
	    "consensus":{
	      "type":"object",
	      "properties":{
	        "strategy":{"type":"string","enum":["synthesize","vote"]},
	        "variants":{
	          "type":"array",
	          "minItems":1,
	          "items":{
	            "type":"object",
	            "properties":{
	              "alias":{"type":"string"},
	              "model":{"type":"string"}
	            },
	            "required":["alias"],
	            "additionalProperties":false
	          }
	        }
	      },
	      "additionalProperties":false
	    },
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":300000,"default":60000},
    "tasks":{
      "type":"array",
      "minItems":1,
      "maxItems":8,
		"items":{
			"type":"object",
			"properties":{
			  "title":{"type":"string"},
			  "prompt":{"type":"string"},
			  "tier":{"type":"string","enum":["heavy","standard","light"],"description":"Optional LLM tier override for this task. Falls back to agent tier, then default tier."},
			  "provider_override":{"type":"object","properties":{"alias":{"type":"string"},"model":{"type":"string"}},"required":["alias"],"additionalProperties":false}
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
				Mode      string `json:"mode,omitempty"`
				Consensus struct {
					Strategy string `json:"strategy,omitempty"`
					Variants []struct {
						Alias string `json:"alias,omitempty"`
						Model string `json:"model,omitempty"`
					} `json:"variants,omitempty"`
				} `json:"consensus,omitempty"`
				TimeoutMS int `json:"timeout_ms,omitempty"`
				Tasks     []struct {
					Title            string                    `json:"title,omitempty"`
					Prompt           string                    `json:"prompt"`
					Tier             string                    `json:"tier,omitempty"`
					ProviderOverride *gateway.ProviderOverride `json:"provider_override,omitempty"`
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
			mode := strings.ToLower(strings.TrimSpace(input.Mode))
			if mode == "" {
				mode = "parallel"
			}
			if mode == "consensus" {
				if len(input.Tasks) != 1 {
					return JSONTextResult(map[string]any{"message": "consensus mode requires exactly one task"}, true), nil
				}
				if len(input.Consensus.Variants) == 0 {
					return JSONTextResult(map[string]any{"message": "consensus variants are required"}, true), nil
				}
			}
			if mode != "consensus" && maxThreads > 0 && len(input.Tasks) > maxThreads {
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
				providerOverride, overrideErr := normalizeProviderOverride(task.ProviderOverride)
				if overrideErr != "" {
					cancelSubagentRuns(runtime, workspaceID, spawnedRuns)
					return JSONTextResult(map[string]any{"message": overrideErr}, true), nil
				}
				// Tier resolution: explicit task tier > agent tier > empty (router default).
				taskTier := strings.ToLower(strings.TrimSpace(task.Tier))
				if taskTier == "" {
					taskTier = strings.ToLower(strings.TrimSpace(info.Tier))
				}
				spawnReq := gateway.SpawnRequest{
					WorkspaceID:      workspaceID,
					Title:            title,
					Prompt:           prompt,
					Agent:            agentName,
					ParentRunID:      parentRunID,
					RootRunID:        rootRunID,
					ParentSessionID:  strings.TrimSpace(meta.SessionID),
					Depth:            nextDepth,
					SessionKind:      "subagent",
					SessionHidden:    true,
					Tier:             taskTier,
					ProviderOverride: providerOverride,
				}
				if mode == "consensus" {
					spawnReq.Mode = "consensus"
					spawnReq.Consensus = &gateway.ConsensusSpec{Strategy: strings.TrimSpace(input.Consensus.Strategy)}
					for _, variant := range input.Consensus.Variants {
						spawnReq.Consensus.Variants = append(spawnReq.Consensus.Variants, gateway.ProviderOverride{Alias: strings.TrimSpace(variant.Alias), Model: strings.TrimSpace(variant.Model)})
					}
				}
				run, err := runtime.Spawn(waitCtx, spawnReq)
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
				Tier            string `json:"tier,omitempty"`
				ConsensusMode   string `json:"consensus_mode,omitempty"`
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
					Tier:            final.Tier,
					ConsensusMode:   final.ConsensusMode,
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

func normalizeProviderOverride(value *gateway.ProviderOverride) (*gateway.ProviderOverride, string) {
	override := gateway.CloneProviderOverride(value)
	if override == nil {
		return nil, ""
	}
	if strings.TrimSpace(override.Alias) == "" {
		return nil, "provider_override.alias is required"
	}
	return override, ""
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

func sanitizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := set[trimmed]; exists {
			continue
		}
		set[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
