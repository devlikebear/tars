package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/heartbeat"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/prompt"
	"github.com/devlikebear/tars/internal/research"
	"github.com/devlikebear/tars/internal/schedule"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type agentPromptTelemetry struct {
	SystemPromptTokens int
	UserPromptTokens   int
	ToolCount          int
}

type agentPromptTelemetryKey struct{}

func withAgentPromptTelemetry(ctx context.Context, telemetry *agentPromptTelemetry) context.Context {
	if telemetry == nil {
		return ctx
	}
	return context.WithValue(ctx, agentPromptTelemetryKey{}, telemetry)
}

func agentPromptTelemetryFromContext(ctx context.Context) *agentPromptTelemetry {
	if ctx == nil {
		return nil
	}
	telemetry, _ := ctx.Value(agentPromptTelemetryKey{}).(*agentPromptTelemetry)
	return telemetry
}

func newBaseToolRegistry(workspaceDir string) *tool.Registry {
	return newBaseToolRegistryWithProcess(workspaceDir, nil)
}

func newBaseToolRegistryWithProcess(workspaceDir string, processManager *tool.ProcessManager, semanticCfg ...memory.SemanticConfig) *tool.Registry {
	registry := tool.NewRegistry()
	memService := buildSemanticMemoryService(workspaceDir, firstSemanticConfig(semanticCfg...))
	registry.Register(tool.NewMemorySearchTool(workspaceDir, memService))
	registry.Register(tool.NewMemoryGetTool(workspaceDir))
	registry.Register(tool.NewMemorySaveTool(workspaceDir, memService, nil))
	registry.Register(tool.NewMemoryKBListTool(workspaceDir))
	registry.Register(tool.NewMemoryKBGetTool(workspaceDir))
	registry.Register(tool.NewMemoryKBUpsertTool(workspaceDir, memService))
	registry.Register(tool.NewMemoryKBDeleteTool(workspaceDir, memService))
	projectStore := project.NewStore(workspaceDir, nil)
	registry.Register(tool.NewProjectCreateTool(projectStore))
	registry.Register(tool.NewProjectListTool(projectStore))
	registry.Register(tool.NewProjectGetTool(projectStore))
	registry.Register(tool.NewProjectUpdateTool(projectStore))
	registry.Register(tool.NewProjectDeleteTool(projectStore))
	registry.Register(tool.NewProjectBoardGetTool(projectStore))
	registry.Register(tool.NewProjectBoardUpdateTool(projectStore))
	registry.Register(tool.NewProjectActivityGetTool(projectStore))
	registry.Register(tool.NewProjectActivityAppendTool(projectStore))
	registry.Register(tool.NewProjectDispatchTool(projectStore, nil, nil))
	registry.Register(tool.NewProjectBriefGetTool(projectStore))
	registry.Register(tool.NewProjectBriefUpdateTool(projectStore))
	registry.Register(tool.NewProjectBriefFinalizeTool(projectStore, session.NewStore(workspaceDir)))
	registry.Register(tool.NewProjectStateGetTool(projectStore))
	registry.Register(tool.NewProjectStateUpdateTool(projectStore))
	opsManager := ops.NewManager(workspaceDir, ops.Options{})
	registry.Register(tool.NewOpsStatusTool(opsManager))
	registry.Register(tool.NewOpsCleanupPlanTool(opsManager))
	registry.Register(tool.NewOpsCleanupApplyTool(opsManager))
	scheduleStore := schedule.NewStore(workspaceDir, cron.NewStore(workspaceDir), schedule.Options{})
	registry.Register(tool.NewScheduleCreateTool(scheduleStore))
	registry.Register(tool.NewScheduleListTool(scheduleStore))
	registry.Register(tool.NewScheduleUpdateTool(scheduleStore))
	registry.Register(tool.NewScheduleDeleteTool(scheduleStore))
	registry.Register(tool.NewScheduleCompleteTool(scheduleStore))
	registry.Register(tool.NewResearchReportTool(research.NewService(workspaceDir, research.Options{})))
	if usageTracker, err := usage.NewTracker(workspaceDir, usage.TrackerOptions{}); err == nil {
		registry.Register(tool.NewUsageReportTool(usageTracker))
	}
	registry.Register(tool.NewReadTool(workspaceDir))
	registry.Register(tool.NewReadFileTool(workspaceDir))
	registry.Register(tool.NewWorkspaceSyspromptGetTool(workspaceDir))
	registry.Register(tool.NewWorkspaceSyspromptSetTool(workspaceDir))
	registry.Register(tool.NewAgentSyspromptGetTool(workspaceDir))
	registry.Register(tool.NewAgentSyspromptSetTool(workspaceDir))
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

func newAgentAskFunc(workspaceDir string, client llm.Client, maxIterations int, logger zerolog.Logger, semanticCfg ...memory.SemanticConfig) heartbeat.AskFunc {
	runner := newAgentPromptRunner(workspaceDir, client, maxIterations, logger, semanticCfg...)
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
	semanticCfg ...memory.SemanticConfig,
) func(ctx context.Context, runLabel string, promptText string) (string, error) {
	runnerWithTools := newAgentPromptRunnerWithToolsAndMemory(
		workspaceDir,
		client,
		maxIterations,
		logger,
		firstSemanticConfig(semanticCfg...),
	)
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
	return newAgentPromptRunnerWithToolsAndMemory(workspaceDir, client, maxIterations, logger, memory.SemanticConfig{}, extraTools...)
}

func newAgentPromptRunnerWithToolsAndMemory(
	workspaceDir string,
	client llm.Client,
	maxIterations int,
	logger zerolog.Logger,
	semanticCfg memory.SemanticConfig,
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

		profile := agentPromptProfileForLabel(label)
		systemPrompt := buildAgentSystemPrompt(targetWorkspaceDir, profile, semanticCfg)
		baseRegistry := newBaseToolRegistryWithProcess(targetWorkspaceDir, nil, semanticCfg)
		for _, extra := range extraTools {
			if strings.TrimSpace(extra.Name) == "" {
				continue
			}
			baseRegistry.Register(extra)
		}
		allowed := normalizeAllowedToolsForRegistry(allowedTools, baseRegistry)
		registry := filterToolRegistryForAgentProfile(baseRegistry, profile, allowed)
		for _, extra := range extraTools {
			if strings.TrimSpace(extra.Name) == "" {
				continue
			}
			registry.Register(extra)
		}
		tools := registry.Schemas()
		if len(allowed) > 0 {
			tools = registry.SchemasForNames(allowed)
		}
		if telemetry := agentPromptTelemetryFromContext(ctx); telemetry != nil {
			telemetry.SystemPromptTokens = promptTokenEstimate(systemPrompt)
			telemetry.UserPromptTokens = promptTokenEstimate(promptText)
			telemetry.ToolCount = len(tools)
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
		loop, _ := setupAgentLoop(client, registry, label, 0, logger, func(string, string, string, string, string, string) {})
		resp, err := loop.Run(ctx, []llm.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: promptText},
		}, agent.RunOptions{
			MaxIterations: resolveAgentMaxIterations(profile.maxIterations(maxIters)),
			Tools:         tools,
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
}

type agentPromptProfile struct {
	minimalPrompt  bool
	includeMemory  bool
	allowedToolIDs []string
	maxIters       int
}

func (p agentPromptProfile) maxIterations(fallback int) int {
	if p.maxIters > 0 {
		return p.maxIters
	}
	return fallback
}

func agentPromptProfileForLabel(label string) agentPromptProfile {
	lower := strings.ToLower(strings.TrimSpace(label))
	if strings.HasPrefix(lower, "cron") {
		return agentPromptProfile{
			minimalPrompt: true,
			includeMemory: false,
			allowedToolIDs: []string{
				"read_file", "write_file", "edit_file", "list_dir", "glob",
				"project_get", "project_update", "project_state_get", "project_state_update",
				"research_report",
			},
			maxIters: 4,
		}
	}
	return agentPromptProfile{includeMemory: true}
}

func buildAgentSystemPrompt(workspaceDir string, profile agentPromptProfile, semanticCfg ...memory.SemanticConfig) string {
	if profile.minimalPrompt {
		return strings.TrimSpace(fmt.Sprintf(
			"You are TARS running an automated background job.\nCurrent time: %s\nKeep output minimal and action-oriented.\nNever echo tool calls, JSON arguments, or pseudo-tool syntax in your final answer.\nIf no durable project change is needed, return a short plain-text summary only.\nIf telegram_send is available and the prompt includes CRON_TELEGRAM_CONTEXT with a default paired chat, you may call telegram_send without chat_id to notify that paired Telegram chat.",
			time.Now().UTC().Format(time.RFC3339),
		)) + "\n"
	}
	systemPrompt := prompt.Build(prompt.BuildOptions{
		WorkspaceDir:   workspaceDir,
		MemorySearcher: buildSemanticMemoryService(workspaceDir, firstSemanticConfig(semanticCfg...)),
	})
	if profile.includeMemory {
		systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
	}
	return systemPrompt
}

func newToolRegistryForAgentProfile(workspaceDir string, profile agentPromptProfile, extraAllowed []string, semanticCfg ...memory.SemanticConfig) *tool.Registry {
	registry := newBaseToolRegistryWithProcess(workspaceDir, nil, semanticCfg...)
	return filterToolRegistryForAgentProfile(registry, profile, extraAllowed)
}

func filterToolRegistryForAgentProfile(registry *tool.Registry, profile agentPromptProfile, extraAllowed []string) *tool.Registry {
	if registry == nil {
		return nil
	}
	names := normalizeToolNames(append(append([]string(nil), profile.allowedToolIDs...), extraAllowed...))
	if len(names) == 0 {
		return registry
	}
	filtered := tool.NewRegistry()
	for _, name := range names {
		if tl, ok := registry.Get(name); ok {
			filtered.Register(tl)
		}
	}
	return filtered
}

func firstSemanticConfig(values ...memory.SemanticConfig) memory.SemanticConfig {
	if len(values) == 0 {
		return memory.SemanticConfig{}
	}
	return values[0]
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
