package focuspipeline

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// inBuild is an approved pipeline in build, limit 3, with stages
// plan, build, review.
func inBuild(t *testing.T) Pipeline {
	t.Helper()
	plan := testPlan(StagePlan, StageBuild, StageReview)
	plan.Limits = map[string]int{"build": 3}
	p, _, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Current != StageBuild {
		t.Fatalf("current = %s", p.Current)
	}
	return p
}

func failed(command, excerpt string) *Verification {
	return &Verification{Passed: false, Results: []VerificationResult{
		{Command: "make vet", ExitCode: 0, Passed: true},
		{Command: command, ExitCode: 2, Excerpt: excerpt},
	}}
}

func passed() *Verification {
	return &Verification{Passed: true, Results: []VerificationResult{{Command: "make test", Passed: true}}}
}

func turn(p Pipeline, t *testing.T, report *Report) (Pipeline, Action) {
	t.Helper()
	got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{Report: report}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	return got, act
}

func verify(p Pipeline, t *testing.T, v *Verification) (Pipeline, Action) {
	t.Helper()
	got, act, err := Apply(p, Event{Kind: EventVerification, Verification: v}, t0)
	if err != nil {
		t.Fatal(err)
	}
	return got, act
}

func stageOf(p Pipeline, id StageID) Stage {
	s, _ := p.Stage(id)
	return s
}

func TestBuildTurnRequestsVerification(t *testing.T) {
	tests := []struct {
		name       string
		report     *Report
		wantAction string
		wantDone   bool
	}{
		{"report runs verification", &Report{Summary: "did t1"}, ActionRunVerification, false},
		{"tasks_done is remembered", &Report{Summary: "all done", TasksDone: true}, ActionRunVerification, true},
		{"a decision pauses the loop", &Report{Summary: "q", TasksDone: true, Decisions: []Decision{{ID: "d1", Question: "which?", Options: []string{"a", "b"}}}}, ActionNone, false},
		{"missing report re-requests without verification", nil, ActionSendTurn, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, act := turn(inBuild(t), t, tc.report)
			if act.Kind != tc.wantAction {
				t.Fatalf("action = %+v, want %s", act, tc.wantAction)
			}
			if got.TasksDone != tc.wantDone {
				t.Fatalf("tasks_done = %v", got.TasksDone)
			}
			if got.Current != StageBuild || stageOf(got, StageBuild).Status != StatusActive {
				t.Fatalf("a turn alone never moves the stage: %+v", got.Stages)
			}
		})
	}
}

func TestReviewTurnDoesNotRunVerification(t *testing.T) {
	p := atStage(t, StageReview)
	_, act := turn(p, t, &Report{Summary: "s", TasksDone: true})
	if act.Kind != ActionNone {
		t.Fatalf("review verification belongs to P3: %+v", act)
	}
}

func TestVerificationOutcomes(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) Pipeline
		v             *Verification
		wantAction    string
		wantCurrent   StageID
		wantBuild     StageStatus
		wantIteration int
		wantGate      string
		wantCard      string
		wantPrompt    []string
	}{
		{
			name: "pass and tasks done advances",
			setup: func(t *testing.T) Pipeline {
				p, _ := turn(inBuild(t), t, &Report{Summary: "done", TasksDone: true})
				return p
			},
			v: passed(), wantAction: ActionSendTurn, wantCurrent: StageReview, wantBuild: StatusDone,
			wantIteration: 1, wantPrompt: []string{"review"},
		},
		{
			name: "pass without tasks done continues",
			setup: func(t *testing.T) Pipeline {
				p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
				return p
			},
			v: passed(), wantAction: ActionSendTurn, wantCurrent: StageBuild, wantBuild: StatusActive,
			wantIteration: 1, wantPrompt: []string{"next task"},
		},
		{
			name: "fail under limit retries with the excerpt",
			setup: func(t *testing.T) Pipeline {
				p, _ := turn(inBuild(t), t, &Report{Summary: "t1", TasksDone: true})
				return p
			},
			v: failed("make test", "--- FAIL: TestX (0.01s)"), wantAction: ActionSendTurn, wantCurrent: StageBuild,
			wantBuild: StatusActive, wantIteration: 2, wantCard: CardFailure,
			wantPrompt: []string{"make test", "exit 2", "--- FAIL: TestX"},
		},
		{
			name: "fail at limit blocks",
			setup: func(t *testing.T) Pipeline {
				p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
				p.Stages[1].Iteration = 3
				return p
			},
			v: failed("make test", "boom"), wantAction: ActionNone, wantCurrent: StageBuild, wantBuild: StatusBlocked,
			wantIteration: 3, wantGate: GateBlocked, wantCard: CardGate,
		},
		{
			name: "same failure twice blocks",
			setup: func(t *testing.T) Pipeline {
				p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
				p, _ = verify(p, t, failed("make test", "--- FAIL: TestX (0.01s)"))
				p, _ = turn(p, t, &Report{Summary: "fixed?"})
				return p
			},
			// Durations differ between runs; the failure is the same.
			v: failed("make test", "--- FAIL: TestX (0.27s)"), wantAction: ActionNone, wantCurrent: StageBuild,
			wantBuild: StatusBlocked, wantIteration: 2, wantGate: GateBlocked, wantCard: CardGate,
		},
		{
			name: "a different failure is progress",
			setup: func(t *testing.T) Pipeline {
				p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
				p, _ = verify(p, t, failed("make test", "--- FAIL: TestX"))
				p, _ = turn(p, t, &Report{Summary: "fixed?"})
				return p
			},
			v: failed("make test", "--- FAIL: TestY"), wantAction: ActionSendTurn, wantCurrent: StageBuild,
			wantBuild: StatusActive, wantIteration: 3, wantCard: CardFailure,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.setup(t)
			got, act := verify(p, t, tc.v)
			if act.Kind != tc.wantAction {
				t.Fatalf("action = %+v, want %s", act, tc.wantAction)
			}
			for _, want := range tc.wantPrompt {
				if !strings.Contains(act.Prompt, want) {
					t.Fatalf("prompt %q lacks %q", act.Prompt, want)
				}
			}
			if got.Current != tc.wantCurrent || stageOf(got, StageBuild).Status != tc.wantBuild {
				t.Fatalf("current = %s build = %+v", got.Current, stageOf(got, StageBuild))
			}
			if it := stageOf(got, StageBuild).Iteration; it != tc.wantIteration {
				t.Fatalf("iteration = %d, want %d", it, tc.wantIteration)
			}
			if got.OpenGate != tc.wantGate {
				t.Fatalf("gate = %q", got.OpenGate)
			}
			added := got.Cards[len(p.Cards):]
			if tc.wantCard == "" {
				if len(added) != 0 {
					t.Fatalf("unexpected cards %+v", added)
				}
				return
			}
			if len(added) != 1 || added[0].Kind != tc.wantCard {
				t.Fatalf("cards = %+v", added)
			}
		})
	}
}

func TestVerificationAdvanceResetsBuildFacts(t *testing.T) {
	p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
	p, _ = verify(p, t, failed("make test", "x"))
	p, _ = turn(p, t, &Report{Summary: "done", TasksDone: true})
	got, _ := verify(p, t, passed())
	if got.Current != StageReview || got.TasksDone || got.LastFailure != nil {
		t.Fatalf("got = %+v", got)
	}
	if s := stageOf(got, StageReview); s.Status != StatusActive || s.Iteration != 1 {
		t.Fatalf("review = %+v", s)
	}
}

func TestVerificationFailureCardPayload(t *testing.T) {
	p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
	got, _, err := Apply(p, Event{Kind: EventVerification, Turn: 3, Verification: failed("make test", "--- FAIL: TestX")}, t0)
	if err != nil {
		t.Fatal(err)
	}
	card := got.Cards[len(got.Cards)-1]
	if !strings.Contains(card.Title, "make test") || card.Stage != StageBuild || card.Turn != 3 {
		t.Fatalf("card = %+v", card)
	}
	var payload FailureFact
	if err := json.Unmarshal(card.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != "make test" || payload.ExitCode != 2 || payload.Excerpt != "--- FAIL: TestX" || payload.Iteration != 1 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestVerificationIgnoredOutsideActiveBuild(t *testing.T) {
	tests := map[string]func(t *testing.T) Pipeline{
		"plan gate open": func(t *testing.T) Pipeline { return planned(t) },
		"review":         func(t *testing.T) Pipeline { return atStage(t, StageReview) },
		"blocked": func(t *testing.T) Pipeline {
			p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
			p.Stages[1].Iteration = 3
			p, _ = verify(p, t, failed("make test", "x"))
			return p
		},
		"decision open": func(t *testing.T) Pipeline {
			p, _ := turn(inBuild(t), t, &Report{Summary: "q", Decisions: []Decision{{ID: "d1", Question: "?"}}})
			return p
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			p := setup(t)
			got, act := verify(p, t, failed("make test", "x"))
			if act.Kind != ActionNone || len(got.Cards) != len(p.Cards) || got.OpenGate != p.OpenGate {
				t.Fatalf("stale verification changed state: %+v %+v", act, got)
			}
		})
	}
}

func TestVerificationNeedsFacts(t *testing.T) {
	p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
	if _, _, err := Apply(p, Event{Kind: EventVerification}, t0); err == nil {
		t.Fatal("a verification event without results is an error")
	}
}

func TestBuildWithoutProgressBlocks(t *testing.T) {
	// Passing turns that never claim tasks_done cannot loop forever.
	p := inBuild(t)
	var act Action
	for i := 0; i < 50 && p.OpenGate == GateNone; i++ {
		p, _ = turn(p, t, &Report{Summary: "more"})
		p, act = verify(p, t, passed())
	}
	if p.OpenGate != GateBlocked || act.Kind != ActionNone {
		t.Fatalf("gate = %q act = %+v", p.OpenGate, act)
	}
	if n := stageOf(p, StageBuild).Turns; n != buildTurnCap(p) {
		t.Fatalf("turns = %d, cap %d", n, buildTurnCap(p))
	}
}

func blocked(t *testing.T) Pipeline {
	t.Helper()
	p, _ := turn(inBuild(t), t, &Report{Summary: "t1"})
	p.Stages[1].Iteration = 3
	p, _ = verify(p, t, failed("make test", "--- FAIL: TestX"))
	if p.OpenGate != GateBlocked {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	return p
}

func TestBlockedGateActions(t *testing.T) {
	tests := []struct {
		name          string
		action        string
		note          string
		wantErr       error
		wantStatus    StageStatus
		wantAction    string
		wantPrompt    string
		wantIteration int
		wantLimit     int
	}{
		{name: "retry", action: GateRetry, wantStatus: StatusActive, wantAction: ActionSendTurn, wantPrompt: "--- FAIL: TestX", wantIteration: 4, wantLimit: 4},
		{name: "instruct", action: GateInstruct, note: "skip the flaky test", wantStatus: StatusActive, wantAction: ActionSendTurn, wantPrompt: "skip the flaky test", wantIteration: 4, wantLimit: 4},
		{name: "instruct needs a note", action: GateInstruct, wantErr: ErrInvalidAction},
		{name: "stop", action: GateStop, wantStatus: StatusBlocked, wantAction: ActionNone, wantIteration: 3, wantLimit: 3},
		{name: "approve is not a blocked action", action: GateApprove, wantErr: ErrInvalidAction},
		{name: "request changes is not a blocked action", action: GateRequestChanges, wantErr: ErrInvalidAction},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := blocked(t)
			got, act, err := Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: tc.action, Note: tc.note}, t0)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
				if got.OpenGate != GateBlocked {
					t.Fatal("a rejected action leaves the gate open")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b := stageOf(got, StageBuild)
			if b.Status != tc.wantStatus || b.Iteration != tc.wantIteration || b.Limit != tc.wantLimit {
				t.Fatalf("build = %+v", b)
			}
			if act.Kind != tc.wantAction || !strings.Contains(act.Prompt, tc.wantPrompt) {
				t.Fatalf("action = %+v", act)
			}
			if got.OpenGate != GateNone {
				t.Fatalf("gate = %q", got.OpenGate)
			}
			card := got.Cards[len(got.Cards)-1]
			if card.Kind != CardGate || card.State != CardDecided || card.Decision != tc.action {
				t.Fatalf("card = %+v", card)
			}
			if tc.action != GateStop && got.LastFailure != nil {
				t.Fatal("a retry starts repeat detection afresh")
			}
		})
	}
}

func TestRetryInstructOnlyOnBlockedGate(t *testing.T) {
	for _, action := range []string{GateRetry, GateInstruct} {
		p := planned(t)
		_, _, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: action, Note: "n"}, t0)
		if !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("%s on the plan gate: err = %v", action, err)
		}
	}
}

func TestBlockedStopEventStillWorks(t *testing.T) {
	// The pipeline's stop button also stops a blocked pipeline.
	p := blocked(t)
	got, _, err := Apply(p, Event{Kind: EventStop}, t0)
	if err != nil || got.OpenGate != GateNone || stageOf(got, StageBuild).Status != StatusBlocked {
		t.Fatalf("err = %v gate = %q", err, got.OpenGate)
	}
	if card := got.Cards[len(got.Cards)-1]; card.State != CardDecided || card.Decision != GateStop {
		t.Fatalf("card = %+v", card)
	}
	if _, _, err := Apply(got, Event{Kind: EventStop}, t0); !errors.Is(err, ErrNotActive) {
		t.Fatalf("second stop: err = %v", err)
	}
}

func TestRecordQATurn(t *testing.T) {
	p := inBuild(t)
	got, err := RecordQATurn(p, "qa1", "c1", 1, t0)
	if err != nil {
		t.Fatal(err)
	}
	got, err = RecordQATurn(got, "qa1", "c1", 2, t0)
	if err != nil {
		t.Fatal(err)
	}
	if got.QASessionID != "qa1" || len(got.QATurns["c1"]) != 2 || got.QATurns["c1"][1] != 2 {
		t.Fatalf("got = %+v", got)
	}
	if p.QATurns != nil {
		t.Fatal("RecordQATurn must not modify its input")
	}
	// Nothing else changes.
	got.QASessionID, got.QATurns, got.UpdatedAt = "", nil, p.UpdatedAt
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(p)
	if string(a) != string(b) {
		t.Fatalf("Q&A changed the pipeline:\n%s\n%s", a, b)
	}
	if _, err := RecordQATurn(p, "qa1", "nope", 1, t0); !errors.Is(err, ErrCardNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGuidanceAsksForTasksDone(t *testing.T) {
	if g := Guidance(inBuild(t)); !strings.Contains(g, `"tasks_done"`) {
		t.Fatalf("build guidance lacks tasks_done:\n%s", g)
	}
	if g := Guidance(atStage(t, StageReview)); strings.Contains(g, `"tasks_done"`) {
		t.Fatal("tasks_done is a build field")
	}
}

func TestParseReportTasksDone(t *testing.T) {
	b := ParseBlocks(`ok <focus-report>{"summary":"s","tasks_done":true}</focus-report>`)
	if b.Report == nil || !b.Report.TasksDone {
		t.Fatalf("report = %+v", b.Report)
	}
}
