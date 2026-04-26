package plugin

import (
	"encoding/json"

	"github.com/devlikebear/tars/internal/config"
)

type ServerConfig = config.MCPServer

type Requires struct {
	Bins []string `json:"bins,omitempty"`
	Env  []string `json:"env,omitempty"`
}

type Policies struct {
	ToolsAllow []string `json:"tools_allow,omitempty"`
	ToolsDeny  []string `json:"tools_deny,omitempty"`
}

// ToolsProvider declares how a plugin provides tools (v3+).
type ToolsProvider struct {
	Type  string `json:"type"`            // "mcp_server", "go_plugin", or "script"
	Entry string `json:"entry,omitempty"` // entrypoint path or command
}

// LifecycleHook describes a single builtin-tool invocation to run at
// server start or stop. Tool must be a non-empty name registered in the
// user-surface tool registry; it must NOT match the lifecycle deny-list
// (bash, exec, shell_exec, process — anything that would re-open the
// arbitrary-shell-command door the legacy string-based format allowed).
type LifecycleHook struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Lifecycle declares typed tool invocations to run at server start or
// stop. The previous string-based "sh -c <cmd>" form was removed
// (RF-008): a plugin manifest that ships with malicious on_start /
// on_stop strings could execute any command in the TARS process user's
// environment, including reading vault tokens, ~/.aws, etc. Manifests
// that still use the legacy string format are rejected at parse time
// with a clear diagnostic so operators can migrate.
type Lifecycle struct {
	OnStart *LifecycleHook `json:"on_start,omitempty"`
	OnStop  *LifecycleHook `json:"on_stop,omitempty"`
}

// HTTPRoute declares an HTTP endpoint a plugin handles (v3+).
type HTTPRoute struct {
	Path    string `json:"path"`
	Handler string `json:"handler,omitempty"`
}

type Source string

const (
	SourceWorkspace Source = "workspace"
	SourceUser      Source = "user"
	SourceBundled   Source = "bundled"
)

// Priority returns the merge priority of a source. Higher wins on conflict
// when Load merges plugin definitions: workspace > user > bundled. Unknown
// sources have priority 0 (bundled-or-lower) so adding a new value without
// updating this method does not silently outrank existing sources.
func (s Source) Priority() int {
	switch s {
	case SourceWorkspace:
		return 3
	case SourceUser:
		return 2
	case SourceBundled:
		return 1
	default:
		return 0
	}
}

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
	ToolsProvider         *ToolsProvider     `json:"tools_provider,omitempty"`
	Lifecycle             *Lifecycle         `json:"lifecycle,omitempty"`
	HTTPRoutes            []HTTPRoute        `json:"http_routes,omitempty"`
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
	ToolsProvider         *ToolsProvider     `json:"tools_provider,omitempty"`
	Lifecycle             *Lifecycle         `json:"lifecycle,omitempty"`
	HTTPRoutes            []HTTPRoute        `json:"http_routes,omitempty"`
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
