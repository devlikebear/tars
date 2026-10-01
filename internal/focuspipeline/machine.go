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
}

// Action is what the server should do next.
type Action struct {
	Kind   string // ActionSendTurn | ActionRunVerification | ActionNone
	Prompt string // the next user turn's text for ActionSendTurn
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
		stage := next.advance()
		return next, Action{Kind: ActionSendTurn, Prompt: approvedPrompt(GateNone, stage)}, nil
	case EventVerification:
		return applyVerification(p, ev, now.UTC())
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
	for _, f := range b.Findings {
		p.addCard(CardFinding, ev.Turn, f.Title, f, now)
	}
	if b.PR != nil {
		// A report-kind card until P4 brings the G3 gate.
		p.addCard(CardReport, ev.Turn, PRDraftTitle, *b.PR, now)
	}
	missing := missingRequiredBlock(p, b)
	if p.Current == StageBuild && !missing {
		act = buildTurnAction(&p, b)
	} else if p.Current == StageBuild {
		buildTurnAction(&p, Blocks{})
	}
	if missing {
		p.addCard(CardNotice, ev.Turn, NoticeFormatMissing, map[string]any{"errors": nonNil(b.Errors)}, now)
		build, _ := p.Stage(StageBuild)
		switch {
		case p.Current == StageBuild && build.Turns >= buildTurnCap(p):
			p.block(BlockedNoProgress, nil, ev.Turn, now)
		case !priorNotice:
			act = Action{Kind: ActionSendTurn, Prompt: reRequestPrompt(p.Current)}
		}
	} else if len(b.Errors) > 0 {
		p.addCard(CardNotice, ev.Turn, NoticeFormatMissing, map[string]any{"errors": b.Errors}, now)
	}
	p.UpdatedAt = now
	return p, act
}

// missingRequiredBlock reports whether the turn lacked the block its stage
// requires. While the plan gate is open a turn is the developer's question,
// not a new plan, so nothing is required.
func missingRequiredBlock(p Pipeline, b Blocks) bool {
	switch p.Current {
	case StagePlan:
		return b.Plan == nil && p.OpenGate != GatePlan
	case StagePR:
		return b.Report == nil && b.Findings == nil && b.PR == nil
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

func reRequestPrompt(stage StageID) string {
	tag := TagReport
	if stage == StagePlan {
		tag = TagPlan
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
	p.Cards = append(p.Cards, Card{
		ID:        fmt.Sprintf("c%d", len(p.Cards)+1),
		Kind:      kind,
		Stage:     p.Current,
		Turn:      turn,
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
		card.State = CardDecided
		card.Decision = decision
	default:
		return p, noAction, fmt.Errorf("%w: %q", ErrInvalidCardState, state)
	}
	next := p.clone()
	next.Cards[idx] = card
	next.UpdatedAt = now.UTC()
	if card.Kind == CardDecision && card.State == CardDecided {
		act := Action{Kind: ActionSendTurn, Prompt: card.Title + " → " + decision}
		return owe(next, act), act, nil
	}
	return next, noAction, nil
}
