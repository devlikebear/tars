package tool

import "strings"

var toolNameAliases = map[string]string{
	"shell_execute":   "exec",
	"shell_exec":      "exec",
	"run_command":     "exec",
	"terminal_exec":   "exec",
	"execute_shell":   "exec",
	"session_list":    "sessions_list",
	"session_history": "sessions_history",
	"session_send":    "sessions_send",
	"session_spawn":   "sessions_spawn",
	"session_runs":    "sessions_runs",
	"agent_runs":      "sessions_runs",
	"gateway_status":  "gateway",
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
