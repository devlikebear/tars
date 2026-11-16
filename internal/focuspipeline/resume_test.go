package focuspipeline

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func stopWith(t *testing.T, p Pipeline, ev Event) Pipeline {
	t.Helper()
	p, act, err := Apply(p, ev, t0)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !p.Stopped() || act.Kind != ActionNone {
		t.Fatalf("not stopped: open gate %q, act %+v", p.OpenGate, act)
	}
	return p
}

func resume(t *testing.T, p Pipeline) (Pipeline, Action) {
	t.Helper()
	before := p.clone()
	got, act, err := Apply(p, Event{Kind: EventResume}, t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !reflect.DeepEqual(p, before) {
		t.Fatal("Apply mutated its input")
	}
	if got.Stopped() {
		t.Fatal("still stopped after resume")
	}
	return got, act
}

// A gate stopped by its own stop button opens again as a new card; the
// stopped card keeps its decision and nothing is sent.
func TestResumeReopensStoppedGate(t *testing.T) {
	interrupted := func(t *testing.T) Pipeline {
		p, ok := Interrupt(building(t), t0)
		if !ok {
			t.Fatal("building pipeline owes no turn")
		}
		return p
	}
	tests := []struct {
		name       string
		start      func(t *testing.T) Pipeline
		gate       string
		wantStatus StageStatus
	}{
		{name: "plan (G1)", start: func(t *testing.T) Pipeline { return planned(t) }, gate: GatePlan, wantStatus: StatusActive},
		{name: "pull request (G3)", start: drafted, gate: GatePR, wantStatus: StatusActive},
		{name: "merge (G4)", start: inMerge, gate: GateMerge, wantStatus: StatusActive},
		{name: "blocked", start: interrupted, gate: GateBlocked, wantStatus: StatusBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open := tt.start(t)
			if open.OpenGate != tt.gate {
				t.Fatalf("fixture gate = %q", open.OpenGate)
			}
			gateCard := open.Cards[open.openGateCard()]
			stopped := stopWith(t, open, Event{Kind: EventGate, Gate: tt.gate, Action: GateStop, By: AnswerPhone})

			p, act := resume(t, stopped)
			if act.Kind != ActionNone || p.PendingTurn != "" {
				t.Fatalf("resume sent something: act %+v pending %q", act, p.PendingTurn)
			}
			if p.OpenGate != tt.gate || statuses(p)[p.Current] != tt.wantStatus {
				t.Fatalf("gate = %q status = %q", p.OpenGate, statuses(p)[p.Current])
			}
			if len(p.Cards) != len(stopped.Cards)+1 {
				t.Fatalf("cards = %d, want one more than %d", len(p.Cards), len(stopped.Cards))
			}
			old := p.Cards[indexOfCard(p, gateCard.ID)]
			if old.State != CardDecided || old.Decision != GateStop || old.DecidedBy != AnswerPhone {
				t.Fatalf("stopped card changed: %+v", old)
			}
			fresh := p.Cards[p.openGateCard()]
			if fresh.ID == gateCard.ID || fresh.State != CardUnseen || fresh.Title != gateCard.Title ||
				fresh.Stage != gateCard.Stage || !bytes.Equal(fresh.Payload, gateCard.Payload) {
				t.Fatalf("reopened card = %+v, stopped card = %+v", fresh, gateCard)
			}
		})
	}
}

// The case that made this exist: G1 stopped from a phone, resumed, then
// approved, goes on to build as if it had never stopped.
func TestResumeAtPlanThenApprove(t *testing.T) {
	p := stopWith(t, planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateStop, By: AnswerPhone})
	p, _ = resume(t, p)
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Current != StageBuild || !p.Active() || act.Kind != ActionSendTurn {
		t.Fatalf("current = %q active = %v act = %+v", p.Current, p.Active(), act)
	}
}

// The blocked gate reopened by resume answers retry as it did before the
// stop: the cut-off turn goes out again.
func TestResumeBlockedGateRetry(t *testing.T) {
	interrupted, ok := Interrupt(building(t), t0)
	if !ok {
		t.Fatal("building pipeline owes no turn")
	}
	owed := interrupted.openBlockedFact().Prompt
	p := stopWith(t, interrupted, Event{Kind: EventGate, Gate: GateBlocked, Action: GateStop})
	p, _ = resume(t, p)
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Active() || act.Kind != ActionSendTurn || act.Prompt != owed {
		t.Fatalf("active = %v act = %+v, want the owed turn %q", p.Active(), act, owed)
	}
}

// A stop with no gate open — mid-turn, mid-verification, between steps —
// reactivates the stage and sends one turn that continues it.
func TestResumeMidStageSendsContinueTurn(t *testing.T) {
	p := stopWith(t, building(t), Event{Kind: EventStop})
	p, act := resume(t, p)
	if !p.Active() || p.OpenGate != GateNone || p.Current != StageBuild {
		t.Fatalf("active = %v gate = %q current = %q", p.Active(), p.OpenGate, p.Current)
	}
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "resumed") || !strings.Contains(act.Prompt, string(StageBuild)) {
		t.Fatalf("act = %+v", act)
	}
	if p.PendingTurn != act.Prompt {
		t.Fatalf("pending turn = %q, want the resume turn owed", p.PendingTurn)
	}
}

// A review stopped while its triage was open reopens triage on the same
// finding cards instead of sending a new review turn.
func TestResumeReopensTriage(t *testing.T) {
	p, _ := reviewTurn(t, inReview(t), twoFindings())
	if p.OpenGate != GateTriage {
		t.Fatalf("fixture gate = %q", p.OpenGate)
	}
	triage := append([]string(nil), p.Review.Triage...)
	p = stopWith(t, p, Event{Kind: EventStop})
	p, act := resume(t, p)
	if p.OpenGate != GateTriage || act.Kind != ActionNone || !p.Active() {
		t.Fatalf("gate = %q act = %+v active = %v", p.OpenGate, act, p.Active())
	}
	if !reflect.DeepEqual(p.Review.Triage, triage) {
		t.Fatalf("triage = %v, want %v", p.Review.Triage, triage)
	}
}

// A pipeline waiting on its pull request goes back to its probes: resume
// sends nothing, the poller drives it as before.
func TestResumeWhileWaitingOnPR(t *testing.T) {
	p := opening(t)
	if p.PRWait == "" {
		t.Fatal("fixture waits on nothing")
	}
	p = stopWith(t, p, Event{Kind: EventStop})
	p, act := resume(t, p)
	if !p.Active() || act.Kind != ActionNone || p.PRWait == "" {
		t.Fatalf("active = %v act = %+v pr wait = %q", p.Active(), act, p.PRWait)
	}
}

func TestResumeRefusesWhatIsNotStopped(t *testing.T) {
	finished, _, err := Apply(building(t, StagePlan, StageBuild), Event{Kind: EventAdvance, Stage: StageBuild}, t0)
	if err != nil {
		t.Fatal(err)
	}
	interrupted, _ := Interrupt(building(t), t0)
	for name, p := range map[string]Pipeline{
		"fresh":           New("s1", "g", t0),
		"active":          building(t),
		"gate open":       planned(t),
		"blocked gate":    interrupted,
		"finished":        finished,
		"resumed already": func() Pipeline { r, _ := resume(t, stopWith(t, building(t), Event{Kind: EventStop})); return r }(),
	} {
		t.Run(name, func(t *testing.T) {
			got, act, err := Apply(p, Event{Kind: EventResume}, t0)
			if !errors.Is(err, ErrNotStopped) || act.Kind != ActionNone || !reflect.DeepEqual(got, p) {
				t.Fatalf("err = %v act = %+v", err, act)
			}
		})
	}
}

// A stop decided on a gate card that is not the newest gate card does not
// say which gate stopped the pipeline: resume falls back to a turn.
func TestStoppedGateCardOnlyTheNewest(t *testing.T) {
	p := stopWith(t, planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateStop})
	p.Cards = append(p.Cards, Card{ID: "cx", Kind: CardGate, Stage: StagePlan, Title: "something else", State: CardDecided, Decision: GateApprove})
	if i := p.stoppedGateCard(); i != -1 {
		t.Fatalf("stoppedGateCard = %d, want -1", i)
	}
	p, act := resume(t, p)
	if p.OpenGate != GateNone || act.Kind != ActionSendTurn {
		t.Fatalf("gate = %q act = %+v", p.OpenGate, act)
	}
}
