package project

import (
	"fmt"
	"strings"
)

const (
	WorkerKindDefault    = "default"
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

type WorkflowExecutionPolicy struct {
	Profile           string
	DefaultWorkerKind string
	ReviewWorkerKind  string
	RequireGitHubAuth bool
	RequireTests      bool
	RequireBuild      bool
	RequireIssue      bool
	RequireBranch     bool
	RequirePR         bool
}

func BuiltinWorkerProfiles() map[string]WorkerProfile {
	return map[string]WorkerProfile{
		WorkerKindDefault: {
			Kind:         WorkerKindDefault,
			ExecutorName: WorkerKindDefault,
			Command:      "",
			Args:         nil,
			Description:  "Runtime default gateway agent used when a dedicated worker alias is unavailable",
		},
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

func ResolveWorkflowExecutionPolicy(project Project) WorkflowExecutionPolicy {
	profile := effectiveWorkflowProfile(project)
	policy := WorkflowExecutionPolicy{
		Profile:           profile,
		DefaultWorkerKind: WorkerKindDefault,
		ReviewWorkerKind:  WorkerKindDefault,
	}

	if profile == "software-dev" {
		policy.DefaultWorkerKind = WorkerKindCodexCLI
		policy.ReviewWorkerKind = WorkerKindClaudeCode
		policy.RequireGitHubAuth = true
		policy.RequireTests = true
		policy.RequireBuild = true
		policy.RequireIssue = true
		policy.RequireBranch = true
		policy.RequirePR = true
	}

	for _, rule := range project.WorkflowRules {
		name := strings.ToLower(strings.TrimSpace(rule.Name))
		switch name {
		case "require_github_auth":
			policy.RequireGitHubAuth = workflowRuleEnabled(rule.Params, true)
		case "require_tests":
			policy.RequireTests = workflowRuleEnabled(rule.Params, true)
		case "require_build":
			policy.RequireBuild = workflowRuleEnabled(rule.Params, true)
		case "require_issue":
			policy.RequireIssue = workflowRuleEnabled(rule.Params, true)
		case "require_branch":
			policy.RequireBranch = workflowRuleEnabled(rule.Params, true)
		case "require_pr":
			policy.RequirePR = workflowRuleEnabled(rule.Params, true)
		case "default_worker":
			if kind := strings.ToLower(strings.TrimSpace(rule.Params["kind"])); kind != "" {
				policy.DefaultWorkerKind = kind
			}
		case "review_worker":
			if kind := strings.ToLower(strings.TrimSpace(rule.Params["kind"])); kind != "" {
				policy.ReviewWorkerKind = kind
			}
		case "worker_kind":
			kind := strings.ToLower(strings.TrimSpace(rule.Params["kind"]))
			role := strings.ToLower(strings.TrimSpace(rule.Params["role"]))
			if kind == "" {
				continue
			}
			switch role {
			case "", "developer", "implementation":
				policy.DefaultWorkerKind = kind
			case "reviewer", "review":
				policy.ReviewWorkerKind = kind
			case "all":
				policy.DefaultWorkerKind = kind
				policy.ReviewWorkerKind = kind
			}
		}
	}
	return policy
}

func effectiveWorkflowProfile(project Project) string {
	if profile := normalizeWorkflowProfile(project.WorkflowProfile); profile != "" {
		return profile
	}
	// Backward-compatible default until all existing projects set workflow_profile explicitly.
	return "software-dev"
}

func workflowRuleEnabled(params map[string]string, defaultValue bool) bool {
	if len(params) == 0 {
		return defaultValue
	}
	raw := strings.ToLower(strings.TrimSpace(params["enabled"]))
	switch raw {
	case "false", "0", "no", "off":
		return false
	case "true", "1", "yes", "on":
		return true
	default:
		return defaultValue
	}
}

func ResolveWorkerProfile(task BoardTask) (WorkerProfile, error) {
	return ResolveWorkerProfileForProject(Project{}, task)
}

func ResolveWorkerProfileForProject(project Project, task BoardTask) (WorkerProfile, error) {
	profiles := BuiltinWorkerProfiles()
	workerKind := strings.ToLower(strings.TrimSpace(task.WorkerKind))
	if workerKind == "" {
		policy := ResolveWorkflowExecutionPolicy(project)
		switch strings.ToLower(strings.TrimSpace(task.Role)) {
		case "reviewer", "review", "pm", "manager":
			workerKind = policy.ReviewWorkerKind
		default:
			workerKind = policy.DefaultWorkerKind
		}
	}
	profile, ok := profiles[workerKind]
	if !ok {
		return WorkerProfile{}, fmt.Errorf("unknown worker kind %q", workerKind)
	}
	return profile, nil
}

// maxSkillContentChars is the per-skill character cap when injecting skill
// content into task prompts.
const maxSkillContentChars = 4000

// maxSkillsInPrompt is the upper bound on skills injected into a single prompt.
const maxSkillsInPrompt = 5

// BuildTaskPrompt assembles the initial instruction for a task worker.
// Optional skills are appended as a ## Project Skills section.
func BuildTaskPrompt(task BoardTask, projectID string, profile WorkerProfile, skills ...SkillContent) string {
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

	writeSkillsSection(&builder, skills)

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

func writeSkillsSection(builder *strings.Builder, skills []SkillContent) {
	if len(skills) == 0 {
		return
	}
	cap := len(skills)
	if cap > maxSkillsInPrompt {
		cap = maxSkillsInPrompt
	}
	builder.WriteString("\n\n## Project Skills\n")
	builder.WriteString("The following skills define how to perform domain-specific operations in this project. Follow these instructions strictly.\n")
	for _, sk := range skills[:cap] {
		name := strings.TrimSpace(sk.Name)
		content := strings.TrimSpace(sk.Content)
		if name == "" || content == "" {
			continue
		}
		if len(content) > maxSkillContentChars {
			content = content[:maxSkillContentChars] + "\n…(truncated)"
		}
		builder.WriteString("\n### ")
		builder.WriteString(name)
		builder.WriteString("\n")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
}
