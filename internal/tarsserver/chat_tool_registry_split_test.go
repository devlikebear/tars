package tarsserver

import (
	"io"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

// buildChatToolRegistry wires tools from both sides of the internal/tool ⇄
// internal/apptool split. The split moved constructors between packages
// without changing any tool's name; asserting the names this registry
// actually produces is what keeps that true, and it covers the wiring that
// otherwise has no direct test.

func TestBuildChatToolRegistry_RegistersToolsFromBothPackages(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	workspaceDir := t.TempDir()

	deps := chatHandlerDeps{
		logger:  zerolog.New(io.Discard),
		tooling: defaultChatToolingOptions(),
	}

	registry := buildChatToolRegistry(
		store,
		"ws",
		main.ID,
		workspaceDir,
		tool.SingleDirPolicy(workspaceDir),
		nil,
		deps,
	)
	if registry == nil {
		t.Fatal("buildChatToolRegistry returned nil")
	}

	names := map[string]bool{}
	for _, schema := range registry.Schemas() {
		names[schema.Function.Name] = true
	}

	// App-package tools.
	for _, want := range []string{"tasks", "session", "subagents_run"} {
		if !names[want] {
			t.Errorf("app tool %q missing from the chat registry", want)
		}
	}
	// Core-package tools, from the base registry.
	for _, want := range []string{"read_file", "exec"} {
		if !names[want] {
			t.Errorf("core tool %q missing from the chat registry", want)
		}
	}
}

func TestBuildChatToolRegistry_UsesUserScope(t *testing.T) {
	// The chat registry is user-scoped, so the system-surface prefixes stay
	// unreachable from a chat turn. The panic lives in the core registry and
	// this pins that the chat path still opts into it.
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	workspaceDir := t.TempDir()

	registry := buildChatToolRegistry(
		store,
		"ws",
		main.ID,
		workspaceDir,
		tool.SingleDirPolicy(workspaceDir),
		nil,
		chatHandlerDeps{logger: zerolog.New(io.Discard), tooling: defaultChatToolingOptions()},
	)

	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic registering a pulse_-prefixed tool into the chat registry")
		}
	}()
	registry.Register(tool.Tool{Name: "pulse_decide"})
}
