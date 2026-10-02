package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Event kinds Apply understands.
const (
	EventTurnCompleted = "turn_completed"
	EventGate          = "gate"
	// EventAdvance passes the current stage by hand: until fact-based
	// completion exists for it (build before P2, PR stages without gh).
	EventAdvance = "advance"
	// EventStop stops the pipeline at any time, gate open or not.
	EventStop = "stop"
	// EventVerification is the result of the verification a build turn
	// asked for (ActionRunVerification).
	EventVerification = "verification"
)

// Gate actions.
const (
	GateApprove        = "approve"
	GateRequestChanges = "request_changes"
	GateStop           = "stop"
)

// Action kinds Apply returns.
const (
	ActionNone     = "none"
	ActionSendTurn = "send_turn"
	// ActionRunVerification asks the server to run the plan's verify
	// commands and feed the result back as EventVerification.
	ActionRunVerification = "run_verification"
)

// NoticeFormatMissing is the title of the card a turn without its required
// block raises.
const NoticeFormatMissing = "report format missing"

// PRDraftTitle is the title of the card a <focus-pr> draft becomes.
const PRDraftTitle = "PR draft"

// DecisionSuperseded marks a plan gate card replaced by a newer plan.
const DecisionSuperseded = "superseded"

var (
	// ErrGateNotOpen is a gate action on a gate that is not the open one: a
	// double click, or a stale tab.
	ErrGateNotOpen = errors.New("gate not open")
	// ErrInvalidAction is a gate action other than approve, request_changes
	// and stop.
	ErrInvalidAction = errors.New("invalid gate action")
	// ErrInvalidEdits is a G1 edit that leaves the plan without tasks.
	ErrInvalidEdits = errors.New("invalid plan edits")
	// ErrCardNotFound names no card of the pipeline.
	ErrCardNotFound = errors.New("card not found")
	// ErrCardDecided is a decision on a card already decided.
	ErrCardDecided = errors.New("card already decided")
	// ErrInvalidCardState is a card update the card cannot take.
	ErrInvalidCardState = errors.New("invalid card state")
	// ErrCannotAdvance is a manual pass of a stage that is not the active
	// current one, of the plan stage (only G1 approval leaves it), or while
	// a gate is open (the gate decides then).
	ErrCannotAdvance = errors.New("cannot advance this stage")
	// ErrNotActive is a stop of a pipeline already finished or stopped.
	ErrNotActive = errors.New("pipeline is not active")
)

// Event is one fact fed to Apply.
type Event struct {
	Kind string // EventTurnCompleted | EventGate
	// Turn is the transcript turn that completed (EventTurnCompleted), or
	// the turn a verification ran after (EventVerification).
	Turn   int
	Blocks Blocks
	// QuestionGate is the question gate that was open when the completed
	// turn started (EventTurnCompleted): a turn begun as the developer's
	// question stays one even if the gate was decided while it ran.
	QuestionGate string
	// Stage is the stage to pass (EventAdvance); it must be Current, so a
	// stale tab cannot pass the stage after it.
	Stage StageID
	// Gate, Action, Edits and Note describe a gate action (EventGate).
	Gate   string
	Action string
	Edits  *Plan
	Note   string
	// Verification is the verification result (EventVerification).
	Verification *Verification
	// PR is G3's edited draft (EventGate on GatePR); Probe the probe's
	// result (EventPRProbe).
	PR    *PRDraft
	Probe *PRProbe
}

// Action is what the server should do next.
type Action struct {
	Kind   string // ActionSendTurn | ActionRunVerification | ActionNone
	Prompt string // the next user turn's text for ActionSendTurn
	// Answers are the decision answers a send_turn delivers: the server
	// merges queued answers and drops those whose stage moved on.
	Answers []Answer
}

var noAction = Action{Kind: ActionNone}

// Apply is the pipeline's pure state machine: it returns the pipeline after
// ev and the action to take. On error the input pipeline is returned
// unchanged. Apply never modifies p. The event that finishes the pipeline
// stamps FinishedAt.
func Apply(p Pipeline, ev Event, now time.Time) (Pipeline, Action, error) {
	next, act, err := applyEvent(backfillFinished(p.clone()), ev, now)
	if err != nil {
		return p, act, err
	}
	next, act = afterPREvent(p, next, ev, act, now.UTC())
	return owe(stampFinished(next, now), act), act, nil
}

// owe records a send_turn action as the turn the server owes the pipeline
// (PendingTurn); a completed turn or a stop cleared the previous one.
// A finished or stopped pipeline owes nothing: its last prompt ("the
// pipeline is complete") is shown, never sent.
func owe(p Pipeline, act Action) Pipeline {
	if !p.Active() {
		p.PendingTurn = ""
		return p
	}
	if act.Kind == ActionSendTurn && p.PendingTurn != act.Prompt {
		p.PendingTurn = act.Prompt
	}
	return p
}

func applyEvent(p Pipeline, ev Event, now time.Time) (Pipeline, Action, error) {
	switch ev.Kind {
	case EventTurnCompleted:
		next, act := applyTurn(p.clone(), ev, now.UTC())
		return next, act, nil
	case EventGate:
		next, act, err := applyGate(p.clone(), ev, now.UTC())
		if err != nil {
			return p, noAction, err
		}
		return next, act, nil
	case EventAdvance:
		// The plan stage's only exit is G1 approval (ADR §4).
		if !p.Active() || p.OpenGate != GateNone || ev.Stage != p.Current || p.Current == StagePlan {
			return p, noAction, ErrCannotAdvance
		}
		next := p.clone()
		next.UpdatedAt = now.UTC()
		// Passing the stage by hand drops a turn it still owed (one cut
		// off by a cancel): the next stage's own turn or gate replaces it.
		next.PendingTurn = ""
		stage := next.advance()
		return next, Action{Kind: ActionSendTurn, Prompt: approvedPrompt(GateNone, stage)}, nil
	case EventVerification:
		return applyVerification(p, ev, now.UTC())
	case EventPRProbe:
		return applyProbe(p, ev, now.UTC())
	case EventStop:
		if !p.Active() && p.OpenGate != GateBlocked {
			return p, noAction, ErrNotActive
		}
		next := p.clone()
		if i := next.openGateCard(); i >= 0 && next.OpenGate != GateNone {
			next.Cards[i].State = CardDecided
			next.Cards[i].Decision = GateStop
		}
		next.OpenGate = GateNone
		next.AwaitingVerification, next.PendingTurn = false, ""
		next.setStatus(next.Current, StatusBlocked)
		next.UpdatedAt = now.UTC()
		return next, noAction, nil
	default:
		return p, noAction, fmt.Errorf("unknown event kind %q", ev.Kind)
	}
}

func applyTurn(p Pipeline, ev Event, now time.Time) (Pipeline, Action) {
	if !p.Active() {
		return p, noAction
	}
	if questionGates[ev.QuestionGate] && p.OpenGate != ev.QuestionGate {
		return lateQuestionTurn(p, ev, now)
	}
	if p.Current == StageReview && p.OpenGate == GateTriage {
		return triageTurn(p, now)
	}
	b := ev.Blocks
	act := noAction
	// A turn completed: the turn the server owed (if any) is not owed any
	// more. Asked before this turn's cards land on top of the last one.
	p.PendingTurn = ""
	priorNotice := lastCardIsFormatNotice(p)
	if p.Current == StagePlan && b.Plan != nil {
		supersedeOpenGate(&p)
		plan := clonePlan(*b.Plan)
		plan.Stages = normalizeStages(plan.Stages)
		p.Plan = &plan
		p.addCard(CardGate, ev.Turn, "Approve plan", plan, now)
		p.OpenGate = GatePlan
	}
	if b.Report != nil {
		p.addCard(CardReport, ev.Turn, reportTitle(*b.Report), *b.Report, now)
		for _, d := range b.Report.Decisions {
			p.addCard(CardDecision, ev.Turn, d.Question, d, now)
		}
	}
	if p.Current != StageReview {
		for _, f := range b.Findings {
			p.addCard(CardFinding, ev.Turn, f.Title, f, now)
		}
	}
	switch {
	case b.PR != nil && p.Current == StagePR && p.PRWait == "":
		p.openPRGate(*b.PR, ev.Turn, now)
	case b.PR != nil:
		p.addCard(CardReport, ev.Turn, PRDraftTitle, *b.PR, now)
	}
	missing := missingRequiredBlock(p, b)
	if p.Current == StageBuild && !missing {
		act = buildTurnAction(&p, b)
	} else if p.Current == StageBuild {
		buildTurnAction(&p, Blocks{})
	} else if p.Current == StageReview && !missing {
		act = reviewTurnAction(&p, b, ev.Turn, now)
	}
	if missing {
		p.addCard(CardNotice, ev.Turn, NoticeFormatMissing, map[string]any{"errors": nonNil(b.Errors)}, now)
		build, _ := p.Stage(StageBuild)
		switch {
		case p.Current == StageBuild && build.Turns >= buildTurnCap(p):
			p.block(BlockedNoProgress, nil, ev.Turn, now)
		case !priorNotice:
			act = Action{Kind: ActionSendTurn, Prompt: reRequestPrompt(p)}
		}
	} else if len(b.Errors) > 0 {
		p.addCard(CardNotice, ev.Turn, NoticeFormatMissing, map[string]any{"errors": b.Errors}, now)
	}
	p.UpdatedAt = now
	return p, act
}

// lateQuestionTurn completes a turn that started as the developer's question
// at a gate decided while it ran: it is still a question, not the stage's
// work — the turn the gate's decision owes stays owed, no block is
// required, and only its report (and the decisions it asks) is recorded.
func lateQuestionTurn(p Pipeline, ev Event, now time.Time) (Pipeline, Action) {
	if r := ev.Blocks.Report; r != nil {
		p.addCard(CardReport, ev.Turn, reportTitle(*r), *r, now)
		for _, d := range r.Decisions {
			p.addCard(CardDecision, ev.Turn, d.Question, d, now)
		}
	}
	p.UpdatedAt = now
	return p, noAction
}

// questionGates are the gates during which a turn is the developer's
// question, not the stage's work: G1 (plan), P3's triage, G3 (PR draft) and
// G4 (merge). Nothing is required of such a turn.
var questionGates = map[string]bool{GatePlan: true, GateTriage: true, GatePR: true, GateMerge: true}

// AnswersMayRun reports whether a turn delivering decision answers may run
// now: the pipeline is active and either no gate is open or the open gate
// takes questions. Behind a blocked gate the answers wait.
func AnswersMayRun(p Pipeline) bool {
	return p.Active() && (p.OpenGate == GateNone || questionGates[p.OpenGate])
}

// missingRequiredBlock reports whether the turn lacked the block its stage
// requires.
func missingRequiredBlock(p Pipeline, b Blocks) bool {
	if questionGates[p.OpenGate] {
		return false
	}
	switch p.Current {
	case StagePlan:
		return b.Plan == nil
	case StagePR:
		return b.Report == nil && b.Findings == nil && b.PR == nil
	case StageReview:
		if p.Review.Fixing {
			return b.Report == nil
		}
		return b.Findings == nil
	default:
		return b.Report == nil && b.Findings == nil
	}
}

func lastCardIsFormatNotice(p Pipeline) bool {
	// The card just added this turn is not there yet.
	if len(p.Cards) == 0 {
		return false
	}
	last := p.Cards[len(p.Cards)-1]
	return last.Kind == CardNotice && last.Title == NoticeFormatMissing
}

func reRequestPrompt(p Pipeline) string {
	tag := TagReport
	switch {
	case p.Current == StagePlan:
		tag = TagPlan
	case p.Current == StageReview && !p.Review.Fixing:
		tag = TagFindings
	}
	return fmt.Sprintf("Your last reply did not end with a well-formed <%s> block. Reply again, following the <focus-stage> instructions, and end with exactly one <%s>…</%s> block containing valid JSON.", tag, tag, tag)
}

func reportTitle(r Report) string {
	summary := strings.TrimSpace(r.Summary)
	if summary == "" {
		return "Stage report"
	}
	if i := strings.IndexAny(summary, "\n"); i >= 0 {
		summary = summary[:i]
	}
	const maxTitle = 120
	if r := []rune(summary); len(r) > maxTitle {
		summary = string(r[:maxTitle-1]) + "…"
	}
	return summary
}

func supersedeOpenGate(p *Pipeline) {
	if p.OpenGate == GateNone {
		return
	}
	if i := p.openGateCard(); i >= 0 {
		p.Cards[i].State = CardDecided
		p.Cards[i].Decision = DecisionSuperseded
	}
	p.OpenGate = GateNone
}

func applyGate(p Pipeline, ev Event, now time.Time) (Pipeline, Action, error) {
	switch ev.Action {
	case GateApprove, GateRequestChanges, GateStop, GateRetry, GateInstruct:
	default:
		return p, noAction, ErrInvalidAction
	}
	if p.OpenGate == GateNone || ev.Gate != p.OpenGate {
		return p, noAction, ErrGateNotOpen
	}
	if p.OpenGate == GateTriage && ev.Action != GateStop {
		return p, noAction, fmt.Errorf("%w: triage closes when every finding is decided", ErrInvalidAction)
	}
	gate := p.OpenGate
	cardIdx := p.openGateCard()
	decide := func() {
		if cardIdx >= 0 {
			p.Cards[cardIdx].State = CardDecided
			p.Cards[cardIdx].Decision = ev.Action
		}
		p.OpenGate = GateNone
		p.UpdatedAt = now
	}
	if gate == GateBlocked {
		return applyBlockedGate(&p, ev, decide)
	}
	switch ev.Action {
	case GateRetry, GateInstruct:
		return p, noAction, fmt.Errorf("%w: %s is only for the blocked gate", ErrInvalidAction, ev.Action)
	case GateStop:
		p.setStatus(p.Current, StatusBlocked)
		p.AwaitingVerification, p.PendingTurn = false, ""
		decide()
		return p, noAction, nil
	case GateRequestChanges:
		decide()
		return p, Action{Kind: ActionSendTurn, Prompt: requestChangesPrompt(gate, ev.Note)}, nil
	}
	// approve
	if gate == GatePlan {
		if ev.Edits != nil {
			plan, err := normalizePlan(clonePlan(*ev.Edits))
			if err != nil {
				return p, noAction, fmt.Errorf("%w: %v", ErrInvalidEdits, err)
			}
			p.Plan = &plan
		}
		if p.Plan == nil {
			return p, noAction, fmt.Errorf("%w: no plan to approve", ErrInvalidEdits)
		}
		p.skipUnplannedStages()
	}
	if gate == GatePR || gate == GateMerge {
		return approvePRGate(&p, gate, ev, decide)
	}
	decide()
	next := p.advance()
	return p, Action{Kind: ActionSendTurn, Prompt: approvedPrompt(gate, next)}, nil
}

func requestChangesPrompt(gate, note string) string {
	note = strings.TrimSpace(note)
	lead := "Changes requested."
	if gate == GatePlan {
		lead = "Revise the plan."
	}
	if note == "" {
		return lead
	}
	return lead + " " + note
}

func approvedPrompt(gate string, next StageID) string {
	lead := "Approved."
	if gate == GatePlan {
		lead = "Plan approved."
	}
	switch next {
	case "":
		return lead + " The pipeline is complete."
	case StageBuild:
		return lead + " Start the build stage with task 1."
	default:
		return fmt.Sprintf("%s Start the %s stage.", lead, next)
	}
}

// skipUnplannedStages marks the stages the plan leaves out as skipped.
func (p *Pipeline) skipUnplannedStages() {
	keep := map[StageID]bool{StagePlan: true}
	for _, id := range p.Plan.Stages {
		keep[id] = true
	}
	for i := range p.Stages {
		if !keep[p.Stages[i].ID] && p.Stages[i].Status == StatusPending {
			p.Stages[i].Status = StatusSkipped
		}
	}
}

// advance finishes the current stage and activates the next pending one,
// returning its id, or "" when none is left (the pipeline is complete and
// Current stays on the last finished stage).
func (p *Pipeline) advance() StageID {
	p.setStatus(p.Current, StatusDone)
	p.TasksDone, p.AwaitingVerification, p.LastFailure = false, false, nil
	p.Review = ReviewState{}
	p.PRWait, p.PRProbes, p.PRUnavailable = "", 0, ""
	for i := range p.Stages {
		s := &p.Stages[i]
		if s.Status != StatusPending {
			continue
		}
		s.Status = StatusActive
		s.Iteration = 1
		s.Limit = p.limitFor(s.ID)
		p.Current = s.ID
		return s.ID
	}
	return ""
}

func (p Pipeline) limitFor(id StageID) int {
	if p.Plan != nil {
		if n, ok := p.Plan.Limits[string(id)]; ok && n > 0 {
			return n
		}
	}
	if id == StagePRReview {
		// The plan's "pr" limit bounds the PR loop's fix rounds.
		return p.limitFor(StagePR)
	}
	return DefaultLimits[id]
}

func (p *Pipeline) setStatus(id StageID, status StageStatus) {
	for i := range p.Stages {
		if p.Stages[i].ID == id {
			p.Stages[i].Status = status
		}
	}
}

// openGateCard is the index of the undecided gate card of the open gate's
// stage, latest first, or -1.
func (p Pipeline) openGateCard() int {
	for i := len(p.Cards) - 1; i >= 0; i-- {
		c := p.Cards[i]
		if c.Kind == CardGate && c.State != CardDecided {
			return i
		}
	}
	return -1
}

func (p *Pipeline) addCard(kind string, turn int, title string, payload any, now time.Time) {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = nil
	}
	stage, _ := p.Stage(p.Current)
	p.Cards = append(p.Cards, Card{
		ID:        fmt.Sprintf("c%d", len(p.Cards)+1),
		Kind:      kind,
		Stage:     p.Current,
		Turn:      turn,
		Iteration: stage.Iteration,
		Title:     title,
		Payload:   raw,
		State:     CardUnseen,
		CreatedAt: now,
	})
}

// AddNotice adds a notice card to the current stage, for facts the server
// observes outside a turn (a failed tasks write, for one).
func AddNotice(p Pipeline, title string, payload any, now time.Time) Pipeline {
	next := p.clone()
	next.addCard(CardNotice, 0, title, payload, now.UTC())
	next.UpdatedAt = now.UTC()
	return next
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// SetCardState marks a card seen, or decides a decision/finding card. A
// decided decision card returns the answer as the next turn's prompt. Gate
// cards are decided only through their gate (Apply). On error p is returned
// unchanged.
func SetCardState(p Pipeline, cardID, state, decision string, now time.Time) (Pipeline, Action, error) {
	idx := -1
	for i, c := range p.Cards {
		if c.ID == cardID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return p, noAction, ErrCardNotFound
	}
	card := p.Cards[idx]
	decision = strings.TrimSpace(decision)
	switch state {
	case CardSeen:
		if card.State != CardUnseen {
			return p, noAction, nil
		}
		card.State = CardSeen
	case CardDecided:
		if card.Kind == CardGate {
			return p, noAction, fmt.Errorf("%w: gate cards are decided through their gate", ErrInvalidCardState)
		}
		if card.State == CardDecided {
			return p, noAction, ErrCardDecided
		}
		if decision == "" && card.Kind == CardDecision {
			return p, noAction, fmt.Errorf("%w: a decision needs an answer", ErrInvalidCardState)
		}
		if card.Kind == CardFinding {
			d, err := validFindingDecision(decision)
			if err != nil {
				return p, noAction, err
			}
			decision = d
		}
		card.State = CardDecided
		card.Decision = decision
	default:
		return p, noAction, fmt.Errorf("%w: %q", ErrInvalidCardState, state)
	}
	next := p.clone()
	next.Cards[idx] = card
	next.UpdatedAt = now.UTC()
	if card.Kind == CardDecision && card.State == CardDecided {
		// The turn's other questions still open: wait, and send every
		// answer as one turn with the last.
		answers, ok := turnAnswers(next, card)
		if !ok {
			return next, noAction, nil
		}
		act := Action{Kind: ActionSendTurn, Prompt: AnswersPrompt(answers), Answers: answers}
		if questionGates[next.OpenGate] {
			// A gate waits for the developer (#1079): the answers go out
			// now as the developer's question, like a typed instruction —
			// the gate stays open and the pipeline owes nothing.
			return next, act, nil
		}
		return owe(next, act), act, nil
	}
	if card.Kind == CardFinding && card.State == CardDecided {
		// One decision path for findings: the review stage's triage gate
		// (P3) closes when its round is decided; in pr_review the PR loop
		// (P4) sends its fix round. Each is a no-op outside its own stage.
		act := closeTriage(&next)
		if act.Kind == ActionNone {
			next, act = next.prFindingDecided(now.UTC())
		}
		return owe(next, act), act, nil
	}
	return next, noAction, nil
}
