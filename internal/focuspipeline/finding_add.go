package focuspipeline

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// The developer's own findings (#1196). Findings otherwise come from the
// agent's <focus-findings> block (review) and from the PR probe (pr_review);
// AddFinding is the entry for what the developer found reading the diff. It
// adds a finding card and nothing else: the card joins the round the stage
// already runs, is decided like any other finding (fix or dismiss, by the
// developer or by goal mode's policy), and reaches the agent in the same
// fix prompt.
//
// When it is allowed:
//
//	review     while the stage is active and no blocked gate is open. With
//	           triage open the card joins it. Otherwise (a review or fix turn
//	           runs, verification runs) it waits in Review.Triage for the
//	           next triage: the one the running review turn opens, the next
//	           round's after a fix, or one opened in place of ending the
//	           stage when verification passes with nothing fixed.
//	pr_review  while the stage is active and no gate is open. The card
//	           belongs to the fix round being collected; one added while a
//	           fix turn runs goes to the round after it.
//
// Anywhere else (another stage, a blocked gate, G4, a finished or stopped
// pipeline) it is refused with ErrCannotAddFinding: nothing there would
// ever pick the card up. A running turn does not refuse it — a card is not
// a turn, and the turn a decision asks for queues behind the running one
// like a card decision's.
//
// A finding added with the decision "fix" is a decided card from the start:
// it never waits for the developer again and rides the round's fix turn
// with the rest. Where it is all the round holds, that turn goes out with
// no further click: pr_review sends its fix round at once, and a review
// triage that opens on nothing but decided cards closes as it opens.

// FindingSourceDeveloper marks a finding the developer added.
const FindingSourceDeveloper = "developer"

// Limits on a developer finding's text.
const (
	findingTitleRunes = 200
	findingFileRunes  = 400
)

var (
	// ErrInvalidFinding is a developer finding without a title, or with a
	// field out of range.
	ErrInvalidFinding = errors.New("invalid finding")
	// ErrCannotAddFinding is a developer finding where no round would take
	// it: outside review and pr_review, or behind a gate other than triage.
	ErrCannotAddFinding = errors.New("a finding cannot be added now")
)

// AddFinding adds the developer's finding to the current review or
// pr_review round and returns the new card's id. decision is "" (decide it
// on the card) or DecisionFix. On error p is returned unchanged.
func AddFinding(p Pipeline, f Finding, decision string, now time.Time) (Pipeline, Action, string, error) {
	f, err := developerFinding(f)
	if err != nil {
		return p, noAction, "", err
	}
	switch decision = strings.ToLower(strings.TrimSpace(decision)); decision {
	case "", DecisionFix:
	default:
		return p, noAction, "", fmt.Errorf("%w: a finding is added undecided or as %q", ErrInvalidFinding, DecisionFix)
	}
	review := p.CurrentKind() == StageReview && (p.OpenGate == GateNone || p.OpenGate == GateTriage)
	prReview := p.Current == StagePRReview && p.OpenGate == GateNone
	if !p.Active() || (!review && !prReview) {
		return p, noAction, "", fmt.Errorf("%w: %s", ErrCannotAddFinding, cannotAddReason(p))
	}
	now = now.UTC()
	next := p.clone()
	id := next.nextCardID()
	f.ID = "dev-" + id
	if review {
		next.addCard(CardFinding, 0, f.Title, f, now)
	} else {
		next.addCard(CardFinding, 0, f.Title, PRFinding{Finding: f, Key: "developer:" + id, Trusted: true}, now)
	}
	if decision == DecisionFix {
		card := &next.Cards[len(next.Cards)-1]
		card.State, card.Decision = CardDecided, DecisionFix
	}
	next.UpdatedAt = now
	if review {
		// An open triage has an undecided card, so this never closes it:
		// the round's last decision does, or the triage opened later.
		next.Review.Triage = append(next.Review.Triage, id)
		return next, noAction, id, nil
	}
	next, act := next.prFindingDecided(now)
	return owe(next, act), act, id, nil
}

// developerFinding validates and normalizes the fields the developer sent.
func developerFinding(f Finding) (Finding, error) {
	out := Finding{
		Severity: strings.ToLower(strings.TrimSpace(f.Severity)),
		File:     strings.TrimSpace(f.File),
		Line:     f.Line,
		Title:    strings.Join(strings.Fields(f.Title), " "),
		Scenario: strings.TrimSpace(f.Scenario),
		Excerpt:  f.Excerpt,
		Source:   FindingSourceDeveloper,
	}
	switch {
	case out.Title == "":
		return f, fmt.Errorf("%w: title is required", ErrInvalidFinding)
	case utf8.RuneCountInString(out.Title) > findingTitleRunes:
		return f, fmt.Errorf("%w: title is longer than %d characters", ErrInvalidFinding, findingTitleRunes)
	case utf8.RuneCountInString(out.Scenario) > excerptRunes:
		return f, fmt.Errorf("%w: scenario is longer than %d characters", ErrInvalidFinding, excerptRunes)
	case utf8.RuneCountInString(out.File) > findingFileRunes || strings.ContainsAny(out.File, "\r\n"):
		return f, fmt.Errorf("%w: file is not a path", ErrInvalidFinding)
	case out.Line < 0:
		return f, fmt.Errorf("%w: line is negative", ErrInvalidFinding)
	}
	switch out.Severity {
	case "":
		out.Severity = "medium"
	case "high", "medium", "low":
	default:
		return f, fmt.Errorf("%w: severity is high, medium or low", ErrInvalidFinding)
	}
	return out, nil
}

func cannotAddReason(p Pipeline) string {
	switch {
	case !p.Active() && p.OpenGate != GateBlocked:
		return "the pipeline is finished or stopped"
	case p.OpenGate == GateBlocked:
		return "decide the blocked gate first"
	case p.OpenGate == GateMerge:
		return "at the merge gate, request changes instead"
	default:
		return "findings are added in the review and pr_review stages"
	}
}

// developerMark tells the agent a listed finding is the developer's own.
func developerMark(f Finding) string {
	if f.Source == FindingSourceDeveloper {
		return " (added by the developer)"
	}
	return ""
}
