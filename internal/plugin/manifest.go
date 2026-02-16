package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	if manifest.ID == "" {
		return Manifest{}, fmt.Errorf("plugin id is required")
	}
	manifest.Skills = normalizeList(manifest.Skills)
	manifest.MCPServers = normalizeMCPServers(manifest.MCPServers)
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
		name := strings.TrimSpace(server.Name)
		command := strings.TrimSpace(server.Command)
		if name == "" || command == "" {
			continue
		}
		copied := ServerConfig{
			Name:    name,
			Command: command,
			Args:    append([]string(nil), server.Args...),
		}
		if len(server.Env) > 0 {
			copied.Env = make(map[string]string, len(server.Env))
			for k, v := range server.Env {
				copied.Env[strings.TrimSpace(k)] = v
			}
		}
		out = append(out, copied)
	}
	return out
}
