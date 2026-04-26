package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/tool"
)

// fakeResolver is a minimal LifecycleToolResolver backed by a name→Tool
// map. The handlers receive the raw args and either return a result or
// signal failure via Tool.Execute behavior.
type fakeResolver map[string]tool.Tool

func (f fakeResolver) Get(name string) (tool.Tool, bool) {
	t, ok := f[name]
	return t, ok
}

func okTool(name string, called *bool, capturedArgs *json.RawMessage) tool.Tool {
	return tool.Tool{
		Name: name,
		Execute: func(_ context.Context, params json.RawMessage) (tool.Result, error) {
			if called != nil {
				*called = true
			}
			if capturedArgs != nil {
				*capturedArgs = append(json.RawMessage(nil), params...)
			}
			return tool.JSONTextResult(map[string]string{"ok": "true"}, false), nil
		},
	}
}

func failingTool(name string) tool.Tool {
	return tool.Tool{
		Name: name,
		Execute: func(_ context.Context, _ json.RawMessage) (tool.Result, error) {
			return tool.Result{}, errors.New("boom")
		},
	}
}

func TestRunLifecycleHooks_OnStart_InvokesResolvedTool(t *testing.T) {
	called := false
	var captured json.RawMessage
	resolver := fakeResolver{
		"memory_search": okTool("memory_search", &called, &captured),
	}

	plugins := []plugin.Definition{
		{
			ID: "test-plugin",
			Lifecycle: &plugin.Lifecycle{
				OnStart: &plugin.LifecycleHook{
					Tool: "memory_search",
					Args: json.RawMessage(`{"query":"hello"}`),
				},
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", 5*time.Second, resolver)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !called {
		t.Fatalf("expected resolved tool to be invoked")
	}
	if string(captured) != `{"query":"hello"}` {
		t.Fatalf("args mismatch: got %s", captured)
	}
}

func TestRunLifecycleHooks_DenyListedTool(t *testing.T) {
	resolver := fakeResolver{}
	plugins := []plugin.Definition{
		{
			ID: "evil",
			Lifecycle: &plugin.Lifecycle{
				OnStart: &plugin.LifecycleHook{Tool: "bash"},
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", time.Second, resolver)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d (%v)", len(diags), diags)
	}
	if !strings.Contains(diags[0], "deny-list") {
		t.Fatalf("expected deny-list diagnostic, got %q", diags[0])
	}
}

func TestRunLifecycleHooks_UnknownTool(t *testing.T) {
	resolver := fakeResolver{}
	plugins := []plugin.Definition{
		{
			ID: "missing",
			Lifecycle: &plugin.Lifecycle{
				OnStart: &plugin.LifecycleHook{Tool: "unregistered"},
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", time.Second, resolver)
	if len(diags) != 1 || !strings.Contains(diags[0], "not registered") {
		t.Fatalf("expected not-registered diagnostic, got %v", diags)
	}
}

func TestRunLifecycleHooks_ToolError(t *testing.T) {
	resolver := fakeResolver{
		"flaky": failingTool("flaky"),
	}
	plugins := []plugin.Definition{
		{
			ID: "p",
			Lifecycle: &plugin.Lifecycle{
				OnStart: &plugin.LifecycleHook{Tool: "flaky"},
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", time.Second, resolver)
	if len(diags) != 1 || !strings.Contains(diags[0], "flaky") {
		t.Fatalf("expected flaky-tool diagnostic, got %v", diags)
	}
}

func TestRunLifecycleHooks_NilResolverProducesSkipDiagnostic(t *testing.T) {
	plugins := []plugin.Definition{
		{
			ID: "p",
			Lifecycle: &plugin.Lifecycle{
				OnStart: &plugin.LifecycleHook{Tool: "memory_search"},
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", time.Second, nil)
	if len(diags) != 1 || !strings.Contains(diags[0], "no tool resolver") {
		t.Fatalf("expected skip diagnostic, got %v", diags)
	}
}

func TestRunLifecycleHooks_NoHooks(t *testing.T) {
	plugins := []plugin.Definition{
		{ID: "no-hooks"},
		{ID: "nil-lifecycle", Lifecycle: &plugin.Lifecycle{}},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", time.Second, fakeResolver{})
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics for plugins without hooks: %v", diags)
	}
}
