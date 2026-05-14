package tarsserver

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/critic"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// criticInjectedMessagePrefix marks every system-role message produced by
// queued critic feedback so the console (and future memory hooks) can
// recognize/suppress these turns.
const criticInjectedMessagePrefix = "(critic feedback — incorporate before responding to the user)"

// criticAsyncReviewTimeout caps how long an async reviewer call may run before
// we abandon it. Generous because the reviewer fires off the request's hot
// path and a slow tier should not be silently truncated.
const criticAsyncReviewTimeout = 2 * time.Minute

// criticAsyncRunner schedules the reviewer body. Defaults to a goroutine
// launch so the chat hot path returns immediately; tests overwrite it with a
// synchronous variant to inspect side effects deterministically.
var criticAsyncRunner = func(fn func()) { go fn() }

// formatSessionCriticPrompt produces the system-prompt section that surfaces
// the critic agent to the main LLM so it knows its responses (and any plan
// transitions) may be reviewed. Returns "" when critic is nil/disabled so
// callers may inline.
func formatSessionCriticPrompt(c *session.SessionCritic) string {
	if !c.IsEnabled() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Critic agent (active)\n")
	b.WriteString("An independent critic reviewer evaluates every assistant turn in this session. Its feedback arrives asynchronously and is injected as a system message at the start of your next turn — read it carefully and revise your answer accordingly when present. Plan transitions (plan_propose / plan_complete) also trigger review with a dedicated prompt.\n")
	fmt.Fprintf(&b, "- Maximum review rounds per plan transition: %d\n", c.EffectiveMaxIterations())
	b.WriteString("- The reviewer cannot call tools; only you can.\n")
	return b.String()
}

// buildCriticAwareTurnEndHook returns an OnTurnEnd callback that launches an
// asynchronous reviewer pass. The returned function always reports
// "no injection" — feedback flows via SessionCritic.PendingFeedback and is
// drained into the LLM thread on the next user turn (see
// drainPendingCriticFeedback). nil is returned when the session has no critic
// enabled.
//
// Trigger detection: plan_proposed/plan_completed take priority when the plan
// is in one of those states; otherwise we fall back to assistant_turn so
// worker / subagent / plan-less main sessions still get coverage.
func buildCriticAwareTurnEndHook(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter) func(context.Context, llm.ChatResponse) error {
	if state.sessionCritic == nil || !state.sessionCritic.IsEnabled() {
		return nil
	}
	if state.store == nil || deps.router == nil {
		return nil
	}
	reviewer := critic.NewLLMReviewer(deps.router, "")
	return func(_ context.Context, lastResp llm.ChatResponse) error {
		sess, err := state.store.Get(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("critic hook: read session failed")
			return nil
		}
		if !sess.Critic.IsEnabled() {
			return nil
		}

		tasks, err := state.store.GetTasks(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("critic hook: read tasks failed")
			return nil
		}
		trigger, sig := detectCriticTrigger(tasks.Plan, lastResp)
		if trigger == "" {
			return nil
		}

		// Per-plan-transition dedupe still applies. Plans that already settled
		// (satisfied/exhausted) do not re-open until the plan itself changes.
		current := sess.Critic
		if trigger == critic.TriggerPlanProposed || trigger == critic.TriggerPlanCompleted {
			if current.LastReviewedPlanSig == sig &&
				(current.Status == session.SessionCriticStatusSatisfied ||
					current.Status == session.SessionCriticStatusExhausted) {
				return nil
			}
			if current.LastReviewedPlanSig != sig {
				if updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
					c.CurrentIteration = 0
					c.Status = session.SessionCriticStatusReviewing
					c.LastTrigger = trigger
					c.LastReviewedPlanSig = sig
					c.LastFeedback = ""
					return c
				}); uerr == nil {
					current = updated.Critic
					if stream != nil {
						stream.criticEvent("started", trigger, current)
					}
				} else {
					deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic hook: reset progress failed")
					return nil
				}
			}
			max := current.EffectiveMaxIterations()
			if current.CurrentIteration >= max {
				if updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
					c.Status = session.SessionCriticStatusExhausted
					return c
				}); uerr == nil && stream != nil {
					stream.criticEvent("exhausted", "max iterations reached", updated.Critic)
				}
				return nil
			}
		} else { // TriggerAssistantTurn
			// Skip if we already reviewed this exact assistant response (sig
			// matches) and the result is on the queue or recently settled.
			if current.LastReviewedTurnSig != "" && current.LastReviewedTurnSig == sig {
				return nil
			}
			if updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
				c.Status = session.SessionCriticStatusReviewing
				c.LastTrigger = trigger
				c.LastReviewedTurnSig = sig
				return c
			}); uerr == nil {
				current = updated.Critic
				if stream != nil {
					stream.criticEvent("started", trigger, current)
				}
			} else {
				deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic hook: mark reviewing failed")
				return nil
			}
		}

		// Snapshot what the goroutine needs — the request-scoped ctx will be
		// gone by the time it runs.
		window := criticReviewWindow(state, lastResp)
		plan := tasks.Plan
		taskList := tasks.Tasks
		max := current.EffectiveMaxIterations()
		_ = sig // dedupe is tracked in store; sig itself is consumed during the sync prep above
		criticAsyncRunner(func() {
			runCriticReview(deps, state, stream, reviewer, trigger, plan, taskList, window, max)
		})
		return nil
	}
}

// runCriticReview executes the reviewer call on a fresh context so the result
// is persisted even after the chat request that triggered it returns. It
// either writes PendingFeedback (when the verdict is unacceptable) or flips
// the status to satisfied — never both.
func runCriticReview(
	deps chatHandlerDeps,
	state chatRunState,
	stream *chatStreamWriter,
	reviewer critic.Reviewer,
	trigger string,
	plan *session.Plan,
	tasks []session.Task,
	window []llm.ChatMessage,
	max int,
) {
	defer func() {
		if r := recover(); r != nil {
			deps.logger.Error().Interface("panic", r).Str("session_id", state.sessionID).Msg("critic async reviewer panicked")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), criticAsyncReviewTimeout)
	defer cancel()

	verdict, reviewErr := reviewer.Review(ctx, trigger, plan, tasks, window)
	if reviewErr != nil {
		deps.logger.Debug().Err(reviewErr).Str("session_id", state.sessionID).Msg("critic async review failed; falling open")
		if stream != nil {
			stream.criticEvent("judge_error", reviewErr.Error(), state.sessionCritic)
		}
		return
	}

	if verdict.Acceptable {
		updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
			c.Status = session.SessionCriticStatusSatisfied
			c.LastFeedback = ""
			return c
		})
		if uerr != nil {
			deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic async: mark satisfied failed")
			return
		}
		if stream != nil {
			stream.criticEvent("satisfied", verdict.Reason, updated.Critic)
		}
		return
	}

	// Non-acceptable: queue feedback for the next user turn. For plan
	// triggers we bump CurrentIteration so we honor max_iterations across
	// turns; assistant_turn does not.
	now := time.Now().UTC()
	updated, uerr := state.store.UpdateCriticProgress(state.sessionID, func(c *session.SessionCritic) *session.SessionCritic {
		c.Status = session.SessionCriticStatusReviewing
		c.LastFeedback = verdict.Feedback
		if trigger == critic.TriggerPlanProposed || trigger == critic.TriggerPlanCompleted {
			c.CurrentIteration++
		}
		round := c.CurrentIteration
		if trigger == critic.TriggerAssistantTurn {
			round = 1
		}
		c.PendingFeedback = composeCriticInjection(trigger, verdict, round, max)
		c.PendingFeedbackTrigger = trigger
		c.PendingFeedbackRound = round
		c.PendingFeedbackAt = &now
		return c
	})
	if uerr != nil {
		deps.logger.Debug().Err(uerr).Str("session_id", state.sessionID).Msg("critic async: queue feedback failed")
		return
	}
	if stream != nil {
		stream.criticEvent("feedback", verdict.Reason, updated.Critic)
	}
}

// buildChatTurnEndHook composes the critic and goal hooks. Critic is now
// async (side-channel via PendingFeedback) so it no longer drives
// auto-continue; the goal judge keeps its synchronous string-injection
// contract. Returns nil only if both inner hooks are nil.
func buildChatTurnEndHook(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter) func(context.Context, llm.ChatResponse) (string, error) {
	criticHook := buildCriticAwareTurnEndHook(deps, state, stream)
	goalHook := buildGoalAwareTurnEndHook(deps, state, stream)
	if criticHook == nil && goalHook == nil {
		return nil
	}
	return func(ctx context.Context, lastResp llm.ChatResponse) (string, error) {
		if criticHook != nil {
			if err := criticHook(ctx, lastResp); err != nil {
				return "", err
			}
		}
		if goalHook == nil {
			return "", nil
		}
		return goalHook(ctx, lastResp)
	}
}

// detectCriticTrigger inspects the plan first, then falls back to
// assistant_turn when no plan transition is in flight. The returned signature
// is used to dedupe repeat firings on the same logical event.
func detectCriticTrigger(plan *session.Plan, lastResp llm.ChatResponse) (trigger string, sig string) {
	if plan != nil {
		status := strings.ToLower(strings.TrimSpace(plan.Status))
		switch status {
		case session.PlanStatusProposed:
			trigger = critic.TriggerPlanProposed
		case session.PlanStatusCompleted:
			trigger = critic.TriggerPlanCompleted
		}
		if trigger != "" {
			stamp := strings.TrimSpace(plan.UpdatedAt)
			if stamp == "" {
				stamp = strings.TrimSpace(plan.CreatedAt)
			}
			sig = status + "@" + stamp
			return trigger, sig
		}
	}
	// Fall through: every assistant turn is reviewable when no plan is in a
	// review-worthy state.
	trigger = critic.TriggerAssistantTurn
	sig = assistantTurnSignature(lastResp)
	return trigger, sig
}

// assistantTurnSignature hashes the response content (plus tool-call ids) so
// the same assistant response is only reviewed once even if the hook fires
// twice for any reason.
func assistantTurnSignature(lastResp llm.ChatResponse) string {
	h := sha1.New()
	h.Write([]byte(lastResp.Message.Content))
	for _, tc := range lastResp.Message.ToolCalls {
		h.Write([]byte(tc.ID))
	}
	if lastResp.Message.Content == "" && len(lastResp.Message.ToolCalls) == 0 {
		return "" // nothing to sign → reviewer has nothing to look at either
	}
	return "turn:" + hex.EncodeToString(h.Sum(nil))[:16]
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
	if trigger == critic.TriggerAssistantTurn {
		fmt.Fprintf(&b, " — trigger=%s", trigger)
	} else {
		fmt.Fprintf(&b, " — round %d/%d, trigger=%s", iteration, max, trigger)
	}
	if reason := strings.TrimSpace(verdict.Reason); reason != "" {
		b.WriteString("\nSummary: ")
		b.WriteString(reason)
	}
	if feedback := strings.TrimSpace(verdict.Feedback); feedback != "" {
		b.WriteString("\nFeedback:\n")
		b.WriteString(feedback)
	}
	if trigger == critic.TriggerAssistantTurn {
		b.WriteString("\nIncorporate the feedback in your next response to the user.")
	} else {
		b.WriteString("\nRevise the plan or completion result accordingly, then re-emit it.")
	}
	return b.String()
}
