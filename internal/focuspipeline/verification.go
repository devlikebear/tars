package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The build loop (ADR §4, P2): a build turn that reports asks for
// verification; its result either loops (failure card + a fix turn), blocks
// (loop limit, the same failure twice, no progress), continues to the next
// task, or ends the stage when the report said every task is done.

// Blocked gate actions, accepted only on GateBlocked.
const (
	GateRetry    = "retry"
	GateInstruct = "instruct"
)

// Reasons a build blocks, in the blocked gate card's payload.
const (
	BlockedLimit      = "limit"
	BlockedRepeated   = "repeated"
	BlockedNoProgress = "no_progress"
	// BlockedInterrupted is a pipeline the server stopped mid-step (a
	// restart): retry resumes exactly the step that was cut off.
	BlockedInterrupted = "interrupted"
	// BlockedTurnFailed is a turn the server sent that ended in an error
	// (provider timeout, crash): retry sends it again.
	BlockedTurnFailed = "turn_failed"
)

// TurnFailedTitle is the title of the gate a failed server turn raises.
const TurnFailedTitle = "Turn failed"

// InterruptedTitle is the title of the gate an interruption raises.
const InterruptedTitle = "Pipeline interrupted"

// BlockedTitle is the title of the blocked gate card.
const BlockedTitle = "Build blocked"

// excerptRunes caps a failure excerpt carried into a prompt and a card.
const excerptRunes = 4000

// ErrInvalidVerification is a verification event without its facts.
var ErrInvalidVerification = errors.New("invalid verification event")

// Verification is one run of the plan's verification commands.
type Verification struct {
	Passed  bool                 `json:"passed"`
	Results []VerificationResult `json:"results"`
}

// VerificationResult is one command's outcome.
type VerificationResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Passed   bool   `json:"passed"`
	TimedOut bool   `json:"timed_out,omitempty"`
	// Excerpt is the tail of the command's output.
	Excerpt string `json:"excerpt,omitempty"`
}

// FailureFact is a failed verification as a failure card shows it.
type FailureFact struct {
	Command   string               `json:"command"`
	ExitCode  int                  `json:"exit_code"`
	TimedOut  bool                 `json:"timed_out,omitempty"`
	Excerpt   string               `json:"excerpt,omitempty"`
	Iteration int                  `json:"iteration"`
	Results   []VerificationResult `json:"results,omitempty"`
}

// BlockedFact is the blocked gate card's payload.
type BlockedFact struct {
	Reason    string       `json:"reason"`
	Iteration int          `json:"iteration"`
	Limit     int          `json:"limit"`
	Failure   *FailureFact `json:"failure,omitempty"`
	// Prompt and Verify are what an interruption cut off: the turn that was
	// owed, or the verification that was running.
	Prompt string `json:"prompt,omitempty"`
	Verify bool   `json:"verify,omitempty"`
	// Error is why a server turn failed (BlockedTurnFailed).
	Error string `json:"error,omitempty"`
}

var (
	digitRun = regexp.MustCompile(`[0-9]+`)
	spaceRun = regexp.MustCompile(`\s+`)
)

// key identifies a failure across runs: the command and its excerpt with
// numbers (durations, line counts, ports) and spacing ignored.
func (f FailureFact) key() string {
	excerpt := digitRun.ReplaceAllString(f.Excerpt, "#")
	excerpt = spaceRun.ReplaceAllString(strings.TrimSpace(excerpt), " ")
	return strings.TrimSpace(f.Command) + "\x00" + excerpt
}

// buildTurnCap is how many build turns may pass verification without the
// report claiming tasks_done before the loop blocks: two turns per task
// plus the loop limit.
func buildTurnCap(p Pipeline) int {
	tasks := 1
	if p.Plan != nil && len(p.Plan.Tasks) > 0 {
		tasks = len(p.Plan.Tasks)
	}
	return 2*tasks + p.stageLimit(StageBuild)
}

func (p Pipeline) stageLimit(id StageID) int {
	if s, ok := p.Stage(id); ok && s.Limit > 0 {
		return s.Limit
	}
	return p.limitFor(id)
}

func (p *Pipeline) stageRef(id StageID) *Stage {
	for i := range p.Stages {
		if p.Stages[i].ID == id {
			return &p.Stages[i]
		}
	}
	return nil
}

// buildTurnAction decides what a completed build turn asks for. A report
// without decisions runs verification; a decision pauses the loop until it
// is answered.
func buildTurnAction(p *Pipeline, b Blocks) Action {
	if s := p.stageRef(StageBuild); s != nil {
		s.Turns++
	}
	if b.Report == nil || len(b.Report.Decisions) > 0 {
		p.AwaitingVerification = false
		return noAction
	}
	p.TasksDone = b.Report.TasksDone
	p.AwaitingVerification = true
	return Action{Kind: ActionRunVerification}
}

func applyVerification(p Pipeline, ev Event, now time.Time) (Pipeline, Action, error) {
	if ev.Verification == nil {
		return p, noAction, fmt.Errorf("%w: no results", ErrInvalidVerification)
	}
	if !p.Active() || (p.Current != StageBuild && p.Current != StageReview) || p.OpenGate != GateNone || !p.AwaitingVerification {
		return p, noAction, nil // stale: the loop has moved on
	}
	next := p.clone()
	next.AwaitingVerification = false
	next.UpdatedAt = now
	v := *ev.Verification
	if next.Current == StageReview {
		if v.Passed {
			return next.reviewVerificationPassed(ev.Turn, now)
		}
		return next.reviewVerificationFailed(v, ev.Turn, now)
	}
	if v.Passed {
		return next.verificationPassed(ev.Turn, now)
	}
	return next.verificationFailed(v, ev.Turn, now)
}

func (p Pipeline) verificationPassed(turn int, now time.Time) (Pipeline, Action, error) {
	p.LastFailure = nil
	if p.TasksDone {
		stage := p.advance()
		return p, Action{Kind: ActionSendTurn, Prompt: buildDonePrompt(stage)}, nil
	}
	build, _ := p.Stage(StageBuild)
	if build.Turns >= buildTurnCap(p) {
		p.block(BlockedNoProgress, nil, turn, now)
		return p, noAction, nil
	}
	return p, Action{Kind: ActionSendTurn, Prompt: "Verification passed. Continue with the next task."}, nil
}

func (p Pipeline) verificationFailed(v Verification, turn int, now time.Time) (Pipeline, Action, error) {
	build := p.stageRef(StageBuild)
	fact := failureFact(v, build.Iteration)
	repeated := p.LastFailure != nil && p.LastFailure.key() == fact.key()
	limit := p.stageLimit(StageBuild)
	switch {
	case repeated:
		p.block(BlockedRepeated, &fact, turn, now)
		return p, noAction, nil
	case build.Iteration >= limit:
		p.block(BlockedLimit, &fact, turn, now)
		return p, noAction, nil
	}
	p.addCard(CardFailure, turn, failureTitle(fact), fact, now)
	p.LastFailure = &fact
	build.Iteration++
	build.Limit = limit
	return p, Action{Kind: ActionSendTurn, Prompt: failurePrompt(fact, build.Iteration, limit)}, nil
}

// block raises the blocked gate: the stage stops until the developer
// retries, instructs, or stops.
func (p *Pipeline) block(reason string, failure *FailureFact, turn int, now time.Time) {
	stage, _ := p.Stage(p.Current)
	if failure != nil {
		p.LastFailure = failure
	}
	p.raiseBlocked(blockedTitle(p.Current), BlockedFact{
		Reason: reason, Iteration: stage.Iteration, Limit: p.stageLimit(p.Current), Failure: failure,
	}, turn, now)
}

// raiseBlocked opens the blocked gate on the current stage with its card.
func (p *Pipeline) raiseBlocked(title string, fact BlockedFact, turn int, now time.Time) {
	p.setStatus(p.Current, StatusBlocked)
	p.OpenGate = GateBlocked
	p.AwaitingVerification = false
	p.PendingTurn = ""
	p.addCard(CardGate, turn, title, fact, now)
}

// Interrupt raises the blocked gate on a pipeline the server stopped
// mid-step — verification awaited or a turn owed — so a restart never
// resumes it silently (ADR §4: the developer decides). ok is false, and p
// is returned unchanged, when nothing was cut off.
func Interrupt(p Pipeline, now time.Time) (Pipeline, bool) {
	if !p.Active() || p.OpenGate != GateNone || (!p.AwaitingVerification && p.PendingTurn == "") {
		return p, false
	}
	next := p.clone()
	stage, _ := next.Stage(next.Current)
	next.raiseBlocked(InterruptedTitle, BlockedFact{
		Reason: BlockedInterrupted, Iteration: stage.Iteration, Limit: next.stageLimit(next.Current),
		Prompt: p.PendingTurn, Verify: p.AwaitingVerification,
	}, 0, now.UTC())
	next.UpdatedAt = now.UTC()
	return next, true
}

// FailTurn raises the blocked gate when the turn the server owed ended in
// an error, so the loop never stops silently; retry sends the turn again.
// ok is false, and p is returned unchanged, when no turn was owed.
func FailTurn(p Pipeline, errText string, now time.Time) (Pipeline, bool) {
	if !p.Active() || p.OpenGate != GateNone || p.PendingTurn == "" {
		return p, false
	}
	next := p.clone()
	stage, _ := next.Stage(next.Current)
	next.raiseBlocked(TurnFailedTitle, BlockedFact{
		Reason: BlockedTurnFailed, Iteration: stage.Iteration, Limit: next.stageLimit(next.Current),
		Prompt: p.PendingTurn, Error: tailRunes(strings.TrimSpace(errText), excerptRunes),
	}, 0, now.UTC())
	next.UpdatedAt = now.UTC()
	return next, true
}

// resumable reports whether a blocked reason resumes the cut-off step on
// retry instead of starting a new round.
func resumable(reason string) bool {
	switch reason {
	case BlockedInterrupted, BlockedTurnFailed, BlockedPRMissing, BlockedPRClosed, BlockedNotMerged:
		return true
	}
	return false
}

// openBlockedFact is the payload of the open blocked gate's card.
func (p Pipeline) openBlockedFact() BlockedFact {
	var fact BlockedFact
	if i := p.openGateCard(); i >= 0 {
		_ = json.Unmarshal(p.Cards[i].Payload, &fact)
	}
	return fact
}

// applyBlockedGate answers the blocked gate: retry and instruct allow one
// more iteration; stop leaves the stage blocked.
func applyBlockedGate(p *Pipeline, ev Event, decide func()) (Pipeline, Action, error) {
	note := strings.TrimSpace(ev.Note)
	switch ev.Action {
	case GateStop:
		p.AwaitingVerification, p.PendingTurn = false, ""
		decide()
		return *p, noAction, nil
	case GateInstruct:
		if note == "" {
			return *p, noAction, fmt.Errorf("%w: instruct needs a note", ErrInvalidAction)
		}
	case GateRetry:
	default:
		return *p, noAction, fmt.Errorf("%w: the blocked gate takes retry, instruct or stop", ErrInvalidAction)
	}
	fact := p.openBlockedFact()
	s := p.stageRef(p.Current)
	s.Status = StatusActive
	if resumable(fact.Reason) {
		// Resume the step that was cut off, in the same round.
		decide()
		if ev.Action == GateRetry && fact.Verify {
			p.AwaitingVerification = true
			return *p, Action{Kind: ActionRunVerification}, nil
		}
		prompt := note
		if ev.Action == GateRetry {
			prompt = fact.Prompt
			if prompt == "" {
				prompt = retryPrompt(p.LastFailure)
			}
		}
		return *p, Action{Kind: ActionSendTurn, Prompt: prompt}, nil
	}
	prompt := note
	if ev.Action == GateRetry {
		prompt = retryPrompt(p.LastFailure)
		if fact.Prompt != "" {
			// The round the limit held back (a pr_review fix turn).
			prompt = fact.Prompt
		}
	}
	s.Limit = p.stageLimit(p.Current) + 1
	s.Iteration++
	s.Turns = 0
	if p.Current == StageReview {
		if retry := reviewRetry(p, ev.Action); retry != "" {
			prompt = retry
		}
	}
	p.LastFailure = nil
	p.AwaitingVerification = false
	decide()
	return *p, Action{Kind: ActionSendTurn, Prompt: prompt}, nil
}

func failureFact(v Verification, iteration int) FailureFact {
	fact := FailureFact{Iteration: iteration, Results: append([]VerificationResult(nil), v.Results...)}
	for _, r := range v.Results {
		if !r.Passed {
			fact.Command, fact.ExitCode, fact.TimedOut = r.Command, r.ExitCode, r.TimedOut
			fact.Excerpt = tailRunes(strings.TrimSpace(r.Excerpt), excerptRunes)
			break
		}
	}
	if fact.Command == "" {
		fact.Command = "verification"
	}
	for i := range fact.Results {
		fact.Results[i].Excerpt = tailRunes(fact.Results[i].Excerpt, excerptRunes)
	}
	return fact
}

func failureTitle(f FailureFact) string {
	if f.TimedOut {
		return fmt.Sprintf("Verification timed out: %s", f.Command)
	}
	return fmt.Sprintf("Verification failed: %s (exit %d)", f.Command, f.ExitCode)
}

func failurePrompt(f FailureFact, iteration, limit int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verification failed (iteration %d of %d): `%s` ", iteration, limit, f.Command)
	if f.TimedOut {
		b.WriteString("timed out.")
	} else {
		fmt.Fprintf(&b, "ended with exit %d.", f.ExitCode)
	}
	writeExcerpt(&b, f.Excerpt)
	b.WriteString("\n\nFix the failure, run the verification commands yourself, and report.")
	return b.String()
}

func retryPrompt(f *FailureFact) string {
	if f == nil {
		return "Try once more: continue the build and report."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Try once more. The last verification failure was `%s` (exit %d).", f.Command, f.ExitCode)
	writeExcerpt(&b, f.Excerpt)
	b.WriteString("\n\nTake a different approach, run the verification commands yourself, and report.")
	return b.String()
}

func writeExcerpt(b *strings.Builder, excerpt string) {
	if excerpt == "" {
		return
	}
	b.WriteString("\n\n```\n")
	b.WriteString(strings.ReplaceAll(excerpt, "```", "ˋˋˋ"))
	b.WriteString("\n```")
}

func buildDonePrompt(next StageID) string {
	const lead = "Verification passed and every task is done."
	if next == "" {
		return lead + " The pipeline is complete."
	}
	return fmt.Sprintf("%s Start the %s stage.", lead, next)
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n+1:])
}

// RecordQATurn stores the pipeline's Q&A session and the Q&A turn asked
// about a card. It changes nothing else (ADR §8). On error p is returned
// unchanged.
func RecordQATurn(p Pipeline, qaSessionID, cardID string, turn int, now time.Time) (Pipeline, error) {
	found := false
	for _, c := range p.Cards {
		if c.ID == cardID {
			found = true
			break
		}
	}
	if !found {
		return p, ErrCardNotFound
	}
	next := p.clone()
	next.QASessionID = qaSessionID
	if next.QATurns == nil {
		next.QATurns = map[string][]int{}
	}
	next.QATurns[cardID] = append(next.QATurns[cardID], turn)
	next.UpdatedAt = now.UTC()
	return next, nil
}
