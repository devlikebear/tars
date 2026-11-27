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
//     idle with a turn owed or nothing owed at all is pushed with one;
//   - a turn that failed on the provider's usage limit ends goal mode: the
//     limit lifts in hours, and every retry until then is refused;
//   - a retry of a verification failure that comes back as the same
//     failure on an unchanged working tree is empty: after
//     GoalEmptyRetryLimit of them in a row goal mode ends
//     (GoalEndNoProgress, GoalCheckRetry) instead of spending the rest of
//     the budget on the same nothing.
//
// Retries, pushes and answer turns draw on a budget (MaxPushes): an agent
// that asks again after every "decide yourself" is a loop like any other.
// When it is spent goal mode ends where it stands and the gate waits for
// the developer. NextGoalStep
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
	// GoalEndUsageLimit: a turn failed on the provider's usage limit
	// (UsageLimitError).
	GoalEndUsageLimit = "usage_limit"
	// GoalEndRepeatedQuestion: the agent asks again a question goal mode
	// already answered in this stage. "Decide yourself" did not settle it
	// the first time and will not the next: answering it again only spends
	// the budget one turn at a time (seen in dogfooding: thirteen turns
	// re-asking whether to keep an end-to-end goal that contradicted the
	// task). The question is left to the developer.
	GoalEndRepeatedQuestion = "repeated_question"
	// GoalEndNoProgress: GoalEmptyRetryLimit retries in a row of a blocked
	// gate came back blocked on the same verification failure with the
	// working tree exactly as before (GoalCheckRetry). Seen in a real run:
	// twenty retries, each ending with the team reporting nothing in the
	// code was wrong, none changing a file. Another retry is the same
	// nothing again, so the gate is left to the person.
	GoalEndNoProgress = "no_progress"
	GoalEndCancelled  = "cancelled"
	GoalEndDisabled   = "disabled"
)

// Push budget.
const (
	DefaultGoalPushes = 20
	MaxGoalPushes     = 100
)

// GoalEmptyRetryLimit is how many empty retries in a row (GoalCheckRetry)
// end goal mode. Not one: a single retry that changed nothing can be a team
// that spent it reading before it acts (each retry is a fresh round of at
// least a fix turn and a verification), and one more costs a round, not
// the budget. Two in a row with the same failure and the same tree is a
// loop: nothing in the next retry's input differs from the last two.
const GoalEmptyRetryLimit = 2

// GoalEmptyRetryWindow is the least time a run of empty retries must span,
// from the first retry of the run to now, before it ends goal mode. Some
// failures pass later with no file changed: a check that fails while a
// port frees, a server warms up or a rate limit lifts. A fast model gets
// through several rounds in seconds, so the count alone would end goal
// mode on a failure that was about to go away (the console's
// easy-workspace e2e has a check that fails for its first 12 seconds).
// Five minutes outlasts that kind of wait and costs a handful of rounds;
// the run that motivated this rule spent far longer on its twenty retries.
const GoalEmptyRetryWindow = 5 * time.Minute

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
// It asks for the decision in the summary and not as the question again:
// in dogfooding (2026-11-15) an agent decided, then copied the same question
// into the next report's decisions, which ends goal mode as a repeated
// question (GoalEndRepeatedQuestion).
const GoalDecisionAnswer = "Decide yourself: choose the option that best serves the goal, say which one and why in your report's " +
	"summary, and continue. This question is settled: do not list it again under decisions."

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
	// LastRetry is what the last retry of a blocked gate saw (its failure
	// and the working tree), and EmptyRetries how many retries in a row
	// found both unchanged from the retry before (GoalCheckRetry).
	LastRetry    *GoalRetryMark `json:"last_retry,omitempty"`
	EmptyRetries int            `json:"empty_retries,omitempty"`
	// PermissionSet says the server put the session in the auto permission
	// mode for goal mode, and RestorePermission what it had before.
	PermissionSet     bool   `json:"permission_set,omitempty"`
	RestorePermission string `json:"restore_permission,omitempty"`
}

// GoalRetryMark is what goal mode saw when it retried a blocked gate: the
// gate's failure (GoalRetryFailure) and the session's working tree
// fingerprint (focusprobe.WorkTreeState), both read by the server, never
// taken from a turn's report.
//
// Since is when the run of empty retries this mark belongs to began: the
// time of the first retry that saw this failure on this tree.
type GoalRetryMark struct {
	Failure string    `json:"failure"`
	Tree    string    `json:"tree"`
	Since   time.Time `json:"since"`
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
	// Retry and EmptyRetries are what a blocked gate's retry saw and how
	// many empty retries in a row it makes (GoalCheckRetry); GoalApplied
	// records them.
	Retry        *GoalRetryMark
	EmptyRetries int
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
	case GoalEndExhausted, GoalEndPRClosed, GoalEndE2EFailed, GoalEndUsageLimit, GoalEndRepeatedQuestion, GoalEndNoProgress, GoalEndCancelled:
		payload := map[string]any{
			"reason": reason, "pushes": next.GoalMode.Pushes, "max_pushes": next.GoalMode.MaxPushes,
		}
		if reason == GoalEndNoProgress {
			payload["empty_retries"] = GoalEmptyRetryLimit
		}
		next.addCard(CardNotice, 0, NoticeGoalEnded, payload, at)
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
	if step.Kind == GoalGate && step.Gate == GateBlocked {
		// Every retry replaces the mark: one without a failure or a tree
		// (a failed turn, an unreadable tree) breaks the run of empty ones.
		g.LastRetry, g.EmptyRetries = step.Retry, step.EmptyRetries
	}
	if step.FailedTurn {
		g.Failures++
	}
	next.UpdatedAt = now.UTC()
	return next
}

// GoalDueOnApproval reports whether answering gate with action on p turns
// goal mode on, on a pipeline that asked for it after the plan
// (GoalAfterPlan, an easy-mode project):
//
//   - the plan's approval, when the pipeline has never been in goal mode;
//   - a retry of a turn the provider's usage limit stopped, when that limit
//     is what ended goal mode (GoalEndUsageLimit): the person waited for
//     the limit to lift and asks the team to go on as it was;
//   - a retry (or a retry with the person's own words, instruct) of the
//     blocked gate goal mode spent its budget on (GoalEndExhausted): the
//     team got stuck, and the person's "Try again" gives it a new budget
//     instead of leaving it to wait at every later step with no one to
//     answer. The same holds for a gate goal mode stopped retrying because
//     the retries changed nothing (GoalEndNoProgress): the person's own
//     words, or their go-ahead, is what differs this time.
//
// Goal mode turned off by a person, or ended for any other reason, stays
// off.
func GoalDueOnApproval(p Pipeline, gate, action string) bool {
	if !p.GoalAfterPlan {
		return false
	}
	switch {
	case gate == GatePlan && action == GateApprove:
		return p.GoalMode == nil
	case gate == GateBlocked && (action == GateRetry || action == GateInstruct):
		g := p.GoalMode
		if g == nil || g.Enabled || p.OpenGate != GateBlocked {
			return false
		}
		switch g.EndReason {
		case GoalEndUsageLimit:
			return action == GateRetry && p.UsageLimitStop() != nil
		case GoalEndExhausted, GoalEndNoProgress:
			return p.UsageLimitStop() == nil
		}
	}
	return false
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
			if spent {
				return GoalStep{Kind: GoalEnd, Reason: GoalEndExhausted}
			}
			if goalAnsweredBefore(p, c) {
				return GoalStep{Kind: GoalEnd, Reason: GoalEndRepeatedQuestion}
			}
			// The last open question of a turn sends the answer turn: that
			// draws on the budget, once however many questions the turn asked.
			return GoalStep{Kind: GoalCard, CardID: c.ID, Decision: GoalDecisionAnswer, Push: openTurnDecisions(p, c) == 1}
		case c.Kind == CardFinding && c.Stage == StagePRReview && p.Current == StagePRReview && i >= p.PRFixCursor:
			return GoalStep{Kind: GoalCard, CardID: c.ID, Decision: FindingFix}
		}
	}
	if !stalled {
		return wait
	}
	if p.PendingTurn != "" || p.AwaitingVerification || p.Review.SecondOpinionOwed {
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

// goalAnsweredBefore reports whether goal mode already answered card's
// question in the same stage: a decided decision card of that stage, decided
// by goal mode, whose question (the card title) reads the same once case,
// spacing and trailing punctuation are set aside.
func goalAnsweredBefore(p Pipeline, card Card) bool {
	question := normalizeQuestion(card.Title)
	if question == "" {
		return false
	}
	for _, c := range p.Cards {
		if c.ID == card.ID || c.Kind != CardDecision || c.State != CardDecided || c.DecidedBy != AnswerGoal || c.Stage != card.Stage {
			continue
		}
		if normalizeQuestion(c.Title) == question {
			return true
		}
	}
	return false
}

func normalizeQuestion(q string) string {
	return strings.TrimRight(strings.Join(strings.Fields(strings.ToLower(q)), " "), "?.!: ")
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
	if p.UsageLimitStop() != nil {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndUsageLimit}
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

// GoalRetryFailure identifies the verification failure the open blocked
// gate stopped on, for comparing one retry with the next: the failed
// commands with their exit codes and time-outs, and the check's own report
// when it printed one. The output excerpt is left out — a log line with a
// time or a build hash differs on every run of the same failure, and the
// working tree comparison beside it already says whether anything could
// have changed. "" when the gate carries no verification failure (a failed
// or interrupted turn, a missing PR): those retries are never empty.
func GoalRetryFailure(p Pipeline) string {
	f := p.openBlockedFact().Failure
	if f == nil {
		return ""
	}
	var b strings.Builder
	failed := 0
	for _, r := range f.Results {
		if r.Passed || r.Skipped {
			continue
		}
		failed++
		fmt.Fprintf(&b, "%s\x00%d\x00%t\x01", strings.TrimSpace(r.Command), r.ExitCode, r.TimedOut)
	}
	if failed == 0 {
		fmt.Fprintf(&b, "%s\x00%d\x00%t\x01", strings.TrimSpace(f.Command), f.ExitCode, f.TimedOut)
	}
	if c := f.Check; c != nil {
		fmt.Fprintf(&b, "check\x00%s\x00%s\x00%s", c.Screen, c.Expected, c.Seen)
	}
	return b.String()
}

// GoalCheckRetry looks at a step NextGoalStep returned with the working
// tree the server just read (tree, "" when it could not be read). A retry
// of the open blocked gate is empty when the gate stopped on the same
// failure as at the last retry (GoalRetryFailure) and the tree is the
// same as it was then: the round that retry started changed nothing and
// failed the same way. Once GoalEmptyRetryLimit retries in a row were
// empty and that run spans at least GoalEmptyRetryWindow (from its first
// retry to now), the step becomes the end of goal mode
// (GoalEndNoProgress); any
// other retry comes back carrying what it saw, for GoalApplied to record,
// which resets the run when the failure or the tree changed. Any other
// step is returned as is.
// Pure: the tree is read by the caller and now passed in.
func GoalCheckRetry(p Pipeline, step GoalStep, tree string, now time.Time) GoalStep {
	if step.Kind != GoalGate || step.Gate != GateBlocked || step.Action != GateRetry || !p.GoalActive() {
		return step
	}
	failure := GoalRetryFailure(p)
	if failure == "" || tree == "" {
		return step // nothing to compare: not empty, and the run breaks
	}
	step.Retry = &GoalRetryMark{Failure: failure, Tree: tree, Since: now.UTC()}
	if last := p.GoalMode.LastRetry; last != nil && last.Failure == failure && last.Tree == tree {
		step.EmptyRetries = p.GoalMode.EmptyRetries + 1
		if !last.Since.IsZero() {
			step.Retry.Since = last.Since
		}
	}
	if step.EmptyRetries >= GoalEmptyRetryLimit && now.Sub(step.Retry.Since) >= GoalEmptyRetryWindow {
		return GoalStep{Kind: GoalEnd, Reason: GoalEndNoProgress, EmptyRetries: step.EmptyRetries}
	}
	return step
}

// usageLimitPhrases are how providers word a spent usage allowance, as
// opposed to a short rate limit: Claude Code's "You've hit your session
// limit · resets 2:40pm" and "Claude AI usage limit reached", Codex's
// "You've hit your usage limit", and TARS's own "usage limit exceeded".
var usageLimitPhrases = []string{
	"session limit",
	"usage limit",
	"weekly limit",
	"5-hour limit",
	"hit your limit",
}

// UsageLimitError reports whether a failed turn's error says the
// provider's usage allowance is spent. Retrying such a turn is refused
// until the limit resets, hours later, and the retry that finally lands
// spends the fresh allowance on the loop that spent the last one.
func UsageLimitError(errText string) bool {
	text := strings.ToLower(errText)
	for _, phrase := range usageLimitPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
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
