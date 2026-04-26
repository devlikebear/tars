package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/tool"
)

const defaultHookTimeout = 30 * time.Second

// LifecycleToolResolver returns the tool registered under name. The
// extensions Manager passes in a small Resolver so lifecycle hooks can
// only see a curated subset of the user-surface registry; no caller
// has to hand the manager a full *tool.Registry just for hook plumbing.
type LifecycleToolResolver interface {
	Get(name string) (tool.Tool, bool)
}

// runLifecycleHooks invokes the declared on_start / on_stop tool calls
// for every plugin that has them. The legacy string-based "sh -c <cmd>"
// form was removed (RF-008): hooks now reference a builtin tool by
// name, which the resolver maps to a concrete tool.Tool, and the
// supplied JSON args are forwarded to Execute. If the resolver is nil,
// no hook runs and a single diagnostic per plugin records that the
// hook was skipped — this is how a partially-initialized server (e.g.
// extensions disabled, or pre-registry bring-up) avoids silently
// dropping declared hooks. Failures (unknown tool, deny-listed tool,
// timeout, tool error) are returned as diagnostics; they do not abort
// the parent Start / Close path because plugin hooks are advisory.
func runLifecycleHooks(ctx context.Context, plugins []plugin.Definition, hook string, timeout time.Duration, resolver LifecycleToolResolver) []string {
	if timeout <= 0 {
		timeout = defaultHookTimeout
	}
	var diagnostics []string
	for _, p := range plugins {
		if p.Lifecycle == nil {
			continue
		}
		var h *plugin.LifecycleHook
		switch hook {
		case "on_start":
			h = p.Lifecycle.OnStart
		case "on_stop":
			h = p.Lifecycle.OnStop
		default:
			continue
		}
		if h == nil {
			continue
		}
		if msg := executeLifecycleHook(ctx, p.ID, hook, h, timeout, resolver); msg != "" {
			diagnostics = append(diagnostics, msg)
		}
	}
	return diagnostics
}

func executeLifecycleHook(parentCtx context.Context, pluginID, hook string, h *plugin.LifecycleHook, timeout time.Duration, resolver LifecycleToolResolver) string {
	name := strings.TrimSpace(h.Tool)
	if name == "" {
		return fmt.Sprintf("plugin %q lifecycle %s missing tool name", pluginID, hook)
	}
	// Defense-in-depth: even if a deny-listed name slipped past
	// manifest parsing (e.g. via a future code path that bypasses
	// validateLifecycleHook), refuse here.
	if _, denied := plugin.LifecycleDeniedTools[name]; denied {
		return fmt.Sprintf("plugin %q lifecycle %s tool %q is on the lifecycle deny-list", pluginID, hook, name)
	}
	if resolver == nil {
		return fmt.Sprintf("plugin %q lifecycle %s skipped: no tool resolver available", pluginID, hook)
	}
	t, ok := resolver.Get(name)
	if !ok {
		return fmt.Sprintf("plugin %q lifecycle %s tool %q is not registered", pluginID, hook, name)
	}

	hookCtx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	args := h.Args
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	result, err := t.Execute(hookCtx, args)
	if err != nil {
		return fmt.Sprintf("plugin %q lifecycle %s tool %q failed: %v", pluginID, hook, name, err)
	}
	if result.IsError {
		return fmt.Sprintf("plugin %q lifecycle %s tool %q reported error: %s", pluginID, hook, name, result.Text())
	}
	return ""
}
