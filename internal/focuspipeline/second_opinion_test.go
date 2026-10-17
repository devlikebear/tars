package focuspipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// inReviewSO is inReview for a pipeline that opted in to the second opinion
// review on the light tier.
func inReviewSO(t *testing.T) Pipeline {
	t.Helper()
	p := inReview(t)
	p.SecondOpinion = &SecondOpinion{Tier: "light"}
	return p
}

func secondOpinion(t *testing.T, p Pipeline, r SecondOpinionResult) (Pipeline, Action) {
	t.Helper()
	got, act, err := Apply(p, Event{Kind: EventSecondOpinion, SecondOpinion: &r}, t0)
	if err != nil {
		t.Fatal(err)
	}
	return got, act
}

func findingOf(t *testing.T, c Card) Finding {
	t.Helper()
	var f Finding
	if err := json.Unmarshal(c.Payload, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSecondOpinionRequestedOnFirstRoundOnly(t *testing.T) {
	p, act := reviewTurn(t, inReviewSO(t), twoFindings())
	if act.Kind != ActionSecondOpinion || !p.Review.SecondOpinionOwed || p.OpenGate != GateNone || p.AwaitingVerification {
		t.Fatalf("first round: act = %+v owed = %v gate = %q", act, p.Review.SecondOpinionOwed, p.OpenGate)
	}
	if n := len(findingCards(p)); n != 2 {
		t.Fatalf("the review's own findings are cards already: %d", n)
	}
	p, act = secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: []Finding{}})
	if p.OpenGate != GateTriage || act.Kind != ActionNone || p.Review.SecondOpinionOwed {
		t.Fatalf("after the second opinion: gate = %q act = %+v", p.OpenGate, act)
	}
	if got := p.SecondOpinion.Done; len(got) != 1 || got[0] != StageReview {
		t.Fatalf("done = %v", got)
	}
	cards := findingCards(p)
	p, _ = decide(t, p, cards[0].ID, DecisionFix)
	p, act = decide(t, p, cards[1].ID, DecisionDismiss)
	if act.Kind != ActionSendTurn || !p.Review.Fixing {
		t.Fatalf("fix turn: %+v", act)
	}
	p, _ = turn(p, t, &Report{Summary: "fixed"})
	p, act = verify(p, t, passed())
	if act.Kind != ActionSendTurn || stageOf(p, StageReview).Iteration != 2 {
		t.Fatalf("second round: %+v", act)
	}
	p, act = reviewTurn(t, p, []Finding{})
	if act.Kind != ActionRunVerification || p.Review.SecondOpinionOwed {
		t.Fatalf("the round after a fix asks for no second opinion: %+v", act)
	}
}

func TestSecondOpinionTail(t *testing.T) {
	tests := []struct {
		name       string
		main       []Finding
		second     []Finding
		wantGate   string
		wantAction string
		wantCards  int
	}{
		{"none and none verify", []Finding{}, []Finding{}, GateNone, ActionRunVerification, 0},
		{"none and one triage", []Finding{}, []Finding{{Severity: "high", File: "c.go", Line: 4, Title: "leak"}}, GateTriage, ActionNone, 1},
		{"two and a new one", twoFindings(), []Finding{{Severity: "medium", File: "c.go", Line: 4, Title: "leak"}}, GateTriage, ActionNone, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := reviewTurn(t, inReviewSO(t), tc.main)
			p, act := secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: tc.second})
			if p.OpenGate != tc.wantGate || act.Kind != tc.wantAction || len(findingCards(p)) != tc.wantCards {
				t.Fatalf("gate = %q act = %s cards = %d", p.OpenGate, act.Kind, len(findingCards(p)))
			}
			if tc.wantAction == ActionRunVerification && !p.AwaitingVerification {
				t.Fatal("verification is awaited")
			}
		})
	}
}

func TestSecondOpinionOverlap(t *testing.T) {
	// twoFindings: a.go:3 "nil deref", b.go:9 "typo".
	tests := []struct {
		name          string
		finding       Finding
		wantConfirmed string // the main card's title it confirms, "" for a new card
	}{
		{"same title other line", Finding{File: "a.go", Line: 40, Title: "NIL DEREF"}, "nil deref"},
		{"line within three", Finding{File: "a.go", Line: 6, Title: "missing check"}, "nil deref"},
		{"line three below", Finding{File: " b.go ", Line: 6, Title: "label"}, "typo"},
		{"line four away", Finding{File: "a.go", Line: 7, Title: "missing check"}, ""},
		{"other file same title", Finding{File: "c.go", Line: 3, Title: "nil deref"}, ""},
		{"no line other title", Finding{File: "a.go", Title: "missing check"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := reviewTurn(t, inReviewSO(t), twoFindings())
			p, _ = secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: []Finding{tc.finding}})
			cards := findingCards(p)
			confirmed := ""
			for _, c := range cards[:2] {
				f := findingOf(t, c)
				if len(f.ConfirmedBy) > 0 {
					if f.ConfirmedBy[0] != FindingSourceSecondOpinion || confirmed != "" {
						t.Fatalf("confirmed_by = %v", f.ConfirmedBy)
					}
					confirmed = f.Title
				}
			}
			if confirmed != tc.wantConfirmed {
				t.Fatalf("confirmed %q, want %q", confirmed, tc.wantConfirmed)
			}
			wantCards := 2
			if tc.wantConfirmed == "" {
				wantCards = 3
				if f := findingOf(t, cards[2]); f.Source != FindingSourceSecondOpinion || len(p.Review.Triage) != 3 {
					t.Fatalf("new card %+v triage %v", f, p.Review.Triage)
				}
			}
			if len(cards) != wantCards {
				t.Fatalf("cards = %d", len(cards))
			}
		})
	}
}

func TestSecondOpinionConfirmsOnce(t *testing.T) {
	p, _ := reviewTurn(t, inReviewSO(t), twoFindings())
	p, _ = secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: []Finding{
		{File: "a.go", Line: 3, Title: "nil deref"}, {File: "a.go", Line: 4, Title: "nil pointer"},
	}})
	if f := findingOf(t, findingCards(p)[0]); len(f.ConfirmedBy) != 1 || len(findingCards(p)) != 2 {
		t.Fatalf("confirmed_by = %v cards = %d", f.ConfirmedBy, len(findingCards(p)))
	}
}

func TestSecondOpinionSkipsDismissed(t *testing.T) {
	p := inReviewSO(t)
	p.Review.Dismissed = []DismissedFinding{{File: "c.go", Line: 4, Title: "leak"}}
	p, _ = reviewTurn(t, p, []Finding{})
	p, act := secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: []Finding{{File: "c.go", Line: 8, Title: "Leak"}}})
	if len(findingCards(p)) != 0 || act.Kind != ActionRunVerification {
		t.Fatalf("a dismissed finding came back: cards = %d act = %s", len(findingCards(p)), act.Kind)
	}
}

func TestSecondOpinionCap(t *testing.T) {
	var many []Finding
	for i := range 13 {
		many = append(many, Finding{Severity: "medium", File: fmt.Sprintf("f%d.go", i), Line: 1, Title: fmt.Sprintf("issue %d", i)})
	}
	p, _ := reviewTurn(t, inReviewSO(t), []Finding{})
	p, _ = secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: many})
	if n := len(findingCards(p)); n != secondOpinionMaxCards {
		t.Fatalf("cards = %d", n)
	}
	last := p.Cards[len(p.Cards)-1]
	var payload map[string]any
	_ = json.Unmarshal(last.Payload, &payload)
	if last.Kind != CardNotice || last.Title != NoticeSecondOpinionCapped || payload["dropped"] != float64(3) || payload["tier"] != "light" {
		t.Fatalf("notice = %+v %v", last, payload)
	}
}

func TestSecondOpinionFailureGoesOn(t *testing.T) {
	for _, outcome := range []string{SecondOpinionTimeout, SecondOpinionError, SecondOpinionNoBlock, SecondOpinionUnavailable, "garbled"} {
		t.Run(outcome, func(t *testing.T) {
			p, _ := reviewTurn(t, inReviewSO(t), []Finding{})
			p, act := secondOpinion(t, p, SecondOpinionResult{Outcome: outcome, Findings: []Finding{{File: "x.go", Title: "ignored"}}})
			if act.Kind != ActionRunVerification || p.Review.SecondOpinionOwed || len(p.SecondOpinion.Done) != 1 || len(findingCards(p)) != 0 {
				t.Fatalf("act = %s owed = %v done = %v", act.Kind, p.Review.SecondOpinionOwed, p.SecondOpinion.Done)
			}
			last := p.Cards[len(p.Cards)-1]
			var payload map[string]string
			_ = json.Unmarshal(last.Payload, &payload)
			want := outcome
			if outcome == "garbled" {
				want = SecondOpinionError
			}
			if last.Kind != CardNotice || last.Title != NoticeSecondOpinionFailed || payload["reason"] != want || payload["tier"] != "light" || len(payload) != 2 {
				t.Fatalf("notice = %s %v", last.Title, payload)
			}
		})
	}
}

func TestSecondOpinionStaleResult(t *testing.T) {
	p, _ := reviewTurn(t, inReviewSO(t), twoFindings())
	stopped, _, err := Apply(p, Event{Kind: EventStop}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Review.SecondOpinionOwed {
		t.Fatal("stop clears the owed second opinion")
	}
	for name, q := range map[string]Pipeline{"stopped": stopped, "never owed": inReviewSO(t)} {
		got, act := secondOpinion(t, q, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: []Finding{{File: "c.go", Title: "late"}}})
		if act.Kind != ActionNone || len(got.Cards) != len(q.Cards) {
			t.Fatalf("%s: a stale result changed the pipeline", name)
		}
	}
	if _, _, err := Apply(p, Event{Kind: EventSecondOpinion}, t0); err == nil {
		t.Fatal("an event without its result is an error")
	}
}

func TestSecondOpinionInterruptRetry(t *testing.T) {
	p, _ := reviewTurn(t, inReviewSO(t), twoFindings())
	p, ok := Interrupt(p, t0)
	if !ok || p.OpenGate != GateBlocked || p.Review.SecondOpinionOwed {
		t.Fatalf("interrupted = %v gate = %q owed = %v", ok, p.OpenGate, p.Review.SecondOpinionOwed)
	}
	if fact := p.openBlockedFact(); fact.Reason != BlockedInterrupted || !fact.SecondOpinion || fact.Verify || fact.Prompt != "" {
		t.Fatalf("fact = %+v", fact)
	}
	retried, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != ActionSecondOpinion || !retried.Review.SecondOpinionOwed || !retried.Active() || stageOf(retried, StageReview).Iteration != 1 {
		t.Fatalf("retry: act = %+v owed = %v", act, retried.Review.SecondOpinionOwed)
	}
	if len(retried.Review.Triage) != 2 {
		t.Fatalf("the round's cards wait for triage: %v", retried.Review.Triage)
	}
	stopped, _, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateStop}, t0)
	if err != nil || stopped.Review.SecondOpinionOwed || stopped.Active() {
		t.Fatalf("stop: err = %v owed = %v", err, stopped.Review.SecondOpinionOwed)
	}
}

func TestSecondOpinionGoalModeDoesNotStall(t *testing.T) {
	p := StartGoal(inReviewSO(t), 0, t0)
	p, _ = reviewTurn(t, p, twoFindings())
	later := t0.Add(time.Minute)
	if step := NextGoalStep(p, later); step.Kind != GoalInterrupt {
		t.Fatalf("a dropped second opinion is interrupted: %+v", step)
	}
	p, _ = Interrupt(p, later)
	step := NextGoalStep(p, later)
	if step.Kind != GoalGate || step.Action != GateRetry {
		t.Fatalf("step = %+v", step)
	}
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry, CardID: step.CardID}, later)
	if err != nil || act.Kind != ActionSecondOpinion {
		t.Fatalf("retry: %v %+v", err, act)
	}
	p, _ = secondOpinion(t, p, SecondOpinionResult{Outcome: SecondOpinionOK, Findings: []Finding{{Severity: "high", File: "c.go", Line: 4, Title: "leak"}}})
	for range 3 {
		step = NextGoalStep(p, later)
		if step.Kind != GoalCard {
			break
		}
		p, act = decide(t, p, step.CardID, step.Decision)
	}
	if act.Kind != ActionSendTurn || !p.Review.Fixing || !strings.Contains(act.Prompt, "leak (second opinion: light)") {
		t.Fatalf("goal mode decided the round: %+v", act)
	}
}

func TestSecondOpinionMarkInFixPrompt(t *testing.T) {
	got := fixFindingsPrompt([]Finding{
		{Severity: "high", File: "a.go", Line: 1, Title: "one", Source: FindingSourceSecondOpinion},
		{Severity: "low", File: "b.go", Title: "two"},
	}, "heavy")
	if !strings.Contains(got, "a.go:1 — one (second opinion: heavy)\n") || strings.Contains(got, "two (second") {
		t.Fatalf("prompt:\n%s", got)
	}
}

func TestParseBlocksDropsConfirmedBy(t *testing.T) {
	b := ParseBlocks(`<focus-findings>[{"file":"a.go","title":"x","source":"second_opinion","confirmed_by":["second_opinion"]}]</focus-findings>`)
	if len(b.Findings) != 1 || b.Findings[0].Source != "" || b.Findings[0].ConfirmedBy != nil {
		t.Fatalf("findings = %+v", b.Findings)
	}
}
