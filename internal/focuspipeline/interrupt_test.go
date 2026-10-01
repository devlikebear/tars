package focuspipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReRequestOnlyOnceWhenOtherCardsArrive(t *testing.T) {
	// A build turn with a <focus-pr> draft and a malformed report: the PR
	// card lands after the notice of the previous turn, but that turn
	// already re-requested, so this one must not.
	p := inBuild(t)
	bad := Blocks{PR: &PRDraft{Title: "t"}, Errors: []string{"focus-report: bad json"}}
	p, act, _ := Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: bad}, t0)
	if act.Kind != ActionSendTurn {
		t.Fatalf("first miss re-requests: %+v", act)
	}
	p, act, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: bad}, t0)
	if act.Kind == ActionSendTurn {
		t.Fatalf("second miss in a row must not re-request again: %+v", act)
	}
}

func TestMissingReportsHitTheBuildTurnCap(t *testing.T) {
	p := inBuild(t)
	for i := 0; i < 40 && p.OpenGate == GateNone; i++ {
		// Alternate a good-but-unfinished report (resets the notice chain)
		// with a missing one, so re-requests never stop on their own.
		if i%2 == 0 {
			p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: i, Blocks: Blocks{}}, t0)
		} else {
			p, _ = turn(p, t, &Report{Summary: "x"})
			p.AwaitingVerification = false
		}
	}
	if p.OpenGate != GateBlocked {
		t.Fatalf("gate = %q turns = %d", p.OpenGate, stageOf(p, StageBuild).Turns)
	}
}

func TestStopClearsPendingWork(t *testing.T) {
	p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
	if !p.AwaitingVerification {
		t.Fatal("setup")
	}
	got, _, err := Apply(p, Event{Kind: EventStop}, t0)
	if err != nil || got.AwaitingVerification || got.PendingTurn != "" {
		t.Fatalf("stop left work pending: %+v err %v", got, err)
	}
	blockedGot, _, _ := Apply(blocked(t), Event{Kind: EventGate, Gate: GateBlocked, Action: GateStop}, t0)
	if blockedGot.AwaitingVerification || blockedGot.PendingTurn != "" {
		t.Fatal("blocked stop left work pending")
	}
}

func TestPendingTurnTracksTheTurnTheServerOwes(t *testing.T) {
	p := planned(t)
	p, act, _ := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if act.Kind != ActionSendTurn || p.PendingTurn != act.Prompt {
		t.Fatalf("approve: pending = %q act = %+v", p.PendingTurn, act)
	}
	// The turn completed: nothing is owed until the next action.
	p, act = turn(p, t, &Report{Summary: "t1"})
	if p.PendingTurn != "" || act.Kind != ActionRunVerification {
		t.Fatalf("after turn: pending = %q act = %+v", p.PendingTurn, act)
	}
	p, act = verify(p, t, failed("make test", "x"))
	if p.PendingTurn != act.Prompt || act.Kind != ActionSendTurn {
		t.Fatalf("fix turn owed: %q", p.PendingTurn)
	}
	// A decision answer is owed too.
	q, _ := turn(p, t, &Report{Summary: "q", Decisions: []Decision{{ID: "d1", Question: "which?", Options: []string{"a"}}}})
	dec := q.Cards[len(q.Cards)-1]
	q, act, err := SetCardState(q, dec.ID, CardDecided, "a", t0)
	if err != nil || q.PendingTurn != act.Prompt || act.Prompt == "" {
		t.Fatalf("decision answer owed: %q err %v", q.PendingTurn, err)
	}
}

func TestInterrupt(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) Pipeline
		want       bool
		wantVerify bool
	}{
		{"mid verification", func(t *testing.T) Pipeline { p, _ := turn(inBuild(t), t, &Report{Summary: "t1"}); return p }, true, true},
		{"turn owed", func(t *testing.T) Pipeline {
			p, _, _ := Apply(planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
			return p
		}, true, false},
		{"idle", func(t *testing.T) Pipeline {
			p, _ := turn(inBuild(t), t, &Report{Summary: "q", Decisions: []Decision{{ID: "d1", Question: "?"}}})
			return p
		}, false, false},
		{"gate open", func(t *testing.T) Pipeline { return planned(t) }, false, false},
		{"blocked", func(t *testing.T) Pipeline { return blocked(t) }, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.setup(t)
			got, ok := Interrupt(p, t0)
			if ok != tc.want {
				t.Fatalf("interrupted = %v", ok)
			}
			if !ok {
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(p)
				if string(a) != string(b) {
					t.Fatal("no interruption changes nothing")
				}
				return
			}
			card := got.Cards[len(got.Cards)-1]
			var fact BlockedFact
			_ = json.Unmarshal(card.Payload, &fact)
			if got.OpenGate != GateBlocked || card.Kind != CardGate || fact.Reason != BlockedInterrupted || fact.Verify != tc.wantVerify {
				t.Fatalf("got gate %q card %+v fact %+v", got.OpenGate, card, fact)
			}
			if got.AwaitingVerification || got.PendingTurn != "" || stageOf(got, got.Current).Status != StatusBlocked {
				t.Fatalf("interrupted pipeline still owes work: %+v", got)
			}

			// Retry resumes exactly what was interrupted, without a new round.
			before := stageOf(got, got.Current)
			r, act, err := Apply(got, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantVerify {
				if act.Kind != ActionRunVerification || !r.AwaitingVerification {
					t.Fatalf("retry = %+v", act)
				}
			} else if act.Kind != ActionSendTurn || act.Prompt != p.PendingTurn || r.PendingTurn != act.Prompt {
				t.Fatalf("retry = %+v, want the owed turn %q", act, p.PendingTurn)
			}
			if s := stageOf(r, r.Current); s.Status != StatusActive || s.Iteration != before.Iteration {
				t.Fatalf("stage after retry = %+v", s)
			}
		})
	}
}

func TestInterruptedNotice(t *testing.T) {
	p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
	got, _ := Interrupt(p, t0)
	if !strings.Contains(got.Cards[len(got.Cards)-1].Title, "interrupted") {
		t.Fatalf("title = %q", got.Cards[len(got.Cards)-1].Title)
	}
}

func TestFailTurn(t *testing.T) {
	p, _, _ := Apply(planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	owed := p.PendingTurn
	got, ok := FailTurn(p, "cli timed out: no output for 15m0s", t0)
	if !ok {
		t.Fatal("a failed owed turn raises the gate")
	}
	var fact BlockedFact
	_ = json.Unmarshal(got.Cards[len(got.Cards)-1].Payload, &fact)
	if got.OpenGate != GateBlocked || fact.Reason != BlockedTurnFailed || fact.Error != "cli timed out: no output for 15m0s" || fact.Prompt != owed {
		t.Fatalf("got %q %+v", got.OpenGate, fact)
	}
	r, act, err := Apply(got, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
	if err != nil || act.Kind != ActionSendTurn || act.Prompt != owed || stageOf(r, r.Current).Status != StatusActive {
		t.Fatalf("retry = %+v err %v", act, err)
	}
	// Nothing owed (a person's turn failed): no gate.
	idle, _ := turn(inBuild(t), t, &Report{Summary: "q", Decisions: []Decision{{ID: "d1", Question: "?"}}})
	if _, ok := FailTurn(idle, "x", t0); ok {
		t.Fatal("nothing was owed")
	}
}
