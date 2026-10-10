package focuspipeline

import (
	"fmt"
	"strings"
	"time"
)

// Goal mode (ADR §4, "Goal mode"). A pipeline in goal mode runs to its end
// with nobody at the gates: the server decides each one itself and pushes
// the pipeline on when it stops for any other reason.
//
// The decisions are a fixed policy, not a model's judgement of whether the
// work is done — the stages still exit on the same facts (verification exit
// codes, CI checks, the merged PR):
//
//   - G1, G3, G4 are approved as proposed (G4 opens only on green checks);
//   - a finding is fixed unless its severity is low, which is dismissed; a
//     pull request finding (a failed check, a review comment) is fixed;
//   - a question the agent asks is answered "decide yourself";
//   - a blocked gate (loop limit, repeated failure, a failed or interrupted
//     turn, a PR that did not appear) is retried, and a pipeline that sits
//     idle with a turn owed or nothing owed at all is pushed with one.
//
// Retries and pushes draw on a budget (MaxPushes); when it is spent goal
// mode ends where it stands and the gate waits for the developer. NextGoalStep
// is pure: the server applies the step it returns through the same paths
// the console's buttons use.

// Goal step kinds.
const (
	GoalWait      = "wait"      // nothing to do now
	GoalGate      = "gate"      // answer the open gate
	GoalCard      = "card"      // decide a decision or finding card
	GoalNudge     = "nudge"     // send a turn: the pipeline sits idle
	GoalInterrupt = "interrupt" // an owed step was dropped: raise the gate
	GoalEnd       = "end"       // goal mode is over
)

// Reasons goal mode ends.
const (
	GoalEndFinished  = "finished"
	GoalEndStopped   = "stopped"
	GoalEndExhausted = "exhausted"
	GoalEndPRClosed  = "pr_closed"
	// GoalEndE2EFailed: the review is blocked on an end-to-end goal. A
	// retry sends the agent after a failure that is usually not in the
	// code (the wrong window, something the accessibility tree cannot
	// show), so the gate is left to the developer.
	GoalEndE2EFailed = "e2e_failed"
	GoalEndCancelled = "cancelled"
	GoalEndDisabled  = "disabled"
)

// Push budget.
const (
	DefaultGoalPushes = 20
	MaxGoalPushes     = 100
)

// GoalStallGrace is how long a pipeline must have sat unchanged before goal
// mode treats it as stalled: a step the server is about to start is not one.
const GoalStallGrace = 20 * time.Second

// goalRetryBase and goalRetryMax bound the wait before a failed turn is
// retried: doubled per consecutive failure (a provider outage or rate limit
// is not answered by hammering it).
const (
	goalRetryBase = 30 * time.Second
	goalRetryMax  = 15 * time.Minute
)

// GoalDecisionAnswer is how goal mode answers a question the agent asked.
const GoalDecisionAnswer = "Decide yourself: choose the option that best serves the goal, say which one and why in your report, and continue."

// NoticeGoalEnded is the title of the notice goal mode leaves when it ends
// before the pipeline does.
const NoticeGoalEnded = "goal mode ended"

// GoalMode is a pipeline's goal mode state.
type GoalMode struct {
	Enabled bool `json:"enabled"`
	// Pushes counts the retries and pushes spent of MaxPushes; Decisions
	// the gates and cards decided.
	Pushes    int `json:"pushes"`
	MaxPushes int `json:"max_pushes"`
	Decisions int `json:"decisions,omitempty"`
	// Failures counts consecutive failed turns, for the retry backoff; a
	// completed turn resets it.
	Failures  int        `json:"failures,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	EndReason string     `json:"end_reason,omitempty"`
	// PermissionSet says the server put the session in the auto permission
	// mode for goal mode, and RestorePermission what it had before.
	PermissionSet     bool   `json:"permission_set,omitempty"`
	RestorePermission string `json:"restore_permission,omitempty"`
}

// GoalActive reports whether the pipeline is in goal mode.
func (p Pipeline) GoalActive() bool {
	return p.GoalMode != nil && p.GoalMode.Enabled
}

// GoalStep is what goal mode does next.
type GoalStep struct {
	Kind string
	// Gate, Action and Note answer the open gate (GoalGate); CardID is its
	// card, so a gate replaced meanwhile refuses the answer.
	Gate, Action, Note string
	// CardID and Decision decide a card (GoalCard).
	CardID, Decision string
	// Prompt is the turn to send (GoalNudge).
	Prompt string
	// Push says the step draws on the push budget, FailedTurn that it
	// retries a failed turn.
	Push, FailedTurn bool
	// Reason is why goal mode ends (GoalEnd).
	Reason string
	// Until is when to look again (GoalWait with a backoff).
	Until time.Time
}

// StartGoal puts a pipeline in goal mode with a fresh budget. maxPushes <= 0
// means the default; it is capped at MaxGoalPushes.
func StartGoal(p Pipeline, maxPushes int, now time.Time) Pipeline {
	next := p.clone()
	if maxPushes <= 0 {
		maxPushes = DefaultGoalPushes
	}
	g := GoalMode{Enabled: true, MaxPushes: min(maxPushes, MaxGoalPushes), StartedAt: now.UTC()}
	if prev := p.GoalMode; prev != nil && prev.Enabled {
		// Started again while on (a new budget): the session's own
		// permission mode is still the one recorded the first time.
		g.PermissionSet, g.RestorePermission = prev.PermissionSet, prev.RestorePermission
	}
	next.GoalMode = &g
	next.UpdatedAt = now.UTC()
	return next
}

// EndGoal ends goal mode. An end that leaves the pipeline unfinished for
// the developer (budget spent, PR closed, a cancelled turn) leaves a notice.
// ok is false, and p is returned unchanged, when goal mode was not on.
func EndGoal(p Pipeline, reason string, now time.Time) (Pipeline, bool) {
	if !p.GoalActive() {
		return p, false
	}
	next := p.clone()
	at := now.UTC()
	next.GoalMode.Enabled = false
	next.GoalMode.EndedAt = &at
	next.GoalMode.EndReason = reason
	// The server puts the session's permission mode back (it read what to
	// restore before this).
	next.GoalMode.PermissionSet, next.GoalMode.RestorePermission = false, ""
	switch reason {
	case GoalEndExhausted, GoalEndPRClosed, GoalEndE2EFailed, GoalEndCancelled:
		next.addCard(CardNotice, 0, NoticeGoalEnded, map[string]any{
			"reason": reason, "pushes": next.GoalMode.Pushes, "max_pushes": next.GoalMode.MaxPushes,
		}, at)
	}
	next.UpdatedAt = at
	return next, true
}

// GoalPermission records that the server set the session's permission mode
// for goal mode, and what the session had before.
func GoalPermission(p Pipeline, restore string, now time.Time) Pipeline {
	if p.GoalMode == nil {
		return p
	}
	next := p.clone()
	next.GoalMode.PermissionSet = true
	next.GoalMode.RestorePermission = restore
	next.UpdatedAt = now.UTC()
	return next
}

// GoalApplied records a step goal mode carried out: a decision, and for a
// push its draw on the budget. A nudge's turn becomes the turn owed.
func GoalApplied(p Pipeline, step GoalStep, now time.Time) Pipeline {
	if !p.GoalActive() {
		return p
	}
	next := p.clone()
	g := next.GoalMode
	switch step.Kind {
	case GoalGate, GoalCard:
		g.Decisions++
	case GoalNudge:
		if next.Active() && next.OpenGate == GateNone {
			next.PendingTurn = step.Prompt
		}
	}
	if step.Push {
		g.Pushes++
	}
	if step.FailedTurn {
		g.Failures++
	}
	next.UpdatedAt = now.UTC()
	return next
}

// NextGoalStep decides what goal mode does next for a pipeline nothing is
// running on (no turn, no verification, no driver run). It never changes p.
func NextGoalStep(p Pipeline, now time.Time) GoalStep {
	wait := GoalStep{Kind: GoalWait}
	if !p.GoalActive() {
		return wait
	}
	if Finished(p) {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndFinished}
	}
	g := *p.GoalMode
	spent := g.Pushes >= g.MaxPushes
	stalled := now.Sub(p.UpdatedAt) >= GoalStallGrace
	switch p.OpenGate {
	case GatePlan, GatePR, GateMerge:
		step := GoalStep{Kind: GoalGate, Gate: p.OpenGate, Action: GateApprove}
		if i := p.openGateCard(); i >= 0 {
			step.CardID = p.Cards[i].ID
		}
		return step
	case GateTriage:
		for _, id := range p.Review.Triage {
			if i := p.cardIndex(id); i >= 0 && p.Cards[i].State != CardDecided {
				return GoalStep{Kind: GoalCard, CardID: id, Decision: goalFindingDecision(p.Cards[i])}
			}
		}
		return wait
	case GateBlocked:
		return goalBlockedStep(p, g, spent, now)
	}
	if !p.Active() {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndStopped}
	}
	for i, c := range p.Cards {
		if c.State == CardDecided {
			continue
		}
		switch {
		case c.Kind == CardDecision:
			return GoalStep{Kind: GoalCard, CardID: c.ID, Decision: GoalDecisionAnswer}
		case c.Kind == CardFinding && c.Stage == StagePRReview && p.Current == StagePRReview && i >= p.PRFixCursor:
			return GoalStep{Kind: GoalCard, CardID: c.ID, Decision: FindingFix}
		}
	}
	if !stalled {
		return wait
	}
	if p.PendingTurn != "" || p.AwaitingVerification {
		// The step the pipeline owes was dropped (nothing runs): raise the
		// interrupted gate, whose retry resumes exactly that step.
		return GoalStep{Kind: GoalInterrupt}
	}
	if p.goalWaitsOnPR() {
		return wait
	}
	if len(p.Cards) == 0 && p.Current == StagePlan {
		// Nothing has run yet (a pipeline started with no console open):
		// the first turn is the goal itself.
		return GoalStep{Kind: GoalNudge, Prompt: p.FirstTurn()}
	}
	if spent {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndExhausted}
	}
	return GoalStep{Kind: GoalNudge, Push: true, Prompt: p.goalNudgePrompt()}
}

func goalBlockedStep(p Pipeline, g GoalMode, spent bool, now time.Time) GoalStep {
	fact := p.openBlockedFact()
	if fact.Reason == BlockedPRClosed {
		// Someone closed the pull request: that is a decision, not a glitch.
		return GoalStep{Kind: GoalEnd, Reason: GoalEndPRClosed}
	}
	if fact.Failure != nil && fact.Failure.E2E {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndE2EFailed}
	}
	if spent {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndExhausted}
	}
	step := GoalStep{Kind: GoalGate, Gate: GateBlocked, Action: GateRetry, Push: true}
	i := p.openGateCard()
	if i >= 0 {
		step.CardID = p.Cards[i].ID
	}
	if fact.Reason == BlockedTurnFailed {
		step.FailedTurn = true
		if i >= 0 {
			if until := p.Cards[i].CreatedAt.Add(goalRetryDelay(g.Failures)); now.Before(until) {
				return GoalStep{Kind: GoalWait, Until: until}
			}
		}
	}
	return step
}

// goalRetryDelay is the wait before retrying after failures consecutive
// failed turns.
func goalRetryDelay(failures int) time.Duration {
	d := goalRetryBase
	for range max(failures, 0) {
		if d *= 2; d >= goalRetryMax {
			return goalRetryMax
		}
	}
	return d
}

// goalFindingDecision is the triage policy: low-severity findings are
// dismissed, everything else (and anything unlabelled) is fixed.
func goalFindingDecision(c Card) string {
	var f Finding
	if decodePayload(c.Payload, &f) && strings.EqualFold(strings.TrimSpace(f.Severity), "low") {
		return DecisionDismiss
	}
	return DecisionFix
}

// goalWaitsOnPR reports whether an idle pipeline is waiting on pull request
// facts the server's probe brings, not on a turn.
func (p Pipeline) goalWaitsOnPR() bool {
	switch p.Current {
	case StagePR:
		return p.PRWait != ""
	case StagePRReview, StageMerge:
		return true
	}
	return false
}

// goalNudgePrompt is the turn that pushes an idle stage on.
func (p Pipeline) goalNudgePrompt() string {
	return fmt.Sprintf("Goal mode: the %s stage stopped with nothing decided. Continue it toward the goal, following the <focus-stage> instructions, and end your reply with the block they ask for.", p.Current)
}

// goalGuidance is the line added to every turn's guidance in goal mode.
const goalGuidance = "Goal mode: nobody is watching this pipeline and its gates are approved automatically. " +
	"Do not ask the developer questions — decide yourself, say what you chose and why in the report's summary, " +
	"and keep going until the goal is done."

// FirstTurn is the text of a pipeline's first turn: its kickoff, or the
// goal when it has none.
func (p Pipeline) FirstTurn() string {
	if kickoff := strings.TrimSpace(p.Kickoff); kickoff != "" {
		return kickoff
	}
	return strings.TrimSpace(p.Goal)
}
