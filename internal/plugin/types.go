package plugin

import "github.com/devlikebear/tars/internal/config"

type ServerConfig = config.MCPServer

type Requires struct {
	Bins []string `json:"bins,omitempty"`
	Env  []string `json:"env,omitempty"`
}

type Policies struct {
	ToolsAllow []string `json:"tools_allow,omitempty"`
	ToolsDeny  []string `json:"tools_deny,omitempty"`
}

type Source string

const (
	SourceWorkspace Source = "workspace"
	SourceUser      Source = "user"
	SourceBundled   Source = "bundled"
)

type Manifest struct {
	SchemaVersion         int                `json:"schema_version,omitempty"`
	ID                    string             `json:"id"`
	Name                  string             `json:"name,omitempty"`
	Description           string             `json:"description,omitempty"`
	Version               string             `json:"version,omitempty"`
	Skills                []string           `json:"skills,omitempty"`
	MCPServers            []config.MCPServer `json:"mcp_servers,omitempty"`
	Requires              Requires           `json:"requires,omitempty"`
	SupportedOS           []string           `json:"supported_os,omitempty"`
	SupportedArch         []string           `json:"supported_arch,omitempty"`
	DefaultProjectProfile string             `json:"default_project_profile,omitempty"`
	Policies              Policies           `json:"policies,omitempty"`
}

type Definition struct {
	SchemaVersion         int                `json:"schema_version,omitempty"`
	ID                    string             `json:"id"`
	Name                  string             `json:"name,omitempty"`
	Description           string             `json:"description,omitempty"`
	Version               string             `json:"version,omitempty"`
	Source                Source             `json:"source"`
	RootDir               string             `json:"root_dir"`
	ManifestPath          string             `json:"manifest_path"`
	Skills                []string           `json:"skills,omitempty"`
	MCPServers            []config.MCPServer `json:"mcp_servers,omitempty"`
	Requires              Requires           `json:"requires,omitempty"`
	SupportedOS           []string           `json:"supported_os,omitempty"`
	SupportedArch         []string           `json:"supported_arch,omitempty"`
	DefaultProjectProfile string             `json:"default_project_profile,omitempty"`
	Policies              Policies           `json:"policies,omitempty"`
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

type AvailabilityOptions struct {
	OS         string
	Arch       string
	HasEnv     func(string) bool
	HasCommand func(string) bool
}

type LoadOptions struct {
	Sources      []SourceDir
	Availability AvailabilityOptions
}
