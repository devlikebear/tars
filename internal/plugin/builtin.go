package plugin

import (
	"net/http"

	"github.com/devlikebear/tars/internal/tool"
)

// BuiltinPlugin is the interface for compiled-in Go plugins registered via init().
type BuiltinPlugin interface {
	// ID returns the unique plugin identifier (must match manifest id).
	ID() string
	// Definition returns the plugin manifest metadata.
	Definition() Definition
	// Init receives dependencies and initializes the plugin.
	Init(ctx PluginContext) error
	// Tools returns the tools this plugin provides.
	Tools() []tool.Tool
	// HTTPHandlers returns HTTP handler entries this plugin provides.
	HTTPHandlers() []HTTPHandlerEntry
	// Close shuts down the plugin and releases resources.
	Close() error
}

// PluginContext carries dependencies injected into a built-in plugin at init time.
type PluginContext struct {
	Config       map[string]any // plugin-specific config (keys vary per plugin)
	WorkspaceDir string
}

// HTTPHandlerEntry pairs an HTTP route pattern with its handler.
type HTTPHandlerEntry struct {
	Pattern string
	Handler http.Handler
}
