package focuspipeline

import (
	"strings"
	"testing"
)

// goalBlockedOn is a pipeline in goal mode whose build stage is blocked on
// the same verification failure twice.
func goalBlockedOn(t *testing.T, fail *Verification) Pipeline {
	t.Helper()
	p := goalBuilding(t, StagePlan, StageBuild)
	for turn := 2; turn <= 3; turn++ {
		p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: turn, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
		p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: turn, Verification: fail}, t0)
	}
	if p.OpenGate != GateBlocked {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	return p
}

func verifyFailing(command string, code int, excerpt string) *Verification {
	return &Verification{Results: []VerificationResult{
		{Command: "npm run build", Passed: true},
		{Command: command, ExitCode: code, Excerpt: excerpt},
	}}
}

func TestGoalRetryFailure(t *testing.T) {
	a := GoalRetryFailure(goalBlockedOn(t, verifyFailing("npm test", 1, "took 12ms")))
	if a == "" {
		t.Fatal("no failure identity on a verification failure")
	}
	if strings.Contains(a, "npm run build") {
		t.Fatalf("a passing command is part of the identity: %q", a)
	}
	// Output noise is not part of the failure.
	if b := GoalRetryFailure(goalBlockedOn(t, verifyFailing("npm test", 1, "took 98ms at 0x1f3a"))); b != a {
		t.Fatalf("same failure, other output: %q != %q", b, a)
	}
	for name, other := range map[string]*Verification{
		"another command":   verifyFailing("npm run lint", 1, ""),
		"another exit code": verifyFailing("npm test", 2, ""),
		"a check report":    verifyFailing("npm test", 1, "CHECK FAILED\nscreen: Home\nexpected: a list\nseen: nothing"),
	} {
		if b := GoalRetryFailure(goalBlockedOn(t, other)); b == a {
			t.Fatalf("%s reads as the same failure", name)
		}
	}
	// A failed turn carries no verification failure.
	turn, _ := FailTurn(goalBuilding(t, StagePlan, StageBuild).withPending("go on"), "timeout", t0)
	if got := GoalRetryFailure(turn); got != "" {
		t.Fatalf("failed turn identity = %q", got)
	}
}

func (p Pipeline) withPending(prompt string) Pipeline {
	next := p.clone()
	next.PendingTurn = prompt
	return next
}

func TestGoalCheckRetry(t *testing.T) {
	blocked := goalBlockedOn(t, verifyFailing("npm test", 1, "boom"))
	failure := GoalRetryFailure(blocked)
	with := func(last *GoalRetryMark, empty int) Pipeline {
		p := blocked.clone()
		p.GoalMode.LastRetry, p.GoalMode.EmptyRetries = last, empty
		return p
	}
	cases := []struct {
		name      string
		p         Pipeline
		tree      string
		wantEnd   bool
		wantEmpty int
		wantMark  bool
	}{
		{name: "first retry", p: with(nil, 0), tree: "T1", wantMark: true},
		{name: "same failure, same tree", p: with(&GoalRetryMark{Failure: failure, Tree: "T1"}, 0), tree: "T1", wantEmpty: 1, wantMark: true},
		{name: "the limit of empty retries", p: with(&GoalRetryMark{Failure: failure, Tree: "T1"}, GoalEmptyRetryLimit-1), tree: "T1", wantEnd: true, wantEmpty: GoalEmptyRetryLimit},
		{name: "the tree changed", p: with(&GoalRetryMark{Failure: failure, Tree: "T1"}, GoalEmptyRetryLimit-1), tree: "T2", wantMark: true},
		{name: "the failure changed", p: with(&GoalRetryMark{Failure: "other", Tree: "T1"}, GoalEmptyRetryLimit-1), tree: "T1", wantMark: true},
		{name: "tree unreadable", p: with(&GoalRetryMark{Failure: failure, Tree: "T1"}, GoalEmptyRetryLimit-1), tree: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step := GoalCheckRetry(c.p, NextGoalStep(c.p, t0), c.tree)
			if c.wantEnd {
				if step.Kind != GoalEnd || step.Reason != GoalEndNoProgress || step.EmptyRetries != c.wantEmpty {
					t.Fatalf("step = %+v", step)
				}
				return
			}
			if step.Kind != GoalGate || step.Action != GateRetry || step.EmptyRetries != c.wantEmpty || (step.Retry != nil) != c.wantMark {
				t.Fatalf("step = %+v", step)
			}
			applied := GoalApplied(c.p, step, t0)
			if applied.GoalMode.EmptyRetries != c.wantEmpty || (applied.GoalMode.LastRetry != nil) != c.wantMark {
				t.Fatalf("recorded = %+v", applied.GoalMode)
			}
		})
	}

	// Other steps pass through untouched.
	gate := GoalStep{Kind: GoalGate, Gate: GatePlan, Action: GateApprove}
	if got := GoalCheckRetry(blocked, gate, "T1"); got.Retry != nil || got.Kind != GoalGate {
		t.Fatalf("plan gate step = %+v", got)
	}
}

func TestGoalModeEndsAfterEmptyRetries(t *testing.T) {
	p := goalBlockedOn(t, verifyFailing("npm test", 1, "boom"))
	retries := 0
	for range DefaultGoalPushes {
		step := GoalCheckRetry(p, NextGoalStep(p, t0), "T1")
		if step.Kind == GoalEnd {
			if step.Reason != GoalEndNoProgress {
				t.Fatalf("end = %+v", step)
			}
			break
		}
		var err error
		p, _, err = Apply(p, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry, CardID: step.CardID}, t0)
		if err != nil {
			t.Fatal(err)
		}
		p = GoalApplied(p, step, t0)
		retries++
		// The round fails the same way twice and blocks again.
		p = goalFailTwice(t, p, verifyFailing("npm test", 1, "boom"))
	}
	if retries != GoalEmptyRetryLimit {
		t.Fatalf("retries before the end = %d, want %d", retries, GoalEmptyRetryLimit)
	}
	ended, ok := EndGoal(p, GoalEndNoProgress, t0)
	last := ended.Cards[len(ended.Cards)-1]
	if !ok || last.Kind != CardNotice || last.Title != NoticeGoalEnded || !strings.Contains(string(last.Payload), GoalEndNoProgress) {
		t.Fatalf("notice = %+v", last)
	}
	// The person's Try again on an easy project brings goal mode back.
	ended.GoalAfterPlan = true
	for _, action := range []string{GateRetry, GateInstruct} {
		if !GoalDueOnApproval(ended, GateBlocked, action) {
			t.Fatalf("not due on %s after no progress", action)
		}
	}
	if again := StartGoal(ended, 0, t0); again.GoalMode.LastRetry != nil || again.GoalMode.EmptyRetries != 0 {
		t.Fatalf("fresh goal mode keeps the run: %+v", again.GoalMode)
	}
}

func goalFailTwice(t *testing.T, p Pipeline, fail *Verification) Pipeline {
	t.Helper()
	for turn := 0; turn < 2 && p.OpenGate != GateBlocked; turn++ {
		p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 10 + turn, Blocks: Blocks{Report: &Report{Summary: "nothing wrong", TasksDone: true}}}, t0)
		p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 10 + turn, Verification: fail}, t0)
	}
	if p.OpenGate != GateBlocked {
		t.Fatalf("not blocked again: gate %q", p.OpenGate)
	}
	return p
}
