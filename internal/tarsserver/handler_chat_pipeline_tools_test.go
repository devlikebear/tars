package tarsserver

import (
	"context"
	"testing"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
)

type chatPhaseEngineStub struct{}

func (chatPhaseEngineStub) Start(context.Context, string) (project.AutopilotRun, error) {
	return project.AutopilotRun{}, nil
}

func (chatPhaseEngineStub) Status(string) (project.AutopilotRun, bool) {
	return project.AutopilotRun{}, false
}

func (chatPhaseEngineStub) Current(string) (project.PhaseSnapshot, bool) {
	return project.PhaseSnapshot{}, false
}

func (chatPhaseEngineStub) Advance(context.Context, string) (project.PhaseSnapshot, error) {
	return project.PhaseSnapshot{}, nil
}

func (chatPhaseEngineStub) EnsureActiveRuns(context.Context) (int, error) {
	return 0, nil
}

func (chatPhaseEngineStub) Escalate(string, string) error {
	return nil
}

func (chatPhaseEngineStub) Resume(context.Context, string) (project.AutopilotRun, error) {
	return project.AutopilotRun{}, nil
}

func (chatPhaseEngineStub) Reset(string) error {
	return nil
}

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

func TestResolveInjectedToolSchemas_ProjectRiskMaxConstrainsAllowedTools(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.NewProcessManager())

	schemas := resolveInjectedToolSchemas(registry, "standard", &project.Project{
		ToolsAllow:   []string{"read_file", "glob"},
		ToolsRiskMax: "low",
	}, "admin", true)
	names := toolNamesFromSchemas(schemas)

	if hasToolName(names, "glob") {
		t.Fatalf("expected glob to be filtered by project tools_risk_max, got %+v", names)
	}
	if !hasToolName(names, "read_file") {
		t.Fatalf("expected read_file to remain allowed, got %+v", names)
	}
}

func TestResolveInjectedToolSchemas_MinimalIncludesProjectRuntimeTools(t *testing.T) {
	registry := newBaseToolRegistryWithProcess(t.TempDir(), tool.NewProcessManager())
	for _, expected := range []string{
		"project_create",
		"project_board_get",
		"project_activity_get",
		"project_dispatch",
		"project_autopilot_advance",
		"project_autopilot_start",
	} {
		registry.Register(tool.Tool{Name: expected})
	}

	schemas := resolveInjectedToolSchemas(registry, "minimal", nil, "user", false)
	names := toolNamesFromSchemas(schemas)
	for _, expected := range []string{
		"project_create",
		"project_board_get",
		"project_activity_get",
		"project_dispatch",
		"project_autopilot_advance",
		"project_autopilot_start",
	} {
		if !hasToolName(names, expected) {
			t.Fatalf("expected %s in minimal tool set, got %+v", expected, names)
		}
	}
}

func TestBuildChatToolRegistry_RegistersAutopilotPhaseToolsFromPhaseEngine(t *testing.T) {
	workspaceDir := t.TempDir()
	registry := buildChatToolRegistry(
		session.NewStore(workspaceDir),
		defaultWorkspaceID,
		"session-1",
		workspaceDir,
		nil,
		chatHandlerDeps{
			tooling: chatToolingOptions{
				ProjectAutopilot: chatPhaseEngineStub{},
			},
		},
	)

	names := toolNamesFromSchemas(registry.Schemas())
	for _, expected := range []string{"project_autopilot_start", "project_autopilot_advance"} {
		if !hasToolName(names, expected) {
			t.Fatalf("expected %s to be registered from phase engine tooling", expected)
		}
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
