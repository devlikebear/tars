package focuspipeline

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

func testPlan(stages ...StageID) *Plan {
	return &Plan{
		Goal:   "g",
		Tasks:  []PlanTask{{Title: "t1", Done: "d1"}, {Title: "t2", Done: "d2"}},
		Stages: stages,
		Verify: []string{"make test"},
		E2E:    []string{"make console-e2e"},
		Limits: map[string]int{"build": 5},
	}
}

// planned is a pipeline whose plan gate is open.
func planned(t *testing.T, stages ...StageID) Pipeline {
	t.Helper()
	p, _, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: testPlan(stages...)}}, t0)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if p.OpenGate != GatePlan {
		t.Fatalf("open gate = %q", p.OpenGate)
	}
	return p
}

func statuses(p Pipeline) map[StageID]StageStatus {
	out := map[StageID]StageStatus{}
	for _, s := range p.Stages {
		out[s.ID] = s.Status
	}
	return out
}

func TestNew(t *testing.T) {
	p := New("s1", "ship it", t0)
	if p.Version != 1 || p.SessionID != "s1" || p.Goal != "ship it" || p.Current != StagePlan || p.OpenGate != GateNone {
		t.Fatalf("pipeline = %+v", p)
	}
	if len(p.Stages) != len(StageOrder) {
		t.Fatalf("stages = %+v", p.Stages)
	}
	for i, s := range p.Stages {
		if s.ID != StageOrder[i] {
			t.Fatalf("stage %d = %s", i, s.ID)
		}
		want := StatusPending
		if i == 0 {
			want = StatusActive
		}
		if s.Status != want {
			t.Fatalf("stage %s status = %s", s.ID, s.Status)
		}
	}
	if p.Cards == nil {
		t.Fatal("cards must be an empty slice so JSON has []")
	}
}

func TestApplyTurnCompletedInPlan(t *testing.T) {
	tests := []struct {
		name       string
		start      func(t *testing.T) Pipeline
		blocks     Blocks
		wantGate   string
		wantKinds  []string
		wantAction string
		wantPrompt string
	}{
		{
			name:       "plan block opens the plan gate",
			start:      func(*testing.T) Pipeline { return New("s1", "g", t0) },
			blocks:     Blocks{Plan: testPlan(StageOrder...)},
			wantGate:   GatePlan,
			wantKinds:  []string{CardGate},
			wantAction: ActionNone,
		},
		{
			name:       "no plan block asks once again",
			start:      func(*testing.T) Pipeline { return New("s1", "g", t0) },
			blocks:     Blocks{},
			wantGate:   GateNone,
			wantKinds:  []string{CardNotice},
			wantAction: ActionSendTurn,
			wantPrompt: "<focus-plan>",
		},
		{
			name:       "malformed plan block is one notice",
			start:      func(*testing.T) Pipeline { return New("s1", "g", t0) },
			blocks:     Blocks{Errors: []string{"focus-plan: bad"}},
			wantGate:   GateNone,
			wantKinds:  []string{CardNotice},
			wantAction: ActionSendTurn,
		},
		{
			name: "second missing block in a row does not re-request again",
			start: func(t *testing.T) Pipeline {
				p, _, err := Apply(New("s1", "g", t0), Event{Kind: EventTurnCompleted, Turn: 1}, t0)
				if err != nil {
					t.Fatal(err)
				}
				return p
			},
			blocks:     Blocks{},
			wantGate:   GateNone,
			wantKinds:  []string{CardNotice, CardNotice},
			wantAction: ActionNone,
		},
		{
			name:       "report with a plan adds report and decision cards",
			start:      func(*testing.T) Pipeline { return New("s1", "g", t0) },
			blocks:     Blocks{Plan: testPlan(), Report: &Report{Summary: "s", Decisions: []Decision{{ID: "d1", Question: "q?", Options: []string{"a", "b"}}}}},
			wantGate:   GatePlan,
			wantKinds:  []string{CardGate, CardReport, CardDecision},
			wantAction: ActionNone,
		},
		{
			name:       "a follow-up turn without a plan keeps the open gate quietly",
			start:      func(t *testing.T) Pipeline { return planned(t) },
			blocks:     Blocks{Report: &Report{Summary: "answered"}},
			wantGate:   GatePlan,
			wantKinds:  []string{CardGate, CardReport},
			wantAction: ActionNone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := tt.start(t)
			before := start.clone()
			p, act, err := Apply(start, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: tt.blocks}, t0.Add(time.Minute))
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !reflect.DeepEqual(start, before) {
				t.Fatal("Apply mutated its input")
			}
			if p.OpenGate != tt.wantGate {
				t.Fatalf("open gate = %q, want %q", p.OpenGate, tt.wantGate)
			}
			var kinds []string
			for _, c := range p.Cards {
				kinds = append(kinds, c.Kind)
			}
			if !reflect.DeepEqual(kinds, tt.wantKinds) {
				t.Fatalf("cards = %v, want %v", kinds, tt.wantKinds)
			}
			if act.Kind != tt.wantAction {
				t.Fatalf("action = %+v", act)
			}
			if tt.wantPrompt != "" && !strings.Contains(act.Prompt, tt.wantPrompt) {
				t.Fatalf("prompt = %q", act.Prompt)
			}
			if p.Current != StagePlan || statuses(p)[StagePlan] != StatusActive {
				t.Fatalf("plan stage must stay active: %+v", p.Stages)
			}
			last := p.Cards[len(p.Cards)-1]
			if last.Turn != 3 || last.State != CardUnseen || last.ID == "" {
				t.Fatalf("card = %+v", last)
			}
		})
	}
}

func TestApplyNoticeTitle(t *testing.T) {
	p, _, err := Apply(New("s1", "g", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Errors: []string{"focus-plan: bad"}}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Cards[0].Title != NoticeFormatMissing {
		t.Fatalf("title = %q", p.Cards[0].Title)
	}
	if !strings.Contains(string(p.Cards[0].Payload), "focus-plan: bad") {
		t.Fatalf("payload = %s", p.Cards[0].Payload)
	}
}

func TestApplyPlanCardPayload(t *testing.T) {
	p := planned(t)
	gate := p.Cards[0]
	if gate.Title != "Approve plan" || gate.Stage != StagePlan {
		t.Fatalf("gate = %+v", gate)
	}
	var plan Plan
	if err := json.Unmarshal(gate.Payload, &plan); err != nil || plan.Goal != "g" {
		t.Fatalf("payload = %s (%v)", gate.Payload, err)
	}
	if p.Plan == nil || len(p.Plan.Tasks) != 2 {
		t.Fatalf("plan = %+v", p.Plan)
	}
}

func TestApplyNewPlanSupersedesOpenGate(t *testing.T) {
	p := planned(t)
	p, _, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Plan: testPlan(StagePlan, StageBuild)}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Cards) != 2 || p.Cards[0].State != CardDecided || p.Cards[0].Decision != DecisionSuperseded || p.Cards[1].State != CardUnseen {
		t.Fatalf("cards = %+v", p.Cards)
	}
	if p.Cards[0].ID == p.Cards[1].ID {
		t.Fatal("card ids must be unique")
	}
}

func TestApplyPlanApprove(t *testing.T) {
	tests := []struct {
		name        string
		stages      []StageID
		edits       *Plan
		wantCurrent StageID
		wantStatus  map[StageID]StageStatus
		wantLimit   int
		wantPrompt  string
	}{
		{
			name:        "all stages",
			stages:      StageOrder,
			wantCurrent: StageBuild,
			wantStatus: map[StageID]StageStatus{
				StagePlan: StatusDone, StageBuild: StatusActive, StageReview: StatusPending,
				StagePR: StatusPending, StagePRReview: StatusPending, StageMerge: StatusPending,
			},
			wantLimit:  5,
			wantPrompt: "Plan approved. Start the build stage with task 1.",
		},
		{
			name:        "skipped stages",
			stages:      []StageID{StagePlan, StageBuild, StagePR, StageMerge},
			wantCurrent: StageBuild,
			wantStatus: map[StageID]StageStatus{
				StagePlan: StatusDone, StageBuild: StatusActive, StageReview: StatusSkipped,
				StagePR: StatusPending, StagePRReview: StatusSkipped, StageMerge: StatusPending,
			},
			wantLimit:  5,
			wantPrompt: "Plan approved. Start the build stage with task 1.",
		},
		{
			name:        "edits replace the plan and may skip build",
			stages:      StageOrder,
			edits:       &Plan{Goal: "edited", Tasks: []PlanTask{{Title: "only", Done: "ok"}}, Stages: []StageID{StagePR, StageMerge}},
			wantCurrent: StagePR,
			wantStatus: map[StageID]StageStatus{
				StagePlan: StatusDone, StageBuild: StatusSkipped, StageReview: StatusSkipped,
				StagePR: StatusActive, StagePRReview: StatusSkipped, StageMerge: StatusPending,
			},
			wantLimit:  3,
			wantPrompt: "Plan approved. Start the pr stage.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planned(t, tt.stages...)
			p, act, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove, Edits: tt.edits}, t0.Add(time.Hour))
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if p.Current != tt.wantCurrent || p.OpenGate != GateNone {
				t.Fatalf("current = %s gate = %q", p.Current, p.OpenGate)
			}
			if got := statuses(p); !reflect.DeepEqual(got, tt.wantStatus) {
				t.Fatalf("statuses = %v, want %v", got, tt.wantStatus)
			}
			cur, _ := p.Stage(p.Current)
			if cur.Limit != tt.wantLimit || cur.Iteration != 1 {
				t.Fatalf("current stage = %+v", cur)
			}
			if act.Kind != ActionSendTurn || act.Prompt != tt.wantPrompt {
				t.Fatalf("action = %+v", act)
			}
			if p.Cards[0].State != CardDecided || p.Cards[0].Decision != GateApprove {
				t.Fatalf("gate card = %+v", p.Cards[0])
			}
			if tt.edits != nil && p.Plan.Goal != "edited" {
				t.Fatalf("plan = %+v", p.Plan)
			}
			if !p.UpdatedAt.Equal(t0.Add(time.Hour)) {
				t.Fatalf("updated_at = %v", p.UpdatedAt)
			}
		})
	}
}

func TestApplyPlanApproveRejectsEmptyEdits(t *testing.T) {
	p := planned(t)
	got, _, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove, Edits: &Plan{Goal: "x"}}, t0)
	if !errors.Is(err, ErrInvalidEdits) {
		t.Fatalf("err = %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Fatal("pipeline changed on error")
	}
}

func TestApplyPlanRequestChanges(t *testing.T) {
	p := planned(t)
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateRequestChanges, Note: "split task 2"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Current != StagePlan || statuses(p)[StagePlan] != StatusActive || p.OpenGate != GateNone {
		t.Fatalf("pipeline = %+v", p)
	}
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "split task 2") {
		t.Fatalf("action = %+v", act)
	}
	if p.Cards[0].State != CardDecided || p.Cards[0].Decision != GateRequestChanges {
		t.Fatalf("card = %+v", p.Cards[0])
	}
}

func TestApplyStop(t *testing.T) {
	p := planned(t)
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateStop}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if statuses(p)[StagePlan] != StatusBlocked || p.OpenGate != GateNone || act.Kind != ActionNone {
		t.Fatalf("pipeline = %+v action = %+v", p, act)
	}
	if Guidance(p) != "" {
		t.Fatal("a stopped pipeline gives no guidance")
	}
	if p.Active() {
		t.Fatal("stopped pipeline is not active")
	}
}

func TestApplyGateNotOpen(t *testing.T) {
	tests := []struct {
		name string
		p    func(t *testing.T) Pipeline
		ev   Event
	}{
		{"no gate open", func(*testing.T) Pipeline { return New("s1", "g", t0) }, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}},
		{"other gate", func(t *testing.T) Pipeline { return planned(t) }, Event{Kind: EventGate, Gate: GateMerge, Action: GateApprove}},
		{"double approve", func(t *testing.T) Pipeline {
			p, _, err := Apply(planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}},
		{"stop on a closed gate", func(*testing.T) Pipeline { return New("s1", "g", t0) }, Event{Kind: EventGate, Gate: GatePlan, Action: GateStop}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p(t)
			got, act, err := Apply(p, tt.ev, t0.Add(time.Hour))
			if !errors.Is(err, ErrGateNotOpen) {
				t.Fatalf("err = %v", err)
			}
			if !reflect.DeepEqual(got, p) || act.Kind != ActionNone {
				t.Fatal("pipeline changed on a closed gate")
			}
		})
	}
}

func TestApplyInvalidInputs(t *testing.T) {
	p := planned(t)
	if _, _, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: "yolo"}, t0); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := Apply(p, Event{Kind: "nope"}, t0); err == nil {
		t.Fatal("unknown event kind must fail")
	}
}

func TestApplyTurnCompletedInBuild(t *testing.T) {
	p, _, err := Apply(planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("report", func(t *testing.T) {
		got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Report: &Report{Summary: "did t1", Decisions: []Decision{{ID: "d1", Question: "q?", Options: []string{"a"}}, {ID: "d2", Question: "r?", Options: []string{"b"}}}}}}, t0)
		if err != nil {
			t.Fatal(err)
		}
		cards := got.Cards[len(p.Cards):]
		if len(cards) != 3 || cards[0].Kind != CardReport || cards[1].Kind != CardDecision || cards[2].Kind != CardDecision {
			t.Fatalf("cards = %+v", cards)
		}
		if cards[0].Stage != StageBuild || act.Kind != ActionNone || got.Current != StageBuild {
			t.Fatalf("got = %+v act = %+v", got, act)
		}
		if got.NeedsInput() != 2 {
			t.Fatalf("needs input = %d", got.NeedsInput())
		}
	})
	t.Run("findings become cards", func(t *testing.T) {
		got, _, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Report: &Report{Summary: "s"}, Findings: []Finding{{ID: "f1", Severity: "high", File: "a.go", Line: 3, Title: "nil deref"}}}}, t0)
		if err != nil {
			t.Fatal(err)
		}
		last := got.Cards[len(got.Cards)-1]
		if last.Kind != CardFinding || last.Title != "nil deref" {
			t.Fatalf("card = %+v", last)
		}
	})
	t.Run("missing report", func(t *testing.T) {
		got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 4}, t0)
		if err != nil {
			t.Fatal(err)
		}
		last := got.Cards[len(got.Cards)-1]
		if last.Kind != CardNotice || act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "<focus-report>") {
			t.Fatalf("card = %+v act = %+v", last, act)
		}
		if got.Current != StageBuild || statuses(got)[StageBuild] != StatusActive {
			t.Fatal("a missing report never moves the stage")
		}
	})
}

func TestApplyTurnCompletedWhenStopped(t *testing.T) {
	p, _, err := Apply(planned(t), Event{Kind: EventGate, Gate: GatePlan, Action: GateStop}, t0)
	if err != nil {
		t.Fatal(err)
	}
	got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 5}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cards) != len(p.Cards) || act.Kind != ActionNone {
		t.Fatalf("stopped pipeline must not nag: %+v %+v", got.Cards, act)
	}
}

func TestSetCardState(t *testing.T) {
	p, _, err := Apply(New("s1", "g", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{
		Plan:   testPlan(),
		Report: &Report{Summary: "s", Decisions: []Decision{{ID: "d1", Question: "Which DB?", Options: []string{"sqlite", "postgres"}}}},
	}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	gateID, reportID, decisionID := p.Cards[0].ID, p.Cards[1].ID, p.Cards[2].ID

	got, act, err := SetCardState(p, reportID, CardSeen, "", t0)
	if err != nil || got.Cards[1].State != CardSeen || act.Kind != ActionNone {
		t.Fatalf("seen: %v %+v %+v", err, got.Cards[1], act)
	}
	got, act, err = SetCardState(got, decisionID, CardDecided, "postgres", t0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cards[2].State != CardDecided || got.Cards[2].Decision != "postgres" {
		t.Fatalf("card = %+v", got.Cards[2])
	}
	if act.Kind != ActionSendTurn || act.Prompt != "Which DB? → postgres" {
		t.Fatalf("action = %+v", act)
	}
	if _, _, err := SetCardState(got, decisionID, CardDecided, "sqlite", t0); !errors.Is(err, ErrCardDecided) {
		t.Fatalf("re-decide err = %v", err)
	}
	if again, _, err := SetCardState(got, decisionID, CardSeen, "", t0); err != nil || again.Cards[2].State != CardDecided {
		t.Fatalf("seen must not downgrade a decided card: %v %+v", err, again.Cards[2])
	}
	if _, _, err := SetCardState(p, "missing", CardSeen, "", t0); !errors.Is(err, ErrCardNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, _, err := SetCardState(p, gateID, CardDecided, "approve", t0); !errors.Is(err, ErrInvalidCardState) {
		t.Fatalf("gate decided via card err = %v", err)
	}
	if _, _, err := SetCardState(p, decisionID, CardDecided, "", t0); !errors.Is(err, ErrInvalidCardState) {
		t.Fatalf("empty decision err = %v", err)
	}
	if _, _, err := SetCardState(p, reportID, "bogus", "", t0); !errors.Is(err, ErrInvalidCardState) {
		t.Fatalf("bogus state err = %v", err)
	}
	if got, _, err := SetCardState(p, gateID, CardSeen, "", t0); err != nil || got.Cards[0].State != CardSeen {
		t.Fatalf("seen gate: %v", err)
	}
}
