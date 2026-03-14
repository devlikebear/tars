package project

import (
	"fmt"
	"strings"
)

const (
	WorkerKindCodexCLI   = "codex-cli"
	WorkerKindClaudeCode = "claude-code"
)

type WorkerProfile struct {
	Kind         string
	ExecutorName string
	Command      string
	Args         []string
	Description  string
}

func BuiltinWorkerProfiles() map[string]WorkerProfile {
	return map[string]WorkerProfile{
		WorkerKindCodexCLI: {
			Kind:         WorkerKindCodexCLI,
			ExecutorName: WorkerKindCodexCLI,
			Command:      "codex",
			Args:         []string{"exec", "--skip-git-repo-check", "--full-auto", "-"},
			Description:  "Codex CLI non-interactive worker for implementation tasks",
		},
		WorkerKindClaudeCode: {
			Kind:         WorkerKindClaudeCode,
			ExecutorName: WorkerKindClaudeCode,
			Command:      "claude",
			Args:         []string{"-p", "--output-format", "text", "--permission-mode", "auto"},
			Description:  "Claude Code CLI non-interactive worker for review and coordination tasks",
		},
	}
}

func ResolveWorkerProfile(task BoardTask) (WorkerProfile, error) {
	profiles := BuiltinWorkerProfiles()
	workerKind := strings.ToLower(strings.TrimSpace(task.WorkerKind))
	if workerKind == "" {
		switch strings.ToLower(strings.TrimSpace(task.Role)) {
		case "reviewer", "review", "pm", "manager":
			workerKind = WorkerKindClaudeCode
		default:
			workerKind = WorkerKindCodexCLI
		}
	}
	profile, ok := profiles[workerKind]
	if !ok {
		return WorkerProfile{}, fmt.Errorf("unknown worker kind %q", workerKind)
	}
	return profile, nil
}

func BuildTaskPrompt(task BoardTask, projectID string, profile WorkerProfile) string {
	var builder strings.Builder
	workerKind := firstNonEmpty(strings.TrimSpace(task.WorkerKind), profile.Kind)
	role := firstNonEmpty(strings.TrimSpace(task.Role), "developer")

	builder.WriteString("You are a TARS project worker.\n")
	builder.WriteString("Follow repository instructions from AGENTS.md and use relevant local skills when they apply.\n")
	builder.WriteString("Use TDD: write or update failing tests first, implement the smallest fix, rerun verification, and summarize results.\n")
	builder.WriteString("\nProject ID: ")
	builder.WriteString(strings.TrimSpace(projectID))
	builder.WriteString("\nTask ID: ")
	builder.WriteString(strings.TrimSpace(task.ID))
	builder.WriteString("\nTask: ")
	builder.WriteString(strings.TrimSpace(task.Title))
	builder.WriteString("\nRole: ")
	builder.WriteString(role)
	builder.WriteString("\nworker_kind: ")
	builder.WriteString(workerKind)
	builder.WriteString("\nexecutor: ")
	builder.WriteString(strings.TrimSpace(profile.ExecutorName))
	if assignee := strings.TrimSpace(task.Assignee); assignee != "" {
		builder.WriteString("\nAssignee: ")
		builder.WriteString(assignee)
	}
	if testCmd := strings.TrimSpace(task.TestCommand); testCmd != "" {
		builder.WriteString("\nTest command: ")
		builder.WriteString(testCmd)
	}
	if buildCmd := strings.TrimSpace(task.BuildCommand); buildCmd != "" {
		builder.WriteString("\nBuild command: ")
		builder.WriteString(buildCmd)
	}

	builder.WriteString("\n\n")
	switch strings.ToLower(role) {
	case "reviewer", "review":
		builder.WriteString("Review the task outcome, rerun the relevant verification when needed, and decide whether the work should be approved.\n")
		builder.WriteString("Return the final result using this exact format:\n")
		builder.WriteString("<task-report>\n")
		builder.WriteString("status: approved|rejected|blocked\n")
		builder.WriteString("summary: <short result summary>\n")
		builder.WriteString("tests: <what ran and what passed/failed>\n")
		builder.WriteString("build: <what ran and what passed/failed>\n")
		builder.WriteString("issue: <linked issue url or id>\n")
		builder.WriteString("branch: <branch name used for the task>\n")
		builder.WriteString("pr: <pull request url or id>\n")
		builder.WriteString("notes: <review findings or follow-up>\n")
		builder.WriteString("</task-report>")
	default:
		builder.WriteString("Implement the task and stop at a review-ready state when review is required.\n")
		builder.WriteString("Return the final result using this exact format:\n")
		builder.WriteString("<task-report>\n")
		builder.WriteString("status: completed|blocked|needs_review\n")
		builder.WriteString("summary: <short result summary>\n")
		builder.WriteString("tests: <what ran and what passed/failed>\n")
		builder.WriteString("build: <what ran and what passed/failed>\n")
		builder.WriteString("issue: <linked issue url or id>\n")
		builder.WriteString("branch: <branch name used for the task>\n")
		builder.WriteString("pr: <pull request url or id>\n")
		builder.WriteString("notes: <important details, blockers, or follow-up>\n")
		builder.WriteString("</task-report>")
	}
	return builder.String()
}
