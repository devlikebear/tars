package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
)

func TestResolveInjectedToolSchemas_FiltersHighRiskToolsForUserRole(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())
	registry.Register(tool.NewApplyPatchTool(t.TempDir(), true))

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "user", false)
	names := toolNamesFromSchemas(schemas)
	for _, denied := range []string{"exec", "process", "write", "write_file", "edit", "edit_file", "apply_patch"} {
		if hasToolName(names, denied) {
			t.Fatalf("expected %s to be filtered for user role, got %+v", denied, names)
		}
	}
	if !hasToolName(names, "read_file") {
		t.Fatalf("expected read_file to remain available for user role, got %+v", names)
	}
}

func TestResolveInjectedToolSchemas_AllowAdminHighRiskTools(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())
	registry.Register(tool.NewApplyPatchTool(t.TempDir(), true))

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", false)
	names := toolNamesFromSchemas(schemas)
	// process comes with exec: a background exec needs it to be waited for.
	for _, expected := range []string{"exec", "process", "write_file", "edit_file", "apply_patch"} {
		if !hasToolName(names, expected) {
			t.Fatalf("expected %s for admin role, got %+v", expected, names)
		}
	}
}

func TestResolveInjectedToolSchemas_AllowHighRiskUserOverride(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "user", true)
	names := toolNamesFromSchemas(schemas)
	for _, expected := range []string{"exec", "process", "write_file", "edit_file"} {
		if !hasToolName(names, expected) {
			t.Fatalf("expected %s when tools_allow_high_risk_user=true, got %+v", expected, names)
		}
	}
}

// Without exec there is nothing for process to manage, so it stays off unless
// the session asks for it by name.
func TestResolveInjectedToolSchemas_ProcessFollowsExec(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	names := toolNamesFromSchemas(resolveInjectedToolSchemas(registry, "standard", nil, "admin", true, session.SessionToolConfig{
		ToolsDisabled: []string{"exec"},
	}))
	if hasToolName(names, "exec") || hasToolName(names, "process") {
		t.Fatalf("expected process to go with a disabled exec, got %+v", names)
	}
	if !hasToolName(names, "read_file") {
		t.Fatalf("expected the rest of the tools untouched, got %+v", names)
	}
}

func TestResolveInjectedToolSchemas_AllowsDeprecatedProcessWhenExplicitlyEnabled(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true, session.SessionToolConfig{
		ToolsCustom:  true,
		ToolsEnabled: []string{"process"},
	})
	names := toolNamesFromSchemas(schemas)
	if len(names) != 1 || !hasToolName(names, "process") {
		t.Fatalf("expected explicit session allowlist to opt into process, got %+v", names)
	}
}

func TestResolveInjectedToolSchemas_HidesSubagentFlowToolsByDefault(t *testing.T) {
	registry := newBaseToolRegistryWithSubagentTools(t)

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true)
	names := toolNamesFromSchemas(schemas)
	if !hasToolName(names, "subagents_run") {
		t.Fatalf("expected subagents_run to remain injected by default, got %+v", names)
	}
	for _, denied := range []string{"subagents_plan", "subagents_orchestrate"} {
		if hasToolName(names, denied) {
			t.Fatalf("expected %s to be hidden from default injection, got %+v", denied, names)
		}
	}
}

func TestResolveInjectedToolSchemas_AllowsSubagentFlowToolsWhenExplicitlyEnabled(t *testing.T) {
	registry := newBaseToolRegistryWithSubagentTools(t)

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true, session.SessionToolConfig{
		ToolsCustom: true,
		ToolsEnabled: []string{
			"subagents_plan",
			"subagents_orchestrate",
		},
	})
	names := toolNamesFromSchemas(schemas)
	if len(names) != 2 || !hasToolName(names, "subagents_plan") || !hasToolName(names, "subagents_orchestrate") {
		t.Fatalf("expected explicit session allowlist to opt into subagent flow tools, got %+v", names)
	}
}

func TestResolveInjectedToolSchemas_PassesThroughWithoutProjectPolicy(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	// Without project policy, all tools should pass through (only role-based filtering)
	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true)
	names := toolNamesFromSchemas(schemas)
	if !hasToolName(names, "read_file") {
		t.Fatalf("expected read_file to be available, got %+v", names)
	}
}

func TestResolveInjectedToolSchemas_RespectsExplicitEmptyAllowlist(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true, session.SessionToolConfig{
		ToolsCustom: true,
	})
	if len(schemas) != 0 {
		t.Fatalf("expected no injected tools for explicit empty allowlist, got %+v", toolNamesFromSchemas(schemas))
	}
}

func TestResolveInjectedToolSchemas_RespectsToolGroups(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true, session.SessionToolConfig{
		ToolsAllowGroups: []string{"file"},
	})
	names := toolNamesFromSchemas(schemas)
	for _, expected := range []string{"read_file", "write_file", "edit_file", "glob", "list_dir"} {
		if !hasToolName(names, expected) {
			t.Fatalf("expected %s to remain available, got %+v", expected, names)
		}
	}
	for _, denied := range []string{"exec", "process", "memory", "session", "web_fetch", "web_search"} {
		if hasToolName(names, denied) {
			t.Fatalf("expected %s to be filtered by group policy, got %+v", denied, names)
		}
	}
}

func TestResolveInjectedToolPolicy_ReturnsBlockedToolMetadata(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	resolved := resolveInjectedToolPolicy(registry, "admin", true, session.SessionToolConfig{
		ToolsAllowGroups: []string{"files"},
		ToolsDenyGroups:  []string{"shell"},
	})
	if _, ok := resolved.Blocked["exec"]; !ok {
		t.Fatalf("expected blocked metadata for exec, got %+v", resolved.Blocked)
	}
	blocked := resolved.Blocked["exec"]
	if blocked.Rule != "group_deny" || blocked.Group != "shell" || blocked.Source != "session" {
		t.Fatalf("unexpected blocked metadata: %+v", blocked)
	}
}

func TestResolveInjectedToolSchemas_GroupsStillApplyWhenToolsCustomIsTrue(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true, session.SessionToolConfig{
		ToolsCustom:      true,
		ToolsAllowGroups: []string{"files"},
	})
	names := toolNamesFromSchemas(schemas)
	if len(names) == 0 {
		t.Fatalf("expected grouped tools to remain available, got none")
	}
	if !hasToolName(names, "read_file") {
		t.Fatalf("expected read_file to remain available, got %+v", names)
	}
	if hasToolName(names, "exec") {
		t.Fatalf("expected exec to be excluded by file group filter, got %+v", names)
	}
}

func hasToolName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func newBaseToolRegistryWithSubagentTools(t *testing.T) *tool.Registry {
	t.Helper()
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())
	registry.Register(apptool.NewSubagentsRunTool(nil))
	registry.Register(apptool.NewSubagentsPlanTool(nil, nil))
	registry.Register(apptool.NewSubagentsOrchestrateTool(nil))
	return registry
}

// subagents_orchestrate is off by default, but with the durable scheduler
// running it submits durable work and is offered without session opt-in.
func TestResolveInjectedToolPolicy_OffersOrchestrateOnlyWithDurableFlows(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.SingleDirPolicy(t.TempDir()), tool.NewProcessManager())
	registry.Register(apptool.NewSubagentsOrchestrateTool(nil))

	without := toolNamesFromSchemas(resolveInjectedToolPolicyFor(registry, "admin", true, false).Schemas)
	if hasToolName(without, "subagents_orchestrate") {
		t.Fatalf("expected subagents_orchestrate hidden without the scheduler, got %+v", without)
	}
	with := toolNamesFromSchemas(resolveInjectedToolPolicyFor(registry, "admin", true, true).Schemas)
	if !hasToolName(with, "subagents_orchestrate") {
		t.Fatalf("expected subagents_orchestrate offered with the scheduler, got %+v", with)
	}
	if hasToolName(with, "subagents_plan") {
		t.Fatalf("the scheduler must not unhide other default-off tools, got %+v", with)
	}
}
