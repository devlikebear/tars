package tool

import "strings"

var toolNameAliases = map[string]string{
	"shell_execute": "exec",
	"shell_exec":    "exec",
	"run_command":   "exec",
	"terminal_exec": "exec",
	"execute_shell": "exec",
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
