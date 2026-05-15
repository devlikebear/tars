package tarsserver

import (
	"strings"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
)

// effectiveSessionView returns the tool_config and prompt_override that the
// chat turn should use, after folding `<cwd>/.tars/settings*.json` overrides
// (when the override service is available) into the raw session fields.
//
// The boolean return indicates whether the result reflects a meaningful
// configuration: false when no base tool_config exists and the override
// service either was not configured or could not resolve a settings file.
// Callers use it to decide whether to feed the result into downstream
// gating helpers that prefer to receive zero arguments rather than empty
// inputs.
func effectiveSessionView(svc *sessionoverride.Service, sess session.Session) (session.SessionToolConfig, string, bool) {
	base := session.SessionToolConfig{}
	hasBase := false
	if sess.ToolConfig != nil {
		base = *sess.ToolConfig
		hasBase = true
	}
	if svc == nil {
		return base, sess.PromptOverride, hasBase
	}
	res, _, err := svc.Resolve(sess.ID)
	if err != nil {
		return base, sess.PromptOverride, hasBase
	}
	hasOverride := false
	for _, src := range res.Sources {
		if src != sessionoverride.SourceBase {
			hasOverride = true
			break
		}
	}
	return res.Effective.ToolConfig, res.Effective.PromptOverride, hasBase || hasOverride
}

// effectiveClaudeCodePermissionMode resolves the per-session
// `.tars/settings*.json` value of `claude_code_cli_permission_mode` (if any)
// and falls back to the global config value otherwise. Returns an empty
// string only when both are empty; the claude-code-cli provider degrades
// empty / unknown values to "auto" so an empty return is safe for callers.
//
// Trimmed whitespace inside the override file is treated as "not set" so a
// user can deliberately clear a shared override locally by setting the value
// to "   " in `.tars/settings.local.json` is NOT supported — explicit empty
// strings (`""`) are honored as "not set" via the loader's omitempty path.
func effectiveClaudeCodePermissionMode(svc *sessionoverride.Service, sess session.Session, fallback string) string {
	if svc != nil {
		if res, _, err := svc.Resolve(sess.ID); err == nil {
			if v := strings.TrimSpace(res.Effective.ClaudeCodeCLIPermissionMode); v != "" {
				return v
			}
		}
	}
	return strings.TrimSpace(fallback)
}

// effectiveClaudeCodePermissionDeny resolves the merged per-session
// `claude_code_cli_permission_deny` rule list from `.tars/settings*.json`.
// There is no global config fallback by design: deny rules are a per-session
// guardrail mechanism, and the merger already unions shared+local layers
// (tightening-only). Returns nil when the override service is unavailable or
// the session has no deny rules, in which case the provider skips --settings.
func effectiveClaudeCodePermissionDeny(svc *sessionoverride.Service, sess session.Session) []string {
	if svc == nil {
		return nil
	}
	res, _, err := svc.Resolve(sess.ID)
	if err != nil {
		return nil
	}
	return res.Effective.ClaudeCodeCLIPermissionDeny
}
