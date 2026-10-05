package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
)

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

func hasToolName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
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
