package focuspipeline

import (
	"slices"
	"testing"
	"time"
)

func alertPipeline() Pipeline {
	return Pipeline{
		SessionID: "s1",
		Goal:      "add dark mode",
		Current:   StageReview,
		Stages: []Stage{
			{ID: StagePlan, Status: StatusDone},
			{ID: StageReview, Status: StatusActive},
		},
		Cards: []Card{
			{ID: "g1", Kind: CardGate, Stage: StagePlan, Title: "Plan", State: CardUnseen},
			{ID: "old", Kind: CardDecision, Stage: StagePlan, Title: "Which DB?", State: CardUnseen},
			{ID: "f1", Kind: CardFinding, Stage: StageReview, Title: "Null check missing", State: CardSeen},
			{ID: "f2", Kind: CardFinding, Stage: StageReview, Title: "Dead code", State: CardDecided},
			{ID: "n1", Kind: CardNotice, Stage: StageReview, Title: "note", State: CardUnseen},
			{ID: "q1", Kind: CardDecision, Stage: StageReview, Title: "", State: CardUnseen},
		},
	}
}

func TestWaitingOf(t *testing.T) {
	p := alertPipeline()
	p.OpenGate = GateTriage
	w := WaitingOf(p)
	if w.Gate != GateTriage || !slices.Equal(w.Cards, []string{"f1", "q1"}) || w.Empty() {
		t.Fatalf("waiting = %+v", w)
	}

	goal := p
	goal.GoalMode = &GoalMode{Enabled: true}
	if !WaitingOf(goal).Empty() {
		t.Fatal("goal mode answers by itself: nothing waits on the developer")
	}
	done := p
	at := time.Now()
	done.FinishedAt = &at
	if !WaitingOf(done).Empty() {
		t.Fatal("a finished pipeline waits on nothing")
	}
	if !WaitingOf(Pipeline{}).Empty() {
		t.Fatal("an empty pipeline waits on nothing")
	}
}

func TestWaitingNewSince(t *testing.T) {
	p := alertPipeline()
	quiet := p
	quiet.Cards = nil
	none := WaitingOf(quiet)

	p.OpenGate = GateTriage
	first := WaitingOf(p)
	if got := first.NewSince(none); !slices.Equal(got, []string{"Findings to triage", "Null check missing", "A card waits for a decision"}) {
		t.Fatalf("first alert = %q", got)
	}
	if got := first.NewSince(first); len(got) != 0 {
		t.Fatalf("still waiting must not alert again: %q", got)
	}

	// A new card alerts alone; the gate that was already open does not.
	p.Cards = append(p.Cards, Card{ID: "f3", Kind: CardFinding, Stage: StageReview, Title: "Race in cache", State: CardUnseen})
	if got := WaitingOf(p).NewSince(first); !slices.Equal(got, []string{"Race in cache"}) {
		t.Fatalf("new card = %q", got)
	}

	for gate, want := range map[string]string{
		GatePlan:    "Plan ready for approval (G1)",
		GatePR:      "Ready to open a pull request (G3)",
		GateMerge:   "Ready to merge (G4)",
		GateBlocked: "Blocked: needs a decision",
		"odd":       `Gate "odd" is open`,
	} {
		w := Waiting{Gate: gate}
		if got := w.NewSince(Waiting{Gate: GateTriage}); !slices.Equal(got, []string{want}) {
			t.Errorf("%s: %q", gate, got)
		}
	}
}
