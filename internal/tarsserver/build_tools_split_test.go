package tarsserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/computeruse"
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

func TestBuildOptionalChatTools_ComputerUseFollowsItsFlag(t *testing.T) {
	cfg := config.Default()
	cfg.WorkspaceDir = t.TempDir()
	if toolNameSet(t, cfg)["computer_use"] {
		t.Fatal("computer_use is registered by default; it must be opt-in")
	}
	cfg.ToolsComputerUseEnabled = true
	if !toolNameSet(t, cfg)["computer_use"] {
		t.Fatal("computer_use missing when tools.computer_use.enabled is on")
	}
}

// With no System One server configured the tool stays registered and answers
// "unavailable" instead of failing the turn or touching the screen.
func TestComputerUseTool_UnavailableWithoutSystemOne(t *testing.T) {
	cfg := config.Default()
	cfg.ToolsComputerUseEnabled = true
	cfg.Jev.BaseURL = ""
	res := newComputerUseEngine(cfg).Run(context.Background(), computeruse.Request{Goal: "open settings"})
	if res.Status != computeruse.StatusUnavailable || res.Hint == "" {
		t.Fatalf("got %+v, want unavailable with a hint", res)
	}
}

func TestComputerUseTool_UnavailableWithoutDriver(t *testing.T) {
	cfg := config.Default()
	cfg.ToolsComputerUseEnabled = true
	cfg.Jev.BaseURL = "http://127.0.0.1:1"
	cfg.ToolsComputerUseCuaDriverPath = filepath.Join(t.TempDir(), "no-such-cua-driver")
	res := newComputerUseEngine(cfg).Run(context.Background(), computeruse.Request{Goal: "open settings"})
	if res.Status != computeruse.StatusUnavailable || !strings.Contains(res.Reason, "cua-driver") {
		t.Fatalf("got %+v, want unavailable naming cua-driver", res)
	}
}
