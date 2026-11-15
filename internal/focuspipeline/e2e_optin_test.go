package focuspipeline

import (
	"encoding/json"
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
	for _, want := range []string{`"e2e":[`, `"e2e_setup":[`, `must start with "@AppName "`, "never what an image or a video", `"@Browser <URL> <goal>"`, "signed in to the"} {
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

// The build and review turns see the goals as the server's checks, not as
// tasks or questions (dogfooding, 2026-11-15: agents asked about a goal they
// judged wrong every turn and never finished), and the list is labeled by
// where each kind of goal runs, not as computer_use alone.
func TestWorkGuidanceTreatsEndToEndGoalsAsChecks(t *testing.T) {
	p := inReview(t)
	p.E2E = true
	p.Plan.E2E = []string{"@Browser http://127.0.0.1:8080/ shows the chart"}
	p.Plan.E2ESetup = []string{"serve 8080"}
	g := Guidance(p)
	for _, want := range []string{"a @Browser goal in the developer's browser", "not tasks for you and not questions", "say so once under risks", "copy or link files into it"} {
		if !strings.Contains(g, want) {
			t.Fatalf("review guidance lacks %q:\n%s", want, g)
		}
	}
	if strings.Contains(g, "End-to-end goals (computer_use") {
		t.Fatalf("goals still labeled as computer_use only:\n%s", g)
	}
	p.Plan.E2ESetup = nil
	if g := Guidance(p); strings.Contains(g, "ask the developer to change the plan") || !strings.Contains(g, "No end-to-end setup is listed") {
		t.Fatalf("no-setup note:\n%s", g)
	}
}

// A passing verification that ran end-to-end goals leaves a notice card with
// each goal and the checker's last lines; before, only a failure was ever
// recorded, so a pipeline could not show its goals had run (dogfooding,
// 2026-11-15). A verification without goals, or a failed one, leaves none.
func TestPassingEndToEndGoalsLeaveANotice(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	pass := &Verification{Passed: true, Results: []VerificationResult{
		{Command: "make test", Passed: true, Excerpt: "ok"},
		{Command: "@Browser http://127.0.0.1:8080/ shows the chart", Passed: true, E2E: true,
			Excerpt: "done\nopened the page\n\nthe chart is drawn with three bars\nVERDICT: PASS"},
		{Command: "@Safari check it", Passed: true, Skipped: true, E2E: true, Excerpt: "unavailable: no driver"},
	}}
	before := len(p.Cards)
	got, _, err := Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: pass}, t0)
	if err != nil {
		t.Fatal(err)
	}
	var notice *Card
	for i := range got.Cards[before:] {
		if c := got.Cards[before+i]; c.Kind == CardNotice && c.Title == NoticeE2EPassed {
			notice = &c
		}
	}
	if notice == nil {
		t.Fatalf("no %q card among %+v", NoticeE2EPassed, got.Cards[before:])
	}
	var payload struct {
		Checks []E2ECheck `json:"checks"`
	}
	if err := json.Unmarshal(notice.Payload, &payload); err != nil || len(payload.Checks) != 2 {
		t.Fatalf("payload = %s (%v)", notice.Payload, err)
	}
	if c := payload.Checks[0]; c.Goal != "@Browser http://127.0.0.1:8080/ shows the chart" || c.Skipped ||
		c.Summary != "opened the page\nthe chart is drawn with three bars\nVERDICT: PASS" {
		t.Fatalf("first check = %+v", c)
	}
	if !payload.Checks[1].Skipped {
		t.Fatalf("second check = %+v", payload.Checks[1])
	}

	noE2E := &Verification{Passed: true, Results: []VerificationResult{{Command: "make test", Passed: true}}}
	if got, _, _ := Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: noE2E}, t0); len(got.Cards) != before {
		t.Fatalf("a verification without goals left cards: %+v", got.Cards[before:])
	}
	if checks := passedE2EChecks(Verification{Results: pass.Results}); checks != nil {
		t.Fatalf("failed verification checks = %+v", checks)
	}
	long := strings.Repeat("x", e2eSummaryRunes+50)
	if s := summaryTail(long); len([]rune(s)) != e2eSummaryRunes+1 || !strings.HasPrefix(s, "…") {
		t.Fatalf("long summary = %d runes", len([]rune(s)))
	}
}
