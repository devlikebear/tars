package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
)

func buildChatToolRegistry(
	reqStore *session.Store,
	workspaceID string,
	sessionID string,
	requestWorkspaceDir string,
	history []session.Message,
	deps chatHandlerDeps,
) *tool.Registry {
	registry := newBaseToolRegistryWithProcess(requestWorkspaceDir, deps.tooling.ProcessManager)
	projectStore := project.NewStore(requestWorkspaceDir, nil)
	registry.Register(tool.NewProjectCreateTool(projectStore))
	registry.Register(tool.NewProjectListTool(projectStore))
	registry.Register(tool.NewProjectGetTool(projectStore))
	registry.Register(tool.NewProjectUpdateTool(projectStore))
	registry.Register(tool.NewProjectDeleteTool(projectStore))
	registry.Register(tool.NewProjectActivateTool(projectStore, reqStore, deps.mainSessionID))
	registry.Register(tool.NewProjectBriefGetTool(projectStore))
	registry.Register(tool.NewProjectBriefUpdateTool(projectStore))
	registry.Register(tool.NewProjectBriefFinalizeTool(projectStore, reqStore))
	registry.Register(tool.NewProjectStateGetTool(projectStore))
	registry.Register(tool.NewProjectStateUpdateTool(projectStore))
	registry.Register(tool.NewOpsStatusTool(deps.tooling.OpsManager))
	registry.Register(tool.NewOpsCleanupPlanTool(deps.tooling.OpsManager))
	registry.Register(tool.NewOpsCleanupApplyTool(deps.tooling.OpsManager))
	registry.Register(tool.NewScheduleCreateTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleListTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleUpdateTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleDeleteTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleCompleteTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewResearchReportTool(deps.tooling.ResearchService))
	if deps.tooling.UsageTracker != nil {
		registry.Register(tool.NewUsageReportTool(deps.tooling.UsageTracker))
	}
	registry.Register(tool.NewSessionsListTool(reqStore))
	registry.Register(tool.NewSessionsHistoryTool(reqStore))
	registry.Register(tool.NewSessionsSendTool(deps.tooling.Gateway))
	registry.Register(tool.NewSessionsSpawnTool(deps.tooling.Gateway))
	registry.Register(tool.NewSessionsRunsTool(deps.tooling.Gateway))
	registry.Register(tool.NewAgentsListTool(deps.tooling.Gateway))
	if deps.tooling.AutomationToolsForWorkspace != nil {
		for _, autoTool := range deps.tooling.AutomationToolsForWorkspace(workspaceID) {
			registry.Register(autoTool)
		}
	}
	for _, extra := range deps.extraTools {
		registry.Register(extra)
	}
	if deps.tooling.Extensions != nil {
		for _, extra := range deps.tooling.Extensions.ChatTools() {
			registry.Register(extra)
		}
	}
	registry.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{
			SessionID:       sessionID,
			HistoryMessages: len(history) + 1,
		}, nil
	}))
	return registry
}

func resolveInjectedToolSchemas(
	registry *tool.Registry,
	toolsDefaultSet string,
	activeProject *project.Project,
	authRole string,
	allowHighRiskUser bool,
) []llm.ToolSchema {
	if registry == nil {
		return nil
	}
	mode := strings.TrimSpace(strings.ToLower(toolsDefaultSet))
	if activeProject == nil {
		if mode == "minimal" {
			names := filterHighRiskToolNamesForRole(defaultMinimalToolNames(), authRole, allowHighRiskUser)
			return registry.SchemasForNames(names)
		}
		if shouldFilterHighRiskTools(authRole, allowHighRiskUser) {
			names := filterHighRiskToolNamesForRole(toolNamesFromSchemas(registry.Schemas()), authRole, allowHighRiskUser)
			return registry.SchemasForNames(names)
		}
		return registry.Schemas()
	}

	names := defaultMinimalToolNames()
	policy := project.NormalizeToolPolicy(project.ToolPolicySpec{
		ToolsAllow:               activeProject.ToolsAllow,
		ToolsAllowExists:         len(activeProject.ToolsAllow) > 0,
		ToolsAllowGroups:         activeProject.ToolsAllowGroups,
		ToolsAllowGroupsExists:   len(activeProject.ToolsAllowGroups) > 0,
		ToolsAllowPatterns:       activeProject.ToolsAllowPatterns,
		ToolsAllowPatternsExists: len(activeProject.ToolsAllowPatterns) > 0,
		ToolsDeny:                activeProject.ToolsDeny,
		ToolsDenyExists:          len(activeProject.ToolsDeny) > 0,
		ToolsRiskMax:             activeProject.ToolsRiskMax,
		ToolsRiskMaxExists:       strings.TrimSpace(activeProject.ToolsRiskMax) != "",
	}, knownToolsFromRegistry(registry), project.ToolPolicyOptions{})
	if len(policy.AllowedTools) > 0 {
		names = append(names, policy.AllowedTools...)
	}
	names = normalizeToolNames(names)
	names = project.ApplyToolConstraints(names, policy)
	names = filterHighRiskToolNamesForRole(names, authRole, allowHighRiskUser)
	if len(names) == 0 {
		return nil
	}
	return registry.SchemasForNames(names)
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
	case "exec", "process", "write", "write_file", "edit", "edit_file", "apply_patch":
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
		"memory_get",
		"memory_search",
		"memory_save",
		"project_get",
		"project_list",
		"project_update",
		"project_activate",
		"project_brief_get",
		"project_brief_update",
		"project_brief_finalize",
		"project_state_get",
		"project_state_update",
		"ops_status",
		"ops_cleanup_plan",
		"schedule_list",
		"schedule_create",
		"research_report",
		"usage_report",
		"session_status",
		"sessions_list",
		"sessions_history",
	}
}
