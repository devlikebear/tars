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
		manifest.Lifecycle.OnStart = strings.TrimSpace(manifest.Lifecycle.OnStart)
		manifest.Lifecycle.OnStop = strings.TrimSpace(manifest.Lifecycle.OnStop)
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
