package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
	zlog "github.com/rs/zerolog/log"
)

func NewSubagentsPlanTool(runtime *gateway.Runtime, router llm.Router) Tool {
	return Tool{
		Name:        "subagents_plan",
		Description: "Use the gateway planner model to create a staged subagent execution plan before calling subagents_orchestrate.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "goal":{"type":"string","description":"What the subagent workflow should accomplish."},
    "agent":{"type":"string","description":"Optional safe prompt agent. Defaults to explorer."},
    "flow_id":{"type":"string","description":"Optional flow identifier for logging and traceability."},
    "targets":{"type":"array","items":{"type":"string"},"description":"Exact file or directory paths that must remain verbatim in the generated plan prompts."},
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":300000},
    "max_steps":{"type":"integer","minimum":1,"maximum":8,"default":4},
    "max_parallel_tasks":{"type":"integer","minimum":1,"maximum":8},
    "constraints":{"type":"array","items":{"type":"string"}},
    "hints":{"type":"array","items":{"type":"string"}},
    "explicit_targets_only":{"type":"boolean","description":"When true, only paths in the targets array become required target paths. Paths mentioned in goal/constraints/hints are not auto-extracted."}
  },
  "required":["goal"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}
			if router == nil {
				return JSONTextResult(map[string]any{"message": "llm router is not configured"}, true), nil
			}

			var input struct {
				Goal                string   `json:"goal"`
				Agent               string   `json:"agent,omitempty"`
				FlowID              string   `json:"flow_id,omitempty"`
				Targets             []string `json:"targets,omitempty"`
				TimeoutMS           int      `json:"timeout_ms,omitempty"`
				MaxSteps            int      `json:"max_steps,omitempty"`
				MaxParallelTasks    int      `json:"max_parallel_tasks,omitempty"`
				Constraints         []string `json:"constraints,omitempty"`
				Hints               []string `json:"hints,omitempty"`
				ExplicitTargetsOnly bool     `json:"explicit_targets_only,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}

			goal := strings.TrimSpace(input.Goal)
			if goal == "" {
				return JSONTextResult(map[string]any{"message": "goal is required"}, true), nil
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

			maxThreads, _ := runtime.SubagentLimits()
			effectiveMaxParallel := maxThreads
			if input.MaxParallelTasks > 0 && (effectiveMaxParallel <= 0 || input.MaxParallelTasks < effectiveMaxParallel) {
				effectiveMaxParallel = input.MaxParallelTasks
			}
			if effectiveMaxParallel <= 0 {
				effectiveMaxParallel = 4
			}

			maxSteps := input.MaxSteps
			if maxSteps <= 0 {
				maxSteps = 4
			}

			flowID := strings.TrimSpace(input.FlowID)
			if flowID == "" {
				flowID = fmt.Sprintf("flow_%d", time.Now().UnixNano())
			}
			requiredTargets := collectPlannerTargets(goal, input.Targets, input.Constraints, input.Hints, input.ExplicitTargetsOnly)

			plannerClient, resolution, err := router.ClientFor(llm.RoleGatewayPlanner)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}

			meta := usage.CallMetaFromContext(ctx)
			plannerCtx := llm.WithSelectionMetadata(ctx, llm.SelectionMetadata{
				Role:      llm.RoleGatewayPlanner,
				Tier:      resolution.Tier,
				Provider:  resolution.Provider,
				Model:     resolution.Model,
				Source:    resolution.Source,
				SessionID: meta.SessionID,
				RunID:     meta.RunID,
				AgentName: agentName,
				FlowID:    flowID,
				StepID:    "plan",
			})

			zlog.Debug().
				Str("flow_id", flowID).
				Str("agent", agentName).
				Str("planner_role", llm.RoleGatewayPlanner.String()).
				Str("planner_tier", string(resolution.Tier)).
				Int("max_steps", maxSteps).
				Int("max_parallel_tasks", effectiveMaxParallel).
				Msg("subagent planner started")

			resp, err := plannerClient.Chat(plannerCtx, buildSubagentsPlannerMessages(info, goal, agentName, flowID, maxSteps, effectiveMaxParallel, input.Constraints, input.Hints, requiredTargets), llm.ChatOptions{})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}

			plan := subagentFlowInput{
				Agent:     agentName,
				FlowID:    flowID,
				TimeoutMS: input.TimeoutMS,
			}
			if err := decodeSubagentFlowInput(resp.Message.Content, &plan); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("planner returned invalid JSON: %v", err)}, true), nil
			}
			plan.Agent = agentName
			plan.FlowID = flowID
			if input.TimeoutMS > 0 {
				plan.TimeoutMS = input.TimeoutMS
			}
			if len(plan.Steps) == 0 {
				return JSONTextResult(map[string]any{"message": "planner returned empty step list"}, true), nil
			}
			if len(plan.Steps) > maxSteps {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("planner returned %d steps, exceeds max_steps=%d", len(plan.Steps), maxSteps)}, true), nil
			}
			normalization, normErr := normalizeSubagentFlowPlan(&plan)
			if normErr != nil {
				return JSONTextResult(map[string]any{"message": normErr.Error()}, true), nil
			}
			targetInjectionCount := ensurePlannerTargetsInPlan(&plan, requiredTargets)
			if normalization.Changed {
				zlog.Debug().
					Str("flow_id", flowID).
					Str("agent", agentName).
					Int("step_id_changes", normalization.StepIDChanges).
					Int("task_id_changes", normalization.TaskIDChanges).
					Int("reference_changes", normalization.ReferenceChanges).
					Msg("subagent planner normalized plan")
			}
			if targetInjectionCount > 0 {
				zlog.Debug().
					Str("flow_id", flowID).
					Str("agent", agentName).
					Int("injected_targets", targetInjectionCount).
					Msg("subagent planner injected required targets into plan")
			}
			if err := validateSubagentFlow(plan.Steps, effectiveMaxParallel); err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}

			zlog.Debug().
				Str("flow_id", flowID).
				Str("agent", agentName).
				Str("planner_tier", string(resolution.Tier)).
				Int("step_count", len(plan.Steps)).
				Msg("subagent planner completed")

			payload := map[string]any{
				"flow_id":          plan.FlowID,
				"agent":            plan.Agent,
				"steps":            plan.Steps,
				"planner_role":     llm.RoleGatewayPlanner.String(),
				"planner_tier":     string(resolution.Tier),
				"planner_provider": resolution.Provider,
				"planner_model":    resolution.Model,
			}
			if plan.TimeoutMS > 0 {
				payload["timeout_ms"] = plan.TimeoutMS
			}
			return JSONTextResult(payload, false), nil
		},
	}
}

func buildSubagentsPlannerMessages(
	info gateway.AgentInfo,
	goal string,
	agentName string,
	flowID string,
	maxSteps int,
	maxParallelTasks int,
	constraints []string,
	hints []string,
	targets []string,
) []llm.ChatMessage {
	systemPrompt := strings.TrimSpace(fmt.Sprintf(`
You are the TARS gateway planner.
Return JSON only. Do not use markdown fences. Do not add explanations.

Plan a subagent workflow for the "subagents_orchestrate" tool.

Output schema:
{
  "steps": [
    {
      "id": "string",
      "mode": "parallel" | "sequential",
      "tasks": [
        {
          "id": "string",
          "title": "string",
          "prompt": "string",
          "tier": "heavy" | "standard" | "light",
          "depends_on": ["task_id"]
        }
      ]
    }
  ]
}

Planning rules:
- Use "parallel" only when tasks are independent.
- Use "sequential" when task order matters or a task consumes earlier results.
- A task may depend only on earlier tasks, never future tasks.
- When later tasks need prior outputs, reference them with placeholders like {{task.backend.summary}} or {{task.backend.response}}.
- Keep prompts concise, concrete, and read-only.
- Use no more than %d steps.
- Use no more than %d tasks in any parallel step.
- All task ids must be unique, short, and stable.
- If exact target files or directories are provided, preserve them verbatim in the generated task prompts. Do not shorten, rewrite, relativize, translate, or omit them.
- The selected execution agent is %q and must stay unchanged.
- Available read-only tools for that agent: %s.
- Do not plan shell execution, file writes, patches, or destructive work.
`, maxSteps, maxParallelTasks, agentName, strings.Join(info.ToolsAllow, ", ")))

	var userPrompt strings.Builder
	userPrompt.WriteString(fmt.Sprintf("Flow ID: %s\n", flowID))
	userPrompt.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	if desc := strings.TrimSpace(info.Description); desc != "" {
		userPrompt.WriteString(fmt.Sprintf("Agent description: %s\n", desc))
	}
	if items := sanitizeStringList(constraints); len(items) > 0 {
		userPrompt.WriteString("Constraints:\n")
		for _, item := range items {
			userPrompt.WriteString("- " + item + "\n")
		}
	}
	if items := sanitizeStringList(hints); len(items) > 0 {
		userPrompt.WriteString("Hints:\n")
		for _, item := range items {
			userPrompt.WriteString("- " + item + "\n")
		}
	}
	if items := sanitizeStringList(targets); len(items) > 0 {
		userPrompt.WriteString("Required exact target paths/directories:\n")
		for _, item := range items {
			userPrompt.WriteString("- " + item + "\n")
		}
	}
	userPrompt.WriteString("Return the JSON execution plan now.")

	return []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt.String()},
	}
}

func decodeSubagentFlowInput(raw string, plan *subagentFlowInput) error {
	if plan == nil {
		return fmt.Errorf("plan target is nil")
	}
	text := strings.TrimSpace(raw)
	if text == "" {
		return fmt.Errorf("empty response")
	}
	if stripped, ok := stripFencedJSON(text); ok {
		text = stripped
	}
	if err := json.Unmarshal([]byte(text), plan); err == nil {
		return nil
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return json.Unmarshal([]byte(text[start:end+1]), plan)
	}
	return fmt.Errorf("planner response is not a JSON object")
}

func stripFencedJSON(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return "", false
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 {
		return "", false
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return "", false
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n")), true
}

type plannerNormalizationResult struct {
	Changed          bool
	StepIDChanges    int
	TaskIDChanges    int
	ReferenceChanges int
}

var plannerIdentifierSanitizer = regexp.MustCompile(`[^a-z0-9]+`)
var plannerTargetExtractor = regexp.MustCompile("(?i)(?:`([^`]+)`|(/[^\\s,;:()\\[\\]{}]+)|((?:\\.?\\.?(?:/[^\\s,;:()\\[\\]{}]+)+)))")

func normalizeSubagentFlowPlan(plan *subagentFlowInput) (plannerNormalizationResult, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return plannerNormalizationResult{}, nil
	}
	var result plannerNormalizationResult
	stepCounts := map[string]int{}
	taskCounts := map[string]int{}
	references := map[string]string{}

	for stepIndex := range plan.Steps {
		step := &plan.Steps[stepIndex]
		originalStepID := strings.TrimSpace(step.ID)
		step.ID = nextPlannerUniqueID(plannerBaseID("step", stepIndex+1, step.ID), stepCounts)
		if step.ID != originalStepID {
			result.Changed = true
			result.StepIDChanges++
		}

		for taskIndex := range step.Tasks {
			task := &step.Tasks[taskIndex]

			if raw := strings.TrimSpace(task.Tier); raw != "" {
				tier, err := llm.ParseTier(raw)
				if err != nil {
					return plannerNormalizationResult{}, fmt.Errorf("task %q: invalid tier %q (must be heavy, standard, or light)", task.ID, raw)
				}
				task.Tier = string(tier)
			}

			rewrittenDependsOn, depChanges := rewritePlannerDependsOn(task.DependsOn, references)
			task.DependsOn = rewrittenDependsOn
			result.ReferenceChanges += depChanges
			if depChanges > 0 {
				result.Changed = true
			}

			rewrittenPrompt, promptChanges := rewritePlannerPromptReferences(task.Prompt, references)
			task.Prompt = rewrittenPrompt
			result.ReferenceChanges += promptChanges
			if promptChanges > 0 {
				result.Changed = true
			}

			originalTaskID := strings.TrimSpace(task.ID)
			task.ID = nextPlannerUniqueID(plannerTaskBaseID(*task, stepIndex, taskIndex), taskCounts)
			if task.ID != originalTaskID {
				result.Changed = true
				result.TaskIDChanges++
			}
			registerPlannerReference(references, originalTaskID, task.ID)
			registerPlannerReference(references, task.ID, task.ID)
		}
	}

	return result, nil
}

func rewritePlannerDependsOn(dependsOn []string, references map[string]string) ([]string, int) {
	if len(dependsOn) == 0 {
		return nil, 0
	}
	out := make([]string, 0, len(dependsOn))
	changes := 0
	for _, dep := range dependsOn {
		trimmed := strings.TrimSpace(dep)
		if trimmed == "" {
			continue
		}
		resolved := resolvePlannerReference(trimmed, references)
		if resolved == "" {
			resolved = trimmed
		}
		if resolved != trimmed {
			changes++
		}
		out = append(out, resolved)
	}
	return sanitizeStringList(out), changes
}

func rewritePlannerPromptReferences(prompt string, references map[string]string) (string, int) {
	if strings.TrimSpace(prompt) == "" {
		return prompt, 0
	}
	changes := 0
	rewritten := orchestrationTaskPlaceholder.ReplaceAllStringFunc(prompt, func(match string) string {
		parts := orchestrationTaskPlaceholder.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		originalRef := strings.TrimSpace(parts[1])
		field := strings.TrimSpace(parts[2])
		resolved := resolvePlannerReference(originalRef, references)
		if resolved == "" || resolved == originalRef {
			return match
		}
		changes++
		return "{{task." + resolved + "." + field + "}}"
	})
	return rewritten, changes
}

func resolvePlannerReference(ref string, references map[string]string) string {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return ""
	}
	for _, candidate := range []string{
		trimmed,
		strings.ToLower(trimmed),
		plannerSanitizeIdentifier(trimmed),
	} {
		if candidate == "" {
			continue
		}
		if resolved := strings.TrimSpace(references[candidate]); resolved != "" {
			return resolved
		}
	}
	return ""
}

func registerPlannerReference(references map[string]string, source string, target string) {
	resolved := strings.TrimSpace(target)
	if resolved == "" {
		return
	}
	for _, candidate := range []string{
		strings.TrimSpace(source),
		strings.ToLower(strings.TrimSpace(source)),
		plannerSanitizeIdentifier(source),
	} {
		if candidate == "" {
			continue
		}
		references[candidate] = resolved
	}
}

func plannerTaskBaseID(task subagentFlowTaskInput, stepIndex int, taskIndex int) string {
	return plannerBaseID(
		"task",
		taskIndex+1,
		task.ID,
		task.Title,
		task.Prompt,
		fmt.Sprintf("step_%d_task_%d", stepIndex+1, taskIndex+1),
	)
}

func plannerBaseID(prefix string, ordinal int, candidates ...string) string {
	for _, candidate := range candidates {
		if sanitized := plannerSanitizeIdentifier(candidate); sanitized != "" {
			return sanitized
		}
	}
	if ordinal <= 0 {
		return prefix
	}
	return fmt.Sprintf("%s_%d", prefix, ordinal)
}

func plannerSanitizeIdentifier(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ""
	}
	sanitized := plannerIdentifierSanitizer.ReplaceAllString(trimmed, "_")
	sanitized = strings.Trim(sanitized, "_.-")
	if sanitized == "" {
		return ""
	}
	return sanitized
}

func nextPlannerUniqueID(base string, counts map[string]int) string {
	sanitizedBase := plannerSanitizeIdentifier(base)
	if sanitizedBase == "" {
		sanitizedBase = "task"
	}
	counts[sanitizedBase]++
	if counts[sanitizedBase] == 1 {
		return sanitizedBase
	}
	return fmt.Sprintf("%s_%d", sanitizedBase, counts[sanitizedBase])
}

func collectPlannerTargets(goal string, explicitTargets, constraints, hints []string, explicitOnly bool) []string {
	out := make([]string, 0, len(explicitTargets))
	seen := map[string]struct{}{}
	add := func(value string) {
		target := normalizePlannerTarget(value)
		if target == "" {
			return
		}
		if _, ok := seen[target]; ok {
			return
		}
		seen[target] = struct{}{}
		out = append(out, target)
	}

	// Explicit targets are always included.
	for _, t := range explicitTargets {
		add(t)
	}

	if explicitOnly {
		return out
	}

	// Auto-extract paths from goal, constraints, and hints.
	for _, candidate := range append(append([]string{goal}, constraints...), hints...) {
		add(candidate)
		for _, match := range plannerTargetExtractor.FindAllStringSubmatch(candidate, -1) {
			for _, part := range match[1:] {
				add(part)
			}
		}
	}
	return out
}

func normalizePlannerTarget(value string) string {
	target := strings.TrimSpace(value)
	target = strings.Trim(target, "\"'`")
	target = strings.TrimRight(target, ".,;:)")
	target = strings.TrimLeft(target, "(")
	if target == "" {
		return ""
	}
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return ""
	}
	if strings.Contains(target, "{{task.") {
		return ""
	}
	if !strings.Contains(target, "/") {
		return ""
	}
	cleaned := filepath.Clean(target)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return ""
	}
	return cleaned
}

func ensurePlannerTargetsInPlan(plan *subagentFlowInput, targets []string) int {
	if plan == nil || len(plan.Steps) == 0 || len(targets) == 0 {
		return 0
	}
	injected := 0
	for _, target := range sanitizeStringList(targets) {
		task := plannerTaskForTarget(plan, target)
		if task == nil {
			continue
		}
		if strings.Contains(strings.ToLower(task.Prompt), strings.ToLower(target)) {
			continue
		}
		task.Prompt = strings.TrimSpace(task.Prompt) + "\n\nRequired exact target path:\n- " + target
		injected++
	}
	return injected
}

func plannerTaskForTarget(plan *subagentFlowInput, target string) *subagentFlowTaskInput {
	var fallback *subagentFlowTaskInput
	targetBase := strings.ToLower(strings.TrimSpace(filepath.Base(target)))
	targetDir := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.Dir(target))))
	bestScore := -1
	var best *subagentFlowTaskInput

	for stepIndex := range plan.Steps {
		for taskIndex := range plan.Steps[stepIndex].Tasks {
			task := &plan.Steps[stepIndex].Tasks[taskIndex]
			if fallback == nil {
				fallback = task
			}
			score := 0
			text := strings.ToLower(strings.Join([]string{task.ID, task.Title, task.Prompt}, " "))
			if targetBase != "" && strings.Contains(text, targetBase) {
				score += 3
			}
			if targetDir != "" && strings.Contains(text, targetDir) {
				score += 2
			}
			if score > bestScore {
				bestScore = score
				best = task
			}
		}
	}
	if best != nil {
		return best
	}
	return fallback
}
