package focuspipeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecisionsRecordWhereTheyCameFrom(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	p := alertPipeline()
	p.OpenGate = GateTriage

	// A finding decided on the phone; a decision decided at the desk.
	next, _, err := SetCardStateBy(p, "f1", CardDecided, DecisionFix, AnswerPhone, now)
	if err != nil {
		t.Fatal(err)
	}
	card := next.Cards[2]
	if card.DecidedBy != AnswerPhone || card.DecidedAt == nil || !card.DecidedAt.Equal(now) {
		t.Fatalf("finding = %+v", card)
	}
	next, _, err = SetCardStateBy(next, "q1", CardDecided, "yes", AnswerDesk, now)
	if err != nil {
		t.Fatal(err)
	}
	if next.Cards[5].DecidedBy != AnswerDesk {
		t.Fatalf("decision = %+v", next.Cards[5])
	}

	// Saying nothing records nothing: older callers and the golden file
	// see the card exactly as before.
	plain, _, err := SetCardState(p, "f1", CardDecided, DecisionFix, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(plain.Cards[2])
	if strings.Contains(string(raw), "decided_by") || strings.Contains(string(raw), "decided_at") {
		t.Fatalf("unsourced decision recorded a source: %s", raw)
	}
}

func TestGateActionRecordsWhereItCameFrom(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	p := planned(t)
	next, _, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove, By: AnswerGoal}, now)
	if err != nil {
		t.Fatal(err)
	}
	var gate *Card
	for i := range next.Cards {
		if next.Cards[i].Kind == CardGate {
			gate = &next.Cards[i]
		}
	}
	if gate == nil || gate.DecidedBy != AnswerGoal || gate.DecidedAt == nil {
		t.Fatalf("gate card = %+v", gate)
	}
}

func TestAnswerSummary(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	at := func(day int) *time.Time {
		v := time.Date(2026, 11, day, 12, 0, 0, 0, time.UTC)
		return &v
	}
	card := func(kind, by string, when *time.Time) Card {
		return Card{Kind: kind, State: CardDecided, DecidedBy: by, DecidedAt: when}
	}
	phone := Pipeline{Cards: []Card{
		card(CardGate, AnswerPhone, at(3)),
		card(CardFinding, AnswerPhone, at(3)),
		card(CardDecision, AnswerDesk, at(4)),
		// Before the window, and one never sourced: neither counts.
		card(CardGate, AnswerPhone, &time.Time{}),
		{Kind: CardFinding, State: CardDecided, Decision: DecisionSuperseded},
	}}
	desk := Pipeline{Cards: []Card{
		card(CardGate, AnswerDesk, at(5)),
		card(CardGate, AnswerGoal, at(6)),
		// Unknown source and an undecided card do not count.
		card(CardDecision, "elsewhere", at(6)),
		{Kind: CardDecision, State: CardUnseen},
	}}
	quiet := Pipeline{Cards: []Card{{Kind: CardGate, State: CardUnseen}}}

	got := AnswerSummary([]Pipeline{phone, desk, quiet}, since)
	want := AnswerReport{
		Since:          since,
		Gates:          AnswerCounts{Phone: 1, Desk: 1, Goal: 1},
		Cards:          AnswerCounts{Phone: 1, Desk: 1},
		Pipelines:      2,
		PhonePipelines: 1,
	}
	if got != want {
		t.Fatalf("summary = %+v\nwant      %+v", got, want)
	}
}
