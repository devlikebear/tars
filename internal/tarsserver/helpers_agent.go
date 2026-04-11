package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/prompt"
	"github.com/devlikebear/tars/internal/research"
	"github.com/devlikebear/tars/internal/serverauth"
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
	return newBaseToolRegistryWithProcess(workspaceDir, tool.SingleDirPolicy(workspaceDir), nil)
}

func newBaseToolRegistryWithProcess(workspaceDir string, policy tool.PathPolicy, processManager *tool.ProcessManager, semanticCfg ...memory.SemanticConfig) *tool.Registry {
	registry := tool.NewRegistryWithScope(tool.RegistryScopeUser)
	memService := buildSemanticMemoryService(workspaceDir, firstSemanticConfig(semanticCfg...))

	// Memory & workspace aggregators
	registry.Register(tool.NewMemoryTool(workspaceDir, memService, nil))
	registry.Register(tool.NewKnowledgeTool(workspaceDir, memService))
	registry.Register(tool.NewWorkspaceTool(workspaceDir))

	// Standalone tools
	registry.Register(tool.NewResearchReportTool(research.NewService(workspaceDir, research.Options{})))
	if usageTracker, err := usage.NewTracker(workspaceDir, usage.TrackerOptions{}); err == nil {
		registry.Register(tool.NewUsageReportTool(usageTracker))
	}

	// File I/O (no aliases — canonical names only)
	registry.Register(tool.NewReadFileToolWithPolicy(policy))
	registry.Register(tool.NewWriteFileToolWithPolicy(policy))
	registry.Register(tool.NewEditFileToolWithPolicy(policy))
	registry.Register(tool.NewListDirToolWithPolicy(policy))
	registry.Register(tool.NewGlobToolWithPolicy(policy))

	// Exec / process
	if processManager != nil {
		registry.Register(tool.NewProcessTool(processManager))
		registry.Register(tool.NewExecToolWithPolicy(policy, processManager))
	} else {
		registry.Register(tool.NewExecToolWithPolicy(policy, nil))
	}
	return registry
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
		nil,
		maxIterations,
		logger,
		firstSemanticConfig(semanticCfg...),
	)
	if runnerWithTools == nil {
		return nil
	}
	return func(ctx context.Context, runLabel string, promptText string) (string, error) {
		return runnerWithTools(ctx, runLabel, promptText, nil, "")
	}
}

func newAgentPromptRunnerWithTools(
	workspaceDir string,
	client llm.Client,
	maxIterations int,
	logger zerolog.Logger,
	extraTools ...tool.Tool,
) gatewayPromptRunner {
	return newAgentPromptRunnerWithToolsAndMemory(workspaceDir, client, nil, maxIterations, logger, memory.SemanticConfig{}, extraTools...)
}

func newAgentPromptRunnerWithToolsAndMemory(
	workspaceDir string,
	client llm.Client,
	router llm.Router,
	maxIterations int,
	logger zerolog.Logger,
	semanticCfg memory.SemanticConfig,
	extraTools ...tool.Tool,
) gatewayPromptRunner {
	if client == nil && router == nil {
		return nil
	}
	maxIters := resolveAgentMaxIterations(maxIterations)
	return func(ctx context.Context, runLabel string, promptText string, allowedTools []string, tier string) (string, error) {
		label := strings.TrimSpace(runLabel)
		if label == "" {
			label = "agent"
		}
		workspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(ctx))
		targetWorkspaceDir := resolveWorkspaceDir(workspaceDir, workspaceID)
		if err := memory.EnsureWorkspace(targetWorkspaceDir); err != nil {
			return "", err
		}

		// Resolve the LLM client for this run. When a tier is set (from
		// the SpawnRequest) AND a router is available, select that tier's
		// client; otherwise fall back to the default chat client.
		runClient := client
		selection := llm.SelectionMetadata{}
		if router != nil {
			tierNorm := strings.ToLower(strings.TrimSpace(tier))
			if tierNorm != "" {
				if parsed, err := llm.ParseTier(tierNorm); err == nil {
					if c, resolution, err := router.ClientForTier(parsed); err == nil {
						runClient = c
						selection.Tier = resolution.Tier
						selection.Provider = resolution.Provider
						selection.Model = resolution.Model
						selection.Source = resolution.Source
					}
				}
			} else {
				if c, resolution, err := router.ClientFor(llm.RoleGatewayDefault); err == nil {
					runClient = c
					selection.Role = llm.RoleGatewayDefault
					selection.Tier = resolution.Tier
					selection.Provider = resolution.Provider
					selection.Model = resolution.Model
					selection.Source = resolution.Source
				}
			}
		}
		ctx = llm.WithSelectionMetadata(ctx, llm.SelectionMetadata{
			Role:      selection.Role,
			Tier:      selection.Tier,
			Provider:  selection.Provider,
			Model:     selection.Model,
			Source:    selection.Source,
			AgentName: agentNameFromRunLabel(label),
		})

		profile := agentPromptProfileForLabel(label)
		systemPrompt := buildAgentSystemPrompt(targetWorkspaceDir, profile, semanticCfg)
		baseRegistry := newBaseToolRegistryWithProcess(targetWorkspaceDir, tool.SingleDirPolicy(targetWorkspaceDir), nil, semanticCfg)
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
		if strings.HasPrefix(lowerLabel, "cron") {
			meta.Source = "cron"
		}
		if idx := strings.Index(label, ":"); idx >= 0 && idx+1 < len(label) {
			meta.RunID = strings.TrimSpace(label[idx+1:])
		}
		ctx = usage.WithCallMeta(ctx, meta)
		loop, _ := setupAgentLoop(runClient, registry, label, 0, logger, func(string, string, string, string, string, string) {})
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

func agentNameFromRunLabel(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
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
	registry := newBaseToolRegistryWithProcess(workspaceDir, tool.SingleDirPolicy(workspaceDir), nil, semanticCfg...)
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
