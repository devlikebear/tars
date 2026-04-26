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
	"read":  "read_file",
	"write": "write_file",
	"edit":  "edit_file",

	// memory aliases → memory aggregator
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
