package plugin

import "github.com/devlikebear/tarsncase/internal/config"

type ServerConfig = config.MCPServer

type Source string

const (
	SourceWorkspace Source = "workspace"
	SourceUser      Source = "user"
	SourceBundled   Source = "bundled"
)

type Manifest struct {
	ID          string             `json:"id"`
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Version     string             `json:"version,omitempty"`
	Skills      []string           `json:"skills,omitempty"`
	MCPServers  []config.MCPServer `json:"mcp_servers,omitempty"`
}

type Definition struct {
	ID           string             `json:"id"`
	Name         string             `json:"name,omitempty"`
	Description  string             `json:"description,omitempty"`
	Version      string             `json:"version,omitempty"`
	Source       Source             `json:"source"`
	RootDir      string             `json:"root_dir"`
	ManifestPath string             `json:"manifest_path"`
	Skills       []string           `json:"skills,omitempty"`
	MCPServers   []config.MCPServer `json:"mcp_servers,omitempty"`
}

type Diagnostic struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type Snapshot struct {
	Version     int64              `json:"version"`
	Plugins     []Definition       `json:"plugins"`
	SkillDirs   []string           `json:"skill_dirs,omitempty"`
	MCPServers  []config.MCPServer `json:"mcp_servers,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

type SourceDir struct {
	Source Source
	Dir    string
}

type LoadOptions struct {
	Sources []SourceDir
}
