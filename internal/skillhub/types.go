package skillhub

import "encoding/json"

// RegistryIndex is the top-level structure of registry.json.
type RegistryIndex struct {
	Version    int             `json:"version"`
	Skills     []RegistryEntry `json:"skills"`
	Plugins    []PluginEntry   `json:"plugins,omitempty"`
	MCPServers []MCPEntry      `json:"mcp_servers,omitempty"`
}

// RegistryFiles accepts both legacy path arrays and checksum-bearing file objects.
type RegistryFiles []RegistryFile

func (files *RegistryFiles) UnmarshalJSON(data []byte) error {
	var legacy []string
	if err := json.Unmarshal(data, &legacy); err == nil {
		normalized := make(RegistryFiles, 0, len(legacy))
		for _, relPath := range legacy {
			normalized = append(normalized, RegistryFile{Path: relPath})
		}
		*files = normalized
		return nil
	}

	var manifest []RegistryFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	*files = RegistryFiles(manifest)
	return nil
}

func (files RegistryFiles) Paths() []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

// PluginEntry describes a plugin in the registry.
type PluginEntry struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Version     string        `json:"version"`
	Author      string        `json:"author"`
	Tags        []string      `json:"tags"`
	Path        string        `json:"path"`
	Files       RegistryFiles `json:"files"`
}

// RegistryFile describes a downloadable file and its checksum.
type RegistryFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// RegistryEntry describes a single skill in the registry.
type RegistryEntry struct {
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Version        string        `json:"version"`
	Author         string        `json:"author"`
	Tags           []string      `json:"tags"`
	Path           string        `json:"path"`
	UserInvocable  bool          `json:"user_invocable"`
	RequiresPlugin string        `json:"requires_plugin,omitempty"`
	Files          RegistryFiles `json:"files,omitempty"`
}

// MCPEntry describes a managed MCP package in the registry.
type MCPEntry struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Author      string         `json:"author"`
	Tags        []string       `json:"tags"`
	Path        string         `json:"path"`
	Manifest    string         `json:"manifest,omitempty"`
	Files       []RegistryFile `json:"files"`
}

// InstalledSkill tracks a skill that has been installed locally.
type InstalledSkill struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"` // "tars-hub" or "openclaw"
	Dir     string `json:"dir"`
}

// InstalledPlugin tracks a plugin that has been installed locally.
type InstalledPlugin struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Dir     string `json:"dir"`
}

// InstalledMCP tracks a managed MCP package that has been installed locally.
type InstalledMCP struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Source   string `json:"source"`
	Dir      string `json:"dir"`
	Manifest string `json:"manifest"`
}
