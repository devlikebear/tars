package session

import (
	"strings"
	"time"
)

// SessionCritic status values.
const (
	SessionCriticStatusIdle      = "idle"
	SessionCriticStatusReviewing = "reviewing"
	SessionCriticStatusSatisfied = "satisfied"
	SessionCriticStatusExhausted = "exhausted"

	// DefaultCriticMaxIterations is the default per-plan-transition review
	// budget. Three rounds matches the goal-judge default and is the value
	// users see in the wizard prose.
	DefaultCriticMaxIterations = 3
	// MaxCriticMaxIterations caps the configurable budget. Five is generous
	// for a single plan transition; anything higher tends to be a runaway
	// rather than productive critique.
	MaxCriticMaxIterations = 5
	// MaxCriticFeedbackLen bounds the feedback string we persist so the
	// transcript JSON does not balloon when a reviewer emits a very long
	// bullet list.
	MaxCriticFeedbackLen = 4000
)

// SessionCritic captures the per-session critic-agent configuration plus the
// runtime state for the active review cycle. The chat handler reads/writes
// this through Store.SetCritic / Store.UpdateCriticProgress, so all mutation
// paths share one normalization routine.
//
// Lifecycle:
//   - User toggles Enabled = true (status starts "idle", iteration 0).
//   - On a plan transition (Proposed or Completed) the hook bumps
//     CurrentIteration, sets Status = "reviewing", records LastTrigger and
//     the plan signature used to dedupe a single transition.
//   - When the reviewer returns Acceptable=true the hook sets Status =
//     "satisfied" and resets CurrentIteration.
//   - When CurrentIteration reaches MaxIterations without acceptance, the
//     hook sets Status = "exhausted" and stops issuing feedback until the
//     next plan transition.
type SessionCritic struct {
	Enabled             bool       `json:"enabled"`
	MaxIterations       int        `json:"max_iterations,omitempty"`
	CurrentIteration    int        `json:"current_iteration,omitempty"`
	Status              string     `json:"status,omitempty"`
	LastFeedback        string     `json:"last_feedback,omitempty"`
	LastTrigger         string     `json:"last_trigger,omitempty"`
	LastReviewedPlanSig string     `json:"last_reviewed_plan_sig,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

// IsEnabled reports whether the critic agent should run for this session.
func (c *SessionCritic) IsEnabled() bool {
	return c != nil && c.Enabled
}

// EffectiveMaxIterations clamps the configured budget into [1,
// MaxCriticMaxIterations] and falls back to DefaultCriticMaxIterations on
// zero/negative input.
func (c *SessionCritic) EffectiveMaxIterations() int {
	if c == nil {
		return DefaultCriticMaxIterations
	}
	limit := c.MaxIterations
	if limit <= 0 {
		limit = DefaultCriticMaxIterations
	}
	if limit > MaxCriticMaxIterations {
		limit = MaxCriticMaxIterations
	}
	return limit
}

// NormalizeCritic trims and clamps fields and defaults Status when unset.
// Returns nil when the input is nil so callers can store-or-clear with a
// single statement.
func NormalizeCritic(c *SessionCritic) *SessionCritic {
	if c == nil {
		return nil
	}
	next := *c
	if next.MaxIterations <= 0 {
		next.MaxIterations = DefaultCriticMaxIterations
	}
	if next.MaxIterations > MaxCriticMaxIterations {
		next.MaxIterations = MaxCriticMaxIterations
	}
	if next.CurrentIteration < 0 {
		next.CurrentIteration = 0
	}
	if next.CurrentIteration > next.MaxIterations {
		next.CurrentIteration = next.MaxIterations
	}
	switch strings.TrimSpace(next.Status) {
	case SessionCriticStatusIdle,
		SessionCriticStatusReviewing,
		SessionCriticStatusSatisfied,
		SessionCriticStatusExhausted:
		next.Status = strings.TrimSpace(next.Status)
	default:
		next.Status = SessionCriticStatusIdle
	}
	next.LastTrigger = strings.TrimSpace(next.LastTrigger)
	next.LastReviewedPlanSig = strings.TrimSpace(next.LastReviewedPlanSig)
	feedback := strings.TrimSpace(next.LastFeedback)
	if len(feedback) > MaxCriticFeedbackLen {
		feedback = feedback[:MaxCriticFeedbackLen]
	}
	next.LastFeedback = feedback
	return &next
}

func criticEqual(a, b *SessionCritic) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Enabled != b.Enabled ||
		a.MaxIterations != b.MaxIterations ||
		a.CurrentIteration != b.CurrentIteration ||
		a.Status != b.Status ||
		a.LastFeedback != b.LastFeedback ||
		a.LastTrigger != b.LastTrigger ||
		a.LastReviewedPlanSig != b.LastReviewedPlanSig {
		return false
	}
	if (a.UpdatedAt == nil) != (b.UpdatedAt == nil) {
		return false
	}
	if a.UpdatedAt != nil && !a.UpdatedAt.Equal(*b.UpdatedAt) {
		return false
	}
	return true
}
