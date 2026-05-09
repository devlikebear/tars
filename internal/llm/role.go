package llm

import (
	"slices"
	"strings"
)

// Role identifies a semantic caller of the LLM layer. Role → Tier is a
// many-to-one mapping controlled by config; code references Role constants
// and never concrete tier names, so that reassigning a role to a different
// tier is a config-only change.
type Role string

const (
	// RoleChatMain is the user-facing chat handler that streams responses
	// to the console/API.
	RoleChatMain Role = "chat_main"

	// RoleContextCompactor runs background transcript compaction and
	// compaction-memory extraction. Light by default.
	RoleContextCompactor Role = "context_compactor"

	// RoleMemoryHook runs per-turn memory maintenance (daily log append,
	// explicit "remember ..." hot path). Light by default.
	RoleMemoryHook Role = "memory_hook"

	// RoleReflectionMemory was the nightly knowledge-base compilation
	// role. The KB system was removed in ID-001; this role is kept in
	// the enumeration for backward-compatible config parsing but no
	// production code resolves it anymore.
	RoleReflectionMemory Role = "reflection_memory"

	// RoleReflectionKB runs nightly KB cleanup. Currently non-LLM, reserved
	// for future use.
	RoleReflectionKB Role = "reflection_kb"

	// RolePulseDecider is the pulse watchdog classifier. Light by default.
	RolePulseDecider Role = "pulse_decider"

	// RoleSessionCleanup analyzes compact session metadata and transcript
	// snippets for user-reviewed archive/delete cleanup suggestions.
	RoleSessionCleanup Role = "session_cleanup"

	// RoleAgentRuntimeDefault is the default executor role for agent runtime agents
	// that do not declare a tier explicitly. Standard by default.
	RoleAgentRuntimeDefault Role = "agentruntime_default"

	// RoleAgentRuntimePlanner is reserved for planner-style agents that benefit
	// from heavy reasoning.
	RoleAgentRuntimePlanner Role = "agentruntime_planner"
)

// AllRoles returns the exhaustive list of roles in canonical order.
// Used for config validation and defaulting.
func AllRoles() []Role {
	return []Role{
		RoleChatMain,
		RoleContextCompactor,
		RoleMemoryHook,
		RoleReflectionMemory,
		RoleReflectionKB,
		RolePulseDecider,
		RoleSessionCleanup,
		RoleAgentRuntimeDefault,
		RoleAgentRuntimePlanner,
	}
}

// String returns the canonical string form of the role.
func (r Role) String() string { return string(r) }

// Valid reports whether r is one of the known role constants.
func (r Role) Valid() bool {
	return slices.Contains(AllRoles(), r)
}

// ParseRole parses a role name (case-insensitive, trimmed). Returns the
// zero value and false when the role is unknown.
func ParseRole(raw string) (Role, bool) {
	normalized := Role(strings.ToLower(strings.TrimSpace(raw)))
	if normalized.Valid() {
		return normalized, true
	}
	return "", false
}
