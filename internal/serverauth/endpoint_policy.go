package serverauth

import (
	"net/http"
	"strings"
)

// EndpointAccess describes the minimum browser-session role needed for an API
// endpoint. Unknown endpoints default to admin so the remote user surface is
// fail-closed as new routes are added.
type EndpointAccess string

const (
	EndpointAccessPublic EndpointAccess = "public"
	EndpointAccessUser   EndpointAccess = "user"
	EndpointAccessAdmin  EndpointAccess = "admin"
)

// EndpointPolicyRule is the remote browser allowlist skeleton. It is not wired
// into middleware yet; PR1b will use this table when cookie auth lands.
type EndpointPolicyRule struct {
	Methods []string
	Pattern string
	Access  EndpointAccess
	Note    string
}

var defaultEndpointPolicyRules = []EndpointPolicyRule{
	publicRule([]string{http.MethodGet}, "/v1/healthz", "health probe"),
	publicRule([]string{http.MethodGet}, "/v1/setup/status", "setup status probe"),

	userRule([]string{http.MethodGet}, "/v1/status", "status summary"),
	userRule([]string{http.MethodGet}, "/v1/auth/whoami", "browser auth status"),
	userRule([]string{http.MethodPost}, "/v1/auth/logout", "browser logout"),
	userRule([]string{http.MethodPatch}, "/v1/auth/users/user/password", "user password change"),
	userRule([]string{http.MethodGet}, "/v1/providers", "provider metadata"),
	userRule([]string{http.MethodGet}, "/v1/models", "model metadata"),

	userRule([]string{http.MethodGet, http.MethodPost}, "/v1/chat", "chat stream"),
	userRule([]string{http.MethodGet, http.MethodPost}, "/v1/chat/*", "chat helpers"),
	userRule([]string{http.MethodGet}, "/v1/sessions", "session public alias"),
	userRule([]string{http.MethodGet, http.MethodPatch}, "/v1/sessions/:id", "session public alias"),
	userRule([]string{http.MethodGet}, "/v1/sessions/:id/history", "session public alias history"),
	userRule([]string{http.MethodGet, http.MethodPost}, "/v1/admin/sessions", "console session list/create"),
	userRule([]string{http.MethodGet, http.MethodPatch, http.MethodDelete}, "/v1/admin/sessions/:id", "console session metadata"),
	userRule([]string{http.MethodGet}, "/v1/admin/sessions/:id/history", "console session history"),
	userRule([]string{http.MethodPost}, "/v1/admin/sessions/:id/fork", "console session fork"),
	userRule([]string{http.MethodGet, http.MethodPost}, "/v1/admin/sessions/:id/promotions", "fork promotion review"),
	userRule([]string{http.MethodPost}, "/v1/admin/sessions/:id/compact", "manual session compaction"),
	userRule([]string{http.MethodGet, http.MethodPost}, "/v1/admin/sessions/:id/tasks", "session task workflow"),
	userRule([]string{http.MethodGet}, "/v1/admin/sessions/:id/plans/archive", "session plan archive"),
	userRule([]string{http.MethodGet}, "/v1/admin/sessions/:id/config", "session config read"),
	userRule([]string{http.MethodGet}, "/v1/admin/sessions/:id/effective-config", "effective session config read"),
	userRule([]string{http.MethodGet, http.MethodPut}, "/v1/admin/sessions/:id/cwd", "active cwd switch among eligible directories"),
	userRule([]string{http.MethodGet}, "/v1/admin/sessions/:id/workdirs", "eligible workdir read"),
	userRule([]string{http.MethodGet, http.MethodPut}, "/v1/admin/sessions/:id/prompt", "session prompt override"),
	userRule([]string{http.MethodGet}, "/v1/admin/sessions/:id/automation-consent", "automation consent read"),
	userRule([]string{http.MethodGet, http.MethodPatch}, "/v1/admin/sessions/:id/style", "session style control"),
	userRule([]string{http.MethodGet}, "/v1/admin/tasks", "global plan list"),
	userRule([]string{http.MethodGet}, "/v1/admin/plans/archive", "plan archive"),

	userRule(nil, "/v1/memory/assets", "memory assets"),
	userRule(nil, "/v1/memory/file", "memory file"),
	userRule(nil, "/v1/memory/search", "memory search"),
	userRule(nil, "/v1/memory/prefetch", "memory prefetch"),
	userRule(nil, "/v1/memory/inbox", "memory inbox"),
	userRule(nil, "/v1/memory/inbox/review", "memory inbox review"),
	userRule(nil, "/v1/workspace/sysprompt/files", "workspace sysprompt files"),
	userRule(nil, "/v1/workspace/sysprompt/file", "workspace sysprompt file"),
	userRule([]string{http.MethodGet}, "/v1/admin/sysprompt/preview", "sysprompt read-only preview"),

	userRule([]string{http.MethodGet}, "/v1/usage/summary", "usage summary"),
	userRule([]string{http.MethodGet}, "/v1/usage/limits", "usage limits"),
	userRule([]string{http.MethodGet}, "/v1/admin/usage/today", "usage today"),
	userRule([]string{http.MethodGet}, "/v1/admin/analytics", "analytics summary"),
	userRule([]string{http.MethodGet}, "/v1/events/stream", "event stream"),
	userRule([]string{http.MethodGet}, "/v1/events/history", "event history"),
	userRule([]string{http.MethodPost}, "/v1/events/read", "event read cursor"),

	userRule([]string{http.MethodGet}, "/v1/skills", "installed skills read"),
	userRule([]string{http.MethodGet}, "/v1/skills/*", "skill detail read"),
	userRule([]string{http.MethodGet}, "/v1/plugins", "installed plugins read"),
	userRule([]string{http.MethodGet}, "/v1/mcp/servers", "mcp server read"),
	userRule([]string{http.MethodGet}, "/v1/mcp/tools", "mcp tool read"),
	userRule([]string{http.MethodGet}, "/v1/hub/registry", "skill hub registry read"),
	userRule([]string{http.MethodGet}, "/v1/hub/installed", "skill hub installed read"),
	userRule([]string{http.MethodGet}, "/v1/hub/skill-content", "skill hub content read"),

	userRule([]string{http.MethodGet}, "/v1/agentruntime/agents", "agent runtime agents read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/runs", "agent runtime run list"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/runs/:id", "agent runtime run read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/runs/:id/events", "agent runtime run event stream"),
	userRule([]string{http.MethodGet}, "/v1/agent/agents", "legacy agent runtime agents read"),
	userRule([]string{http.MethodGet}, "/v1/agent/runs", "legacy agent runtime run list"),
	userRule([]string{http.MethodGet}, "/v1/agent/runs/:id", "legacy agent runtime run read"),
	userRule([]string{http.MethodGet}, "/v1/agent/runs/:id/events", "legacy agent runtime run event stream"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/subagents", "subagent read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/subagents/:name", "subagent read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/status", "agent runtime status read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/reports/summary", "agent runtime report read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/reports/runs", "agent runtime report read"),
	userRule([]string{http.MethodGet}, "/v1/agentruntime/reports/channels", "agent runtime report read"),

	userRule([]string{http.MethodPost}, "/v1/compact", "manual compaction"),
	userRule([]string{http.MethodGet, http.MethodPost, http.MethodPatch}, "/v1/workspace/files", "workspace-scoped file access"),
	userRule([]string{http.MethodGet, http.MethodPost, http.MethodPatch}, "/v1/workspace/files/*", "workspace-scoped file access"),
}

func publicRule(methods []string, pattern, note string) EndpointPolicyRule {
	return EndpointPolicyRule{Methods: methods, Pattern: pattern, Access: EndpointAccessPublic, Note: note}
}

func userRule(methods []string, pattern, note string) EndpointPolicyRule {
	return EndpointPolicyRule{Methods: methods, Pattern: pattern, Access: EndpointAccessUser, Note: note}
}

func DefaultEndpointPolicyRules() []EndpointPolicyRule {
	out := make([]EndpointPolicyRule, len(defaultEndpointPolicyRules))
	for i, rule := range defaultEndpointPolicyRules {
		out[i] = rule
		if len(rule.Methods) > 0 {
			out[i].Methods = append([]string(nil), rule.Methods...)
		}
	}
	return out
}

func ResolveEndpointAccess(method, path string) EndpointAccess {
	method = normalizeEndpointMethod(method)
	path = normalizeEndpointPath(path)
	for _, rule := range defaultEndpointPolicyRules {
		if endpointMethodsMatch(method, rule.Methods) && endpointPatternMatches(rule.Pattern, path) {
			return rule.Access
		}
	}
	return EndpointAccessAdmin
}

func EndpointAllowsRole(method, path, role string) bool {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case RoleAdmin:
		return true
	case RoleUser:
		access := ResolveEndpointAccess(method, path)
		return access == EndpointAccessUser || access == EndpointAccessPublic
	case "":
		return ResolveEndpointAccess(method, path) == EndpointAccessPublic
	default:
		return false
	}
}

func endpointMethodsMatch(method string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		normalized := normalizeEndpointMethod(candidate)
		if normalized == method {
			return true
		}
		if method == http.MethodHead && normalized == http.MethodGet {
			return true
		}
	}
	return false
}

func endpointPatternMatches(pattern, path string) bool {
	pattern = normalizeEndpointPath(pattern)
	path = normalizeEndpointPath(path)
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return prefix != "" && strings.HasPrefix(path, prefix)
	}
	if strings.Contains(pattern, ":") {
		return endpointSegmentPatternMatches(pattern, path)
	}
	return path == pattern
}

func endpointSegmentPatternMatches(pattern, path string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if len(patternParts) != len(pathParts) {
		return false
	}
	for idx, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, ":") {
			if pathParts[idx] == "" {
				return false
			}
			continue
		}
		if patternPart != pathParts[idx] {
			return false
		}
	}
	return true
}

func normalizeEndpointMethod(method string) string {
	trimmed := strings.TrimSpace(method)
	if trimmed == "" {
		return http.MethodGet
	}
	return strings.ToUpper(trimmed)
}

func normalizeEndpointPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/"
	}
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return trimmed
}
