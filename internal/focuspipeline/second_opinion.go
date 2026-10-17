package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The second opinion review (docs/decisions/focus-mode.md §5.1). A pipeline
// that opted in when it was created (Pipeline.SecondOpinion) has the first
// round of each review-kind stage reviewed twice: the stage's own review
// turn, and then one read-only review turn on another tier, run by the
// server in a hidden worker session. The second review sees the same diff
// but not what the first one reported.
//
// It only adds finding cards. reviewTurnAction asks for it in place of its
// usual tail (triage, or verification with nothing to triage), and the
// result (EventSecondOpinion) takes that same tail: no new gate, no new
// transition, and nothing asks a model whether the two reviews agree. A
// finding the stage's review already reported is not carded twice — the
// card gets ConfirmedBy instead, decided by file, title and line alone. A
// second opinion that fails leaves one notice and the round goes on as if
// it had reported nothing; it is never retried by itself.

// FindingSourceSecondOpinion marks a finding the second opinion review
// reported, and in ConfirmedBy a finding it reported too.
const FindingSourceSecondOpinion = "second_opinion"

// Second opinion outcomes (SecondOpinionResult.Outcome).
const (
	SecondOpinionOK = "ok"
	// SecondOpinionTimeout: the turn ran out of time.
	SecondOpinionTimeout = "timeout"
	// SecondOpinionError: the turn ended in an error.
	SecondOpinionError = "error"
	// SecondOpinionNoBlock: the reply, asked twice, had no well-formed
	// <focus-findings> block.
	SecondOpinionNoBlock = "no_block"
	// SecondOpinionUnavailable: the server could not run the turn at all
	// (no worker session, the tier gone from the configuration).
	SecondOpinionUnavailable = "unavailable"
)

// Notice titles the second opinion review leaves.
const (
	NoticeSecondOpinionFailed = "second opinion not received"
	NoticeSecondOpinionCapped = "second opinion findings capped"
)

const (
	// secondOpinionMaxCards caps the cards one second opinion adds.
	secondOpinionMaxCards = 10
	// secondOpinionLineSlack is how far apart two findings' lines in one
	// file may be and still be the same finding.
	secondOpinionLineSlack = 3
)

// ErrInvalidSecondOpinion is a second opinion event without its result.
var ErrInvalidSecondOpinion = errors.New("invalid second opinion event")

// SecondOpinion is a pipeline's second opinion review setting and record.
type SecondOpinion struct {
	// Tier is the tier the review runs on: heavy, standard or light.
	Tier string `json:"tier"`
	// SessionID is the hidden worker session the review runs in, made the
	// first time it runs.
	SessionID string `json:"session_id,omitempty"`
	// Done lists the review-kind stage ids whose first round already had
	// its second opinion, so a stage gets one per pipeline run.
	Done []StageID `json:"done,omitempty"`
}

// SecondOpinionResult is what the second opinion review came back with.
type SecondOpinionResult struct {
	// Findings are the reply's <focus-findings>, with excerpts added by the
	// server; empty unless Outcome is SecondOpinionOK.
	Findings []Finding
	Outcome  string
}

// secondOpinionDue reports whether the review round now ending waits for a
// second opinion: the pipeline opted in and the current review-kind stage
// has not had one yet (so it is the stage's first round).
func (p Pipeline) secondOpinionDue() bool {
	return p.SecondOpinion != nil && p.CurrentKind() == StageReview &&
		!slices.Contains(p.SecondOpinion.Done, p.Current)
}

// secondOpinionTier is the opted-in tier, "" when the pipeline did not opt in.
func (p Pipeline) secondOpinionTier() string {
	if p.SecondOpinion == nil {
		return ""
	}
	return p.SecondOpinion.Tier
}

// applySecondOpinionEvent applies the second opinion's result. One that
// arrives when the round no longer waits for it (stopped, gated, moved on)
// changes nothing.
func applySecondOpinionEvent(p Pipeline, ev Event, now time.Time) (Pipeline, Action, error) {
	if ev.SecondOpinion == nil {
		return p, noAction, fmt.Errorf("%w: no result", ErrInvalidSecondOpinion)
	}
	if !p.Active() || p.CurrentKind() != StageReview || p.OpenGate != GateNone || !p.Review.SecondOpinionOwed || p.SecondOpinion == nil {
		return p, noAction, nil // stale: the round has moved on
	}
	next := p.clone()
	act := next.applySecondOpinion(*ev.SecondOpinion, now)
	next.UpdatedAt = now
	return next, act, nil
}

// applySecondOpinion turns the result into cards, marks the stage done and
// takes the round's usual tail.
func (p *Pipeline) applySecondOpinion(r SecondOpinionResult, now time.Time) Action {
	tier := p.secondOpinionTier()
	if r.Outcome == SecondOpinionOK {
		var fresh []Finding
		for _, f := range r.Findings {
			if p.Review.dismissed(f) {
				continue
			}
			if i := p.overlappingReviewCard(f); i >= 0 {
				p.confirmCard(i)
				continue
			}
			fresh = append(fresh, f)
		}
		shown := fresh[:min(len(fresh), secondOpinionMaxCards)]
		for _, f := range shown {
			f.Source = FindingSourceSecondOpinion
			f.ConfirmedBy = nil
			p.addCard(CardFinding, 0, f.Title, f, now)
			p.Review.Triage = append(p.Review.Triage, p.Cards[len(p.Cards)-1].ID)
		}
		if dropped := len(fresh) - len(shown); dropped > 0 {
			p.addCard(CardNotice, 0, NoticeSecondOpinionCapped, map[string]any{"dropped": dropped, "tier": tier}, now)
		}
	} else {
		p.addCard(CardNotice, 0, NoticeSecondOpinionFailed, map[string]string{"reason": secondOpinionReason(r.Outcome), "tier": tier}, now)
	}
	p.SecondOpinion.Done = append(p.SecondOpinion.Done, p.Current)
	p.Review.SecondOpinionOwed = false
	return p.reviewFindingsDone()
}

// secondOpinionReason is a failed outcome's code; anything unknown is an
// error. The error text itself never reaches the card.
func secondOpinionReason(outcome string) string {
	switch outcome {
	case SecondOpinionTimeout, SecondOpinionNoBlock, SecondOpinionUnavailable:
		return outcome
	}
	return SecondOpinionError
}

// overlappingReviewCard is the index of the round's finding card from the
// stage's own review that reports the same thing as f, or -1: the same
// file, and the same title (case ignored) or lines at most
// secondOpinionLineSlack apart.
func (p Pipeline) overlappingReviewCard(f Finding) int {
	file, title := strings.TrimSpace(f.File), strings.TrimSpace(f.Title)
	for _, id := range p.Review.Triage {
		i := p.cardIndex(id)
		if i < 0 || p.Cards[i].Kind != CardFinding {
			continue
		}
		var own Finding
		if !decodePayload(p.Cards[i].Payload, &own) || own.Source != "" {
			continue // the developer's, or another second opinion card
		}
		if strings.TrimSpace(own.File) != file {
			continue
		}
		sameTitle := strings.EqualFold(strings.TrimSpace(own.Title), title)
		nearLine := own.Line > 0 && f.Line > 0 && abs(own.Line-f.Line) <= secondOpinionLineSlack
		if sameTitle || nearLine {
			return i
		}
	}
	return -1
}

// confirmCard marks card i as reported by the second opinion too.
func (p *Pipeline) confirmCard(i int) {
	var f Finding
	if !decodePayload(p.Cards[i].Payload, &f) || slices.Contains(f.ConfirmedBy, FindingSourceSecondOpinion) {
		return
	}
	f.ConfirmedBy = append(f.ConfirmedBy, FindingSourceSecondOpinion)
	raw, err := json.Marshal(f)
	if err != nil {
		return
	}
	p.Cards[i].Payload = raw
}

// secondOpinionMark tells the agent a listed finding came from the second
// opinion review, and on which tier.
func secondOpinionMark(f Finding, tier string) string {
	if f.Source != FindingSourceSecondOpinion {
		return ""
	}
	if tier == "" {
		return " (second opinion)"
	}
	return " (second opinion: " + tier + ")"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
