package tarsserver

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
)

type sessionStyleValues struct {
	Directness int `json:"directness"`
	Humor      int `json:"humor"`
	Caution    int `json:"caution"`
	Autonomy   int `json:"autonomy"`
}

type sessionStyleResponse struct {
	StyleControl *session.SessionStyleControl `json:"style_control,omitempty"`
	Effective    sessionStyleValues           `json:"effective"`
	Defaults     sessionStyleValues           `json:"defaults"`
	Preview      []string                     `json:"preview"`
}

func sessionStyleDefaultsFromConfig(cfg config.Config) sessionStyleValues {
	return sessionStyleValues{
		Directness: clampSessionStyleScore(cfg.StyleDirectnessDefault),
		Humor:      clampSessionStyleScore(cfg.StyleHumorDefault),
		Caution:    clampSessionStyleScore(cfg.StyleCautionDefault),
		Autonomy:   clampSessionStyleScore(cfg.StyleAutonomyDefault),
	}
}

func effectiveSessionStyle(defaults sessionStyleValues, style *session.SessionStyleControl) sessionStyleValues {
	effective := sessionStyleValues{
		Directness: normalizeSessionStyleDefault(defaults.Directness, 70),
		Humor:      normalizeSessionStyleDefault(defaults.Humor, 20),
		Caution:    normalizeSessionStyleDefault(defaults.Caution, 60),
		Autonomy:   normalizeSessionStyleDefault(defaults.Autonomy, 40),
	}
	if style == nil {
		return effective
	}
	normalized := session.NormalizeStyleControl(style)
	if normalized.Directness != nil {
		effective.Directness = *normalized.Directness
	}
	if normalized.Humor != nil {
		effective.Humor = *normalized.Humor
	}
	if normalized.Caution != nil {
		effective.Caution = *normalized.Caution
	}
	if normalized.Autonomy != nil {
		effective.Autonomy = *normalized.Autonomy
	}
	return effective
}

func buildSessionStyleResponse(defaults sessionStyleValues, sess session.Session) sessionStyleResponse {
	effective := effectiveSessionStyle(defaults, sess.StyleControl)
	return sessionStyleResponse{
		StyleControl: sess.StyleControl,
		Effective:    effective,
		Defaults:     effectiveSessionStyle(defaults, nil),
		Preview:      sessionStylePreview(effective),
	}
}

func formatSessionStylePrompt(style sessionStyleValues, consent *session.SessionAutomationConsent) string {
	style = clampSessionStyleValues(style)
	lines := []string{
		"## TARS Style Controls",
		fmt.Sprintf("- Directness %d/100: %s.", style.Directness, directnessPrompt(style.Directness)),
		fmt.Sprintf("- Humor %d/100: %s.", style.Humor, humorPrompt(style.Humor)),
		fmt.Sprintf("- Caution %d/100: %s.", style.Caution, cautionPrompt(style.Caution)),
		fmt.Sprintf("- Autonomy %d/100: %s.", style.Autonomy, autonomyPrompt(style.Autonomy)),
	}
	if consent != nil && consent.AllowsAutoResume() {
		lines = append(lines, fmt.Sprintf("- Auto-resume is permitted after %d minutes for modes: %s.",
			consent.EffectiveAutoResumeAfterMinutes(),
			strings.Join(consent.EffectiveAllowedResumeModes(), ", "),
		))
	} else {
		lines = append(lines, "- Auto-resume is disabled unless the session explicitly enables it.")
	}
	if consent != nil && consent.AllowsAutonomousMutation() {
		lines = append(lines, "- Autonomous workspace mutation consent is enabled, but destructive work still needs the configured approval flow.")
	} else {
		lines = append(lines, "- Autonomous workspace mutation consent is disabled.")
	}
	lines = append(lines, "- These settings never override tool permissions, approval requirements, or session automation consent.")
	return "\n\n" + strings.Join(lines, "\n") + "\n"
}

func sessionStylePreview(style sessionStyleValues) []string {
	style = clampSessionStyleValues(style)
	return []string{
		fmt.Sprintf("%s; %s.", directnessPreview(style.Directness), humorPreview(style.Humor)),
		fmt.Sprintf("%s; autonomy stays bounded by explicit consent.", cautionPreview(style.Caution)),
	}
}

func normalizeSessionStyleDefault(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return clampSessionStyleScore(value)
}

func clampSessionStyleValues(style sessionStyleValues) sessionStyleValues {
	return sessionStyleValues{
		Directness: clampSessionStyleScore(style.Directness),
		Humor:      clampSessionStyleScore(style.Humor),
		Caution:    clampSessionStyleScore(style.Caution),
		Autonomy:   clampSessionStyleScore(style.Autonomy),
	}
}

func clampSessionStyleScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func directnessPrompt(value int) string {
	if value >= 70 {
		return "state conclusions early, keep tradeoffs explicit, and avoid unnecessary preamble"
	}
	if value <= 30 {
		return "prefer a gentle, exploratory tone before firm recommendations"
	}
	return "balance concise answers with enough context for the user to steer"
}

func humorPrompt(value int) string {
	if value >= 70 {
		return "light humor is welcome when it does not obscure the work"
	}
	if value <= 30 {
		return "keep humor rare and understated"
	}
	return "use occasional warmth without turning technical work into banter"
}

func cautionPrompt(value int) string {
	if value >= 70 {
		return "verify assumptions, name risks, and ask before irreversible choices"
	}
	if value <= 30 {
		return "move quickly through reversible work and keep caveats brief"
	}
	return "call out meaningful uncertainty without stalling routine progress"
}

func autonomyPrompt(value int) string {
	if value >= 70 {
		return "continue through reversible next steps once the task is clear"
	}
	if value <= 30 {
		return "pause more often for user confirmation when direction is ambiguous"
	}
	return "make reasonable assumptions for low-risk steps and report them clearly"
}

func directnessPreview(value int) string {
	if value >= 70 {
		return "direct answers first"
	}
	if value <= 30 {
		return "softer exploratory answers"
	}
	return "balanced directness"
}

func humorPreview(value int) string {
	if value <= 30 {
		return "rare humor"
	}
	if value >= 70 {
		return "warmer humor"
	}
	return "occasional warmth"
}

func cautionPreview(value int) string {
	if value >= 70 {
		return "more verify-before-act behavior"
	}
	if value <= 30 {
		return "fewer caveats on reversible work"
	}
	return "moderate risk checks"
}
