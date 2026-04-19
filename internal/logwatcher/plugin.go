// Package logwatcher provides the log-watcher builtin plugin that exposes
// docker and file-tail log inspection tools to TARS chat sessions.
package logwatcher

import (
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/tool"
)

const pluginID = "tars-log-watcher"

type Plugin struct{}

func (p *Plugin) ID() string { return pluginID }

func (p *Plugin) Definition() plugin.Definition {
	return plugin.Definition{
		SchemaVersion: 3,
		ID:            pluginID,
		Name:          "Log Watcher",
		Description:   "Inspects Docker container logs and tails local log files for anomaly detection workflows.",
		Version:       "0.1.0",
		Source:        plugin.SourceBundled,
		Requires: plugin.Requires{
			Bins: []string{"docker"},
		},
		ToolsProvider: &plugin.ToolsProvider{
			Type:  "go_plugin",
			Entry: "builtin:" + pluginID,
		},
	}
}

func (p *Plugin) Init(_ plugin.PluginContext) error { return nil }

func (p *Plugin) Close() error { return nil }

func (p *Plugin) HTTPHandlers() []plugin.HTTPHandlerEntry { return nil }

func (p *Plugin) Tools() []tool.Tool {
	return []tool.Tool{
		newDockerLogsTool(defaultDockerRunner),
		newFileTailTool(),
	}
}
