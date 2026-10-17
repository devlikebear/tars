package focuspipeline

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata")

// TestNoOptInGolden records a development pipeline that never opted in to
// anything (no second opinion, no end-to-end goals) from its plan through a
// review round with findings, a fix, a second review round and the PR
// stage: the pipeline JSON and the stage guidance after every step. The
// golden file was written before the second opinion review existed, so a
// pipeline without the option stays byte for byte what it was.
func TestNoOptInGolden(t *testing.T) {
	var out strings.Builder
	step := func(name string, p Pipeline, act Action) {
		t.Helper()
		raw, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "== %s\naction: %s %q\npipeline:\n%s\nguidance:\n%s\n\n", name, act.Kind, act.Prompt, raw, Guidance(p))
	}
	p := New("s1", "goal", t0)
	step("new", p, noAction)
	p, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: testPlan(StagePlan, StageBuild, StageReview, StagePR)}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	step("plan proposed", p, act)
	if p, act, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0); err != nil {
		t.Fatal(err)
	}
	step("plan approved", p, act)
	p.BaseCommit = "0123456789abcdef0123456789abcdef01234567"
	p, act = turn(p, t, &Report{Summary: "built", TasksDone: true})
	step("build reported", p, act)
	p, act = verify(p, t, passed())
	step("build verified", p, act)
	p, act = reviewTurn(t, p, twoFindings())
	step("review findings", p, act)
	p, act = decide(t, p, findingCards(p)[0].ID, DecisionFix)
	step("first decided", p, act)
	p, act = decide(t, p, findingCards(p)[1].ID, DecisionDismiss)
	step("triage closed", p, act)
	p, act = turn(p, t, &Report{Summary: "fixed"})
	step("fix reported", p, act)
	p, act = verify(p, t, passed())
	step("fix verified", p, act)
	p, act = reviewTurn(t, p, []Finding{})
	step("second round clean", p, act)
	p, act = verify(p, t, passed())
	step("review done", p, act)

	path := filepath.Join("testdata", "no_opt_in.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to write it): %v", err)
	}
	if got := out.String(); got != string(want) {
		t.Fatalf("a pipeline without the option changed; diff %s against the output:\n%s", path, got)
	}
}
