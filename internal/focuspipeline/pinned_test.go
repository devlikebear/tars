package focuspipeline

import (
	"strings"
	"testing"
)

var pinnedForTest = &PinnedChecks{
	Verify:      []string{"test -s index.html"},
	E2E:         []string{"@Browser http://127.0.0.1:48731/ shows a giraffe"},
	E2ESetup:    []string{"serve 48731"},
	E2ETeardown: []string{"stop 48731"},
}

func TestCleanPinnedChecks(t *testing.T) {
	if c, err := CleanPinnedChecks(nil, true); c != nil || err != nil {
		t.Fatalf("nil = %+v %v", c, err)
	}
	if c, err := CleanPinnedChecks(&PinnedChecks{Verify: []string{" ", ""}}, true); c != nil || err != nil {
		t.Fatalf("blank = %+v %v", c, err)
	}
	c, err := CleanPinnedChecks(&PinnedChecks{Verify: []string{" make test ", ""}, E2E: []string{" @Browser x "}}, true)
	if err != nil || strings.Join(c.Verify, ",") != "make test" || strings.Join(c.E2E, ",") != "@Browser x" {
		t.Fatalf("cleaned = %+v %v", c, err)
	}
	if _, err := CleanPinnedChecks(&PinnedChecks{E2ESetup: []string{"serve"}}, false); err == nil {
		t.Fatal("end-to-end checks without e2e must be refused")
	}
	if c, err := CleanPinnedChecks(&PinnedChecks{Verify: []string{"make test"}}, false); err != nil || c == nil {
		t.Fatalf("verify without e2e = %+v %v", c, err)
	}
	many := make([]string, MaxPinnedChecks+1)
	for i := range many {
		many[i] = "x"
	}
	if _, err := CleanPinnedChecks(&PinnedChecks{Verify: many}, true); err == nil {
		t.Fatal("too many pinned checks must be refused")
	}
}

// The dogfooding run of 2026-11-15: the plan stage rewrote a failing
// end-to-end goal it was told to use into goals it could pass, and goal
// mode approved it. A pinned goal survives whatever the plan lists.
func TestPinnedChecksSurviveThePlan(t *testing.T) {
	p := New("s1", "goal", t0)
	p.E2E = true
	p.Checks = pinnedForTest
	if g := Guidance(p); !strings.Contains(g, "pinned these checks") || !strings.Contains(g, "shows a giraffe") {
		t.Fatalf("plan guidance does not name the pinned checks:\n%s", g)
	}
	plan := testPlan()
	plan.E2E = []string{"@Browser http://127.0.0.1:48731/ shows a heading"}
	plan.E2ESetup = []string{"agent serve 9999"}
	got, _, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"verify":       "make test,test -s index.html",
		"e2e":          "@Browser http://127.0.0.1:48731/ shows a heading,@Browser http://127.0.0.1:48731/ shows a giraffe",
		"e2e_setup":    "serve 48731",
		"e2e_teardown": "stop 48731",
	}
	have := map[string][]string{"verify": got.Plan.Verify, "e2e": got.Plan.E2E, "e2e_setup": got.Plan.E2ESetup, "e2e_teardown": got.Plan.E2ETeardown}
	for name, w := range want {
		if strings.Join(have[name], ",") != w {
			t.Fatalf("%s = %q, want %q", name, have[name], w)
		}
	}
	if pinnedForTest.Verify[0] != "test -s index.html" || len(pinnedForTest.Verify) != 1 {
		t.Fatal("fitting a plan changed the pinned checks")
	}

	// Pinned checks without e2e on pin only verify.
	off := New("s2", "goal", t0)
	off.Checks = &PinnedChecks{Verify: []string{"make lint"}}
	got, _, _ = Apply(off, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: testPlan()}}, t0)
	if strings.Join(got.Plan.Verify, ",") != "make test,make lint" || len(got.Plan.E2E) != 0 {
		t.Fatalf("plan without e2e = %+v", got.Plan)
	}
}

func TestPinnedChecksSurviveEdits(t *testing.T) {
	p := inReview(t)
	p.E2E = true
	p.Checks = pinnedForTest
	p.Plan.Verify = p.pinList("verify", p.Plan.Verify)
	p.Plan.E2E = p.pinList("e2e", p.Plan.E2E)
	p.Plan.E2ESetup = p.pinList("e2e_setup", p.Plan.E2ESetup)
	p.Plan.E2ETeardown = p.pinList("e2e_teardown", p.Plan.E2ETeardown)

	got, _, err := EditPlan(p, PlanEdit{Verify: strs("make lint"), E2E: strs(), E2ESetup: strs("other"), E2ETeardown: strs()}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Plan.Verify, ",") != "make lint,test -s index.html" ||
		strings.Join(got.Plan.E2E, ",") != strings.Join(pinnedForTest.E2E, ",") ||
		strings.Join(got.Plan.E2ESetup, ",") != "serve 48731" || strings.Join(got.Plan.E2ETeardown, ",") != "stop 48731" {
		t.Fatalf("plan after an edit that drops pinned checks = %+v", got.Plan)
	}

	// In goal mode an agent edit listing the plan without its pinned checks
	// changes nothing, so it is not a locked change.
	p.GoalMode = &GoalMode{Enabled: true}
	if locked := GoalModeLockedFields(p, PlanEdit{E2E: strs("make console-e2e"), E2ETeardown: strs()}); len(locked) != 0 {
		t.Fatalf("locked = %v", locked)
	}

	c := p.clone()
	c.Checks.Verify[0] = "changed"
	if p.Checks.Verify[0] != "test -s index.html" {
		t.Fatal("clone shares the pinned checks")
	}
}
