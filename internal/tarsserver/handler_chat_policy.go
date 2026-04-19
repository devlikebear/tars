package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
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

	// Standalone tools
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
	registry.Register(tool.NewSubagentsOrchestrateTool(deps.tooling.Gateway))
	if deps.router != nil {
		registry.Register(tool.NewSubagentsPlanTool(deps.tooling.Gateway, deps.router))
	}

	// Automation (cron aggregator; pulse/reflection live on the system surface)
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
	return resolveInjectedToolPolicy(registry, authRole, allowHighRiskUser, sessionConfig...).Schemas
}

type injectedToolPolicy struct {
	Schemas []llm.ToolSchema
	Blocked map[string]tool.BlockedToolError
}

func resolveInjectedToolPolicy(
	registry *tool.Registry,
	authRole string,
	allowHighRiskUser bool,
	sessionConfig ...session.SessionToolConfig,
) injectedToolPolicy {
	if registry == nil {
		return injectedToolPolicy{}
	}

	names := toolNamesFromSchemas(registry.Schemas())
	blocked := map[string]tool.BlockedToolError{}

	// Apply session-level tool config (if provided)
	if len(sessionConfig) > 0 {
		resolved := resolveSessionToolPolicy(names, sessionConfig[0], "session")
		names = resolved.Allowed
		mergeBlockedToolErrors(blocked, resolved.Blocked)
	}

	var highRiskBlocked map[string]tool.BlockedToolError
	names, highRiskBlocked = filterHighRiskToolNamesForRoleDetailed(names, authRole, allowHighRiskUser)
	mergeBlockedToolErrors(blocked, highRiskBlocked)
	if len(names) == 0 {
		return injectedToolPolicy{Blocked: blocked}
	}
	return injectedToolPolicy{
		Schemas: registry.SchemasForNames(names),
		Blocked: blocked,
	}
}

// applySessionToolConfig filters tool names based on per-session configuration.
func applySessionToolConfig(names []string, config session.SessionToolConfig) []string {
	return resolveSessionToolPolicy(names, config, "session").Allowed
}

func resolveSessionToolPolicy(names []string, config session.SessionToolConfig, source string) tool.PolicyResolution {
	useAllowTools := len(config.ToolsEnabled) > 0 || (config.ToolsCustom && len(config.ToolsAllowGroups) == 0)
	policy := tool.Policy{
		AllowTools:     config.ToolsEnabled,
		DenyTools:      config.ToolsDisabled,
		AllowGroups:    config.ToolsAllowGroups,
		DenyGroups:     config.ToolsDenyGroups,
		UseAllowTools:  useAllowTools,
		UseAllowGroups: len(config.ToolsAllowGroups) > 0,
	}
	return policy.Resolve(names, source)
}

func applySessionSkillConfig(skills []skill.Definition, config session.SessionToolConfig) []skill.Definition {
	if !config.SkillsCustom && len(config.SkillsEnabled) == 0 {
		return append([]skill.Definition(nil), skills...)
	}
	allowed := map[string]struct{}{}
	for _, name := range config.SkillsEnabled {
		normalized := strings.TrimSpace(strings.ToLower(name))
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	filtered := make([]skill.Definition, 0, len(skills))
	for _, def := range skills {
		normalized := strings.TrimSpace(strings.ToLower(def.Name))
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; !ok {
			continue
		}
		filtered = append(filtered, def)
	}
	return filtered
}

func filterExtensionsSnapshotForSession(snapshot extensions.Snapshot, sessionConfig ...session.SessionToolConfig) extensions.Snapshot {
	if len(sessionConfig) == 0 {
		return snapshot
	}
	out := snapshot
	out.Skills = applySessionSkillConfig(snapshot.Skills, sessionConfig[0])
	out.SkillPrompt = skill.FormatAvailableSkills(out.Skills)
	return out
}

func shouldFilterHighRiskTools(authRole string, allowHighRiskUser bool) bool {
	if allowHighRiskUser {
		return false
	}
	return strings.TrimSpace(strings.ToLower(authRole)) != serverauth.RoleAdmin
}

func filterHighRiskToolNamesForRole(names []string, authRole string, allowHighRiskUser bool) []string {
	filtered, _ := filterHighRiskToolNamesForRoleDetailed(names, authRole, allowHighRiskUser)
	return filtered
}

func filterHighRiskToolNamesForRoleDetailed(names []string, authRole string, allowHighRiskUser bool) ([]string, map[string]tool.BlockedToolError) {
	if !shouldFilterHighRiskTools(authRole, allowHighRiskUser) {
		return names, nil
	}
	filtered := make([]string, 0, len(names))
	blocked := map[string]tool.BlockedToolError{}
	for _, name := range names {
		canonical := tool.CanonicalToolName(name)
		if isHighRiskToolName(name) {
			blocked[canonical] = tool.BlockedToolError{
				Tool:   canonical,
				Rule:   "risk_deny",
				Group:  tool.ToolGroupForName(canonical),
				Source: "config_default",
			}
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered, blocked
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
		"usage_report",
		"session",
	}
}

func mergeBlockedToolErrors(dst map[string]tool.BlockedToolError, src map[string]tool.BlockedToolError) {
	if len(src) == 0 {
		return
	}
	for key, value := range src {
		dst[key] = value
	}
}
