package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
	zlog "github.com/rs/zerolog/log"
)

var orchestrationTaskPlaceholder = regexp.MustCompile(`\{\{\s*task\.([a-zA-Z0-9._-]+)\.(summary|response|error)\s*\}\}`)

var (
	subagentFlowSpawn = func(runtime *gateway.Runtime, ctx context.Context, req gateway.SpawnRequest) (gateway.Run, error) {
		return runtime.Spawn(ctx, req)
	}
	subagentFlowWait = func(runtime *gateway.Runtime, ctx context.Context, runID string) (gateway.Run, error) {
		return runtime.Wait(ctx, runID)
	}
	subagentFlowCancel = func(runtime *gateway.Runtime, workspaceID string, runs []gateway.Run) {
		cancelSubagentRuns(runtime, workspaceID, runs)
	}
)

type subagentFlowInput struct {
	Agent     string                  `json:"agent,omitempty"`
	FlowID    string                  `json:"flow_id,omitempty"`
	TimeoutMS int                     `json:"timeout_ms,omitempty"`
	Steps     []subagentFlowStepInput `json:"steps"`
}

type subagentFlowStepInput struct {
	ID    string                  `json:"id,omitempty"`
	Mode  string                  `json:"mode"`
	Tasks []subagentFlowTaskInput `json:"tasks"`
}

type subagentFlowTaskInput struct {
	ID        string   `json:"id"`
	Title     string   `json:"title,omitempty"`
	Prompt    string   `json:"prompt"`
	Tier      string   `json:"tier,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

type subagentCompletedTask struct {
	Response string
	Summary  string
	Error    string
}

type subagentTaskOutput struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	RunID           string   `json:"run_id"`
	SessionID       string   `json:"session_id"`
	Agent           string   `json:"agent"`
	Status          string   `json:"status"`
	Tier            string   `json:"tier,omitempty"`
	DependsOn       []string `json:"depends_on,omitempty"`
	ParentRunID     string   `json:"parent_run_id,omitempty"`
	ParentSessionID string   `json:"parent_session_id,omitempty"`
	Depth           int      `json:"depth,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Response        string   `json:"response,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type subagentStepOutput struct {
	ID          string               `json:"id"`
	Mode        string               `json:"mode"`
	TaskCount   int                  `json:"task_count"`
	Status      string               `json:"status"`
	Summary     string               `json:"summary,omitempty"`
	Tasks       []subagentTaskOutput `json:"tasks"`
	FailedTasks int                  `json:"failed_tasks,omitempty"`
}

func NewSubagentsOrchestrateTool(runtime *gateway.Runtime) Tool {
	return Tool{
		Name:        "subagents_orchestrate",
		Description: "Execute a staged subagent flow: use parallel steps for independent work and sequential steps for dependency-aware follow-up tasks.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "agent":{"type":"string","description":"Optional safe prompt agent. Defaults to explorer."},
    "flow_id":{"type":"string","description":"Optional flow identifier for logging and traceability."},
    "timeout_ms":{"type":"integer","minimum":1000,"maximum":300000,"default":60000},
    "steps":{
      "type":"array",
      "minItems":1,
      "items":{
        "type":"object",
        "properties":{
          "id":{"type":"string"},
          "mode":{"type":"string","enum":["parallel","sequential"]},
          "tasks":{
            "type":"array",
            "minItems":1,
            "items":{
              "type":"object",
              "properties":{
                "id":{"type":"string"},
                "title":{"type":"string"},
                "prompt":{"type":"string","description":"Supports placeholders like {{task.backend.summary}} and {{task.backend.response}}."},
                "tier":{"type":"string","enum":["heavy","standard","light"],"description":"Optional LLM tier override for this task. Falls back to agent tier, then default tier."},
                "depends_on":{"type":"array","items":{"type":"string"}}
              },
              "required":["id","prompt"],
              "additionalProperties":false
            }
          }
        },
        "required":["mode","tasks"],
        "additionalProperties":false
      }
    }
  },
  "required":["steps"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if runtime == nil {
				return JSONTextResult(map[string]any{"message": "gateway runtime is not configured"}, true), nil
			}

			var input subagentFlowInput
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if len(input.Steps) == 0 {
				return JSONTextResult(map[string]any{"message": "steps must contain at least one item"}, true), nil
			}

			workspaceID := serverauth.WorkspaceIDFromContext(ctx)
			meta := usage.CallMetaFromContext(ctx)
			maxThreads, maxDepth := runtime.SubagentLimits()

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

			parentRunID, rootRunID, nextDepth, err := resolveSubagentParentContext(runtime, workspaceID, meta.RunID)
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			if maxDepth > 0 && nextDepth > maxDepth {
				return JSONTextResult(map[string]any{
					"message": fmt.Sprintf("subagent depth %d exceeds gateway_subagents_max_depth=%d", nextDepth, maxDepth),
				}, true), nil
			}

			if err := validateSubagentFlow(input.Steps, maxThreads); err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}

			timeout := input.TimeoutMS
			if timeout <= 0 {
				timeout = 60000
			}
			waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
			defer cancel()

			flowID := strings.TrimSpace(input.FlowID)
			if flowID == "" {
				flowID = fmt.Sprintf("flow_%d", time.Now().UnixNano())
			}

			zlog.Debug().
				Str("flow_id", flowID).
				Str("agent", agentName).
				Int("step_count", len(input.Steps)).
				Msg("subagent orchestration started")

			completed := map[string]subagentCompletedTask{}
			steps := make([]subagentStepOutput, 0, len(input.Steps))
			totalTasks := 0
			hadFailure := false

			for stepIndex, step := range input.Steps {
				stepID := strings.TrimSpace(step.ID)
				if stepID == "" {
					stepID = fmt.Sprintf("step_%d", stepIndex+1)
				}
				mode := strings.ToLower(strings.TrimSpace(step.Mode))
				zlog.Debug().
					Str("flow_id", flowID).
					Str("step_id", stepID).
					Str("mode", mode).
					Int("task_count", len(step.Tasks)).
					Msg("subagent orchestration step started")

				out := subagentStepOutput{
					ID:        stepID,
					Mode:      mode,
					TaskCount: len(step.Tasks),
					Status:    "completed",
					Tasks:     make([]subagentTaskOutput, 0, len(step.Tasks)),
				}
				totalTasks += len(step.Tasks)

				if mode == "parallel" {
					type pendingTask struct {
						id        string
						title     string
						tier      string
						dependsOn []string
						run       gateway.Run
					}
					pending := make([]pendingTask, 0, len(step.Tasks))
					spawnedRuns := make([]gateway.Run, 0, len(step.Tasks))
					for _, task := range step.Tasks {
						renderedPrompt, renderErr := renderSubagentFlowPrompt(task.Prompt, completed)
						if renderErr != nil {
							subagentFlowCancel(runtime, workspaceID, spawnedRuns)
							return JSONTextResult(map[string]any{"message": renderErr.Error()}, true), nil
						}
						taskTier := strings.ToLower(strings.TrimSpace(task.Tier))
						if taskTier == "" {
							taskTier = strings.ToLower(strings.TrimSpace(info.Tier))
						}
						title := strings.TrimSpace(task.Title)
						if title == "" {
							title = strings.TrimSpace(task.ID)
						}
						if title == "" {
							title = "subagent"
						}
						run, spawnErr := subagentFlowSpawn(runtime, waitCtx, gateway.SpawnRequest{
							WorkspaceID:     workspaceID,
							Title:           title,
							Prompt:          renderedPrompt,
							Agent:           agentName,
							ParentRunID:     parentRunID,
							RootRunID:       rootRunID,
							ParentSessionID: strings.TrimSpace(meta.SessionID),
							Depth:           nextDepth,
							SessionKind:     "subagent",
							SessionHidden:   true,
							FlowID:          flowID,
							StepID:          stepID,
							Tier:            taskTier,
						})
						if spawnErr != nil {
							subagentFlowCancel(runtime, workspaceID, spawnedRuns)
							return JSONTextResult(map[string]any{"message": spawnErr.Error()}, true), nil
						}
						zlog.Debug().
							Str("flow_id", flowID).
							Str("step_id", stepID).
							Str("task_id", strings.TrimSpace(task.ID)).
							Str("run_id", run.ID).
							Str("tier", taskTier).
							Msg("subagent orchestration task spawned")
						spawnedRuns = append(spawnedRuns, run)
						pending = append(pending, pendingTask{
							id:        strings.TrimSpace(task.ID),
							title:     title,
							tier:      taskTier,
							dependsOn: append([]string(nil), task.DependsOn...),
							run:       run,
						})
					}
					for _, task := range pending {
						final, waitErr := subagentFlowWait(runtime, waitCtx, task.run.ID)
						if waitErr != nil {
							subagentFlowCancel(runtime, workspaceID, spawnedRuns)
							return JSONTextResult(map[string]any{"message": fmt.Sprintf("wait subagent %s failed: %v", task.run.ID, waitErr)}, true), nil
						}
						taskOut := buildSubagentTaskOutput(task.id, task.title, task.dependsOn, final)
						out.Tasks = append(out.Tasks, taskOut)
						completed[task.id] = subagentCompletedTask{
							Response: taskOut.Response,
							Summary:  taskOut.Summary,
							Error:    taskOut.Error,
						}
						if final.Status != gateway.RunStatusCompleted {
							hadFailure = true
							out.Status = "failed"
							out.FailedTasks++
						}
					}
				} else {
					for _, task := range step.Tasks {
						renderedPrompt, renderErr := renderSubagentFlowPrompt(task.Prompt, completed)
						if renderErr != nil {
							return JSONTextResult(map[string]any{"message": renderErr.Error()}, true), nil
						}
						taskTier := strings.ToLower(strings.TrimSpace(task.Tier))
						if taskTier == "" {
							taskTier = strings.ToLower(strings.TrimSpace(info.Tier))
						}
						title := strings.TrimSpace(task.Title)
						if title == "" {
							title = strings.TrimSpace(task.ID)
						}
						if title == "" {
							title = "subagent"
						}
						run, spawnErr := subagentFlowSpawn(runtime, waitCtx, gateway.SpawnRequest{
							WorkspaceID:     workspaceID,
							Title:           title,
							Prompt:          renderedPrompt,
							Agent:           agentName,
							ParentRunID:     parentRunID,
							RootRunID:       rootRunID,
							ParentSessionID: strings.TrimSpace(meta.SessionID),
							Depth:           nextDepth,
							SessionKind:     "subagent",
							SessionHidden:   true,
							FlowID:          flowID,
							StepID:          stepID,
							Tier:            taskTier,
						})
						if spawnErr != nil {
							return JSONTextResult(map[string]any{"message": spawnErr.Error()}, true), nil
						}
						zlog.Debug().
							Str("flow_id", flowID).
							Str("step_id", stepID).
							Str("task_id", strings.TrimSpace(task.ID)).
							Str("run_id", run.ID).
							Str("tier", taskTier).
							Msg("subagent orchestration task spawned")
						final, waitErr := subagentFlowWait(runtime, waitCtx, run.ID)
						if waitErr != nil {
							_, _ = runtime.CancelByWorkspace(workspaceID, run.ID)
							return JSONTextResult(map[string]any{"message": fmt.Sprintf("wait subagent %s failed: %v", run.ID, waitErr)}, true), nil
						}
						taskOut := buildSubagentTaskOutput(strings.TrimSpace(task.ID), title, task.DependsOn, final)
						out.Tasks = append(out.Tasks, taskOut)
						completed[strings.TrimSpace(task.ID)] = subagentCompletedTask{
							Response: taskOut.Response,
							Summary:  taskOut.Summary,
							Error:    taskOut.Error,
						}
						if final.Status != gateway.RunStatusCompleted {
							hadFailure = true
							out.Status = "failed"
							out.FailedTasks++
							break
						}
					}
				}

				out.Summary = buildSubagentStepSummary(out)
				zlog.Debug().
					Str("flow_id", flowID).
					Str("step_id", stepID).
					Str("status", out.Status).
					Int("failed_tasks", out.FailedTasks).
					Msg("subagent orchestration step completed")
				steps = append(steps, out)
			}

			zlog.Debug().
				Str("flow_id", flowID).
				Str("agent", agentName).
				Int("step_count", len(steps)).
				Int("task_count", totalTasks).
				Bool("had_failure", hadFailure).
				Msg("subagent orchestration completed")

			return JSONTextResult(map[string]any{
				"flow_id":    flowID,
				"agent":      agentName,
				"step_count": len(steps),
				"task_count": totalTasks,
				"steps":      steps,
			}, hadFailure), nil
		},
	}
}

func resolveSubagentParentContext(runtime *gateway.Runtime, workspaceID string, parentRunID string) (string, string, int, error) {
	trimmedParent := strings.TrimSpace(parentRunID)
	rootRunID := ""
	nextDepth := 1
	if trimmedParent == "" {
		return "", "", nextDepth, nil
	}
	parentRun, found := runtime.GetByWorkspace(workspaceID, trimmedParent)
	if !found {
		return "", "", 0, fmt.Errorf("parent run not found: %s", trimmedParent)
	}
	rootRunID = strings.TrimSpace(parentRun.RootRunID)
	if rootRunID == "" {
		rootRunID = strings.TrimSpace(parentRun.ID)
	}
	nextDepth = parentRun.Depth + 1
	return trimmedParent, rootRunID, nextDepth, nil
}

func validateSubagentFlow(steps []subagentFlowStepInput, maxThreads int) error {
	seen := map[string]struct{}{}
	for stepIndex, step := range steps {
		mode := strings.ToLower(strings.TrimSpace(step.Mode))
		if mode != "parallel" && mode != "sequential" {
			return fmt.Errorf("step %d mode must be one of: parallel, sequential", stepIndex+1)
		}
		if len(step.Tasks) == 0 {
			return fmt.Errorf("step %d must contain at least one task", stepIndex+1)
		}
		if mode == "parallel" && maxThreads > 0 && len(step.Tasks) > maxThreads {
			return fmt.Errorf("step %d requested %d tasks exceeds gateway_subagents_max_threads=%d", stepIndex+1, len(step.Tasks), maxThreads)
		}
		stepSeen := map[string]struct{}{}
		for taskIndex, task := range step.Tasks {
			taskID := strings.TrimSpace(task.ID)
			if taskID == "" {
				return fmt.Errorf("step %d task %d id is required", stepIndex+1, taskIndex+1)
			}
			if _, exists := seen[taskID]; exists {
				return fmt.Errorf("duplicate subagent task id: %s", taskID)
			}
			for _, dep := range task.DependsOn {
				depID := strings.TrimSpace(dep)
				if depID == "" {
					continue
				}
				if _, ok := stepSeen[depID]; ok && mode == "parallel" {
					return fmt.Errorf("parallel step %d task %s cannot depend on same-step task %s", stepIndex+1, taskID, depID)
				}
				if _, ok := stepSeen[depID]; !ok {
					if _, ok := seen[depID]; !ok {
						return fmt.Errorf("step %d task %s depends on unknown or future task %s", stepIndex+1, taskID, depID)
					}
				}
			}
			if strings.TrimSpace(task.Prompt) == "" {
				return fmt.Errorf("step %d task %s prompt is required", stepIndex+1, taskID)
			}
			stepSeen[taskID] = struct{}{}
		}
		for taskID := range stepSeen {
			seen[taskID] = struct{}{}
		}
	}
	return nil
}

func renderSubagentFlowPrompt(prompt string, completed map[string]subagentCompletedTask) (string, error) {
	var renderErr error
	rendered := orchestrationTaskPlaceholder.ReplaceAllStringFunc(prompt, func(match string) string {
		if renderErr != nil {
			return ""
		}
		parts := orchestrationTaskPlaceholder.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		taskID := strings.TrimSpace(parts[1])
		field := strings.TrimSpace(parts[2])
		item, ok := completed[taskID]
		if !ok {
			renderErr = fmt.Errorf("subagent flow placeholder references incomplete task: %s", taskID)
			return ""
		}
		switch field {
		case "summary":
			return item.Summary
		case "response":
			return item.Response
		case "error":
			return item.Error
		default:
			return ""
		}
	})
	if renderErr != nil {
		return "", renderErr
	}
	return strings.TrimSpace(rendered), nil
}

func buildSubagentTaskOutput(taskID, title string, dependsOn []string, final gateway.Run) subagentTaskOutput {
	summary := trimSubagentSummary(final.Response, 220)
	if summary == "" {
		summary = trimSubagentSummary(final.Error, 220)
	}
	response := strings.TrimSpace(final.Response)
	if len(response) > 2000 {
		response = response[:1997] + "..."
	}
	return subagentTaskOutput{
		ID:              strings.TrimSpace(taskID),
		Title:           strings.TrimSpace(title),
		RunID:           final.ID,
		SessionID:       final.SessionID,
		Agent:           final.Agent,
		Status:          string(final.Status),
		Tier:            final.Tier,
		DependsOn:       sanitizeStringList(dependsOn),
		ParentRunID:     final.ParentRunID,
		ParentSessionID: final.ParentSessionID,
		Depth:           final.Depth,
		Summary:         summary,
		Response:        response,
		Error:           strings.TrimSpace(final.Error),
	}
}

func buildSubagentStepSummary(step subagentStepOutput) string {
	parts := make([]string, 0, len(step.Tasks))
	for _, task := range step.Tasks {
		if task.Summary == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", task.ID, task.Summary))
	}
	return strings.Join(parts, " | ")
}
