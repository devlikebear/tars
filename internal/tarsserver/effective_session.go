package tarsserver

import (
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
