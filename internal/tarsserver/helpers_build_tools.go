package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/heartbeat"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
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
	toolsDefaultSet string,
	toolsAllowHighRiskUser bool,
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
		APIMaxInflightChat:     apiMaxInflightChat,
		UsageTracker:           usageTracker,
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
