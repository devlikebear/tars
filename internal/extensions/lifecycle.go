package extensions

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/plugin"
)

const defaultHookTimeout = 30 * time.Second

// runLifecycleHooks executes lifecycle hook commands for all plugins that declare them.
// hook must be "on_start" or "on_stop". Failures are collected as diagnostics, not fatal errors.
func runLifecycleHooks(ctx context.Context, plugins []plugin.Definition, hook string, timeout time.Duration) []string {
	if timeout <= 0 {
		timeout = defaultHookTimeout
	}
	var diagnostics []string
	for _, p := range plugins {
		if p.Lifecycle == nil {
			continue
		}
		var cmd string
		switch hook {
		case "on_start":
			cmd = p.Lifecycle.OnStart
		case "on_stop":
			cmd = p.Lifecycle.OnStop
		default:
			continue
		}
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}

		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		c := exec.CommandContext(hookCtx, "sh", "-c", cmd)
		c.Dir = p.RootDir
		var stderr bytes.Buffer
		c.Stderr = &stderr
		if err := c.Run(); err != nil {
			msg := fmt.Sprintf("plugin %q lifecycle %s failed: %v", p.ID, hook, err)
			if s := strings.TrimSpace(stderr.String()); s != "" {
				msg += ": " + s
			}
			diagnostics = append(diagnostics, msg)
		}
		cancel()
	}
	return diagnostics
}
