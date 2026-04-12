package tarsserver

import (
	"testing"

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
