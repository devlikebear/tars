package apptool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
)

func NewSubagentsRunTool(runtime *agentruntime.Runtime) Tool {
	return Tool{
		Name:        "subagents_run",
		Description: "Run multiple independent read-only subagents in parallel and return compact summaries.",
		Parameters:  subagentsRunToolParameters(runtime),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "agent runtime is not configured"}, true), nil
			}
			var input struct {
				Agent     string `json:"agent,omitempty"`
				Mode      string `json:"mode,omitempty"`
				Consensus struct {
					Strategy             string  `json:"strategy,omitempty"`
					Automatic            bool    `json:"automatic,omitempty"`
					BaselineID           string  `json:"baseline_id,omitempty"`
					ExpectedQualityDelta float64 `json:"expected_quality_delta,omitempty"`
					DecisionReason       string  `json:"decision_reason,omitempty"`
					Variants             []struct {
						Alias string `json:"alias,omitempty"`
						Model string `json:"model,omitempty"`
					} `json:"variants,omitempty"`
				} `json:"consensus,omitempty"`
				TimeoutMS int                     `json:"timeout_ms,omitempty"`
				Tasks     []subagentsRunTaskInput `json:"tasks"`
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
			switch mode {
			case "parallel", "consensus", "compare":
			default:
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("unsupported subagent mode %q", mode)}, true), nil
			}
			if mode == "consensus" {
				if !runtime.ConsensusEnabled() {
					return JSONTextResult(map[string]any{"message": "consensus mode is disabled (agentruntime_consensus_enabled=false)"}, true), nil
				}
				if len(input.Tasks) != 1 {
					return JSONTextResult(map[string]any{"message": "consensus mode requires exactly one task"}, true), nil
				}
				if len(input.Consensus.Variants) == 0 {
					return JSONTextResult(map[string]any{"message": "consensus variants are required"}, true), nil
				}
			}
			if mode == "compare" {
				if len(input.Tasks) < 2 || len(input.Tasks) > 3 {
					return JSONTextResult(map[string]any{"message": "compare mode requires 2-3 tasks"}, true), nil
				}
				if !subagentComparePromptsMatch(input.Tasks) {
					return JSONTextResult(map[string]any{"message": "compare mode requires all task prompts to match"}, true), nil
				}
			}
			if mode != "consensus" && maxThreads > 0 && len(input.Tasks) > maxThreads {
				return JSONTextResult(map[string]any{
					"message": fmt.Sprintf("requested %d tasks exceeds agentruntime_subagents_max_threads=%d", len(input.Tasks), maxThreads),
				}, true), nil
			}

			agentName := strings.TrimSpace(input.Agent)
			if agentName == "" {
				agentName = "explorer"
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
					"message": fmt.Sprintf("subagent depth %d exceeds agentruntime_subagents_max_depth=%d", nextDepth, maxDepth),
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
				run    agentruntime.Run
			}
			requests := make([]subagentRequest, 0, len(input.Tasks))
			spawnedRuns := make([]agentruntime.Run, 0, len(input.Tasks))
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
				taskAgent := strings.TrimSpace(task.Agent)
				if taskAgent == "" {
					taskAgent = agentName
				}
				info, ok := runtime.LookupAgent(taskAgent)
				if !ok {
					cancelSubagentRuns(runtime, workspaceID, spawnedRuns)
					return JSONTextResult(map[string]any{"message": fmt.Sprintf("subagent %q is not available", taskAgent)}, true), nil
				}
				if msg := validateSafeSubagent(info); msg != "" {
					cancelSubagentRuns(runtime, workspaceID, spawnedRuns)
					return JSONTextResult(map[string]any{"message": msg}, true), nil
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
				spawnReq := agentruntime.SpawnRequest{
					WorkspaceID:      workspaceID,
					Title:            title,
					Prompt:           prompt,
					Agent:            taskAgent,
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
					spawnReq.Consensus = &agentruntime.ConsensusSpec{
						Strategy: strings.TrimSpace(input.Consensus.Strategy), Automatic: input.Consensus.Automatic,
						BaselineID:           strings.TrimSpace(input.Consensus.BaselineID),
						ExpectedQualityDelta: input.Consensus.ExpectedQualityDelta,
						DecisionReason:       strings.TrimSpace(input.Consensus.DecisionReason),
					}
					for _, variant := range input.Consensus.Variants {
						spawnReq.Consensus.Variants = append(spawnReq.Consensus.Variants, agentruntime.ProviderOverride{Alias: strings.TrimSpace(variant.Alias), Model: strings.TrimSpace(variant.Model)})
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

			results := make([]subagentsRunResult, 0, len(requests))
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
				if final.Status != agentruntime.RunStatusCompleted {
					hadFailure = true
				}
				results = append(results, subagentsRunResult{
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
					Response:        strings.TrimSpace(final.Response),
				})
			}

			payload := map[string]any{
				"count":     len(results),
				"agent":     subagentsRunAgentLabel(results, agentName),
				"mode":      mode,
				"subagents": results,
			}
			if mode == "compare" {
				payload["comparison"] = buildSubagentsRunComparison(results)
			}
			return JSONTextResult(payload, hadFailure), nil
		},
	}
}

type subagentsRunTaskInput struct {
	Title            string                         `json:"title,omitempty"`
	Agent            string                         `json:"agent,omitempty"`
	Prompt           string                         `json:"prompt"`
	Tier             string                         `json:"tier,omitempty"`
	ProviderOverride *agentruntime.ProviderOverride `json:"provider_override,omitempty"`
}

type subagentsRunResult struct {
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
	Response        string `json:"-"`
}

type subagentsRunComparison struct {
	CommonFindings []string                      `json:"common_findings"`
	Conflicts      []string                      `json:"conflicts"`
	Evidence       []subagentsRunCompareEvidence `json:"evidence"`
	SideBySide     []subagentsRunCompareOutput   `json:"side_by_side"`
}

type subagentsRunCompareEvidence struct {
	RunID string `json:"run_id"`
	Title string `json:"title,omitempty"`
	Agent string `json:"agent,omitempty"`
	Text  string `json:"text"`
}

type subagentsRunCompareOutput struct {
	RunID    string `json:"run_id"`
	Title    string `json:"title,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Status   string `json:"status,omitempty"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

func subagentsRunToolParameters(runtime *agentruntime.Runtime) json.RawMessage {
	if runtime.ConsensusEnabled() {
		return json.RawMessage(`{
  "type":"object",
  "properties":{
	    "agent":{"type":"string","description":"Optional safe prompt agent. Defaults to explorer."},
	    "mode":{"type":"string","enum":["parallel","consensus","compare"],"description":"Execution mode. Defaults to parallel. Compare mode requires 2-3 tasks with matching prompts."},
	    "consensus":{
	      "type":"object",
	      "properties":{
	        "strategy":{"type":"string","enum":["synthesize"]},
	        "automatic":{"type":"boolean","description":"Marks policy-selected fan-out; requires OH-001 baseline evidence and expected benefit."},
	        "baseline_id":{"type":"string"},
	        "expected_quality_delta":{"type":"number","exclusiveMinimum":0},
	        "decision_reason":{"type":"string"},
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
			  "agent":{"type":"string","description":"Optional task-specific safe prompt agent. Falls back to top-level agent."},
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
}`)
	}
	return json.RawMessage(`{
  "type":"object",
  "properties":{
	    "agent":{"type":"string","description":"Optional safe prompt agent. Defaults to explorer."},
	    "mode":{"type":"string","enum":["parallel","compare"],"description":"Execution mode. Defaults to parallel. Compare mode requires 2-3 tasks with matching prompts."},
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":300000,"default":60000},
    "tasks":{
      "type":"array",
      "minItems":1,
      "maxItems":8,
		"items":{
			"type":"object",
			"properties":{
			  "title":{"type":"string"},
			  "agent":{"type":"string","description":"Optional task-specific safe prompt agent. Falls back to top-level agent."},
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
}`)
}

func subagentComparePromptsMatch(tasks []subagentsRunTaskInput) bool {
	if len(tasks) == 0 {
		return false
	}
	first := normalizeSubagentComparePrompt(tasks[0].Prompt)
	if first == "" {
		return false
	}
	for _, task := range tasks[1:] {
		if normalizeSubagentComparePrompt(task.Prompt) != first {
			return false
		}
	}
	return true
}

func normalizeSubagentComparePrompt(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func subagentsRunAgentLabel(results []subagentsRunResult, fallback string) string {
	seen := map[string]struct{}{}
	for _, result := range results {
		agent := strings.TrimSpace(result.Agent)
		if agent == "" {
			continue
		}
		seen[agent] = struct{}{}
	}
	if len(seen) == 1 {
		for agent := range seen {
			return agent
		}
	}
	if len(seen) > 1 {
		return "mixed"
	}
	return strings.TrimSpace(fallback)
}

func buildSubagentsRunComparison(results []subagentsRunResult) subagentsRunComparison {
	comparison := subagentsRunComparison{
		CommonFindings: []string{},
		Conflicts:      []string{},
		Evidence:       []subagentsRunCompareEvidence{},
		SideBySide:     make([]subagentsRunCompareOutput, 0, len(results)),
	}
	allStatements := make([]compareStatement, 0, len(results)*4)
	seenCommon := map[string]int{}
	commonText := map[string]string{}
	for _, result := range results {
		output := strings.TrimSpace(result.Response)
		if output == "" {
			output = strings.TrimSpace(result.Error)
		}
		comparison.SideBySide = append(comparison.SideBySide, subagentsRunCompareOutput{
			RunID:    result.RunID,
			Title:    result.Title,
			Agent:    result.Agent,
			Status:   result.Status,
			Response: trimSubagentSummary(output, 2400),
			Error:    result.Error,
		})
		statements := extractCompareStatements(output)
		resultSeen := map[string]struct{}{}
		for index, statement := range statements {
			normalized := normalizeCompareStatement(statement)
			if normalized == "" {
				continue
			}
			allStatements = append(allStatements, compareStatement{
				RunID:      result.RunID,
				Title:      result.Title,
				Agent:      result.Agent,
				Text:       statement,
				Normalized: normalized,
			})
			if _, exists := resultSeen[normalized]; !exists {
				seenCommon[normalized]++
				resultSeen[normalized] = struct{}{}
			}
			if _, exists := commonText[normalized]; !exists {
				commonText[normalized] = statement
			}
			if index < 2 && result.Status == string(agentruntime.RunStatusCompleted) {
				comparison.Evidence = append(comparison.Evidence, subagentsRunCompareEvidence{
					RunID: result.RunID,
					Title: result.Title,
					Agent: result.Agent,
					Text:  statement,
				})
			}
		}
	}
	for _, statement := range allStatements {
		if len(comparison.CommonFindings) >= 6 {
			break
		}
		if seenCommon[statement.Normalized] < 2 {
			continue
		}
		text := commonText[statement.Normalized]
		if containsString(comparison.CommonFindings, text) {
			continue
		}
		comparison.CommonFindings = append(comparison.CommonFindings, text)
	}
	comparison.Conflicts = findCompareConflicts(allStatements)
	if len(comparison.Evidence) > 8 {
		comparison.Evidence = comparison.Evidence[:8]
	}
	return comparison
}

type compareStatement struct {
	RunID      string
	Title      string
	Agent      string
	Text       string
	Normalized string
}

func extractCompareStatements(text string) []string {
	value := strings.TrimSpace(text)
	if value == "" {
		return nil
	}
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	if len(lines) == 1 {
		lines = strings.Split(value, ". ")
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		cleaned := cleanCompareStatement(line)
		if len([]rune(cleaned)) < 8 {
			continue
		}
		out = append(out, cleaned)
	}
	return out
}

func cleanCompareStatement(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.TrimLeft(cleaned, "-* \t")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimLeftFunc(cleaned, func(r rune) bool {
		return unicode.IsDigit(r) || r == '.' || r == ')' || r == ' '
	})
	return strings.TrimSpace(cleaned)
}

func normalizeCompareStatement(value string) string {
	cleaned := strings.ToLower(cleanCompareStatement(value))
	cleaned = strings.Trim(cleaned, ".:;!?")
	return strings.Join(strings.Fields(cleaned), " ")
}

func findCompareConflicts(statements []compareStatement) []string {
	conflicts := []string{}
	for i := range statements {
		for j := i + 1; j < len(statements); j++ {
			if statements[i].RunID == statements[j].RunID {
				continue
			}
			leftNegated := compareStatementNegated(statements[i].Normalized)
			rightNegated := compareStatementNegated(statements[j].Normalized)
			if leftNegated == rightNegated {
				continue
			}
			if compareTokenOverlap(statements[i].Normalized, statements[j].Normalized) < 2 {
				continue
			}
			conflict := fmt.Sprintf("%s: %s <-> %s: %s", compareStatementLabel(statements[i]), statements[i].Text, compareStatementLabel(statements[j]), statements[j].Text)
			if containsString(conflicts, conflict) {
				continue
			}
			conflicts = append(conflicts, conflict)
			if len(conflicts) >= 4 {
				return conflicts
			}
		}
	}
	return conflicts
}

func compareStatementNegated(value string) bool {
	negativeMarkers := []string{" no ", " not ", " never ", " cannot ", " can't ", " without ", "none "}
	padded := " " + strings.ToLower(value) + " "
	for _, marker := range negativeMarkers {
		if strings.Contains(padded, marker) {
			return true
		}
	}
	return false
}

func compareTokenOverlap(left string, right string) int {
	leftTokens := compareTokenSet(left)
	count := 0
	for token := range compareTokenSet(right) {
		if _, ok := leftTokens[token]; ok {
			count++
		}
	}
	return count
}

func compareTokenSet(value string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		token = strings.TrimSpace(token)
		if len([]rune(token)) < 3 {
			continue
		}
		tokens[token] = struct{}{}
	}
	return tokens
}

func compareStatementLabel(statement compareStatement) string {
	if title := strings.TrimSpace(statement.Title); title != "" {
		return title
	}
	if agent := strings.TrimSpace(statement.Agent); agent != "" {
		return agent
	}
	return statement.RunID
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cancelSubagentRuns(runtime *agentruntime.Runtime, workspaceID string, runs []agentruntime.Run) {
	if runtime == nil {
		return
	}
	for _, run := range runs {
		_, _ = runtime.CancelByWorkspace(workspaceID, run.ID)
	}
}

func normalizeProviderOverride(value *agentruntime.ProviderOverride) (*agentruntime.ProviderOverride, string) {
	override := agentruntime.CloneProviderOverride(value)
	if override == nil {
		return nil, ""
	}
	if strings.TrimSpace(override.Alias) == "" {
		return nil, "provider_override.alias is required"
	}
	return override, ""
}

func validateSafeSubagent(info agentruntime.AgentInfo) string {
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
