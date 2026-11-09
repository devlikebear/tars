package focuspipeline

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func strs(v ...string) *[]string { return &v }

func lastCard(t *testing.T, p Pipeline) Card {
	t.Helper()
	if len(p.Cards) == 0 {
		t.Fatal("no cards")
	}
	return p.Cards[len(p.Cards)-1]
}

func TestEditPlanDropsAnEndToEndGoal(t *testing.T) {
	p := inReview(t)
	p.LastFailure = &FailureFact{Command: "make console-e2e", ExitCode: -1}
	goal := " ship the dialog "
	got, changed, err := EditPlan(p, PlanEdit{Goal: &goal, E2E: strs(), E2ESetup: strs("make build", " ")}, t0)
	if err != nil || !changed {
		t.Fatalf("changed = %v err = %v", changed, err)
	}
	if got.Goal != "ship the dialog" || got.Plan.Goal != "ship the dialog" || len(got.Plan.E2E) != 0 {
		t.Fatalf("plan = %+v goal = %q", got.Plan, got.Goal)
	}
	if strings.Join(got.Plan.E2ESetup, ",") != "make build" || strings.Join(got.Plan.Verify, ",") != "make test" {
		t.Fatalf("lists = %+v", got.Plan)
	}
	if got.LastFailure != nil {
		t.Fatal("the failure of a removed check must not count as repeated")
	}
	if got.Current != p.Current || got.OpenGate != p.OpenGate || len(p.Plan.E2E) != 1 {
		t.Fatal("an edit moves no stage and leaves its input alone")
	}
	card := lastCard(t, got)
	var payload struct {
		Changes []string `json:"changes"`
	}
	if card.Kind != CardNotice || card.Title != NoticePlanEdited || json.Unmarshal(card.Payload, &payload) != nil {
		t.Fatalf("card = %+v", card)
	}
	joined := strings.Join(payload.Changes, "\n")
	for _, want := range []string{"goal: g → ship the dialog", "e2e − make console-e2e", "e2e_setup + make build"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("changes %q lack %q", payload.Changes, want)
		}
	}
}

func TestEditPlanRefusals(t *testing.T) {
	fresh := New("s1", "goal", t0)
	if _, _, err := EditPlan(fresh, PlanEdit{E2E: strs()}, t0); !errors.Is(err, ErrNoApprovedPlan) {
		t.Fatalf("before approval: %v", err)
	}
	p := inReview(t)
	empty := "  "
	if _, _, err := EditPlan(p, PlanEdit{Goal: &empty}, t0); !errors.Is(err, ErrInvalidEdits) {
		t.Fatalf("empty goal: %v", err)
	}
	got, changed, err := EditPlan(p, PlanEdit{Verify: strs("make test")}, t0)
	if err != nil || changed || len(got.Cards) != len(p.Cards) {
		t.Fatalf("no-op edit: changed = %v err = %v", changed, err)
	}

	// A failure of a check the plan keeps stays remembered; a list put in
	// another order is still a change.
	p.Plan.Verify = []string{"make test", "make lint"}
	p.LastFailure = &FailureFact{Command: "make lint", ExitCode: 1}
	got, changed, err = EditPlan(p, PlanEdit{Verify: strs("make lint", "make test")}, t0)
	if err != nil || !changed || got.LastFailure == nil || !strings.Contains(string(lastCard(t, got).Payload), "verify: reordered") {
		t.Fatalf("reorder: changed = %v err = %v card = %s", changed, err, lastCard(t, got).Payload)
	}
}

func TestPlanEditBlockAppliesInATurn(t *testing.T) {
	blocks := ParseBlocks("removed it\n" +
		`<focus-plan-edit>{"e2e":[]}</focus-plan-edit>` + "\n" +
		`<focus-findings>[]</focus-findings>` + "\n" +
		`<focus-report>{"summary":"dropped the live check"}</focus-report>`)
	if len(blocks.Errors) != 0 || blocks.PlanEdit == nil || blocks.PlanEdit.E2E == nil || blocks.Report == nil {
		t.Fatalf("blocks = %+v", blocks)
	}
	if bad := ParseBlocks(`<focus-plan-edit>{}</focus-plan-edit>`); bad.PlanEdit != nil || len(bad.Errors) != 1 {
		t.Fatalf("empty edit: %+v", bad)
	}
	if text := StripBlocks("a\n<focus-plan-edit>{\"e2e\":[]}</focus-plan-edit>"); text != "a" {
		t.Fatalf("stripped = %q", text)
	}

	p := inReview(t)
	got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 7, Blocks: blocks}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan.E2E) != 0 || act.Kind != ActionRunVerification {
		t.Fatalf("e2e = %q act = %+v", got.Plan.E2E, act)
	}
	edited := false
	for _, c := range got.Cards {
		edited = edited || c.Title == NoticePlanEdited
	}
	if !edited {
		t.Fatal("no plan edited notice")
	}
}

func TestGuidanceCoversPlanEditAndEndToEndSetup(t *testing.T) {
	p := inReview(t)
	g := Guidance(p)
	for _, want := range []string{"<focus-plan-edit>", "focus_plan_edit", "No end-to-end setup is listed"} {
		if !strings.Contains(g, want) {
			t.Fatalf("guidance lacks %q:\n%s", want, g)
		}
	}
	p.Plan.E2ESetup = []string{"make build && ./bin/app &"}
	p.Plan.E2ETeardown = []string{"pkill app"}
	g = Guidance(p)
	if strings.Contains(g, "No end-to-end setup is listed") ||
		!strings.Contains(g, "- make build && ./bin/app &") || !strings.Contains(g, "- pkill app") {
		t.Fatalf("guidance with setup:\n%s", g)
	}
	optedIn := New("s1", "goal", t0)
	optedIn.E2E = true
	if plan := Guidance(optedIn); !strings.Contains(plan, `"e2e_setup"`) || !strings.Contains(plan, "never builds or launches") || strings.Contains(plan, "<focus-plan-edit>") {
		t.Fatalf("plan guidance:\n%s", plan)
	}
}

func TestGoalModeLockedFields(t *testing.T) {
	off := inReview(t)
	on := StartGoal(off, 0, t0)
	goal := "a narrower goal"
	cases := []struct {
		name string
		edit PlanEdit
		want string
	}{
		{"goal only", PlanEdit{Goal: &goal}, ""},
		{"verify restated, cleaned", PlanEdit{Goal: &goal, Verify: strs(" make test ", "")}, ""},
		{"verify replaced", PlanEdit{Verify: strs("true")}, "verify"},
		{"verify reordered", PlanEdit{Verify: strs("make test", "make lint")}, "verify"},
		{"verify cleared", PlanEdit{Verify: strs()}, "verify"},
		{"e2e dropped", PlanEdit{E2E: strs()}, "e2e"},
		{"setup added", PlanEdit{E2ESetup: strs("make build")}, "e2e_setup"},
		{"teardown added", PlanEdit{E2ETeardown: strs("pkill app")}, "e2e_teardown"},
		{"all at once", PlanEdit{Verify: strs("true"), E2E: strs(), E2ESetup: strs("x"), E2ETeardown: strs("y")}, "verify,e2e,e2e_setup,e2e_teardown"},
	}
	for _, c := range cases {
		if got := GoalModeLockedFields(off, c.edit); got != nil {
			t.Fatalf("%s: goal mode off locks %v", c.name, got)
		}
		if got := strings.Join(GoalModeLockedFields(on, c.edit), ","); got != c.want {
			t.Fatalf("%s: locked = %q, want %q", c.name, got, c.want)
		}
		_, _, err := EditPlanByAgent(on, c.edit, t0)
		if refused := errors.Is(err, ErrGoalModeChecks); refused != (c.want != "") {
			t.Fatalf("%s: err = %v", c.name, err)
		}
		if c.want != "" && !strings.HasSuffix(err.Error(), "(asked to change "+strings.ReplaceAll(c.want, ",", ", ")+")") {
			t.Fatalf("%s: the refusal does not name %q: %v", c.name, c.want, err)
		}
	}
	ended, _ := EndGoal(on, "done", t0)
	if got := GoalModeLockedFields(ended, PlanEdit{Verify: strs("true")}); got != nil {
		t.Fatalf("ended goal mode still locks %v", got)
	}
	// The developer's EditPlan is never locked.
	if got, changed, err := EditPlan(on, PlanEdit{Verify: strs("true")}, t0); err != nil || !changed || got.Plan.Verify[0] != "true" {
		t.Fatalf("developer edit in goal mode: changed = %v err = %v", changed, err)
	}
}

func TestPlanEditBlockRefusedInGoalMode(t *testing.T) {
	p := StartGoal(inReview(t), 0, t0)
	blocks := ParseBlocks(`<focus-plan-edit>{"goal":"new goal","verify":["true"]}</focus-plan-edit>` + "\n" +
		`<focus-findings>[]</focus-findings>` + "\n" + `<focus-report>{"summary":"s"}</focus-report>`)
	got, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 7, Blocks: blocks}, t0)
	if err != nil {
		t.Fatal(err)
	}
	// The whole edit is refused, its goal too; the turn still counts.
	if got.Goal != p.Goal || strings.Join(got.Plan.Verify, ",") != "make test" || act.Kind != ActionRunVerification {
		t.Fatalf("goal = %q verify = %q act = %+v", got.Goal, got.Plan.Verify, act)
	}
	refused := false
	for _, c := range got.Cards {
		if c.Title == NoticePlanEdited {
			t.Fatal("a refused edit left a plan edited card")
		}
		refused = refused || (c.Title == NoticeFormatMissing && strings.Contains(string(c.Payload), "goal mode"))
	}
	if !refused {
		t.Fatalf("no notice for the refused edit: %+v", got.Cards)
	}
}

func TestPlanEditGuidanceInGoalMode(t *testing.T) {
	p := inReview(t)
	off := p.planEditGuidance()
	on := StartGoal(p, 0, t0).planEditGuidance()
	if strings.Contains(off, "Goal mode") || !strings.Contains(off, `"verify"`) {
		t.Fatalf("guidance without goal mode changed:\n%s", off)
	}
	if !strings.Contains(on, "cannot be changed") || strings.Contains(on, `"verify"`) || strings.Contains(on, `"e2e"`) ||
		!strings.Contains(on, "end-to-end goals") || !strings.Contains(on, `{"goal":"…"}`) {
		t.Fatalf("goal mode guidance:\n%s", on)
	}
	p.E2E = false
	if g := StartGoal(p, 0, t0).planEditGuidance(); strings.Contains(g, "end-to-end") {
		t.Fatalf("goal mode guidance names end-to-end goals the pipeline does not have:\n%s", g)
	}
	if !strings.Contains(Guidance(StartGoal(inReview(t), 0, t0)), "cannot be changed in this run") {
		t.Fatal("the turn guidance must carry the goal mode wording")
	}
}
