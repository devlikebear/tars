package tarsserver

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
)

func buildAutomationTools(
	cronStore *cron.Store,
	cronRunner func(ctx context.Context, job cron.Job) (string, error),
) []tool.Tool {
	return []tool.Tool{
		tool.NewCronTool(cronStore, cronRunner),
	}
}

func buildChatToolingOptions(
	processManager *tool.ProcessManager,
	manager *extensions.Manager,
	agentRuntime *agentruntime.Runtime,
	compaction chatCompactionOptions,
	toolsDefaultSet string,
	toolsAllowHighRiskUser bool,
	memorySemanticConfig memory.SemanticConfig,
	apiMaxInflightChat int,
	usageTracker *usage.Tracker,
	planClarifyMode string,
	styleDefaults sessionStyleValues,
) chatToolingOptions {
	var extensionManager *extensions.Manager
	extensionManager = manager
	return chatToolingOptions{
		ProcessManager:         processManager,
		Extensions:             extensionManager,
		AgentRuntime:           agentRuntime,
		ToolsDefaultSet:        strings.TrimSpace(strings.ToLower(toolsDefaultSet)),
		ToolsAllowHighRiskUser: toolsAllowHighRiskUser,
		MemorySemanticConfig:   memorySemanticConfig,
		MemoryCache:            newMemoryCache(defaultMemoryCacheTTL),
		APIMaxInflightChat:     apiMaxInflightChat,
		UsageTracker:           usageTracker,
		Compaction:             compaction,
		PlanClarifyMode:        strings.TrimSpace(strings.ToLower(planClarifyMode)),
		StyleDefaults:          effectiveSessionStyle(styleDefaults, nil),
	}
}

func buildOptionalChatTools(cfg config.Config, agentRuntime *agentruntime.Runtime) []tool.Tool {
	out := []tool.Tool{}
	if cfg.ToolsMessageEnabled {
		out = append(out, tool.NewMessageTool(agentRuntime, true))
	}
	if cfg.ToolsAgentRuntimeEnabled {
		out = append(out, tool.NewAgentRuntimeTool(agentRuntime, true))
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
