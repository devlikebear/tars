package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

// These cover the wiring that reaches across the internal/tool ⇄
// internal/apptool split. The split moved tool constructors between packages
// without changing any tool's name or schema; the cheapest way to keep that
// true is to assert the names this wiring actually produces.

func toolNameSet(t *testing.T, cfg config.Config) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, tl := range buildOptionalChatTools(cfg, nil) {
		if names[tl.Name] {
			t.Errorf("duplicate tool name %q from buildOptionalChatTools", tl.Name)
		}
		names[tl.Name] = true
	}
	return names
}

func TestBuildOptionalChatTools_RegistersAppAndCoreToolsUnderStableNames(t *testing.T) {
	cfg := config.Default()
	cfg.WorkspaceDir = t.TempDir()
	cfg.ToolsMessageEnabled = true
	cfg.ToolsAgentRuntimeEnabled = true
	cfg.ToolsApplyPatchEnabled = true

	names := toolNameSet(t, cfg)

	// One from each side of the split, so a constructor that moved to the
	// wrong package or lost its name fails here rather than at runtime.
	for _, want := range []string{"message", "agentruntime", "apply_patch"} {
		if !names[want] {
			t.Errorf("tool %q missing from the built set; got %v", want, names)
		}
	}
}

func TestBuildOptionalChatTools_RespectsDisabledFlags(t *testing.T) {
	cfg := config.Default()
	cfg.WorkspaceDir = t.TempDir()
	cfg.ToolsMessageEnabled = false
	cfg.ToolsAgentRuntimeEnabled = false
	cfg.ToolsApplyPatchEnabled = false
	cfg.ToolsWebFetchEnabled = false
	cfg.ToolsWebSearchEnabled = false

	if names := toolNameSet(t, cfg); len(names) != 0 {
		t.Fatalf("expected no optional tools with every flag off, got %v", names)
	}
}

func TestBuildOptionalChatTools_WebToolsFollowTheirFlags(t *testing.T) {
	cfg := config.Default()
	cfg.WorkspaceDir = t.TempDir()
	cfg.ToolsMessageEnabled = false
	cfg.ToolsAgentRuntimeEnabled = false
	cfg.ToolsApplyPatchEnabled = false
	cfg.ToolsWebFetchEnabled = true
	cfg.ToolsWebSearchEnabled = true
	cfg.ToolsWebSearchAPIKey = "test-key"

	names := toolNameSet(t, cfg)
	for _, want := range []string{"web_fetch", "web_search"} {
		if !names[want] {
			t.Errorf("tool %q missing when its flag is on; got %v", want, names)
		}
	}
}
