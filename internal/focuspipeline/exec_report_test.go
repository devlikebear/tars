package focuspipeline

import (
	"math"
	"testing"
	"time"
)

func TestExecFactsOf(t *testing.T) {
	ended := t0.Add(time.Hour)
	stages := func(statuses ...StageStatus) []Stage {
		ids := []StageID{StagePlan, StageBuild, StageReview, StagePR, StagePRReview, StageMerge}
		out := make([]Stage, len(ids))
		for i, id := range ids {
			out[i] = Stage{ID: id, Status: statuses[i], Iteration: 1}
		}
		return out
	}
	done := stages(StatusDone, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone)
	done[2].Iteration, done[4].Iteration = 3, 2

	cases := []struct {
		name string
		p    Pipeline
		want ExecFacts
	}{
		{
			name: "no opt-in, running",
			p:    Pipeline{SessionID: "a", Current: StageBuild, Stages: stages(StatusDone, StatusActive, StatusPending, StatusPending, StatusPending, StatusPending)},
			want: ExecFacts{SessionID: "a", Tier: ExecTierSession, Library: ExecLibraryUnrecorded, Outcome: ExecOutcomeRunning},
		},
		{
			name: "light with a library, finished in goal mode",
			p: Pipeline{
				SessionID: "b", Current: StageMerge, Stages: done,
				ExecTier: &ExecTier{Tier: "light", Repo: "/r", Lessons: 2, At: t0},
				Cards:    []Card{{Kind: CardFailure}, {Kind: CardFailure}, {Kind: CardNotice}},
				GoalMode: &GoalMode{Pushes: 4, StartedAt: t0.Add(time.Minute), EndedAt: &ended, EndReason: GoalEndFinished},
			},
			want: ExecFacts{
				SessionID: "b", Tier: "light", Library: ExecLibraryApplied, Repo: "/r", Outcome: ExecOutcomeFinished, Start: t0,
				FixRounds: 3, FailedVerifies: 2, Goal: true, GoalEnded: true, GoalFinished: true, GoalPushes: 4,
			},
		},
		{
			name: "heavy without a library, stopped; goal start fills nothing past the label",
			p: Pipeline{
				SessionID: "c", Current: StageBuild, Stages: stages(StatusDone, StatusBlocked, StatusPending, StatusPending, StatusPending, StatusPending),
				ExecTier: &ExecTier{Tier: "heavy", At: t0},
				GoalMode: &GoalMode{Pushes: 20, StartedAt: t0.Add(time.Minute), EndedAt: &ended, EndReason: GoalEndExhausted},
			},
			want: ExecFacts{
				SessionID: "c", Tier: "heavy", Library: ExecLibraryNone, Outcome: ExecOutcomeStopped, Start: t0,
				Goal: true, GoalEnded: true, GoalPushes: 20,
			},
		},
		{
			name: "no opt-in in goal mode starts at goal mode",
			p: Pipeline{
				SessionID: "d", Current: StageBuild, Stages: stages(StatusDone, StatusActive, StatusPending, StatusPending, StatusPending, StatusPending),
				GoalMode: &GoalMode{Enabled: true, StartedAt: t0},
			},
			want: ExecFacts{SessionID: "d", Tier: ExecTierSession, Library: ExecLibraryUnrecorded, Outcome: ExecOutcomeRunning, Start: t0, Goal: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExecFactsOf(tc.p); got != tc.want {
				t.Fatalf("facts\n got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestExecReportGroupsAndRates(t *testing.T) {
	facts := []ExecFacts{
		// A: heavy, no library — 2 finished, 1 stopped.
		{Tier: "heavy", Library: ExecLibraryNone, Outcome: ExecOutcomeFinished, CostUSD: 3, FixRounds: 1, Goal: true, GoalEnded: true, GoalFinished: true, GoalPushes: 2},
		{Tier: "heavy", Library: ExecLibraryNone, Outcome: ExecOutcomeFinished, CostUSD: 4, FailedVerifies: 1, Goal: true, GoalEnded: true, GoalFinished: true, GoalPushes: 3},
		{Tier: "heavy", Library: ExecLibraryNone, Outcome: ExecOutcomeStopped, CostUSD: 2, Goal: true, GoalEnded: true, GoalPushes: 20},
		// A running pipeline counts but is not measured.
		{Tier: "heavy", Library: ExecLibraryNone, Outcome: ExecOutcomeRunning, CostUSD: 50, FixRounds: 9},
		// C: light with a library — 3 finished, cheaper.
		{Tier: "light", Library: ExecLibraryApplied, Outcome: ExecOutcomeFinished, CostUSD: 1, FixRounds: 2, Goal: true, GoalEnded: true, GoalFinished: true, GoalPushes: 4, UnpricedCalls: 1},
		{Tier: "light", Library: ExecLibraryApplied, Outcome: ExecOutcomeFinished, CostUSD: 1, FixRounds: 1, Goal: true, GoalEnded: true, GoalFinished: true, GoalPushes: 2},
		{Tier: "light", Library: ExecLibraryApplied, Outcome: ExecOutcomeFinished, CostUSD: 1, Goal: true, GoalEnded: true, GoalFinished: true},
		// B and unlabeled.
		{Tier: "light", Library: ExecLibraryNone, Outcome: ExecOutcomeStopped, CostUSD: 0.5},
		{Tier: ExecTierSession, Library: ExecLibraryUnrecorded, Outcome: ExecOutcomeFinished, CostUSD: 7},
		{Tier: "standard", Library: ExecLibraryNone, Outcome: ExecOutcomeRunning},
	}
	got := ExecReport(facts)
	order := [][2]string{{"heavy", "none"}, {"standard", "none"}, {"light", "none"}, {"light", "applied"}, {"session", "unrecorded"}}
	if len(got.Rows) != len(order) {
		t.Fatalf("rows %+v", got.Rows)
	}
	for i, k := range order {
		if got.Rows[i].Tier != k[0] || got.Rows[i].Library != k[1] {
			t.Fatalf("row %d is %s/%s, want %s/%s", i, got.Rows[i].Tier, got.Rows[i].Library, k[0], k[1])
		}
	}
	a := got.Rows[0]
	if a.Pipelines != 4 || a.Finished != 2 || a.Stopped != 1 || a.Running != 1 {
		t.Fatalf("A counts %+v", a)
	}
	if !near(a.CompletionRate, 2.0/3) || !near(a.CostUSD, 9) || !near(a.MeanCostUSD, 3) || !near(a.MeanFixRounds, 1.0/3) ||
		!near(a.MeanFailedVerifies, 1.0/3) || !near(a.MeanGoalPushes, 25.0/3) || a.GoalRuns != 3 || a.GoalFinished != 2 || !near(a.GoalCompletionRate, 2.0/3) {
		t.Fatalf("A numbers %+v", a)
	}
	standard := got.Rows[1]
	if standard.Running != 1 || standard.CompletionRate != 0 || standard.MeanCostUSD != 0 {
		t.Fatalf("a configuration with nothing ended has no rates: %+v", standard)
	}
	c := got.Rows[3]
	if c.UnpricedCalls != 1 || !near(c.MeanCostUSD, 1) || c.GoalCompletionRate != 1 {
		t.Fatalf("C numbers %+v", c)
	}
	if len(got.Comparisons) != 1 {
		t.Fatalf("comparisons %+v", got.Comparisons)
	}
	if cmp := got.Comparisons[0]; cmp != (ExecComparison{Tier: "light", Enough: true, Cheaper: true, CompletionOK: true, Rate: "goal_completion", Recommend: true}) {
		t.Fatalf("comparison %+v", cmp)
	}
}

func TestExecReportComparison(t *testing.T) {
	row := func(tier, library string, outcomes []string, cost float64, goal bool) []ExecFacts {
		var out []ExecFacts
		for _, o := range outcomes {
			out = append(out, ExecFacts{Tier: tier, Library: library, Outcome: o, CostUSD: cost, Goal: goal, GoalEnded: goal, GoalFinished: goal && o == ExecOutcomeFinished})
		}
		return out
	}
	fin, stop := ExecOutcomeFinished, ExecOutcomeStopped
	cases := []struct {
		name  string
		facts [][]ExecFacts
		want  []ExecComparison
	}{
		{
			name:  "no A row",
			facts: [][]ExecFacts{row("light", ExecLibraryApplied, []string{fin, fin, fin}, 1, false)},
			want:  []ExecComparison{},
		},
		{
			name:  "too few samples",
			facts: [][]ExecFacts{row("heavy", ExecLibraryNone, []string{fin, fin, fin}, 3, false), row("standard", ExecLibraryApplied, []string{fin, fin}, 1, false)},
			want:  []ExecComparison{{Tier: "standard", Rate: "completion", Cheaper: true, CompletionOK: true}},
		},
		{
			name:  "cheaper but completes less",
			facts: [][]ExecFacts{row("heavy", ExecLibraryNone, []string{fin, fin, fin}, 3, false), row("light", ExecLibraryApplied, []string{fin, fin, stop}, 1, false)},
			want:  []ExecComparison{{Tier: "light", Enough: true, Rate: "completion", Cheaper: true}},
		},
		{
			name:  "completes as often but costs more",
			facts: [][]ExecFacts{row("heavy", ExecLibraryNone, []string{fin, fin, fin}, 3, true), row("light", ExecLibraryApplied, []string{fin, fin, fin}, 4, true)},
			want:  []ExecComparison{{Tier: "light", Enough: true, Rate: "goal_completion", CompletionOK: true}},
		},
		{
			name:  "C with nothing ended",
			facts: [][]ExecFacts{row("heavy", ExecLibraryNone, []string{fin, fin, fin}, 3, false), row("light", ExecLibraryApplied, []string{ExecOutcomeRunning}, 1, false)},
			want:  []ExecComparison{{Tier: "light", Rate: "completion"}},
		},
		{
			name:  "B and heavy with a library are not C",
			facts: [][]ExecFacts{row("heavy", ExecLibraryNone, []string{fin}, 3, false), row("light", ExecLibraryNone, []string{fin}, 1, false), row("heavy", ExecLibraryApplied, []string{fin}, 1, false)},
			want:  []ExecComparison{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var all []ExecFacts
			for _, f := range tc.facts {
				all = append(all, f...)
			}
			got := ExecReport(all).Comparisons
			if len(got) != len(tc.want) {
				t.Fatalf("comparisons %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("comparison %+v, want %+v", got[i], tc.want[i])
				}
			}
		})
	}
}

func TestExecReportEmpty(t *testing.T) {
	got := ExecReport(nil)
	if got.Rows == nil || got.Comparisons == nil || len(got.Rows) != 0 {
		t.Fatalf("empty report %+v", got)
	}
}
