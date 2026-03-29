package project

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PlanTasks calls the LLM to generate board tasks from the project brief and state.
// Returns a list of tasks with status="todo" ready for dispatch.
func (o *Orchestrator) PlanTasks(ctx context.Context, projectID string) ([]BoardTask, error) {
	if o == nil || o.store == nil || o.runner == nil {
		return nil, fmt.Errorf("orchestrator not configured")
	}

	prompt, err := o.buildPlanningPrompt(projectID)
	if err != nil {
		return nil, fmt.Errorf("build planning prompt: %w", err)
	}

	run, err := o.runner.Start(ctx, TaskRunRequest{
		ProjectID:  projectID,
		TaskID:     "planning",
		Title:      "Generate project backlog",
		Prompt:     prompt,
		Agent:      "",
		Role:       "planner",
		WorkerKind: "main",
	})
	if err != nil {
		return nil, fmt.Errorf("start planning run: %w", err)
	}

	result, err := o.runner.Wait(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("wait planning run: %w", err)
	}
	if result.Status == TaskRunStatusFailed {
		return nil, fmt.Errorf("planning run failed: %s", result.Error)
	}

	tasks, err := parsePlanningResponse(result.Response)
	if err != nil {
		return nil, fmt.Errorf("parse planning response: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("planning produced no tasks")
	}
	return tasks, nil
}

func (o *Orchestrator) buildPlanningPrompt(projectID string) (string, error) {
	var parts []string

	parts = append(parts, "You are a project planner. Based on the project information below, generate a list of concrete tasks for the next phase.")
	parts = append(parts, "")

	// Project brief
	brief, err := o.store.GetBrief(projectID)
	if err == nil {
		parts = append(parts, "## Project Brief")
		if brief.Title != "" {
			parts = append(parts, fmt.Sprintf("Title: %s", brief.Title))
		}
		if brief.Goal != "" {
			parts = append(parts, fmt.Sprintf("Goal: %s", brief.Goal))
		}
		if brief.Premise != "" {
			parts = append(parts, fmt.Sprintf("Premise: %s", brief.Premise))
		}
		if brief.Kind != "" {
			parts = append(parts, fmt.Sprintf("Kind: %s", brief.Kind))
		}
		parts = append(parts, "")
	}

	// Project state
	state, err := o.store.GetState(projectID)
	if err == nil {
		parts = append(parts, "## Current State")
		if state.Phase != "" {
			parts = append(parts, fmt.Sprintf("Phase: %s", state.Phase))
		}
		if state.NextAction != "" {
			parts = append(parts, fmt.Sprintf("Next action: %s", state.NextAction))
		}
		if state.LastRunSummary != "" {
			parts = append(parts, fmt.Sprintf("Last run: %s", state.LastRunSummary))
		}
		if len(state.RemainingTasks) > 0 {
			parts = append(parts, fmt.Sprintf("Remaining tasks: %s", strings.Join(state.RemainingTasks, ", ")))
		}
		parts = append(parts, "")
	}

	parts = append(parts, "## Instructions")
	parts = append(parts, "Generate 1-5 concrete tasks for the next phase. Each task should be a small, actionable unit of work.")
	parts = append(parts, "")
	parts = append(parts, "Respond with a JSON array of task objects. Each task must have:")
	parts = append(parts, `- "id": unique short identifier (e.g., "task-1")`)
	parts = append(parts, `- "title": clear description of what to do`)
	parts = append(parts, `- "status": always "todo"`)
	parts = append(parts, "")
	parts = append(parts, "Example response:")
	parts = append(parts, "```json")
	parts = append(parts, `[{"id":"task-1","title":"Write chapter outline","status":"todo"},{"id":"task-2","title":"Draft opening scene","status":"todo"}]`)
	parts = append(parts, "```")
	parts = append(parts, "")
	parts = append(parts, "Respond with ONLY the JSON array, no other text.")

	return strings.Join(parts, "\n"), nil
}

func parsePlanningResponse(response string) ([]BoardTask, error) {
	response = strings.TrimSpace(response)

	// Extract JSON from markdown code block if present
	if idx := strings.Index(response, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(response[start:], "```")
		if end >= 0 {
			response = strings.TrimSpace(response[start : start+end])
		}
	} else if idx := strings.Index(response, "```"); idx >= 0 {
		start := idx + len("```")
		end := strings.Index(response[start:], "```")
		if end >= 0 {
			response = strings.TrimSpace(response[start : start+end])
		}
	}

	// Find JSON array bounds
	arrStart := strings.Index(response, "[")
	arrEnd := strings.LastIndex(response, "]")
	if arrStart >= 0 && arrEnd > arrStart {
		response = response[arrStart : arrEnd+1]
	}

	var tasks []BoardTask
	if err := json.Unmarshal([]byte(response), &tasks); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate and normalize
	valid := make([]BoardTask, 0, len(tasks))
	for _, t := range tasks {
		t.ID = strings.TrimSpace(t.ID)
		t.Title = strings.TrimSpace(t.Title)
		if t.ID == "" || t.Title == "" {
			continue
		}
		t.Status = "todo"
		valid = append(valid, t)
	}
	return valid, nil
}
