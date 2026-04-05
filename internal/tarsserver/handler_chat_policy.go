package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
)

func buildChatToolRegistry(
	reqStore *session.Store,
	workspaceID string,
	sessionID string,
	requestWorkspaceDir string,
	policy tool.PathPolicy,
	history []session.Message,
	deps chatHandlerDeps,
) *tool.Registry {
	registry := newBaseToolRegistryWithProcess(requestWorkspaceDir, policy, deps.tooling.ProcessManager, deps.tooling.MemorySemanticConfig)

	// Re-register ops aggregator with deps manager
	registry.Register(tool.NewOpsTool(deps.tooling.OpsManager))

	// Standalone tools
	registry.Register(tool.NewResearchReportTool(deps.tooling.ResearchService))
	if deps.tooling.UsageTracker != nil {
		registry.Register(tool.NewUsageReportTool(deps.tooling.UsageTracker))
	}

	// Tasks aggregator (session-scoped plan + tasks)
	registry.Register(tool.NewTasksTool(reqStore, requestWorkspaceDir, func() string { return sessionID }))

	// Session aggregator + subagents
	registry.Register(tool.NewSessionTool(reqStore, deps.tooling.Gateway, func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{
			SessionID:       sessionID,
			HistoryMessages: len(history) + 1,
		}, nil
	}))
	registry.Register(tool.NewSubagentsRunTool(deps.tooling.Gateway))

	// Automation (cron + heartbeat aggregators)
	if deps.tooling.AutomationToolsForWorkspace != nil {
		for _, autoTool := range deps.tooling.AutomationToolsForWorkspace(workspaceID) {
			registry.Register(autoTool)
		}
	}

	// Extra tools + extensions
	for _, extra := range deps.extraTools {
		registry.Register(extra)
	}
	if deps.tooling.Extensions != nil {
		for _, extra := range deps.tooling.Extensions.ChatTools() {
			registry.Register(extra)
		}
	}
	return registry
}

func resolveInjectedToolSchemas(
	registry *tool.Registry,
	_ string, // toolsDefaultSet — deprecated, individual tool toggles + high-risk filter used instead
	_ any, // activeProject — removed (was *project.Project)
	authRole string,
	allowHighRiskUser bool,
	sessionConfig ...session.SessionToolConfig,
) []llm.ToolSchema {
	if registry == nil {
		return nil
	}

	names := toolNamesFromSchemas(registry.Schemas())

	// Apply session-level tool config (if provided)
	if len(sessionConfig) > 0 {
		names = applySessionToolConfig(names, sessionConfig[0])
	}

	names = filterHighRiskToolNamesForRole(names, authRole, allowHighRiskUser)
	if len(names) == 0 {
		return nil
	}
	return registry.SchemasForNames(names)
}

// applySessionToolConfig filters tool names based on per-session configuration.
func applySessionToolConfig(names []string, config session.SessionToolConfig) []string {
	// If ToolsEnabled is set, use it as an allowlist
	if len(config.ToolsEnabled) > 0 {
		allowed := map[string]struct{}{}
		for _, name := range config.ToolsEnabled {
			canonical := tool.CanonicalToolName(name)
			if canonical != "" {
				allowed[canonical] = struct{}{}
			}
		}
		filtered := make([]string, 0, len(names))
		for _, name := range names {
			canonical := tool.CanonicalToolName(name)
			if _, ok := allowed[canonical]; ok {
				filtered = append(filtered, name)
			}
		}
		names = filtered
	}
	// Apply deny list
	if len(config.ToolsDisabled) > 0 {
		denied := map[string]struct{}{}
		for _, name := range config.ToolsDisabled {
			canonical := tool.CanonicalToolName(name)
			if canonical != "" {
				denied[canonical] = struct{}{}
			}
		}
		filtered := make([]string, 0, len(names))
		for _, name := range names {
			canonical := tool.CanonicalToolName(name)
			if _, ok := denied[canonical]; !ok {
				filtered = append(filtered, name)
			}
		}
		names = filtered
	}
	return names
}

func shouldFilterHighRiskTools(authRole string, allowHighRiskUser bool) bool {
	if allowHighRiskUser {
		return false
	}
	return strings.TrimSpace(strings.ToLower(authRole)) != serverauth.RoleAdmin
}

func filterHighRiskToolNamesForRole(names []string, authRole string, allowHighRiskUser bool) []string {
	if !shouldFilterHighRiskTools(authRole, allowHighRiskUser) {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if isHighRiskToolName(name) {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func isHighRiskToolName(name string) bool {
	canonical := tool.CanonicalToolName(name)
	if canonical == "" {
		return false
	}
	switch canonical {
	case "exec", "process", "write_file", "edit_file", "apply_patch", "workspace":
		return true
	}
	return strings.HasPrefix(canonical, "write_") || strings.HasPrefix(canonical, "edit_")
}

func knownToolsFromRegistry(registry *tool.Registry) map[string]struct{} {
	out := map[string]struct{}{}
	if registry == nil {
		return out
	}
	for _, schema := range registry.Schemas() {
		name := tool.CanonicalToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func normalizeToolNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, item := range names {
		name := tool.CanonicalToolName(item)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func defaultMinimalToolNames() []string {
	return []string{
		"memory",
		"knowledge",
		"workspace",
		"ops",
		"cron",
		"tasks",
		"research_report",
		"usage_report",
		"session",
	}
}
