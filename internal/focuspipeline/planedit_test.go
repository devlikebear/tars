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
	if plan := Guidance(New("s1", "goal", t0)); !strings.Contains(plan, `"e2e_setup"`) || !strings.Contains(plan, "never builds or launches") || strings.Contains(plan, "<focus-plan-edit>") {
		t.Fatalf("plan guidance:\n%s", plan)
	}
}
