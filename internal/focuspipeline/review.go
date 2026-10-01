package focuspipeline

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The review loop (ADR §4, P3): a review turn reports findings; each
// becomes a finding card and the triage gate opens. When every card of the
// round is decided, accepted findings go to one fix turn, and verification
// (the plan's verify and end-to-end commands) runs after it. Verification
// passing after fixes starts a new review round; passing with nothing fixed
// ends the stage. A failure loops through a fix turn; the loop limit, or
// the same failure twice, blocks.

// Finding card decisions.
const (
	DecisionFix     = "fix"
	DecisionDismiss = "dismiss"
)

// ReviewBlockedTitle is the title of the review stage's blocked gate card.
const ReviewBlockedTitle = "Review blocked"

// ReviewState is the review loop's position within one round. It is reset
// when the stage ends.
type ReviewState struct {
	// Triage lists the finding cards of the open triage gate.
	Triage []string `json:"triage,omitempty"`
	// Fixing is set while the turn owed or running is a fix turn, which
	// reports instead of reviewing.
	Fixing bool `json:"fixing,omitempty"`
	// Fixed is set once a fix turn reported this round: passing
	// verification then starts a new review round instead of ending.
	Fixed bool `json:"fixed,omitempty"`
	// Failures counts the round's verification failures, capped by the
	// stage limit.
	Failures int `json:"failures,omitempty"`
	// Dismissed are the findings dismissed in earlier rounds of the stage:
	// listed in the review guidance, and dropped when reported again.
	Dismissed []DismissedFinding `json:"dismissed,omitempty"`
}

// DismissedFinding identifies a dismissed finding across rounds.
type DismissedFinding struct {
	File  string `json:"file"`
	Line  int    `json:"line,omitempty"`
	Title string `json:"title"`
}

func (r ReviewState) clone() ReviewState {
	r.Triage = append([]string(nil), r.Triage...)
	r.Dismissed = append([]DismissedFinding(nil), r.Dismissed...)
	return r
}

// nextRound is the state a new review round starts from: only the
// dismissals carry over.
func (r ReviewState) nextRound() ReviewState {
	return ReviewState{Dismissed: append([]DismissedFinding(nil), r.Dismissed...)}
}

// dismissed reports whether f matches a finding dismissed earlier, by file
// and title.
func (r ReviewState) dismissed(f Finding) bool {
	file, title := strings.TrimSpace(f.File), strings.TrimSpace(f.Title)
	return slices.ContainsFunc(r.Dismissed, func(d DismissedFinding) bool {
		return d.File == file && strings.EqualFold(d.Title, title)
	})
}

// triageTurn handles a turn that completed while triage is open: the
// developer's question, answered in the transcript. It changes nothing but
// the owed turn — no cards, no blocks required, findings ignored.
func triageTurn(p Pipeline, now time.Time) (Pipeline, Action) {
	p.PendingTurn = ""
	p.UpdatedAt = now
	return p, noAction
}

// reviewTurnAction handles a completed review-stage turn that carried the
// block it needed (missingRequiredBlock).
func reviewTurnAction(p *Pipeline, b Blocks, turn int, now time.Time) Action {
	if p.Review.Fixing {
		if b.Report != nil && len(b.Report.Decisions) > 0 {
			return noAction // a question pauses the loop; its answer is the next fix turn
		}
		p.Review.Fixing = false
		p.Review.Fixed = true
		p.AwaitingVerification = true
		return Action{Kind: ActionRunVerification}
	}
	findings := slices.DeleteFunc(slices.Clone(b.Findings), p.Review.dismissed)
	if len(findings) == 0 {
		p.AwaitingVerification = true
		return Action{Kind: ActionRunVerification}
	}
	p.Review.Triage = p.Review.Triage[:0]
	for _, f := range findings {
		p.addCard(CardFinding, turn, f.Title, f, now)
		p.Review.Triage = append(p.Review.Triage, p.Cards[len(p.Cards)-1].ID)
	}
	p.OpenGate = GateTriage
	return noAction
}

// validFindingDecision normalizes a finding card's decision.
func validFindingDecision(decision string) (string, error) {
	switch d := strings.ToLower(strings.TrimSpace(decision)); d {
	case DecisionFix, DecisionDismiss:
		return d, nil
	default:
		return "", fmt.Errorf("%w: a finding is decided with fix or dismiss", ErrInvalidCardState)
	}
}

// closeTriage closes the triage gate once every card of the round is
// decided: accepted findings get a fix turn; none accepted runs
// verification, whose pass ends the stage.
func closeTriage(p *Pipeline) Action {
	if p.OpenGate != GateTriage {
		return noAction
	}
	var accepted []Finding
	var dismissed []DismissedFinding
	for _, id := range p.Review.Triage {
		i := p.cardIndex(id)
		if i < 0 {
			continue
		}
		c := p.Cards[i]
		if c.State != CardDecided {
			return noAction
		}
		var f Finding
		if !decodePayload(c.Payload, &f) {
			continue
		}
		switch c.Decision {
		case DecisionFix:
			accepted = append(accepted, f)
		case DecisionDismiss:
			dismissed = append(dismissed, DismissedFinding{
				File: strings.TrimSpace(f.File), Line: f.Line, Title: strings.TrimSpace(f.Title),
			})
		}
	}
	p.OpenGate = GateNone
	p.Review.Triage = nil
	p.Review.Dismissed = append(p.Review.Dismissed, dismissed...)
	if len(accepted) == 0 {
		p.AwaitingVerification = true
		return Action{Kind: ActionRunVerification}
	}
	p.Review.Fixing = true
	return Action{Kind: ActionSendTurn, Prompt: fixFindingsPrompt(accepted)}
}

func (p Pipeline) cardIndex(id string) int {
	return slices.IndexFunc(p.Cards, func(c Card) bool { return c.ID == id })
}

func fixFindingsPrompt(findings []Finding) string {
	var b strings.Builder
	b.WriteString("Fix these findings:\n")
	for i, f := range findings {
		fmt.Fprintf(&b, "%d. [%s] %s — %s\n", i+1, orDash(f.Severity), findingLocation(f), strings.TrimSpace(f.Title))
		if s := strings.TrimSpace(f.Scenario); s != "" {
			fmt.Fprintf(&b, "   Scenario: %s\n", s)
		}
	}
	b.WriteString("\nFix only these findings, run the verification commands yourself, and report.")
	return b.String()
}

func findingLocation(f Finding) string {
	file := strings.TrimSpace(f.File)
	if file == "" {
		return "(no file)"
	}
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", file, f.Line)
	}
	return file
}

// reviewVerificationPassed: after fixes a new review round starts (or the
// limit blocks); with nothing fixed the stage is done.
func (p Pipeline) reviewVerificationPassed(turn int, now time.Time) (Pipeline, Action, error) {
	p.LastFailure = nil
	if !p.Review.Fixed {
		stage := p.advance()
		return p, Action{Kind: ActionSendTurn, Prompt: reviewDonePrompt(stage)}, nil
	}
	s := p.stageRef(StageReview)
	limit := p.stageLimit(StageReview)
	if s.Iteration >= limit {
		p.block(BlockedLimit, nil, turn, now)
		return p, noAction, nil
	}
	s.Iteration++
	s.Limit = limit
	p.Review = p.Review.nextRound()
	return p, Action{Kind: ActionSendTurn, Prompt: reviewAgainPrompt(s.Iteration, limit)}, nil
}

func (p Pipeline) reviewVerificationFailed(v Verification, turn int, now time.Time) (Pipeline, Action, error) {
	s := p.stageRef(StageReview)
	fact := failureFact(v, s.Iteration)
	limit := p.stageLimit(StageReview)
	switch {
	case p.LastFailure != nil && p.LastFailure.key() == fact.key():
		p.block(BlockedRepeated, &fact, turn, now)
		return p, noAction, nil
	case p.Review.Failures >= limit:
		p.block(BlockedLimit, &fact, turn, now)
		return p, noAction, nil
	}
	p.addCard(CardFailure, turn, failureTitle(fact), fact, now)
	p.LastFailure = &fact
	p.Review.Failures++
	p.Review.Fixing = true
	return p, Action{Kind: ActionSendTurn, Prompt: failurePrompt(fact, p.Review.Failures, limit)}, nil
}

func reviewAgainPrompt(iteration, limit int) string {
	return fmt.Sprintf("Verification passed after the fixes. Review the changes again (round %d of %d) and report findings.", iteration, limit)
}

func reviewDonePrompt(next StageID) string {
	const lead = "Verification passed and no accepted finding is open."
	if next == "" {
		return lead + " The pipeline is complete."
	}
	return fmt.Sprintf("%s Start the %s stage.", lead, next)
}

// reviewRetry starts a new review round after the review blocked gate
// (retry), or makes the developer's instruction a fix turn (instruct). It
// runs after the gate raised the stage's iteration and limit, so the
// prompt names the round that starts.
func reviewRetry(p *Pipeline, action string) string {
	p.Review = p.Review.nextRound()
	if action == GateInstruct {
		p.Review.Fixing = true
		return ""
	}
	s, _ := p.Stage(StageReview)
	return reviewAgainRetryPrompt(s.Iteration, p.stageLimit(StageReview))
}

func reviewAgainRetryPrompt(iteration, limit int) string {
	return fmt.Sprintf("Try once more. Review the changes again (round %d of %d) and report findings.", iteration, limit)
}

func blockedTitle(stage StageID) string {
	if stage == StageReview {
		return ReviewBlockedTitle
	}
	return BlockedTitle
}

func decodePayload(raw json.RawMessage, v any) bool {
	return len(raw) > 0 && json.Unmarshal(raw, v) == nil
}

// reviewGuidance is the review stage's instructions and required blocks: a
// review turn reviews the diff since the pipeline's base commit and reports
// findings; a fix turn fixes only the findings it was sent and reports.
func reviewGuidance(p Pipeline) (instructions, blocks string) {
	if p.OpenGate == GateTriage {
		return "Triage in progress: the developer is deciding the reported findings one at a time. " +
			"Answer the developer's question only; do not edit files and do not report findings in this turn.", ""
	}
	if p.Review.Fixing {
		return "Fix only the findings listed in this message (or the verification failure it quotes); " +
				"do not change anything else. Run the verification commands yourself before you finish.",
			reportBlock()
	}
	what := "Review the changes made for this goal"
	if base := strings.TrimSpace(p.BaseCommit); base != "" {
		what = fmt.Sprintf("Review the diff since the stage started (`git diff %s...HEAD`, and `git diff %s` for changes not yet committed)", base, base)
	}
	return what + "; do not edit files in this turn. " +
			"Report every finding in the <focus-findings> block (an empty array when there are none). " +
			"Each finding needs its file and line and a concrete failure scenario (inputs or state → wrong output or crash)." +
			dismissedList(p.Review.Dismissed),
		requiredBlocks(StageReview)
}

func dismissedList(dismissed []DismissedFinding) string {
	if len(dismissed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nAlready dismissed — do not re-report:")
	for _, d := range dismissed {
		fmt.Fprintf(&b, "\n- %s — %s", findingLocation(Finding{File: d.File, Line: d.Line}), d.Title)
	}
	return b.String()
}
