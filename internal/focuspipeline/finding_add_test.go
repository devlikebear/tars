package focuspipeline

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func devFinding() Finding {
	return Finding{Severity: "high", File: "c.go", Line: 12, Title: "  stale row\nstays ", Scenario: "fail once → the row never clears"}
}

func addFinding(t *testing.T, p Pipeline, f Finding, decision string) (Pipeline, Action, string) {
	t.Helper()
	got, act, id, err := AddFinding(p, f, decision, t0)
	if err != nil {
		t.Fatalf("add finding: %v", err)
	}
	return got, act, id
}

func cardByID(t *testing.T, p Pipeline, id string) Card {
	t.Helper()
	i := p.cardIndex(id)
	if i < 0 {
		t.Fatalf("no card %s", id)
	}
	return p.Cards[i]
}

func TestAddFindingJoinsTheOpenTriage(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	before := len(p.Cards)
	got, act, id := addFinding(t, p, devFinding(), "")
	if act.Kind != ActionNone || got.OpenGate != GateTriage || len(got.Review.Triage) != 3 || got.Review.Triage[2] != id {
		t.Fatalf("act = %+v gate = %q triage = %v", act, got.OpenGate, got.Review.Triage)
	}
	if len(p.Cards) != before || len(p.Review.Triage) != 2 {
		t.Fatal("AddFinding changed its input")
	}
	card := cardByID(t, got, id)
	var f Finding
	if !decodePayload(card.Payload, &f) {
		t.Fatal("payload")
	}
	if card.Kind != CardFinding || card.Stage != StageReview || card.State != CardUnseen || card.Title != "stale row stays" {
		t.Fatalf("card = %+v", card)
	}
	if f.Source != FindingSourceDeveloper || f.Severity != "high" || f.File != "c.go" || f.Line != 12 || f.ID == "" {
		t.Fatalf("finding = %+v", f)
	}
	// Decided with the agent's: all three ride one fix prompt.
	triage := got.Review.Triage
	got, _ = decide(t, got, triage[0], DecisionFix)
	got, _ = decide(t, got, triage[1], DecisionDismiss)
	got, act = decide(t, got, id, DecisionFix)
	if act.Kind != ActionSendTurn || got.OpenGate != GateNone || !got.Review.Fixing {
		t.Fatalf("act = %+v gate = %q", act, got.OpenGate)
	}
	for _, want := range []string{"nil deref", "[high] c.go:12 — stale row stays (added by the developer)", "Scenario: fail once"} {
		if !strings.Contains(act.Prompt, want) {
			t.Fatalf("fix prompt lacks %q:\n%s", want, act.Prompt)
		}
	}
	if strings.Contains(act.Prompt, "typo") || strings.Contains(act.Prompt, "nil deref (added") {
		t.Fatalf("fix prompt:\n%s", act.Prompt)
	}
}

func TestAddFindingAsFixIsDecidedFromTheStart(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	// The agent's findings still wait: the gate stays open.
	got, act, id := addFinding(t, p, devFinding(), " Fix ")
	if card := cardByID(t, got, id); card.State != CardDecided || card.Decision != DecisionFix {
		t.Fatalf("card = %+v", card)
	}
	if act.Kind != ActionNone || got.OpenGate != GateTriage {
		t.Fatalf("act = %+v gate = %q", act, got.OpenGate)
	}
	if _, _, err := SetCardState(got, id, CardDecided, DecisionDismiss, t0); !errors.Is(err, ErrCardDecided) {
		t.Fatalf("deciding it again: %v", err)
	}

	// Once the agent's are decided the triage closes with it accepted.
	got, _ = decide(t, got, got.Review.Triage[0], DecisionDismiss)
	got, act = decide(t, got, got.Review.Triage[1], DecisionDismiss)
	if act.Kind != ActionSendTurn || got.OpenGate != GateNone || !strings.Contains(act.Prompt, "stale row stays") || strings.Contains(act.Prompt, "nil deref") {
		t.Fatalf("act = %+v gate = %q", act, got.OpenGate)
	}
}

func TestAddedFindingIsDismissedLikeAnyOther(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings()[:1])
	p, _, id := addFinding(t, p, devFinding(), "")
	p, _ = decide(t, p, p.Review.Triage[0], DecisionDismiss)
	if p.OpenGate != GateTriage {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	p, _ = decide(t, p, id, DecisionDismiss)
	if p.OpenGate != GateNone || !p.AwaitingVerification {
		t.Fatalf("all dismissed: gate %q awaiting %v", p.OpenGate, p.AwaitingVerification)
	}
	// A dismissed developer finding is remembered like any other.
	if !p.Review.dismissed(Finding{File: "c.go", Title: "Stale row stays"}) {
		t.Fatalf("dismissed = %+v", p.Review.Dismissed)
	}
}

func TestAddFindingDuringTheReviewTurnJoinsItsTriage(t *testing.T) {
	tests := []struct {
		name       string
		decision   string
		agent      []Finding
		wantGate   string
		wantAction string
		wantTriage int
	}{
		{name: "with agent findings", agent: twoFindings(), wantGate: GateTriage, wantAction: ActionNone, wantTriage: 3},
		{name: "agent found nothing", agent: []Finding{}, wantGate: GateTriage, wantAction: ActionNone, wantTriage: 1},
		{name: "as fix, agent found nothing", decision: DecisionFix, agent: []Finding{}, wantGate: GateNone, wantAction: ActionSendTurn},
		{name: "as fix, with agent findings", decision: DecisionFix, agent: twoFindings(), wantGate: GateTriage, wantAction: ActionNone, wantTriage: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, act, id := addFinding(t, inReview(t), devFinding(), tt.decision)
			if act.Kind != ActionNone || p.OpenGate != GateNone || len(p.Review.Triage) != 1 || p.PendingTurn == "" {
				t.Fatalf("before the turn ends: act %+v gate %q triage %v pending %q", act, p.OpenGate, p.Review.Triage, p.PendingTurn)
			}
			got, act := reviewTurn(t, p, tt.agent)
			if got.OpenGate != tt.wantGate || act.Kind != tt.wantAction || len(got.Review.Triage) != tt.wantTriage {
				t.Fatalf("gate %q act %+v triage %v", got.OpenGate, act, got.Review.Triage)
			}
			if tt.wantTriage > 0 && got.Review.Triage[0] != id {
				t.Fatalf("triage = %v, want %s first", got.Review.Triage, id)
			}
			if tt.wantAction == ActionSendTurn && (!got.Review.Fixing || !strings.Contains(act.Prompt, "stale row stays")) {
				t.Fatalf("fix turn: %+v", act)
			}
		})
	}
}

func TestAddFindingDuringAFixTurnWaitsForTheNextRound(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings()[:1])
	p, act := decide(t, p, p.Review.Triage[0], DecisionFix)
	if act.Kind != ActionSendTurn || !p.Review.Fixing {
		t.Fatalf("fix turn: %+v", act)
	}
	p, act, id := addFinding(t, p, devFinding(), DecisionFix)
	if act.Kind != ActionNone || p.OpenGate != GateNone || !p.Review.Fixing {
		t.Fatalf("during the fix turn: %+v gate %q", act, p.OpenGate)
	}
	p, act = turn(p, t, &Report{Summary: "fixed"})
	if act.Kind != ActionRunVerification {
		t.Fatalf("after the fix turn: %+v", act)
	}
	// Verification runs; a second one added meanwhile waits too.
	p, _, second := addFinding(t, p, Finding{Title: "missing timer"}, "")
	p, act = verify(p, t, passed())
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "Review the changes again") || len(p.Review.Triage) != 2 {
		t.Fatalf("new round: %+v triage %v", act, p.Review.Triage)
	}
	// The round's review finds nothing: triage opens on the developer's two.
	p, act = reviewTurn(t, p, []Finding{})
	if p.OpenGate != GateTriage || act.Kind != ActionNone || p.Review.Triage[0] != id || p.Review.Triage[1] != second {
		t.Fatalf("gate %q act %+v triage %v", p.OpenGate, act, p.Review.Triage)
	}
	p, act = decide(t, p, second, DecisionFix)
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "stale row stays") || !strings.Contains(act.Prompt, "[medium] (no file) — missing timer") {
		t.Fatalf("fix turn: %+v", act)
	}
	if p.Review.Triage != nil {
		t.Fatalf("triage after closing = %v", p.Review.Triage)
	}
}

func TestAddFindingWhileTheLastVerificationRunsKeepsTheStageOpen(t *testing.T) {
	p, act := reviewTurn(t, inReview(t), []Finding{})
	if act.Kind != ActionRunVerification {
		t.Fatalf("act = %+v", act)
	}
	undecided, _, _ := addFinding(t, p, devFinding(), "")
	got, act := verify(undecided, t, passed())
	if got.Current != StageReview || got.OpenGate != GateTriage || act.Kind != ActionNone {
		t.Fatalf("current %s gate %q act %+v", got.Current, got.OpenGate, act)
	}
	asFix, _, _ := addFinding(t, p, devFinding(), DecisionFix)
	got, act = verify(asFix, t, passed())
	if got.Current != StageReview || got.OpenGate != GateNone || act.Kind != ActionSendTurn || !got.Review.Fixing || got.PendingTurn != act.Prompt {
		t.Fatalf("current %s gate %q act %+v", got.Current, got.OpenGate, act)
	}
	// A failed verification is fixed first; the finding still waits.
	got, act = verify(asFix, t, failed("make test", "boom"))
	if act.Kind != ActionSendTurn || len(got.Review.Triage) != 1 || got.Current != StageReview {
		t.Fatalf("failed verification: %+v triage %v", act, got.Review.Triage)
	}
}

func TestAddFindingInPRReview(t *testing.T) {
	p := inPRReview(t, failing("test"))
	p, act, id := addFinding(t, p, devFinding(), "")
	card := cardByID(t, p, id)
	var f PRFinding
	if !decodePayload(card.Payload, &f) || f.Key != "developer:"+id || !f.Trusted || f.Source != FindingSourceDeveloper || card.Stage != StagePRReview {
		t.Fatalf("card %+v finding %+v", card, f)
	}
	if act.Kind != ActionNone || p.PRWait != "" {
		t.Fatalf("undecided: %+v wait %q", act, p.PRWait)
	}
	// Green checks do not pass the stage over an undecided finding.
	green := probeFound(PRCheck{Name: "test", State: CheckPass})
	if held, _ := mustApply(t, p, Event{Kind: EventPRProbe, Probe: green}); held.Current != StagePRReview {
		t.Fatal("advanced past an undecided developer finding")
	}
	p, act = decideAll(t, p, FindingFix)
	if act.Kind != ActionSendTurn || p.PRWait != PRWaitFix {
		t.Fatalf("fix round: %+v wait %q", act, p.PRWait)
	}
	for _, want := range []string{"- CI check failed: test", "- stale row stays — c.go:12 (added by the developer)\n  Scenario: fail once", "git push"} {
		if !strings.Contains(act.Prompt, want) {
			t.Fatalf("fix prompt lacks %q:\n%s", want, act.Prompt)
		}
	}
	if strings.Contains(act.Prompt, "pr-comment") {
		t.Fatalf("the developer's words quoted as a comment:\n%s", act.Prompt)
	}

	// Added as fix while that fix turn runs: the round after it.
	p, act, _ = addFinding(t, p, Finding{Title: "wrong subject"}, DecisionFix)
	if act.Kind != ActionNone || p.PRWait != PRWaitFix {
		t.Fatalf("during the fix turn: %+v", act)
	}
	p, act = mustApply(t, p, Event{Kind: EventTurnCompleted, Turn: 6, Blocks: Blocks{Report: &Report{Summary: "fixed"}}})
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "wrong subject") || strings.Contains(act.Prompt, "stale row") || p.PRWait != PRWaitFix {
		t.Fatalf("next fix round: %+v wait %q", act, p.PRWait)
	}
}

func TestAddFindingAsFixInPRReviewSendsTheFixRound(t *testing.T) {
	p := inPRReview(t, PRCheck{Name: "test", State: CheckPending})
	p, act, _ := addFinding(t, p, devFinding(), DecisionFix)
	if act.Kind != ActionSendTurn || p.PRWait != PRWaitFix || p.PendingTurn != act.Prompt || stageOf(p, StagePRReview).Iteration != 2 {
		t.Fatalf("act %+v wait %q", act, p.PRWait)
	}
	// With another finding still undecided it waits for that decision.
	p = inPRReview(t, failing("test"))
	if _, act, _ := addFinding(t, p, devFinding(), DecisionFix); act.Kind != ActionNone {
		t.Fatalf("act = %+v", act)
	}
}

func TestAddFindingRefused(t *testing.T) {
	blocked := inReview(t)
	blocked.block(BlockedLimit, nil, 0, t0)
	stopped, _, err := Apply(inReview(t), Event{Kind: EventStop}, t0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		p    Pipeline
		want string
	}{
		{name: "plan stage", p: New("s1", "goal", t0), want: "review and pr_review"},
		{name: "build stage", p: goalBuilding(t, StagePlan, StageBuild, StageReview), want: "review and pr_review"},
		{name: "blocked gate", p: blocked, want: "blocked gate"},
		{name: "merge gate", p: inMerge(t), want: "request changes"},
		{name: "stopped", p: stopped, want: "finished or stopped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, act, id, err := AddFinding(tt.p, devFinding(), "", t0)
			if !errors.Is(err, ErrCannotAddFinding) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v", err)
			}
			if id != "" || act.Kind != ActionNone || len(got.Cards) != len(tt.p.Cards) {
				t.Fatalf("id %q act %+v", id, act)
			}
		})
	}
}

func TestAddFindingValidates(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	tests := []struct {
		name     string
		f        Finding
		decision string
	}{
		{name: "no title", f: Finding{Title: " \n "}},
		{name: "long title", f: Finding{Title: strings.Repeat("가", findingTitleRunes+1)}},
		{name: "long scenario", f: Finding{Title: "x", Scenario: strings.Repeat("s", excerptRunes+1)}},
		{name: "file with a newline", f: Finding{Title: "x", File: "a.go\nb.go"}},
		{name: "negative line", f: Finding{Title: "x", Line: -1}},
		{name: "unknown severity", f: Finding{Title: "x", Severity: "critical"}},
		{name: "added as dismissed", f: Finding{Title: "x"}, decision: DecisionDismiss},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := AddFinding(p, tt.f, tt.decision, t0); !errors.Is(err, ErrInvalidFinding) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	got, _, id := addFinding(t, p, Finding{Title: strings.Repeat("가", findingTitleRunes), Severity: " LOW "}, "")
	var f Finding
	if !decodePayload(cardByID(t, got, id).Payload, &f) || f.Severity != "low" {
		t.Fatalf("finding = %+v", f)
	}
}

func TestParsedFindingsNeverClaimTheDeveloper(t *testing.T) {
	b := ParseBlocks(`<focus-findings>[{"id":"f1","severity":"high","file":"a.go","line":1,"title":"x","scenario":"y","source":"developer"}]</focus-findings>`)
	if len(b.Findings) != 1 || b.Findings[0].Source != "" {
		t.Fatalf("findings = %+v", b.Findings)
	}
}

// Goal mode's policy is unchanged for the developer's findings: low is
// dismissed, anything else fixed; in pr_review they are fixed.
func TestNextGoalStepDecidesDeveloperFindings(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild, StageReview)
	p, _ = turn(p, t, &Report{Summary: "done", TasksDone: true})
	p, _ = verify(p, t, passed())
	p, _, low := addFinding(t, p, Finding{Title: "nit", Severity: "low"}, "")
	p, _, high := addFinding(t, p, Finding{Title: "crash"}, "")
	// No triage yet: goal mode leaves them to the round's triage.
	if step := NextGoalStep(p, t0); step.Kind == GoalCard {
		t.Fatalf("step before triage = %+v", step)
	}
	p, _ = reviewTurn(t, p, []Finding{})
	want := map[string]string{low: DecisionDismiss, high: DecisionFix}
	var act Action
	for range 2 {
		step := NextGoalStep(p, t0)
		if step.Kind != GoalCard || step.Decision != want[step.CardID] {
			t.Fatalf("step = %+v", step)
		}
		p, act = decide(t, p, step.CardID, step.Decision)
	}
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "crash") || strings.Contains(act.Prompt, "nit") {
		t.Fatalf("fix turn = %+v", act)
	}

	pr := StartGoal(inPRReview(t, PRCheck{Name: "test", State: CheckPending}), 0, t0)
	pr, _, id := addFinding(t, pr, Finding{Title: "nit", Severity: "low"}, "")
	if step := NextGoalStep(pr, t0.Add(time.Second)); step.Kind != GoalCard || step.CardID != id || step.Decision != FindingFix {
		t.Fatalf("pr_review step = %+v", step)
	}
}
