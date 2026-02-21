package tarsapp

import (
	"context"
	"strings"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

func newBaseToolRegistry(workspaceDir string) *tool.Registry {
	return newBaseToolRegistryWithProcess(workspaceDir, nil)
}

func newBaseToolRegistryWithProcess(workspaceDir string, processManager *tool.ProcessManager) *tool.Registry {
	registry := tool.NewRegistry()
	registry.Register(tool.NewMemorySearchTool(workspaceDir))
	registry.Register(tool.NewMemoryGetTool(workspaceDir))
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
		tools := registry.Schemas()
		allowed := normalizeAllowedToolsForRegistry(allowedTools, registry)
		if len(allowed) > 0 {
			tools = registry.SchemasForNames(allowed)
		}
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
