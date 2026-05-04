package pulse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/pulse/autofix"
	"github.com/devlikebear/tars/internal/session"
	zlog "github.com/rs/zerolog/log"
)

// AutoContinueGoalCandidate describes a chat session whose plan has just
// completed and is opted in to auto-continue. The pulse autofix decides
// whether to run one more turn that asks the LLM to either declare the
// session goal achieved (terminate) or propose a follow-up plan.
type AutoContinueGoalCandidate struct {
	SessionID         string `json:"session_id"`
	Title             string `json:"title,omitempty"`
	PlanGoal          string `json:"plan_goal,omitempty"`
	PlanCreatedAt     string `json:"plan_created_at,omitempty"`
	MaxIterations     int    `json:"max_iterations"`
	AutoResumeEnabled bool   `json:"auto_resume_enabled"`
	CanAutoContinue   bool   `json:"can_auto_continue"`
	BlockReason       string `json:"block_reason,omitempty"`
}

// DetectAutoContinueGoalCandidates returns chat sessions whose active plan
// has reached PlanStatusCompleted with AutoContinueEnabled=true and which
// still have iterations remaining within the configured cap.
//
// The detector also requires AutomationConsent.AllowsAutoResume() — the
// session must already be opted in to pulse-driven automation. We piggy-back
// on the same consent flag rather than adding a new one for now.
func DetectAutoContinueGoalCandidates(ctx context.Context, source ChatSessionSource, _ time.Time) ([]AutoContinueGoalCandidate, error) {
	if source == nil {
		return nil, nil
	}
	sessions, err := source.List()
	if err != nil {
		return nil, err
	}
	var candidates []AutoContinueGoalCandidate
	for _, sess := range sessions {
		if err := ctx.Err(); err != nil {
			return candidates, err
		}
		tasks, err := source.GetTasks(sess.ID)
		if err != nil {
			zlog.Logger.Debug().Err(err).Str("session_id", sess.ID).Msg("pulse: skip auto-continue-goal task read failure")
			continue
		}
		plan := tasks.Plan
		if plan == nil {
			continue
		}
		if !plan.AutoContinueEnabled {
			continue
		}
		if strings.TrimSpace(plan.Status) != session.PlanStatusCompleted {
			continue
		}
		consent := sess.AutomationConsent
		autoResumeEnabled := false
		if consent != nil {
			autoResumeEnabled = consent.AllowsAutoResume()
		}

		// The iteration cap itself is enforced by the autofix controller
		// using the automation audit log (so it survives plan replacement
		// when the LLM proposes a follow-up plan). Detection simply surfaces
		// the candidate; final eligibility is decided downstream.
		canAutoContinue := autoResumeEnabled

		candidates = append(candidates, AutoContinueGoalCandidate{
			SessionID:         sess.ID,
			Title:             sess.Title,
			PlanGoal:          plan.Goal,
			PlanCreatedAt:     plan.CreatedAt,
			MaxIterations:     plan.EffectiveAutoContinueMaxIterations(),
			AutoResumeEnabled: autoResumeEnabled,
			CanAutoContinue:   canAutoContinue,
		})
	}
	return candidates, nil
}

// scanAutoContinueGoals emits a SignalKindAutoContinueGoal when at least one
// session has a completed plan with auto-continue enabled.
func (s *Scanner) scanAutoContinueGoals(ctx context.Context, now time.Time) *Signal {
	if s.sources.ChatSessions == nil {
		return nil
	}
	candidates, err := DetectAutoContinueGoalCandidates(ctx, s.sources.ChatSessions, now)
	if err != nil {
		zlog.Logger.Warn().Err(err).Msg("pulse: auto-continue-goal scan failed; skipping this tick")
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	primary := candidates[0]
	canContinue := false
	for _, c := range candidates {
		if c.CanAutoContinue {
			canContinue = true
			break
		}
	}
	details := map[string]any{
		"candidate_count":     len(candidates),
		"session_id":          primary.SessionID,
		"session_title":       primary.Title,
		"plan_goal":           primary.PlanGoal,
		"max_iterations":      primary.MaxIterations,
		"can_auto_continue":   primary.CanAutoContinue,
		"auto_resume_enabled": primary.AutoResumeEnabled,
		"block_reason":        primary.BlockReason,
		"autofix_candidate":   autofix.AutoContinueGoalPlanName,
		"sessions":            candidates,
	}
	if canContinue {
		details["has_auto_continue_candidate"] = true
	}
	return &Signal{
		Kind:     SignalKindAutoContinueGoal,
		Severity: SeverityInfo,
		Summary:  fmt.Sprintf("%d chat session(s) have a completed plan opted in to auto-continue", len(candidates)),
		Details:  details,
		At:       now,
	}
}
