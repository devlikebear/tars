package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/tool"
)

func TestResolveInjectedToolSchemas_FiltersHighRiskToolsForUserRole(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.NewProcessManager())
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
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.NewProcessManager())
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
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "user", true)
	names := toolNamesFromSchemas(schemas)
	for _, expected := range []string{"exec", "process", "write_file", "edit_file"} {
		if !hasToolName(names, expected) {
			t.Fatalf("expected %s when tools_allow_high_risk_user=true, got %+v", expected, names)
		}
	}
}

func TestResolveInjectedToolSchemas_PassesThroughWithoutProjectPolicy(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.NewProcessManager())

	// Without project policy, all tools should pass through (only role-based filtering)
	schemas := resolveInjectedToolSchemas(registry, "standard", nil, "admin", true)
	names := toolNamesFromSchemas(schemas)
	if !hasToolName(names, "read_file") {
		t.Fatalf("expected read_file to be available, got %+v", names)
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
