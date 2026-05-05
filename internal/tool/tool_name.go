package tool

import "strings"

var toolNameAliases = map[string]string{
	// exec aliases
	"shell_execute": "exec",
	"shell_exec":    "exec",
	"run_command":   "exec",
	"terminal_exec": "exec",
	"execute_shell": "exec",

	// file I/O aliases (short → canonical)
	"read":                  "read_file",
	"write":                 "write_file",
	"edit":                  "edit_file",
	"skill_create":          "project_skill",
	"slash_command_create":  "project_skill",
	"project_command_write": "project_skill",

	// memory aliases → memory aggregator
	"knowledge":     "memory",
	"memory_save":   "memory",
	"memory_search": "memory",
	"memory_get":    "memory",

	// sysprompt aliases → workspace aggregator
	"workspace_sysprompt_get": "workspace",
	"workspace_sysprompt_set": "workspace",
	"agent_sysprompt_get":     "workspace",
	"agent_sysprompt_set":     "workspace",

	// session aliases → session aggregator
	"session_list":     "session",
	"session_history":  "session",
	"session_send":     "session",
	"session_spawn":    "session",
	"session_runs":     "session",
	"sessions_list":    "session",
	"sessions_history": "session",
	"sessions_send":    "session",
	"sessions_spawn":   "session",
	"sessions_runs":    "session",
	"agents_list":      "session",
	"session_status":   "session",
	"agent_runs":       "session",

	// other
	"subagent_plan":        "subagents_plan",
	"subagent_run":         "subagents_run",
	"subagent_orchestrate": "subagents_orchestrate",
}

func CanonicalToolName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return ""
	}
	if canonical, ok := toolNameAliases[normalized]; ok {
		return canonical
	}
	return normalized
}

func ToolNameAliases() map[string]string {
	out := make(map[string]string, len(toolNameAliases))
	for alias, canonical := range toolNameAliases {
		out[alias] = canonical
	}
	return out
}

func IsExecToolName(name string) bool {
	return CanonicalToolName(name) == "exec"
}

// IsHighRiskToolName reports whether a tool's failure or invocation has
// non-trivial side effects (mutating, executing, or reaching outside the
// workspace). Used both at chat policy enforcement and by pulse auto-resume
// to refuse retrying a failed turn whose last action could have already
// mutated state — re-running it could double-apply.
func IsHighRiskToolName(name string) bool {
	canonical := CanonicalToolName(name)
	if canonical == "" {
		return false
	}
	switch canonical {
	case "exec", "process", "write_file", "edit_file", "apply_patch", "workspace", "project_skill":
		return true
	}
	return strings.HasPrefix(canonical, "write_") || strings.HasPrefix(canonical, "edit_")
}
