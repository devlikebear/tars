package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/plugin"
	"github.com/devlikebear/tarsncase/internal/skill"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

func buildAutomationTools(
	cronStore *cron.Store,
	cronRunner func(ctx context.Context, job cron.Job) (string, error),
	heartbeatRunner func(ctx context.Context) (heartbeat.RunResult, error),
	heartbeatStatusProvider func(ctx context.Context) (tool.HeartbeatStatus, error),
	nowFn func() time.Time,
) []tool.Tool {
	return []tool.Tool{
		tool.NewCronTool(cronStore, cronRunner),
		tool.NewCronListTool(cronStore),
		tool.NewCronGetTool(cronStore),
		tool.NewCronRunsTool(cronStore),
		tool.NewCronCreateTool(cronStore),
		tool.NewCronUpdateTool(cronStore),
		tool.NewCronDeleteTool(cronStore),
		tool.NewCronRunTool(cronStore, cronRunner),
		tool.NewHeartbeatTool(
			heartbeatStatusProvider,
			func(ctx context.Context) (tool.HeartbeatRunResult, error) {
				if heartbeatRunner == nil {
					return tool.HeartbeatRunResult{}, fmt.Errorf("heartbeat runner is not configured")
				}
				ranAt := nowFn().UTC()
				result, err := heartbeatRunner(ctx)
				return tool.HeartbeatRunResult{
					Response:     result.Response,
					Skipped:      result.Skipped,
					SkipReason:   result.SkipReason,
					Logged:       result.Logged,
					Acknowledged: result.Acknowledged,
					RanAt:        ranAt,
				}, err
			},
		),
		tool.NewHeartbeatStatusTool(heartbeatStatusProvider),
		tool.NewHeartbeatRunOnceTool(func(ctx context.Context) (tool.HeartbeatRunResult, error) {
			if heartbeatRunner == nil {
				return tool.HeartbeatRunResult{}, fmt.Errorf("heartbeat runner is not configured")
			}
			ranAt := nowFn().UTC()
			result, err := heartbeatRunner(ctx)
			return tool.HeartbeatRunResult{
				Response:     result.Response,
				Skipped:      result.Skipped,
				SkipReason:   result.SkipReason,
				Logged:       result.Logged,
				Acknowledged: result.Acknowledged,
				RanAt:        ranAt,
			}, err
		}),
	}
}

func buildChatToolingOptions(
	processManager *tool.ProcessManager,
	manager *extensions.Manager,
	gatewayRuntime *gateway.Runtime,
) chatToolingOptions {
	var extensionManager *extensions.Manager
	extensionManager = manager
	return chatToolingOptions{
		ProcessManager: processManager,
		Extensions:     extensionManager,
		Gateway:        gatewayRuntime,
	}
}

func buildOptionalChatTools(cfg config.Config, gatewayRuntime *gateway.Runtime) []tool.Tool {
	out := []tool.Tool{}
	if cfg.ToolsMessageEnabled {
		out = append(out, tool.NewMessageTool(gatewayRuntime, true))
	}
	if cfg.ToolsBrowserEnabled {
		out = append(out, tool.NewBrowserTool(gatewayRuntime, true))
	}
	if cfg.ToolsNodesEnabled {
		out = append(out, tool.NewNodesTool(gatewayRuntime, true))
	}
	if cfg.ToolsGatewayEnabled {
		out = append(out, tool.NewGatewayTool(gatewayRuntime, true))
	}
	if cfg.ToolsApplyPatchEnabled {
		out = append(out, tool.NewApplyPatchTool(cfg.WorkspaceDir, true))
	}
	if cfg.ToolsWebFetchEnabled {
		out = append(out, tool.NewWebFetchToolWithOptions(tool.WebFetchOptions{
			Enabled:              true,
			AllowPrivateHosts:    cfg.ToolsWebFetchAllowPrivateHosts,
			PrivateHostAllowlist: cfg.ToolsWebFetchPrivateHostAllowlist,
		}))
	}
	if cfg.ToolsWebSearchEnabled {
		out = append(out, tool.NewWebSearchToolWithOptions(tool.WebSearchOptions{
			Enabled:           true,
			Provider:          cfg.ToolsWebSearchProvider,
			BraveAPIKey:       cfg.ToolsWebSearchAPIKey,
			PerplexityAPIKey:  cfg.ToolsWebSearchPerplexityAPIKey,
			PerplexityModel:   cfg.ToolsWebSearchPerplexityModel,
			PerplexityBaseURL: cfg.ToolsWebSearchPerplexityBaseURL,
			CacheTTL:          time.Duration(cfg.ToolsWebSearchCacheTTLSeconds) * time.Second,
		}))
	}
	return out
}

func buildGatewayExecutors(
	cfg config.Config,
	runPrompt gatewayPromptRunner,
	logger zerolog.Logger,
) []gateway.AgentExecutor {
	out := make([]gateway.AgentExecutor, 0, len(cfg.GatewayAgents))
	registeredNames := map[string]struct{}{}
	for _, spec := range cfg.GatewayAgents {
		if !spec.Enabled {
			continue
		}
		name := strings.TrimSpace(spec.Name)
		command := strings.TrimSpace(os.ExpandEnv(spec.Command))
		if name == "" || command == "" {
			logger.Warn().
				Str("agent", name).
				Str("command", command).
				Msg("skipping invalid gateway agent executor config")
			continue
		}

		args := make([]string, 0, len(spec.Args))
		for _, arg := range spec.Args {
			args = append(args, os.ExpandEnv(arg))
		}

		env := map[string]string{}
		for key, value := range spec.Env {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			env[trimmed] = os.ExpandEnv(value)
		}

		workDir := strings.TrimSpace(os.ExpandEnv(spec.WorkingDir))
		if workDir == "" {
			workDir = strings.TrimSpace(cfg.WorkspaceDir)
		} else if !filepath.IsAbs(workDir) && strings.TrimSpace(cfg.WorkspaceDir) != "" {
			workDir = filepath.Join(cfg.WorkspaceDir, workDir)
		}

		timeout := time.Duration(spec.TimeoutSeconds) * time.Second
		executor, err := gateway.NewCommandExecutor(gateway.CommandExecutorOptions{
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Source:      "config",
			Command:     command,
			Args:        args,
			Env:         env,
			WorkDir:     workDir,
			Timeout:     timeout,
		})
		if err != nil {
			logger.Warn().Err(err).Str("agent", name).Msg("failed to build gateway agent executor")
			continue
		}
		out = append(out, executor)
		registeredNames[strings.ToLower(name)] = struct{}{}
	}

	if runPrompt == nil {
		return out
	}

	workspaceAgents, diagnostics, err := loadWorkspaceGatewayAgents(cfg.WorkspaceDir)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load workspace markdown agents")
		return out
	}
	for _, diag := range diagnostics {
		logger.Warn().Str("diagnostic", strings.TrimSpace(diag)).Msg("workspace gateway agent diagnostic")
	}
	for _, def := range workspaceAgents {
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if key == "" {
			continue
		}
		if _, exists := registeredNames[key]; exists {
			continue
		}
		executor, err := newWorkspacePromptExecutor(def, runPrompt)
		if err != nil {
			logger.Warn().Err(err).Str("agent", def.Name).Msg("failed to build workspace prompt executor")
			continue
		}
		out = append(out, executor)
		registeredNames[key] = struct{}{}
	}
	return out
}

func buildExtensionsManager(cfg config.Config, runtime extensions.MPRuntime) (*extensions.Manager, error) {
	manager, err := extensions.NewManager(extensions.Options{
		WorkspaceDir:   cfg.WorkspaceDir,
		SkillsEnabled:  cfg.SkillsEnabled,
		PluginsEnabled: cfg.PluginsEnabled,
		SkillSources:   buildSkillSources(cfg),
		PluginSources:  buildPluginSources(cfg),
		MCPBaseServers: append([]config.MCPServer(nil), cfg.MCPServers...),
		MCPRuntime:     runtime,
		WatchSkills:    cfg.SkillsWatch,
		WatchPlugins:   cfg.PluginsWatch,
		WatchDebounce:  resolveExtensionsWatchDebounce(cfg),
	})
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func buildSkillSources(cfg config.Config) []skill.SourceDir {
	out := make([]skill.SourceDir, 0)
	appendSource := func(source skill.Source, path string) {
		trimmed := strings.TrimSpace(os.ExpandEnv(path))
		if trimmed == "" {
			return
		}
		out = append(out, skill.SourceDir{Source: source, Dir: trimmed})
	}

	appendSource(skill.SourceBundled, cfg.SkillsBundledDir)
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		appendSource(skill.SourceUser, filepath.Join(home, ".tarsncase", "skills"))
	}
	for _, extra := range cfg.SkillsExtraDirs {
		appendSource(skill.SourceUser, extra)
	}
	appendSource(skill.SourceWorkspace, filepath.Join(cfg.WorkspaceDir, "skills"))
	return out
}

func buildPluginSources(cfg config.Config) []extensions.PluginSourceDir {
	out := make([]extensions.PluginSourceDir, 0)
	appendSource := func(source plugin.Source, path string) {
		trimmed := strings.TrimSpace(os.ExpandEnv(path))
		if trimmed == "" {
			return
		}
		out = append(out, extensions.PluginSourceDir{Source: source, Dir: trimmed})
	}

	appendSource(plugin.SourceBundled, cfg.PluginsBundledDir)
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		appendSource(plugin.SourceUser, filepath.Join(home, ".tarsncase", "plugins"))
	}
	for _, extra := range cfg.PluginsExtraDirs {
		appendSource(plugin.SourceUser, extra)
	}
	appendSource(plugin.SourceWorkspace, filepath.Join(cfg.WorkspaceDir, "plugins"))
	return out
}

func resolveExtensionsWatchDebounce(cfg config.Config) time.Duration {
	values := []int{cfg.SkillsWatchDebounceMS, cfg.PluginsWatchDebounceMS}
	min := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if min == 0 || value < min {
			min = value
		}
	}
	if min <= 0 {
		min = 200
	}
	return time.Duration(min) * time.Millisecond
}
