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
	if manifest.SchemaVersion < 1 || manifest.SchemaVersion > 2 {
		return Manifest{}, fmt.Errorf("unsupported plugin manifest schema_version %d", manifest.SchemaVersion)
	}
	if manifest.ID == "" {
		return Manifest{}, fmt.Errorf("plugin id is required")
	}
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
