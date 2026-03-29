package plugin

import "sync"

var (
	builtinMu      sync.RWMutex
	builtinPlugins []BuiltinPlugin
)

// RegisterBuiltin registers a compiled-in plugin. Call from init().
func RegisterBuiltin(p BuiltinPlugin) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	builtinPlugins = append(builtinPlugins, p)
}

// BuiltinPlugins returns all registered built-in plugins.
func BuiltinPlugins() []BuiltinPlugin {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	return append([]BuiltinPlugin(nil), builtinPlugins...)
}
