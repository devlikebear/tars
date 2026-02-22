package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/devlikebear/tarsncase/internal/usage"
	"github.com/rs/zerolog"
)

func newBaseToolRegistry(workspaceDir string) *tool.Registry {
	return newBaseToolRegistryWithProcess(workspaceDir, nil)
}

func newBaseToolRegistryWithProcess(workspaceDir string, processManager *tool.ProcessManager) *tool.Registry {
	registry := tool.NewRegistry()
	registry.Register(tool.NewMemorySearchTool(workspaceDir))
	registry.Register(tool.NewMemoryGetTool(workspaceDir))
	registry.Register(tool.NewMemorySaveTool(workspaceDir, nil))
	projectStore := project.NewStore(workspaceDir, nil)
	registry.Register(tool.NewProjectCreateTool(projectStore))
	registry.Register(tool.NewProjectListTool(projectStore))
	registry.Register(tool.NewProjectGetTool(projectStore))
	registry.Register(tool.NewProjectUpdateTool(projectStore))
	registry.Register(tool.NewProjectDeleteTool(projectStore))
	if usageTracker, err := usage.NewTracker(workspaceDir, usage.TrackerOptions{}); err == nil {
		registry.Register(tool.NewUsageReportTool(usageTracker))
	}
	registry.Register(tool.NewReadTool(workspaceDir))
	registry.Register(tool.NewReadFileTool(workspaceDir))
	registry.Register(tool.NewWriteTool(workspaceDir))
	registry.Register(tool.NewWriteFileTool(workspaceDir))
	registry.Register(tool.NewEditTool(workspaceDir))
	registry.Register(tool.NewEditFileTool(workspaceDir))
	registry.Register(tool.NewListDirTool(workspaceDir))
	registry.Register(tool.NewGlobTool(workspaceDir))
	if processManager != nil {
		registry.Register(tool.NewProcessTool(processManager))
		registry.Register(tool.NewExecToolWithManager(workspaceDir, processManager))
	} else {
		registry.Register(tool.NewExecTool(workspaceDir))
	}
	return registry
}

func newAgentAskFunc(workspaceDir string, client llm.Client, maxIterations int, logger zerolog.Logger) heartbeat.AskFunc {
	runner := newAgentPromptRunner(workspaceDir, client, maxIterations, logger)
	if runner == nil {
		return nil
	}
	return func(ctx context.Context, promptText string) (string, error) {
		return runner(ctx, "heartbeat", promptText)
	}
}

func newAgentPromptRunner(
	workspaceDir string,
	client llm.Client,
	maxIterations int,
	logger zerolog.Logger,
) func(ctx context.Context, runLabel string, promptText string) (string, error) {
	runnerWithTools := newAgentPromptRunnerWithTools(workspaceDir, client, maxIterations, logger)
	if runnerWithTools == nil {
		return nil
	}
	return func(ctx context.Context, runLabel string, promptText string) (string, error) {
		return runnerWithTools(ctx, runLabel, promptText, nil)
	}
}

func newAgentPromptRunnerWithTools(
	workspaceDir string,
	client llm.Client,
	maxIterations int,
	logger zerolog.Logger,
	extraTools ...tool.Tool,
) gatewayPromptRunner {
	if client == nil {
		return nil
	}
	maxIters := resolveAgentMaxIterations(maxIterations)
	return func(ctx context.Context, runLabel string, promptText string, allowedTools []string) (string, error) {
		label := strings.TrimSpace(runLabel)
		if label == "" {
			label = "agent"
		}
		workspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(ctx))
		targetWorkspaceDir := resolveWorkspaceDir(workspaceDir, workspaceID)
		if err := memory.EnsureWorkspace(targetWorkspaceDir); err != nil {
			return "", err
		}

		systemPrompt := prompt.Build(prompt.BuildOptions{WorkspaceDir: targetWorkspaceDir})
		systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
		registry := newBaseToolRegistry(targetWorkspaceDir)
		for _, extra := range extraTools {
			if strings.TrimSpace(extra.Name) == "" {
				continue
			}
			registry.Register(extra)
		}
		tools := registry.Schemas()
		allowed := normalizeAllowedToolsForRegistry(allowedTools, registry)
		if len(allowed) > 0 {
			tools = registry.SchemasForNames(allowed)
		}
		meta := usage.CallMeta{Source: "agent_run"}
		lowerLabel := strings.ToLower(label)
		switch {
		case strings.HasPrefix(lowerLabel, "cron"):
			meta.Source = "cron"
		case strings.HasPrefix(lowerLabel, "heartbeat"):
			meta.Source = "heartbeat"
		}
		if idx := strings.Index(label, ":"); idx >= 0 && idx+1 < len(label) {
			meta.RunID = strings.TrimSpace(label[idx+1:])
		}
		ctx = usage.WithCallMeta(ctx, meta)
		loop := setupAgentLoop(client, registry, label, 0, logger, func(string, string, string, string, string, string) {})
		resp, err := loop.Run(ctx, []llm.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: promptText},
		}, agent.RunOptions{
			MaxIterations: maxIters,
			Tools:         tools,
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
}

func normalizeAllowedToolsForRegistry(raw []string, registry *tool.Registry) []string {
	if registry == nil || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		name := tool.CanonicalToolName(item)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		if _, ok := registry.Get(name); !ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
