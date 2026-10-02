package focuspipeline

import (
	"testing"
)

// #1079: a decision answered while a gate the developer can ask at is open
// is delivered right away as the developer's question — the gate stays
// open and the pipeline owes nothing — instead of being held as the
// pipeline's owed turn, which the driver never sends while a gate is open.
func TestAnswerWhileAGateIsOpen(t *testing.T) {
	asks := &Report{Summary: "s", Decisions: []Decision{{ID: "d1", Question: "Which base?", Options: []string{"main", "release"}}}}
	tests := []struct {
		name     string
		p        func(t *testing.T) Pipeline
		wantGate string
		wantOwed bool
	}{
		{"plan gate", func(t *testing.T) Pipeline {
			p, _ := mustApply(t, New("s1", "g", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: testPlan(), Report: asks}})
			return p
		}, GatePlan, false},
		{"pr gate", func(t *testing.T) Pipeline {
			p, _ := mustApply(t, atStage(t, StagePR), Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{PR: testDraft, Report: asks}})
			return p
		}, GatePR, false},
		{"merge gate", func(t *testing.T) Pipeline {
			p, _ := mustApply(t, inMerge(t), Event{Kind: EventTurnCompleted, Turn: 9, Blocks: Blocks{Report: asks}})
			return p
		}, GateMerge, false},
		{"triage gate", func(t *testing.T) Pipeline {
			p, _ := mustApply(t, inReview(t), Event{Kind: EventTurnCompleted, Turn: 7, Blocks: Blocks{Findings: twoFindings(), Report: asks}})
			return p
		}, GateTriage, false},
		{"no gate open", func(t *testing.T) Pipeline {
			p, _ := mustApply(t, atStage(t, StagePRReview), Event{Kind: EventTurnCompleted, Turn: 6, Blocks: Blocks{Report: asks}})
			return p
		}, GateNone, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p(t)
			if p.OpenGate != tt.wantGate {
				t.Fatalf("setup gate = %q, want %q", p.OpenGate, tt.wantGate)
			}
			var cardID string
			for _, c := range p.Cards {
				if c.Kind == CardDecision {
					cardID = c.ID
				}
			}
			got, act, err := SetCardState(p, cardID, CardDecided, "main", t0)
			if err != nil {
				t.Fatal(err)
			}
			if act.Kind != ActionSendTurn || act.Prompt != "Which base? → main" || len(act.Answers) != 1 {
				t.Fatalf("action = %+v", act)
			}
			if got.OpenGate != tt.wantGate {
				t.Fatalf("gate = %q, want %q (the answer must not close it)", got.OpenGate, tt.wantGate)
			}
			if owed := got.PendingTurn == act.Prompt; owed != tt.wantOwed {
				t.Fatalf("pending_turn = %q, owed want %v", got.PendingTurn, tt.wantOwed)
			}
			if !AnswersMayRun(got) {
				t.Fatal("the answers turn must be allowed to run now")
			}
			// The question turn that answers it changes nothing at the gate.
			after, next := mustApply(t, got, Event{Kind: EventTurnCompleted, Turn: 20, Blocks: Blocks{Report: &Report{Summary: "noted"}}})
			if tt.wantGate != GateNone && (after.OpenGate != tt.wantGate || after.PendingTurn != "" || next.Kind != ActionNone) {
				t.Fatalf("after the question turn: gate %q pending %q act %+v", after.OpenGate, after.PendingTurn, next)
			}
		})
	}
}

func TestAnswersMayRun(t *testing.T) {
	blocked := atStage(t, StageBuild)
	blocked.OpenGate = GateBlocked
	stopped, _ := mustApply(t, atStage(t, StageBuild), Event{Kind: EventStop})
	tests := []struct {
		name string
		p    Pipeline
		want bool
	}{
		{"no gate", atStage(t, StageBuild), true},
		{"pr gate", drafted(t), true},
		{"merge gate", inMerge(t), true},
		{"plan gate", planned(t), true},
		{"blocked gate holds", blocked, false},
		{"stopped", stopped, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnswersMayRun(tt.p); got != tt.want {
				t.Fatalf("AnswersMayRun = %v", got)
			}
		})
	}
}
