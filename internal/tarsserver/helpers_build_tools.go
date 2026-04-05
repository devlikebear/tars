package tarsserver

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/gateway"
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
	gatewayRuntime *gateway.Runtime,
	toolsDefaultSet string,
	toolsAllowHighRiskUser bool,
	memorySemanticConfig memory.SemanticConfig,
	apiMaxInflightChat int,
	usageTracker *usage.Tracker,
) chatToolingOptions {
	var extensionManager *extensions.Manager
	extensionManager = manager
	return chatToolingOptions{
		ProcessManager:         processManager,
		Extensions:             extensionManager,
		Gateway:                gatewayRuntime,
		ToolsDefaultSet:        strings.TrimSpace(strings.ToLower(toolsDefaultSet)),
		ToolsAllowHighRiskUser: toolsAllowHighRiskUser,
		MemorySemanticConfig:   memorySemanticConfig,
		MemoryCache:            newMemoryCache(defaultMemoryCacheTTL),
		APIMaxInflightChat:     apiMaxInflightChat,
		UsageTracker:           usageTracker,
	}
}

func buildOptionalChatTools(cfg config.Config, gatewayRuntime *gateway.Runtime) []tool.Tool {
	out := []tool.Tool{}
	if cfg.ToolsMessageEnabled {
		out = append(out, tool.NewMessageTool(gatewayRuntime, true))
	}
	// Browser tool is now provided by the browserplugin via extensions manager
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
