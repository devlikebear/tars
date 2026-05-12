package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/goal"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

// autoContinueInjectedMessage is the user-role message appended when the
// agent loop is told to continue chasing an unmet goal. Kept short so it
// does not bias the LLM's plan and is easy to recognize in transcripts.
const autoContinueInjectedMessage = "(auto-continue: keep working toward the active session goal; take the next concrete step.)"

// buildGoalAwareTurnEndHook returns an agent.RunOptions.OnTurnEnd callback
// that consults the goal judge after each natural stopping point. The hook
// is a no-op when the session has no active goal — callers may pass the
// result unconditionally.
//
// Behavior:
//   - judge says satisfied → mark goal satisfied, clear it, emit SSE, stop.
//   - judge says not satisfied & budget remaining → bump count, persist,
//     emit SSE, return a continuation prompt to drive another iteration.
//   - budget exhausted → mark exhausted, emit SSE, stop.
//   - judge errors / configuration missing → fail-open (stop), emit SSE.
func buildGoalAwareTurnEndHook(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter) func(context.Context, llm.ChatResponse) (string, error) {
	if state.sessionGoal == nil || !state.sessionGoal.IsActive() {
		return nil
	}
	if state.store == nil || deps.router == nil {
		return nil
	}
	judger := goal.NewLLMJudger(deps.router, "")
	return func(ctx context.Context, lastResp llm.ChatResponse) (string, error) {
		// Re-read the live goal so a concurrent DELETE/PUT through the admin
		// API takes effect immediately.
		sess, err := state.store.Get(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("goal hook: read session failed")
			return "", nil
		}
		current := sess.Goal
		if !current.IsActive() {
			return "", nil
		}

		window := goalJudgeWindow(state, lastResp)
		verdict, judgeErr := judger.Judge(ctx, current.Description, window)
		if judgeErr != nil {
			deps.logger.Debug().Err(judgeErr).Str("session_id", state.sessionID).Msg("goal judge failed; stopping (fail-open)")
			if stream != nil {
				stream.goalEvent("judge_error", judgeErr.Error(), current)
			}
			return "", nil
		}

		if verdict.Satisfied {
			if _, err := state.store.UpdateGoalProgress(state.sessionID, func(g *session.SessionGoal) *session.SessionGoal {
				g.Status = session.SessionGoalStatusSatisfied
				return g
			}); err != nil {
				deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("goal hook: mark satisfied failed")
			}
			if _, err := state.store.ClearGoal(state.sessionID); err != nil {
				deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("goal hook: clear after satisfied failed")
			}
			if stream != nil {
				stream.goalEvent("satisfied", verdict.Reason, current)
			}
			return "", nil
		}

		if current.AutoContinueCount >= current.MaxAutoContinues {
			updated, err := state.store.UpdateGoalProgress(state.sessionID, func(g *session.SessionGoal) *session.SessionGoal {
				g.Status = session.SessionGoalStatusExhausted
				return g
			})
			if err != nil {
				deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("goal hook: mark exhausted failed")
			}
			if stream != nil {
				if updated.Goal != nil {
					stream.goalEvent("exhausted", verdict.Reason, updated.Goal)
				} else {
					stream.goalEvent("exhausted", verdict.Reason, current)
				}
			}
			return "", nil
		}

		updated, err := state.store.UpdateGoalProgress(state.sessionID, func(g *session.SessionGoal) *session.SessionGoal {
			g.AutoContinueCount++
			return g
		})
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("goal hook: bump count failed")
			return "", nil
		}
		if stream != nil {
			stream.goalEvent("auto_continue", verdict.Reason, updated.Goal)
		}
		return autoContinueInjectedMessage, nil
	}
}

// goalJudgeWindow returns the trailing chat messages the judge should
// inspect: the latest user request that triggered this turn plus the
// assistant content just produced. Earlier history is intentionally elided
// to keep the judge prompt focused and cheap.
func goalJudgeWindow(state chatRunState, lastResp llm.ChatResponse) []llm.ChatMessage {
	window := make([]llm.ChatMessage, 0, 4)
	// Walk back through llmMessages to find the most recent user message.
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
