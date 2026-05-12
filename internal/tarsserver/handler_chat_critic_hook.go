package tarsserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/critic"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// criticInjectedMessagePrefix is prepended to every system-role injection
// the critic hook emits. It keeps the marker stable so the console and any
// future memory hooks can recognize / suppress these turns.
const criticInjectedMessagePrefix = "(critic feedback — incorporate before responding to the user)"

// criticDecisionStatus encodes what the critic hook did this turn so the
// outer chaining hook can decide whether to fall through to the goal hook.
type criticDecisionStatus int

const (
	criticStatusSkip       criticDecisionStatus = iota // no plan transition, no critic enabled, etc.
	criticStatusFeedback                               // emitted feedback; chat loop should auto-continue
	criticStatusSatisfied                              // verdict acceptable; fall through to goal hook
	criticStatusExhausted                              // budget hit without acceptance; fall through
)

// criticDecision is the structured return of the critic-only hook. Only
// Injection is meaningful when Status == criticStatusFeedback.
type criticDecision struct {
	Status    criticDecisionStatus
	Injection string
}

// formatSessionCriticPrompt produces the system-prompt section that surfaces
// the critic agent to the main LLM so it knows its plan/completion may be
// reviewed. Returns "" when critic is nil/disabled so callers may inline.
func formatSessionCriticPrompt(c *session.SessionCritic) string {
	if !c.IsEnabled() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Critic agent (active)\n")
	b.WriteString("A critic reviewer is watching this session. When you call plan_propose or mark a plan completed, an independent reviewer will inspect the plan/result and may inject concrete improvement feedback as a system message. Treat that feedback as authoritative direction and revise before responding to the user.\n")
	fmt.Fprintf(&b, "- Maximum review rounds per plan transition: %d\n", c.EffectiveMaxIterations())
	b.WriteString("- The reviewer cannot call tools; only you can.\n")
	return b.String()
}

// buildCriticAwareTurnEndHook returns an OnTurnEnd callback that runs at most
// MaxIterations reviewer calls per plan transition. The returned function is
// nil when the session has no critic enabled — callers may pass the result
// unconditionally into the chaining hook below.
//
// Plan-transition detection: we compute a small signature from the active
// plan's status+updated-at fields. The reviewer fires when the signature
// differs from SessionCritic.LastReviewedPlanSig, which lets us re-enter on
// every fresh plan_propose or plan_complete edit without re-reviewing static
// state.
func buildCriticAwareTurnEndHook(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter) func(context.Context, llm.ChatResponse) (criticDecision, error) {
	if state.sessionCritic == nil || !state.sessionCritic.IsEnabled() {
		return nil
	}
	if state.store == nil || deps.router == nil {
		return nil
	}
	reviewer := critic.NewLLMReviewer(deps.router, "")
	return func(ctx context.Context, lastResp llm.ChatResponse) (criticDecision, error) {
		sess, err := state.store.Get(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("critic hook: read session failed")
			return criticDecision{Status: criticStatusSkip}, nil
		}
		current := sess.Critic
		if !current.IsEnabled() {
			return criticDecision{Status: criticStatusSkip}, nil
		}

		tasks, err := state.store.GetTasks(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("critic hook: read tasks failed")
			return criticDecision{Status: criticStatusSkip}, nil
		}
		trigger, sig := detectCriticTrigger(tasks.Plan)
		if trigger == "" {
			return criticDecision{Status: criticStatusSkip}, nil
		}

		// Already finished a cycle for this exact plan signature → don't
		// re-enter unless the plan itself changes (new updated_at or new
		// status transition).
		if current.LastReviewedPlanSig == sig &&
			(current.Status == session.SessionCriticStatusSatisfied ||
				current.Status == session.SessionCriticStatusExhausted) {
			return criticDecision{Status: criticStatusSkip}, nil
		}

		// Fresh transition: reset iteration counter.
		if current.LastReviewedPlanSig != sig {
			updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
				c.CurrentIteration = 0
				c.Status = session.SessionCriticStatusReviewing
				c.LastTrigger = trigger
				c.LastReviewedPlanSig = sig
				c.LastFeedback = ""
				return c
			})
			if uerr != nil {
				deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic hook: reset progress failed")
				return criticDecision{Status: criticStatusSkip}, nil
			}
			current = updated.Critic
			if stream != nil {
				stream.criticEvent("started", trigger, current)
			}
		}

		max := current.EffectiveMaxIterations()
		if current.CurrentIteration >= max {
			updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
				c.Status = session.SessionCriticStatusExhausted
				return c
			})
			if uerr != nil {
				deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic hook: mark exhausted failed")
			}
			if stream != nil {
				if updated.Critic != nil {
					stream.criticEvent("exhausted", "max iterations reached", updated.Critic)
				} else {
					stream.criticEvent("exhausted", "max iterations reached", current)
				}
			}
			return criticDecision{Status: criticStatusExhausted}, nil
		}

		window := criticReviewWindow(state, lastResp)
		verdict, reviewErr := reviewer.Review(ctx, trigger, tasks.Plan, tasks.Tasks, window)
		if reviewErr != nil {
			deps.logger.Debug().Err(reviewErr).Str("session_id", state.sessionID).Msg("critic review failed; falling open")
			if stream != nil {
				stream.criticEvent("judge_error", reviewErr.Error(), current)
			}
			return criticDecision{Status: criticStatusSkip}, nil
		}

		if verdict.Acceptable {
			updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
				c.Status = session.SessionCriticStatusSatisfied
				c.LastFeedback = ""
				return c
			})
			if uerr != nil {
				deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic hook: mark satisfied failed")
			}
			if stream != nil {
				if updated.Critic != nil {
					stream.criticEvent("satisfied", verdict.Reason, updated.Critic)
				} else {
					stream.criticEvent("satisfied", verdict.Reason, current)
				}
			}
			return criticDecision{Status: criticStatusSatisfied}, nil
		}

		// Not acceptable: persist feedback, bump iteration, inject.
		updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
			c.CurrentIteration++
			c.Status = session.SessionCriticStatusReviewing
			c.LastFeedback = verdict.Feedback
			return c
		})
		if uerr != nil {
			deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic hook: bump iteration failed")
			return criticDecision{Status: criticStatusSkip}, nil
		}
		if stream != nil {
			stream.criticEvent("feedback", verdict.Reason, updated.Critic)
		}
		return criticDecision{
			Status:    criticStatusFeedback,
			Injection: composeCriticInjection(trigger, verdict, updated.Critic.CurrentIteration, max),
		}, nil
	}
}

// buildChatTurnEndHook combines the critic and goal hooks per the design:
// the critic loop runs first, and only when it is satisfied (or skipped) does
// the goal hook get a vote. This stops the cost-multiplier scenario where
// /goal would auto-continue an unrevised plan.
func buildChatTurnEndHook(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter) func(context.Context, llm.ChatResponse) (string, error) {
	criticHook := buildCriticAwareTurnEndHook(deps, state, stream)
	goalHook := buildGoalAwareTurnEndHook(deps, state, stream)
	if criticHook == nil {
		return goalHook
	}
	return func(ctx context.Context, lastResp llm.ChatResponse) (string, error) {
		decision, err := criticHook(ctx, lastResp)
		if err != nil {
			return "", err
		}
		if decision.Status == criticStatusFeedback {
			// Critic is still iterating — defer the goal judge until the
			// next natural stopping point so we do not stack two
			// auto-continue verdicts in one turn.
			return decision.Injection, nil
		}
		if goalHook == nil {
			return "", nil
		}
		return goalHook(ctx, lastResp)
	}
}

// detectCriticTrigger maps the current plan state to a trigger kind plus a
// signature used to dedupe a single plan transition. Returns empty trigger
// when the plan is in a non-actionable state.
func detectCriticTrigger(plan *session.Plan) (trigger string, sig string) {
	if plan == nil {
		return "", ""
	}
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	switch status {
	case session.PlanStatusProposed:
		trigger = critic.TriggerPlanProposed
	case session.PlanStatusCompleted:
		trigger = critic.TriggerPlanCompleted
	default:
		return "", ""
	}
	// Signature includes status + the plan's UpdatedAt (or CreatedAt as
	// fallback). A user editing the plan goal/constraints bumps UpdatedAt,
	// which legitimately re-opens critic review.
	stamp := strings.TrimSpace(plan.UpdatedAt)
	if stamp == "" {
		stamp = strings.TrimSpace(plan.CreatedAt)
	}
	sig = status + "@" + stamp
	return trigger, sig
}

func criticReviewWindow(state chatRunState, lastResp llm.ChatResponse) []llm.ChatMessage {
	window := make([]llm.ChatMessage, 0, 4)
	for i := len(state.llmMessages) - 1; i >= 0; i-- {
		m := state.llmMessages[i]
		if strings.EqualFold(strings.TrimSpace(m.Role), "user") {
			window = append(window, m)
			break
		}
	}
	if strings.TrimSpace(lastResp.Message.Content) != "" {
		window = append(window, lastResp.Message)
	}
	return window
}

func composeCriticInjection(trigger string, verdict critic.Verdict, iteration, max int) string {
	var b strings.Builder
	b.WriteString(criticInjectedMessagePrefix)
	fmt.Fprintf(&b, " — round %d/%d, trigger=%s", iteration, max, trigger)
	if reason := strings.TrimSpace(verdict.Reason); reason != "" {
		b.WriteString("\nSummary: ")
		b.WriteString(reason)
	}
	if feedback := strings.TrimSpace(verdict.Feedback); feedback != "" {
		b.WriteString("\nFeedback:\n")
		b.WriteString(feedback)
	}
	b.WriteString("\nRevise the plan or completion result accordingly, then re-emit it.")
	return b.String()
}
