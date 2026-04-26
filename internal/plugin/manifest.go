package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/devlikebear/tars/internal/config"
)

func parseManifestFile(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read plugin manifest: %w", err)
	}
	if err := rejectLegacyShellLifecycle(data); err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode plugin manifest: %w", err)
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.DefaultProjectProfile = strings.TrimSpace(manifest.DefaultProjectProfile)
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = 2
	}
	if manifest.SchemaVersion < 1 || manifest.SchemaVersion > 3 {
		return Manifest{}, fmt.Errorf("unsupported plugin manifest schema_version %d", manifest.SchemaVersion)
	}
	if manifest.ID == "" {
		return Manifest{}, fmt.Errorf("plugin id is required")
	}

	// v3 fields must not appear in v1/v2 manifests
	if manifest.SchemaVersion < 3 {
		if manifest.ToolsProvider != nil {
			return Manifest{}, fmt.Errorf("tools_provider requires schema_version >= 3")
		}
		if manifest.Lifecycle != nil {
			return Manifest{}, fmt.Errorf("lifecycle requires schema_version >= 3")
		}
		if len(manifest.HTTPRoutes) > 0 {
			return Manifest{}, fmt.Errorf("http_routes requires schema_version >= 3")
		}
	}

	// validate v3-specific fields
	if manifest.ToolsProvider != nil {
		manifest.ToolsProvider.Type = strings.TrimSpace(manifest.ToolsProvider.Type)
		manifest.ToolsProvider.Entry = strings.TrimSpace(manifest.ToolsProvider.Entry)
		switch manifest.ToolsProvider.Type {
		case "mcp_server", "go_plugin", "script":
		default:
			return Manifest{}, fmt.Errorf("unsupported tools_provider type %q (must be mcp_server, go_plugin, or script)", manifest.ToolsProvider.Type)
		}
	}
	if manifest.Lifecycle != nil {
		if err := validateLifecycleHook(manifest.Lifecycle.OnStart, "on_start"); err != nil {
			return Manifest{}, err
		}
		if err := validateLifecycleHook(manifest.Lifecycle.OnStop, "on_stop"); err != nil {
			return Manifest{}, err
		}
	}
	manifest.HTTPRoutes = normalizeHTTPRoutes(manifest.HTTPRoutes)

	manifest.Skills = normalizeList(manifest.Skills)
	manifest.MCPServers = normalizeMCPServers(manifest.MCPServers)
	manifest.Requires = Requires{
		Bins: normalizeList(manifest.Requires.Bins),
		Env:  normalizeList(manifest.Requires.Env),
	}
	manifest.SupportedOS = normalizeList(manifest.SupportedOS)
	manifest.SupportedArch = normalizeList(manifest.SupportedArch)
	manifest.Policies = Policies{
		ToolsAllow: normalizeList(manifest.Policies.ToolsAllow),
		ToolsDeny:  normalizeList(manifest.Policies.ToolsDeny),
	}
	return manifest, nil
}

func normalizeHTTPRoutes(routes []HTTPRoute) []HTTPRoute {
	out := make([]HTTPRoute, 0, len(routes))
	for _, route := range routes {
		path := strings.TrimSpace(route.Path)
		if path == "" {
			continue
		}
		out = append(out, HTTPRoute{
			Path:    path,
			Handler: strings.TrimSpace(route.Handler),
		})
	}
	return out
}

func normalizeList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// LifecycleDeniedTools is the set of tool names a plugin lifecycle hook
// must never invoke. These tools allow arbitrary command execution and
// would re-introduce the security hole the legacy string-based "sh -c"
// lifecycle hook had.
var LifecycleDeniedTools = map[string]struct{}{
	"bash":       {},
	"exec":       {},
	"shell_exec": {},
	"process":    {},
}

// rejectLegacyShellLifecycle inspects the raw JSON for the pre-RF-008
// string-based lifecycle hook form ("on_start": "echo …") and rejects
// it with an explicit migration message. The new struct form has Tool
// and Args fields; any object value is fine, only string values match
// the old shell-command shape.
func rejectLegacyShellLifecycle(raw []byte) error {
	var probe struct {
		Lifecycle *struct {
			OnStart json.RawMessage `json:"on_start"`
			OnStop  json.RawMessage `json:"on_stop"`
		} `json:"lifecycle"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		// Defer the proper unmarshal error to parseManifestFile.
		return nil
	}
	if probe.Lifecycle == nil {
		return nil
	}
	for field, value := range map[string]json.RawMessage{
		"on_start": probe.Lifecycle.OnStart,
		"on_stop":  probe.Lifecycle.OnStop,
	} {
		if len(value) == 0 {
			continue
		}
		trimmed := strings.TrimSpace(string(value))
		if strings.HasPrefix(trimmed, `"`) {
			return fmt.Errorf(
				"plugin manifest uses removed string form lifecycle.%s; "+
					"replace with object {\"tool\": \"<builtin-tool-name>\", \"args\": {...}} "+
					"(arbitrary shell commands were removed for security in RF-008)",
				field,
			)
		}
	}
	return nil
}

// validateLifecycleHook enforces the non-empty Tool name + deny-list
// rule for a single LifecycleHook. The deny-list is duplicated at hook
// invocation time so the executor refuses even if a deny-listed name
// somehow slipped past parsing.
func validateLifecycleHook(hook *LifecycleHook, field string) error {
	if hook == nil {
		return nil
	}
	hook.Tool = strings.TrimSpace(hook.Tool)
	if hook.Tool == "" {
		return fmt.Errorf("plugin manifest lifecycle.%s.tool is required", field)
	}
	if _, denied := LifecycleDeniedTools[hook.Tool]; denied {
		return fmt.Errorf(
			"plugin manifest lifecycle.%s.tool %q is on the lifecycle deny-list "+
				"(arbitrary shell tools cannot run from plugin hooks)",
			field, hook.Tool,
		)
	}
	return nil
}

func normalizeMCPServers(servers []ServerConfig) []ServerConfig {
	out := make([]ServerConfig, 0, len(servers))
	for _, server := range servers {
		normalized := config.NormalizeMCPServer(server)
		if !config.MCPServerEnabled(normalized) {
			continue
		}
		out = append(out, normalized)
	}
	return out
}
