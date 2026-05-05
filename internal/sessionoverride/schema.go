// Package sessionoverride loads and merges per-session-cwd configuration
// overrides from `.tars/settings.json` (team-shared) and
// `.tars/settings.local.json` (per-user, gitignore-recommended).
//
// Only an explicit allow-list of fields is honoured — anything else is
// dropped with a diagnostic so a malicious or accidental local file cannot
// rebind credentials, register hooks, or otherwise widen the trust boundary.
package sessionoverride

import "github.com/devlikebear/tars/internal/session"

// Source identifies which layer contributed the effective value of a given
// configuration field. The layers are ordered base < shared < local; later
// values win on conflict.
type Source string

const (
	// SourceBase corresponds to the value persisted on the session itself
	// (sessions.json), or the system default when the session has not yet
	// configured anything.
	SourceBase Source = "base"

	// SourceShared corresponds to `<cwd>/.tars/settings.json` — intended to
	// be checked into the project repo and shared across the team.
	SourceShared Source = "shared"

	// SourceLocal corresponds to `<cwd>/.tars/settings.local.json` — intended
	// to be gitignored and contain personal preferences.
	SourceLocal Source = "local"
)

// MCPServerExtra is the narrowed schema permitted for an MCP server entry
// declared in a session-cwd override file. Credentials and arbitrary env
// passthroughs are intentionally absent; if a project needs those they
// belong in the user-global config, not in a checked-in override file.
type MCPServerExtra struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Override captures the parsed contents of one settings file
// (`.tars/settings.json` or `.tars/settings.local.json`). All fields are
// pointers / nilable so the merger can distinguish "explicitly set to the
// zero value" from "not set at all".
type Override struct {
	ToolConfig        *session.SessionToolConfig `json:"tool_config,omitempty"`
	PromptOverride    *string                    `json:"prompt_override,omitempty"`
	MCPServersExtra   []MCPServerExtra           `json:"mcp_servers_extra,omitempty"`
	ModelTierOverride *string                    `json:"model_tier_override,omitempty"`

	// Presence records every override path the file explicitly touched.
	// Keys are dotted paths (e.g. "tool_config.tools_enabled"); the loader
	// populates this so the merger knows whether to use this layer's value
	// for that path. Not serialized.
	Presence map[string]bool `json:"-"`
}

// AllowedTopLevelFields enumerates JSON keys honoured at the top level of
// a settings override file. Anything outside this set is dropped with a
// diagnostic.
var AllowedTopLevelFields = map[string]struct{}{
	"tool_config":         {},
	"prompt_override":     {},
	"mcp_servers_extra":   {},
	"model_tier_override": {},
}

// BlockedTopLevelFields enumerates JSON keys that, if present, generate a
// SeverityError diagnostic instead of the more neutral SeverityWarn used
// for unknown keys. These are fields known to be sensitive (credentials,
// authority widening) and must never be silently ignored.
var BlockedTopLevelFields = map[string]struct{}{
	"llm_providers":  {},
	"api_key":        {},
	"auth":           {},
	"auth_token":     {},
	"hooks":          {},
	"server_command": {},
}

// AllowedToolConfigFields enumerates the JSON keys honoured inside the
// `tool_config` object. Mirrors session.SessionToolConfig. `skills_disabled`
// is intentionally absent for now — Phase 4 (skill registry) will add it
// once the consumer side knows what to do with the list.
var AllowedToolConfigFields = map[string]struct{}{
	"tools_enabled":      {},
	"tools_custom":       {},
	"tools_disabled":     {},
	"tools_allow_groups": {},
	"tools_deny_groups":  {},
	"skills_enabled":     {},
	"skills_custom":      {},
	"commands_enabled":   {},
	"commands_custom":    {},
	"mcp_enabled":        {},
}

// AllPaths enumerates every leaf override path the merger tracks in its
// sources map. Phase 6's UI uses these to render badges per item.
func AllPaths() []string {
	return []string{
		"prompt_override",
		"mcp_servers_extra",
		"model_tier_override",
		"tool_config.tools_enabled",
		"tool_config.tools_custom",
		"tool_config.tools_disabled",
		"tool_config.tools_allow_groups",
		"tool_config.tools_deny_groups",
		"tool_config.skills_enabled",
		"tool_config.skills_custom",
		"tool_config.commands_enabled",
		"tool_config.commands_custom",
		"tool_config.mcp_enabled",
	}
}

// Severity classifies a Diagnostic.
type Severity string

const (
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Diagnostic describes a single issue noticed while loading a settings
// override file. Diagnostics never fail the load — they are surfaced to
// the operator via API + logs so they can fix the file.
type Diagnostic struct {
	Path     string   `json:"path"`     // dotted JSON path; e.g. "llm_providers"
	Severity Severity `json:"severity"` // "warn" | "error"
	Message  string   `json:"message"`  // human-readable explanation
	File     string   `json:"file"`     // absolute path to the file that produced it
}

// EffectiveConfig is the merger output: a flattened, fully-resolved view of
// what the chat turn / skill registry / etc. should use.
type EffectiveConfig struct {
	ToolConfig        session.SessionToolConfig `json:"tool_config"`
	PromptOverride    string                    `json:"prompt_override"`
	MCPServersExtra   []MCPServerExtra          `json:"mcp_servers_extra,omitempty"`
	ModelTierOverride string                    `json:"model_tier_override,omitempty"`
}
