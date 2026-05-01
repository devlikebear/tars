package tarsserver

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func TestSessionStylePromptLimitsAutonomyByConsent(t *testing.T) {
	style := sessionStyleValues{
		Directness: 90,
		Humor:      10,
		Caution:    75,
		Autonomy:   95,
	}

	prompt := formatSessionStylePrompt(style, nil)
	for _, want := range []string{
		"## TARS Style Controls",
		"Directness 90/100",
		"Humor 10/100",
		"Caution 75/100",
		"Autonomy 95/100",
		"Auto-resume is disabled",
		"never override tool permissions, approval requirements, or session automation consent",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}

	consent := &session.SessionAutomationConsent{
		AutoResume:             true,
		AutoResumeAfterMinutes: 15,
		AllowedResumeModes:     []string{session.AutoResumeModeMoveToNextTask},
		AutonomousMutations:    true,
	}
	prompt = formatSessionStylePrompt(style, consent)
	for _, want := range []string{
		"Auto-resume is permitted after 15 minutes",
		session.AutoResumeModeMoveToNextTask,
		"Autonomous workspace mutation consent is enabled",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected consent-aware prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

func TestEffectiveSessionStyleUsesSessionOverrides(t *testing.T) {
	defaults := sessionStyleValues{
		Directness: 70,
		Humor:      20,
		Caution:    60,
		Autonomy:   40,
	}
	style := &session.SessionStyleControl{
		Directness: styleIntPtr(95),
		Autonomy:   styleIntPtr(80),
	}

	got := effectiveSessionStyle(defaults, style)
	if got.Directness != 95 || got.Humor != 20 || got.Caution != 60 || got.Autonomy != 80 {
		t.Fatalf("unexpected effective style: %+v", got)
	}

	zero := &session.SessionStyleControl{Humor: styleIntPtr(0)}
	got = effectiveSessionStyle(defaults, zero)
	if got.Humor != 0 {
		t.Fatalf("expected zero-valued session override to survive, got %+v", got)
	}
	if prompt := formatSessionStylePrompt(got, nil); !strings.Contains(prompt, "Humor 0/100") {
		t.Fatalf("expected prompt to preserve zero override, got:\n%s", prompt)
	}
}

func styleIntPtr(value int) *int {
	return &value
}
