package focuspipeline

import (
	"errors"
	"strings"
	"testing"
)

// End-to-end goals are opt-in: a pipeline that did not ask for them is not
// told about them, keeps none in its plan, and cannot be edited into them.
func TestEndToEndGoalsAreOffByDefault(t *testing.T) {
	fresh := New("s1", "goal", t0)
	if g := Guidance(fresh); strings.Contains(g, "e2e") || strings.Contains(g, "computer_use") {
		t.Fatalf("plan guidance of a default pipeline mentions end-to-end goals:\n%s", g)
	}

	plan := testPlan(StagePlan, StageBuild, StageReview)
	plan.E2ESetup, plan.E2ETeardown = []string{"make build"}, []string{"pkill app"}
	p, _, err := Apply(fresh, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Plan.E2E)+len(p.Plan.E2ESetup)+len(p.Plan.E2ETeardown) != 0 {
		t.Fatalf("a default pipeline kept the plan's end-to-end fields: %+v", p.Plan)
	}
	if strings.Join(p.Plan.Verify, ",") != "make test" {
		t.Fatalf("verify = %q", p.Plan.Verify)
	}
	if p, _, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0); err != nil {
		t.Fatal(err)
	}
	if g := Guidance(p); strings.Contains(g, "e2e") || strings.Contains(g, "End-to-end") {
		t.Fatalf("build guidance of a default pipeline mentions end-to-end goals:\n%s", g)
	}
	if _, _, err := EditPlan(p, PlanEdit{E2E: strs("@Safari check it")}, t0); !errors.Is(err, ErrInvalidEdits) {
		t.Fatalf("editing end-to-end goals into a default pipeline: err = %v", err)
	}
	if _, changed, err := EditPlan(p, PlanEdit{Verify: strs("make test", "make lint")}, t0); err != nil || !changed {
		t.Fatalf("verify stays editable: changed = %v err = %v", changed, err)
	}
}

func TestOptedInPlanGuidanceRequiresAnApp(t *testing.T) {
	p := New("s1", "goal", t0)
	p.E2E = true
	g := Guidance(p)
	for _, want := range []string{`"e2e":[`, `"e2e_setup":[`, `must start with "@AppName "`, "never what an image or a video"} {
		if !strings.Contains(g, want) {
			t.Fatalf("opted-in plan guidance lacks %q:\n%s", want, g)
		}
	}
}

// A review blocked on an end-to-end goal ends goal mode instead of sending
// the agent after it again: the retry loop used to run until the budget or
// the usage limit ran out.
func TestGoalModeStopsAtABlockedEndToEndGoal(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	fail := &Verification{Results: []VerificationResult{
		{Command: "make test", Passed: true},
		{Command: "@Safari check the page", ExitCode: -1, E2E: true, Excerpt: "stuck"},
	}}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: fail}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{Report: &Report{Summary: "again", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 3, Verification: fail}, t0)
	if p.OpenGate != GateBlocked {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	if !p.openBlockedFact().Failure.E2E {
		t.Fatalf("the blocked fact lost the end-to-end mark: %+v", p.openBlockedFact().Failure)
	}
	step := NextGoalStep(p, t0)
	if step.Kind != GoalEnd || step.Reason != GoalEndE2EFailed {
		t.Fatalf("step = %+v, want goal mode to end with %s", step, GoalEndE2EFailed)
	}
}
