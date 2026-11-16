package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Resuming a stopped pipeline. Stop (EventStop, or stop on a gate) leaves
// the current stage blocked with no gate open; before resume there was no
// way back, so a developer who stopped by mistake — or typed "resume" into
// the session's chat, which only ever ran a plain chat turn with no stage
// guidance — lost the pipeline.

// EventResume picks a stopped pipeline up at the stage it stopped in.
const EventResume = "resume"

// ErrNotStopped is a resume of a pipeline that is not stopped: active,
// waiting at a gate, or finished.
var ErrNotStopped = errors.New("pipeline is not stopped")

// Stopped reports whether the pipeline was stopped: its current stage is
// blocked with no gate open (a blocked stage with a gate open waits for the
// developer instead) and it has not finished.
func (p Pipeline) Stopped() bool {
	stage, ok := p.Stage(p.Current)
	return ok && stage.Status == StatusBlocked && p.OpenGate == GateNone && !Finished(p)
}

// applyResume reopens what the stop closed. A gate stopped by its own stop
// action opens again as a new card with the same payload (the stopped card
// keeps its decision, so the history still says it was stopped), and the
// developer decides it as before; no turn is sent. A review stopped at
// triage reopens triage on its finding cards. Anything else — a stop while
// a turn or a verification ran, or between steps — reactivates the stage:
// a pipeline waiting on its pull request goes back to its probes, and the
// rest gets one turn that continues the stage.
func applyResume(p Pipeline, now time.Time) (Pipeline, Action, error) {
	if !p.Stopped() {
		return p, noAction, ErrNotStopped
	}
	next := p.clone()
	next.UpdatedAt = now
	if i := next.stoppedGateCard(); i >= 0 {
		c := next.Cards[i]
		gate := stoppedGateOf(next, c)
		next.addCard(CardGate, c.Turn, c.Title, json.RawMessage(c.Payload), now)
		next.OpenGate = gate
		if gate != GateBlocked {
			// A blocked gate keeps its stage blocked until it is answered.
			next.setStatus(next.Current, StatusActive)
		}
		return next, noAction, nil
	}
	next.setStatus(next.Current, StatusActive)
	if next.CurrentKind() == StageReview && len(next.Review.Triage) > 0 && !next.Review.Fixing {
		return next, next.openTriage(), nil
	}
	if next.PRWait != "" {
		return next, noAction, nil
	}
	return next, Action{Kind: ActionSendTurn, Prompt: resumePrompt(next)}, nil
}

// stoppedGateCard is the index of the gate card a stop decided, when that
// stop is what stopped the pipeline: the newest gate card, of the current
// stage, decided "stop" with a gate the card still names. -1 otherwise.
func (p Pipeline) stoppedGateCard() int {
	for i := len(p.Cards) - 1; i >= 0; i-- {
		c := p.Cards[i]
		if c.Kind != CardGate {
			continue
		}
		if c.Stage != p.Current || c.State != CardDecided || c.Decision != GateStop || stoppedGateOf(p, c) == GateNone {
			return -1
		}
		return i
	}
	return -1
}

// stoppedGateOf reads which gate a gate card was. Cards do not record it,
// so it comes from what each gate's card carries: a blocked gate's payload
// has its reason, and the plan, PR and merge gates have their own titles
// in the stages they open in. GateNone when the card is none of them.
func stoppedGateOf(p Pipeline, c Card) string {
	var fact BlockedFact
	if len(c.Payload) > 0 && json.Unmarshal(c.Payload, &fact) == nil && fact.Reason != "" {
		return GateBlocked
	}
	stage, _ := p.Stage(c.Stage)
	switch {
	case stage.KindOf() == StagePlan && c.Title == PlanGateTitle:
		return GatePlan
	case stage.KindOf() == StagePR && c.Title == PRGateTitle:
		return GatePR
	case stage.KindOf() == StageMerge && c.Title == MergeGateTitle:
		return GateMerge
	}
	return GateNone
}

// resumePrompt is the turn that continues a stage stopped mid-step. The
// stop may have cut a turn off halfway, so the agent reads where the work
// stands before repeating any of it.
func resumePrompt(p Pipeline) string {
	return fmt.Sprintf("The developer stopped this pipeline and has now resumed it. Continue the %s stage from where it stopped: "+
		"a turn may have been cut off partway, so check the working tree and what is already done before repeating any of it.", p.Current)
}
