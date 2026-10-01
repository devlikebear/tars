package focuspipeline

import (
	"errors"
	"strings"
	"testing"
)

// inReview is an approved pipeline that passed build into review (limit 2),
// stages plan, build, review, pr.
func inReview(t *testing.T) Pipeline {
	t.Helper()
	plan := testPlan(StagePlan, StageBuild, StageReview, StagePR)
	plan.E2E = []string{"make console-e2e"}
	p, _, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p, _, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0); err != nil {
		t.Fatal(err)
	}
	p, _ = turn(p, t, &Report{Summary: "built", TasksDone: true})
	p, _ = verify(p, t, passed())
	if p.Current != StageReview || stageOf(p, StageReview).Limit != 2 {
		t.Fatalf("not in review: %+v", p)
	}
	return p
}

func twoFindings() []Finding {
	return []Finding{
		{ID: "f1", Severity: "high", File: "a.go", Line: 3, Title: "nil deref", Scenario: "nil input → panic"},
		{ID: "f2", Severity: "low", File: "b.go", Line: 9, Title: "typo", Scenario: "label reads wrong"},
	}
}

func reviewTurn(t *testing.T, p Pipeline, findings []Finding) (Pipeline, Action) {
	t.Helper()
	got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 7, Blocks: Blocks{
		Findings: findings, Report: &Report{Summary: "reviewed"},
	}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	return got, act
}

func decide(t *testing.T, p Pipeline, cardID, decision string) (Pipeline, Action) {
	t.Helper()
	got, act, err := SetCardState(p, cardID, CardDecided, decision, t0)
	if err != nil {
		t.Fatalf("decide %s %s: %v", cardID, decision, err)
	}
	return got, act
}

func findingCards(p Pipeline) []Card {
	var out []Card
	for _, c := range p.Cards {
		if c.Kind == CardFinding {
			out = append(out, c)
		}
	}
	return out
}

func TestReviewTurnOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		findings   []Finding
		wantGate   string
		wantAction string
		wantCards  int
		wantVerify bool
	}{
		{name: "findings open triage", findings: twoFindings(), wantGate: GateTriage, wantAction: ActionNone, wantCards: 2},
		{name: "zero findings run verification", findings: []Finding{}, wantGate: GateNone, wantAction: ActionRunVerification, wantVerify: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, act := reviewTurn(t, inReview(t), tt.findings)
			if got.OpenGate != tt.wantGate || act.Kind != tt.wantAction || got.AwaitingVerification != tt.wantVerify {
				t.Fatalf("gate = %q act = %+v awaiting = %v", got.OpenGate, act, got.AwaitingVerification)
			}
			if n := len(findingCards(got)); n != tt.wantCards {
				t.Fatalf("finding cards = %d", n)
			}
			if len(got.Review.Triage) != tt.wantCards {
				t.Fatalf("triage = %v", got.Review.Triage)
			}
		})
	}
}

func TestReviewTurnWithoutFindingsBlockReRequests(t *testing.T) {
	got, act := turn(inReview(t), t, &Report{Summary: "looked"})
	last := got.Cards[len(got.Cards)-1]
	if last.Kind != CardNotice || act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "<focus-findings>") {
		t.Fatalf("card = %+v act = %+v", last, act)
	}
	if got.OpenGate != GateNone || got.AwaitingVerification || got.Current != StageReview {
		t.Fatalf("state moved: %+v", got)
	}
}

func TestTriageDecisions(t *testing.T) {
	tests := []struct {
		name       string
		decisions  []string // for f1, f2
		wantAction string
		wantPrompt []string
		notPrompt  []string
		wantFixing bool
	}{
		{
			name: "mixed sends a fix turn with accepted findings only", decisions: []string{"fix", "dismiss"},
			wantAction: ActionSendTurn, wantPrompt: []string{"Fix these findings", "a.go:3", "nil deref", "nil input → panic"},
			notPrompt: []string{"b.go:9", "typo"}, wantFixing: true,
		},
		{name: "all dismissed runs verification", decisions: []string{"dismiss", "dismiss"}, wantAction: ActionRunVerification},
		{
			name: "all fixed", decisions: []string{"fix", "fix"}, wantAction: ActionSendTurn,
			wantPrompt: []string{"a.go:3", "b.go:9"}, wantFixing: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := reviewTurn(t, inReview(t), twoFindings())
			ids := p.Review.Triage
			p, act := decide(t, p, ids[0], tt.decisions[0])
			if act.Kind != ActionNone || p.OpenGate != GateTriage {
				t.Fatalf("first decision closed triage early: %+v %q", act, p.OpenGate)
			}
			p, act = decide(t, p, ids[1], tt.decisions[1])
			if act.Kind != tt.wantAction || p.OpenGate != GateNone {
				t.Fatalf("act = %+v gate = %q", act, p.OpenGate)
			}
			for _, s := range tt.wantPrompt {
				if !strings.Contains(act.Prompt, s) {
					t.Errorf("prompt lacks %q:\n%s", s, act.Prompt)
				}
			}
			for _, s := range tt.notPrompt {
				if strings.Contains(act.Prompt, s) {
					t.Errorf("prompt has dismissed %q:\n%s", s, act.Prompt)
				}
			}
			if p.Review.Fixing != tt.wantFixing || p.AwaitingVerification == tt.wantFixing {
				t.Fatalf("review = %+v awaiting = %v", p.Review, p.AwaitingVerification)
			}
			if tt.wantAction == ActionSendTurn && p.PendingTurn != act.Prompt {
				t.Fatalf("fix turn not owed: %q", p.PendingTurn)
			}
		})
	}
}

func TestFindingDecisionValidation(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	id := p.Review.Triage[0]
	for _, bad := range []string{"", "maybe", "approve"} {
		if _, _, err := SetCardState(p, id, CardDecided, bad, t0); !errors.Is(err, ErrInvalidCardState) {
			t.Fatalf("decision %q err = %v", bad, err)
		}
	}
	got, _ := decide(t, p, id, " fix ")
	if got.Cards[indexOfCard(got, id)].Decision != DecisionFix {
		t.Fatalf("decision not trimmed: %+v", got.Cards)
	}
	if _, _, err := SetCardState(got, id, CardDecided, "dismiss", t0); !errors.Is(err, ErrCardDecided) {
		t.Fatalf("double decision err = %v", err)
	}
}

func TestTriageGateTakesNoGateActions(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	for _, action := range []string{GateApprove, GateRequestChanges, GateRetry} {
		if _, _, err := Apply(p, Event{Kind: EventGate, Gate: GateTriage, Action: action}, t0); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("%s err = %v", action, err)
		}
	}
	stopped, _, err := Apply(p, Event{Kind: EventStop}, t0)
	if err != nil || stopped.OpenGate != GateNone || statuses(stopped)[StageReview] != StatusBlocked {
		t.Fatalf("stop: %v %+v", err, stopped)
	}
}

func TestReviewLoop(t *testing.T) {
	t.Run("fix then verify then review again", func(t *testing.T) {
		p, _ := reviewTurn(t, inReview(t), twoFindings())
		p, _ = decide(t, p, p.Review.Triage[0], "fix")
		p, _ = decide(t, p, p.Review.Triage[1], "dismiss")
		p, act := turn(p, t, &Report{Summary: "fixed"})
		if act.Kind != ActionRunVerification || !p.Review.Fixed || p.Review.Fixing {
			t.Fatalf("fix turn: act = %+v review = %+v", act, p.Review)
		}
		p, act = verify(p, t, passed())
		if act.Kind != ActionSendTurn || p.Current != StageReview || stageOf(p, StageReview).Iteration != 2 || p.Review.Fixed {
			t.Fatalf("re-review: act = %+v stage = %+v review = %+v", act, stageOf(p, StageReview), p.Review)
		}
		if !strings.Contains(act.Prompt, "Review the changes again") {
			t.Fatalf("prompt = %q", act.Prompt)
		}
		p, act = reviewTurn(t, p, []Finding{})
		if act.Kind != ActionRunVerification {
			t.Fatalf("clean review: %+v", act)
		}
		p, act = verify(p, t, passed())
		if p.Current != StagePR || statuses(p)[StageReview] != StatusDone || act.Kind != ActionSendTurn {
			t.Fatalf("done: current = %s act = %+v", p.Current, act)
		}
		if p.Review.Fixed || p.Review.Fixing || len(p.Review.Triage) > 0 {
			t.Fatalf("review state not cleared: %+v", p.Review)
		}
	})
	t.Run("all dismissed and verified ends the stage", func(t *testing.T) {
		p, _ := reviewTurn(t, inReview(t), twoFindings())
		p, _ = decide(t, p, p.Review.Triage[0], "dismiss")
		p, _ = decide(t, p, p.Review.Triage[1], "dismiss")
		p, _ = verify(p, t, passed())
		if p.Current != StagePR || statuses(p)[StageReview] != StatusDone {
			t.Fatalf("current = %s", p.Current)
		}
	})
	t.Run("last stage completes the pipeline", func(t *testing.T) {
		p := inReview(t)
		p.setStatus(StagePR, StatusSkipped)
		p.Plan.Stages = []StageID{StagePlan, StageBuild, StageReview}
		p, _ = reviewTurn(t, p, []Finding{})
		p, act := verify(p, t, passed())
		if p.Active() || act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "complete") || p.FinishedAt == nil {
			t.Fatalf("finished: %+v act = %+v", p, act)
		}
	})
	t.Run("limit blocks the next round", func(t *testing.T) {
		p := inReview(t)
		for round := 1; round <= 2; round++ {
			p, _ = reviewTurn(t, p, twoFindings()[:1])
			p, _ = decide(t, p, p.Review.Triage[0], "fix")
			p, _ = turn(p, t, &Report{Summary: "fixed"})
			p, _ = verify(p, t, passed())
		}
		if p.OpenGate != GateBlocked || statuses(p)[StageReview] != StatusBlocked {
			t.Fatalf("not blocked: gate = %q", p.OpenGate)
		}
		last := p.Cards[len(p.Cards)-1]
		if last.Title != ReviewBlockedTitle || !strings.Contains(string(last.Payload), `"reason":"limit"`) {
			t.Fatalf("blocked card = %+v", last)
		}
		p, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
		if err != nil || act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "Review the changes again") {
			t.Fatalf("retry: %v %+v", err, act)
		}
		if s := stageOf(p, StageReview); s.Status != StatusActive || s.Iteration != 3 || s.Limit != 3 || p.Review.Fixing {
			t.Fatalf("after retry: %+v %+v", s, p.Review)
		}
	})
	t.Run("instruct on the review blocked gate is a fix turn", func(t *testing.T) {
		p := inReview(t)
		p.block(BlockedLimit, nil, 7, t0)
		p, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateInstruct, Note: "rename the field"}, t0)
		if err != nil || act.Prompt != "rename the field" || !p.Review.Fixing {
			t.Fatalf("instruct: %v %+v %+v", err, act, p.Review)
		}
	})
}

func TestReviewVerificationFailure(t *testing.T) {
	t.Run("failure card and a fix turn, then review again", func(t *testing.T) {
		p, _ := reviewTurn(t, inReview(t), []Finding{})
		p, act := verify(p, t, failed("make console-e2e", "✘ focus.spec.ts"))
		last := p.Cards[len(p.Cards)-1]
		if last.Kind != CardFailure || act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "make console-e2e") {
			t.Fatalf("card = %+v act = %+v", last, act)
		}
		if !p.Review.Fixing || p.Review.Failures != 1 {
			t.Fatalf("review = %+v", p.Review)
		}
		p, act = turn(p, t, &Report{Summary: "fixed e2e"})
		if act.Kind != ActionRunVerification {
			t.Fatalf("fix turn: %+v", act)
		}
		p, act = verify(p, t, passed())
		if act.Kind != ActionSendTurn || stageOf(p, StageReview).Iteration != 2 || p.Review.Failures != 0 {
			t.Fatalf("re-review after failure fix: %+v %+v", act, p.Review)
		}
	})
	t.Run("the same failure twice blocks", func(t *testing.T) {
		p, _ := reviewTurn(t, inReview(t), []Finding{})
		p, _ = verify(p, t, failed("make test", "FAIL x 0.12s"))
		p, _ = turn(p, t, &Report{Summary: "fixed"})
		p, _ = verify(p, t, failed("make test", "FAIL x 0.31s"))
		last := p.Cards[len(p.Cards)-1]
		if p.OpenGate != GateBlocked || !strings.Contains(string(last.Payload), `"reason":"repeated"`) {
			t.Fatalf("not blocked: %+v", last)
		}
	})
	t.Run("failures beyond the limit block", func(t *testing.T) {
		p, _ := reviewTurn(t, inReview(t), []Finding{})
		p, _ = verify(p, t, failed("make test", "one"))
		p, _ = turn(p, t, &Report{Summary: "fixed"})
		p, _ = verify(p, t, failed("make vet", "two"))
		p, _ = turn(p, t, &Report{Summary: "fixed"})
		p, _ = verify(p, t, failed("make lint", "three"))
		if p.OpenGate != GateBlocked {
			t.Fatalf("not blocked after %d failures", p.Review.Failures)
		}
	})
}

func TestReviewStaleVerificationIgnored(t *testing.T) {
	tests := map[string]func(t *testing.T) Pipeline{
		"triage open":     func(t *testing.T) Pipeline { p, _ := reviewTurn(t, inReview(t), twoFindings()); return p },
		"nothing awaited": func(t *testing.T) Pipeline { return inReview(t) },
		"fix turn in flight": func(t *testing.T) Pipeline {
			p, _ := reviewTurn(t, inReview(t), twoFindings()[:1])
			p, _ = decide(t, p, p.Review.Triage[0], "fix")
			return p
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			p := setup(t)
			got, act := verify(p, t, passed())
			if act.Kind != ActionNone || got.Current != p.Current || len(got.Cards) != len(p.Cards) || got.OpenGate != p.OpenGate {
				t.Fatalf("stale verification changed state: %+v", act)
			}
		})
	}
}

func TestFindingsOutsideReviewOpenNoTriage(t *testing.T) {
	got, _, err := Apply(inBuild(t), Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{
		Report: &Report{Summary: "s"}, Findings: twoFindings(),
	}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenGate != GateNone || len(got.Review.Triage) != 0 {
		t.Fatalf("build findings opened triage: %+v", got.Review)
	}
	if got, _ := decide(t, got, findingCards(got)[0].ID, "dismiss"); got.OpenGate != GateNone {
		t.Fatal("deciding a build finding changed the gate")
	}
}

func TestReviewCloneIsDeep(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	before := p.Review.Triage[0]
	_, _, _ = SetCardState(p, before, CardDecided, "fix", t0)
	c := p.clone()
	c.Review.Triage[0] = "x"
	if p.Review.Triage[0] != before {
		t.Fatal("clone shares the triage slice")
	}
}

func indexOfCard(p Pipeline, id string) int {
	for i, c := range p.Cards {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func TestReviewGuidance(t *testing.T) {
	tests := []struct {
		name string
		edit func(p *Pipeline)
		want []string
		not  []string
	}{
		{
			name: "review turn with base",
			edit: func(p *Pipeline) { p.BaseCommit = "abc1234" },
			want: []string{"git diff abc1234...HEAD", "do not edit files", "empty array", "<focus-findings>"},
			not:  []string{"Fix only"},
		},
		{
			name: "review turn without base",
			edit: func(*Pipeline) {},
			want: []string{"Review the changes made for this goal", "<focus-findings>", "empty array"},
			not:  []string{"git diff", "Fix only"},
		},
		{
			name: "fix turn",
			edit: func(p *Pipeline) { p.BaseCommit = "abc1234"; p.Review.Fixing = true },
			want: []string{"Fix only the findings listed", "<focus-report>", "- make test"},
			not:  []string{"<focus-findings>", "do not edit files"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := atStage(t, StageReview)
			tt.edit(&p)
			g := Guidance(p)
			for _, s := range tt.want {
				if !strings.Contains(g, s) {
					t.Errorf("guidance lacks %q:\n%s", s, g)
				}
			}
			for _, s := range tt.not {
				if strings.Contains(g, s) {
					t.Errorf("guidance has %q:\n%s", s, g)
				}
			}
		})
	}
}

func TestBaseCommitSurvivesClone(t *testing.T) {
	p := inReview(t)
	p.BaseCommit = "abc1234"
	got, _ := reviewTurn(t, p, []Finding{})
	if got.BaseCommit != "abc1234" {
		t.Fatalf("base = %q", got.BaseCommit)
	}
}

// I1 / f1: a turn while triage is open is the developer's question.
func TestTurnDuringTriageIsAQuestion(t *testing.T) {
	tests := []struct {
		name   string
		blocks Blocks
	}{
		{name: "findings in the reply", blocks: Blocks{Findings: []Finding{{ID: "n1", File: "c.go", Line: 1, Title: "new"}}, Report: &Report{Summary: "s"}}},
		{name: "empty findings", blocks: Blocks{Findings: []Finding{}}},
		{name: "no block", blocks: Blocks{}},
		{name: "malformed block", blocks: Blocks{Errors: []string{"focus-findings: want a JSON array"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := reviewTurn(t, inReview(t), twoFindings())
			p.PendingTurn = ""
			got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 9, Blocks: tt.blocks}, t0)
			if err != nil {
				t.Fatal(err)
			}
			if act.Kind != ActionNone || got.AwaitingVerification || got.PendingTurn != "" {
				t.Fatalf("act = %+v awaiting = %v pending = %q", act, got.AwaitingVerification, got.PendingTurn)
			}
			if got.OpenGate != GateTriage || len(got.Cards) != len(p.Cards) || strings.Join(got.Review.Triage, ",") != strings.Join(p.Review.Triage, ",") {
				t.Fatalf("state changed: gate = %q cards %d→%d triage = %v", got.OpenGate, len(p.Cards), len(got.Cards), got.Review.Triage)
			}
		})
	}
}

func TestGuidanceDuringTriage(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	g := Guidance(p)
	for _, s := range []string{"Triage in progress", "Answer the developer's question only"} {
		if !strings.Contains(g, s) {
			t.Errorf("guidance lacks %q:\n%s", s, g)
		}
	}
	for _, s := range []string{"Report every finding", "<focus-findings>", "<focus-report>"} {
		if strings.Contains(g, s) {
			t.Errorf("guidance during triage has %q:\n%s", s, g)
		}
	}
}

// I3: dismissed findings are remembered across rounds, listed in the
// re-review guidance, and dropped when reported again.
func TestDismissedFindingsStayDismissed(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	p, _ = decide(t, p, p.Review.Triage[0], "fix")
	p, _ = decide(t, p, p.Review.Triage[1], "dismiss")
	p, _ = turn(p, t, &Report{Summary: "fixed"})
	p, _ = verify(p, t, passed())
	if stageOf(p, StageReview).Iteration != 2 {
		t.Fatalf("not in round 2: %+v", stageOf(p, StageReview))
	}
	if len(p.Review.Dismissed) != 1 || p.Review.Dismissed[0] != (DismissedFinding{File: "b.go", Line: 9, Title: "typo"}) {
		t.Fatalf("dismissed = %+v", p.Review.Dismissed)
	}
	g := Guidance(p)
	if !strings.Contains(g, "Already dismissed — do not re-report") || !strings.Contains(g, "b.go:9 — typo") {
		t.Fatalf("guidance lacks the dismissed list:\n%s", g)
	}

	t.Run("only dismissed findings come back", func(t *testing.T) {
		got, act := reviewTurn(t, p, []Finding{{ID: "x", Severity: "low", File: " b.go ", Line: 12, Title: "Typo"}})
		if act.Kind != ActionRunVerification || got.OpenGate != GateNone || len(findingCards(got)) != 2 {
			t.Fatalf("a re-reported dismissal opened triage: act = %+v gate = %q", act, got.OpenGate)
		}
	})
	t.Run("new findings still open triage", func(t *testing.T) {
		got, _ := reviewTurn(t, p, []Finding{{ID: "x", File: "b.go", Line: 9, Title: "typo"}, {ID: "y", File: "c.go", Line: 1, Title: "new"}})
		if got.OpenGate != GateTriage || len(got.Review.Triage) != 1 {
			t.Fatalf("triage = %v", got.Review.Triage)
		}
	})
	t.Run("kept by retry, cleared when the stage ends", func(t *testing.T) {
		q := p.clone()
		q.block(BlockedLimit, nil, 7, t0)
		q, _, err := Apply(q, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
		if err != nil || len(q.Review.Dismissed) != 1 {
			t.Fatalf("retry lost dismissals: %v %+v", err, q.Review)
		}
		done, _ := reviewTurn(t, p, []Finding{})
		done, _ = verify(done, t, passed())
		if len(done.Review.Dismissed) != 0 {
			t.Fatalf("dismissals outlived the stage: %+v", done.Review)
		}
	})
}

// I5 / f4: the retry prompt names the round and limit after the bump.
func TestReviewRetryPromptNamesTheNewRound(t *testing.T) {
	p := inReview(t)
	s := p.stageRef(StageReview)
	s.Iteration = 2
	p.block(BlockedLimit, nil, 7, t0)
	got, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if st := stageOf(got, StageReview); st.Iteration != 3 || st.Limit != 3 || !strings.Contains(act.Prompt, "round 3 of 3") {
		t.Fatalf("stage = %+v prompt = %q", st, act.Prompt)
	}
}
