package focuspipeline

import (
	"strings"
	"testing"
	"time"
)

// goalBuilding is a pipeline in goal mode whose build stage is running.
func goalBuilding(t *testing.T, stages ...StageID) Pipeline {
	t.Helper()
	p := StartGoal(planned(t, stages...), 0, t0)
	p, _, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var later = t0.Add(time.Hour)

func TestStartAndEndGoal(t *testing.T) {
	p := StartGoal(New("s1", "g", t0), 0, t0)
	if !p.GoalActive() || p.GoalMode.MaxPushes != DefaultGoalPushes || p.GoalMode.Pushes != 0 {
		t.Fatalf("goal = %+v", p.GoalMode)
	}
	if got := StartGoal(p, MaxGoalPushes+50, t0).GoalMode.MaxPushes; got != MaxGoalPushes {
		t.Fatalf("max pushes = %d", got)
	}
	p = GoalPermission(p, "manual", t0)
	if !p.GoalMode.PermissionSet || p.GoalMode.RestorePermission != "manual" {
		t.Fatalf("goal = %+v", p.GoalMode)
	}
	// A new budget while on keeps what the session's mode was.
	if again := StartGoal(p, 5, t0); !again.GoalMode.PermissionSet || again.GoalMode.RestorePermission != "manual" || again.GoalMode.MaxPushes != 5 {
		t.Fatalf("restarted goal = %+v", again.GoalMode)
	}
	if plain := GoalPermission(New("s1", "g", t0), "manual", t0); plain.GoalMode != nil {
		t.Fatalf("goal permission on a pipeline without goal mode: %+v", plain.GoalMode)
	}

	ended, ok := EndGoal(p, GoalEndExhausted, later)
	if !ok || ended.GoalActive() || ended.GoalMode.EndReason != GoalEndExhausted || ended.GoalMode.EndedAt == nil || ended.GoalMode.PermissionSet {
		t.Fatalf("ended = %+v", ended.GoalMode)
	}
	if last := ended.Cards[len(ended.Cards)-1]; last.Kind != CardNotice || last.Title != NoticeGoalEnded {
		t.Fatalf("last card = %+v", last)
	}
	if p.GoalMode.EndedAt != nil || !p.GoalActive() {
		t.Fatal("EndGoal changed its input")
	}
	if quiet, _ := EndGoal(p, GoalEndFinished, later); len(quiet.Cards) != len(p.Cards) {
		t.Fatal("a finished goal left a notice")
	}
	if _, ok := EndGoal(ended, GoalEndDisabled, later); ok {
		t.Fatal("ended a goal that was not on")
	}
	// Turned on again after it ended: nothing of the old permission record.
	if again := StartGoal(ended, 0, later); again.GoalMode.PermissionSet || again.GoalMode.EndReason != "" {
		t.Fatalf("goal after an end = %+v", again.GoalMode)
	}
	if step := NextGoalStep(ended, later); step.Kind != GoalWait {
		t.Fatalf("step without goal mode = %+v", step)
	}
}

func TestNextGoalStepApprovesGates(t *testing.T) {
	p := StartGoal(planned(t, StagePlan, StageBuild, StagePR, StageMerge), 0, t0)
	step := NextGoalStep(p, t0)
	if step.Kind != GoalGate || step.Gate != GatePlan || step.Action != GateApprove || step.Push || step.CardID == "" {
		t.Fatalf("plan gate step = %+v", step)
	}
	p = GoalApplied(p, step, t0)
	if p.GoalMode.Decisions != 1 || p.GoalMode.Pushes != 0 {
		t.Fatalf("goal = %+v", p.GoalMode)
	}

	// G3: the PR draft.
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: &Verification{Passed: true}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{PR: &PRDraft{Title: "feat: x"}, Report: &Report{Summary: "draft"}}}, t0)
	if step = NextGoalStep(p, t0); step.Kind != GoalGate || step.Gate != GatePR || step.Action != GateApprove {
		t.Fatalf("pr gate step = %+v", step)
	}
	// Approved: the open turn is owed, and after it the probe is awaited.
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GatePR, Action: GateApprove}, t0)
	if step = NextGoalStep(p, t0.Add(time.Second)); step.Kind != GoalWait {
		t.Fatalf("step right after an approval = %+v", step)
	}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Report: &Report{Summary: "opened"}}}, t0)
	if step = NextGoalStep(p, later); step.Kind != GoalWait {
		t.Fatalf("step while the PR is awaited = %+v", step)
	}

	// G4 opens on a green probe; goal mode approves it.
	probe := PRProbe{Status: ProbeFound, Number: 7, State: PRStateOpen, HeadOID: "abc", Checks: []PRCheck{{Name: "ci", State: CheckPass}}}
	p, _, err := Apply(p, Event{Kind: EventPRProbe, Probe: &probe}, t0)
	if err != nil || p.OpenGate != GateMerge {
		t.Fatalf("probe: %v gate=%q current=%s", err, p.OpenGate, p.Current)
	}
	if step = NextGoalStep(p, t0); step.Kind != GoalGate || step.Gate != GateMerge || step.Action != GateApprove {
		t.Fatalf("merge gate step = %+v", step)
	}
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GateMerge, Action: GateApprove, CardID: step.CardID}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 5, Blocks: Blocks{Report: &Report{Summary: "merged"}}}, t0)
	merged := PRProbe{Status: ProbeFound, Number: 7, State: PRStateMerged, HeadOID: "abc"}
	p, _, _ = Apply(p, Event{Kind: EventPRProbe, Probe: &merged}, t0)
	if step = NextGoalStep(p, later); step.Kind != GoalEnd || step.Reason != GoalEndFinished {
		t.Fatalf("step on a finished pipeline = %+v (finished=%v)", step, Finished(p))
	}
}

// TestNextGoalStepRunsReleaseStage is TestNextGoalStepApprovesGates' plan
// but of a template with a stage after merge: the merge that ended
// goal mode there instead hands off to it, and only its own turn finishes
// the pipeline.
func TestNextGoalStepRunsReleaseStage(t *testing.T) {
	p := StartGoal(plannedFrom(t, shipTemplate(), StagePlan, StageBuild, StagePR, StageMerge, shipStage), 0, t0)
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: &Verification{Passed: true}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{PR: &PRDraft{Title: "feat: x"}, Report: &Report{Summary: "draft"}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GatePR, Action: GateApprove}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Report: &Report{Summary: "opened"}}}, t0)
	probe := PRProbe{Status: ProbeFound, Number: 7, State: PRStateOpen, HeadOID: "abc", Checks: []PRCheck{{Name: "ci", State: CheckPass}}}
	p, _, err := Apply(p, Event{Kind: EventPRProbe, Probe: &probe}, t0)
	if err != nil || p.OpenGate != GateMerge {
		t.Fatalf("probe: %v gate=%q", err, p.OpenGate)
	}
	step := NextGoalStep(p, t0)
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GateMerge, Action: GateApprove, CardID: step.CardID}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 5, Blocks: Blocks{Report: &Report{Summary: "merged"}}}, t0)

	merged := PRProbe{Status: ProbeFound, Number: 7, State: PRStateMerged, HeadOID: "abc"}
	p, act, err := Apply(p, Event{Kind: EventPRProbe, Probe: &merged}, t0)
	if err != nil || Finished(p) || p.Current != shipStage || !p.Active() ||
		act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "release stage") {
		t.Fatalf("merge should hand off to release: current=%s finished=%v act=%+v", p.Current, Finished(p), act)
	}
	if p.PendingTurn != act.Prompt {
		t.Fatalf("release turn not owed: pending=%q", p.PendingTurn)
	}
	if step := NextGoalStep(p, t0); step.Kind == GoalEnd {
		t.Fatalf("goal mode ended before the release stage ran: %+v", step)
	}

	// The release stage runs like any other build stage.
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 6, Blocks: Blocks{Report: &Report{Summary: "released", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 6, Verification: &Verification{Passed: true}}, t0)
	if !Finished(p) || !Releasable(p) {
		t.Fatalf("release stage should finish the pipeline: finished=%v releasable=%v", Finished(p), Releasable(p))
	}
	if step := NextGoalStep(p, later); step.Kind != GoalEnd || step.Reason != GoalEndFinished {
		t.Fatalf("step after release = %+v", step)
	}
}

func TestNextGoalStepTriagesBySeverity(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild, StageReview)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: &Verification{Passed: true}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{Findings: []Finding{
		{ID: "f1", Severity: "high", File: "a.go", Line: 1, Title: "crash"},
		{ID: "f2", Severity: " Low ", File: "a.go", Line: 2, Title: "nit"},
		{ID: "f3", File: "a.go", Line: 3, Title: "unlabelled"},
	}}}, t0)
	if p.OpenGate != GateTriage {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	var decisions []string
	var act Action
	for range 3 {
		step := NextGoalStep(p, t0)
		if step.Kind != GoalCard {
			t.Fatalf("triage step = %+v", step)
		}
		decisions = append(decisions, step.Decision)
		var err error
		if p, act, err = SetCardState(p, step.CardID, CardDecided, step.Decision, t0); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(decisions, ",") != "fix,dismiss,fix" {
		t.Fatalf("decisions = %v", decisions)
	}
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "crash") || strings.Contains(act.Prompt, "nit") || p.OpenGate != GateNone {
		t.Fatalf("after triage: %+v gate=%q", act, p.OpenGate)
	}
}

func TestNextGoalStepAnswersDecisions(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	p, act, _ := Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{
		Summary: "which one", Decisions: []Decision{{ID: "d1", Question: "A or B?", Options: []string{"A", "B"}}},
	}}}, t0)
	if act.Kind != ActionNone {
		t.Fatalf("a decision did not pause the loop: %+v", act)
	}
	step := NextGoalStep(p, t0)
	if step.Kind != GoalCard || step.Decision != GoalDecisionAnswer {
		t.Fatalf("decision step = %+v", step)
	}
	p, act, err := SetCardState(p, step.CardID, CardDecided, step.Decision, t0)
	if err != nil || act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "Decide yourself") {
		t.Fatalf("answer: %v %+v", err, act)
	}
	if g := Guidance(p); !strings.Contains(g, "Goal mode: nobody is watching") {
		t.Fatalf("guidance in goal mode:\n%s", g)
	}
	off, _ := EndGoal(p, GoalEndDisabled, t0)
	if g := Guidance(off); strings.Contains(g, "Goal mode") {
		t.Fatalf("guidance after goal mode ended:\n%s", g)
	}
}

func TestNextGoalStepFixesPRFindings(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild, StagePR, StagePRReview, StageMerge)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: &Verification{Passed: true}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{PR: &PRDraft{Title: "feat: x"}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventGate, Gate: GatePR, Action: GateApprove}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Report: &Report{Summary: "opened"}}}, t0)
	failing := PRProbe{Status: ProbeFound, Number: 7, State: PRStateOpen, HeadOID: "abc", Checks: []PRCheck{{Name: "ci", State: CheckFail}}}
	p, _, err := Apply(p, Event{Kind: EventPRProbe, Probe: &failing}, t0)
	if err != nil || p.Current != StagePRReview {
		t.Fatalf("probe: %v current=%s", err, p.Current)
	}
	step := NextGoalStep(p, t0)
	if step.Kind != GoalCard || step.Decision != FindingFix {
		t.Fatalf("pr finding step = %+v", step)
	}
	p, act, err := SetCardState(p, step.CardID, CardDecided, step.Decision, t0)
	if err != nil || act.Kind != ActionSendTurn || p.PRWait != PRWaitFix {
		t.Fatalf("fix: %v %+v wait=%q", err, act, p.PRWait)
	}
	if step = NextGoalStep(p, t0.Add(time.Second)); step.Kind != GoalWait {
		t.Fatalf("step while the fix turn is owed = %+v", step)
	}
}

func TestNextGoalStepRetriesBlockedGates(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	fail := &Verification{Results: []VerificationResult{{Command: "make test", ExitCode: 1, Excerpt: "boom"}}}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: fail}, t0)
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: Blocks{Report: &Report{Summary: "again", TasksDone: true}}}, t0)
	p, _, _ = Apply(p, Event{Kind: EventVerification, Turn: 3, Verification: fail}, t0)
	if p.OpenGate != GateBlocked {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	step := NextGoalStep(p, t0)
	if step.Kind != GoalGate || step.Gate != GateBlocked || step.Action != GateRetry || !step.Push || step.FailedTurn {
		t.Fatalf("blocked step = %+v", step)
	}
	applied := GoalApplied(p, step, t0)
	if applied.GoalMode.Pushes != 1 || applied.GoalMode.Decisions != 1 || applied.GoalMode.Failures != 0 {
		t.Fatalf("goal = %+v", applied.GoalMode)
	}

	// The budget spent: goal mode ends at the gate.
	spent := p.clone()
	spent.GoalMode.Pushes = spent.GoalMode.MaxPushes
	if step = NextGoalStep(spent, t0); step.Kind != GoalEnd || step.Reason != GoalEndExhausted {
		t.Fatalf("step with the budget spent = %+v", step)
	}

	// A closed pull request is somebody's decision.
	closed := p.clone()
	closed.Cards[closed.openGateCard()].Payload = []byte(`{"reason":"pr_closed"}`)
	if step = NextGoalStep(closed, t0); step.Kind != GoalEnd || step.Reason != GoalEndPRClosed {
		t.Fatalf("step on a closed PR = %+v", step)
	}

	// Stopped by the developer.
	stopped, _, err := Apply(p, Event{Kind: EventStop}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if step = NextGoalStep(stopped, later); step.Kind != GoalEnd || step.Reason != GoalEndStopped {
		t.Fatalf("step on a stopped pipeline = %+v", step)
	}
}

func TestNextGoalStepBacksOffAfterFailedTurns(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	failed, ok := FailTurn(p, "provider timeout", t0)
	if !ok {
		t.Fatal("no turn was owed")
	}
	if step := NextGoalStep(failed, t0.Add(10*time.Second)); step.Kind != GoalWait || !step.Until.Equal(t0.Add(goalRetryBase)) {
		t.Fatalf("step right after a failed turn = %+v", step)
	}
	step := NextGoalStep(failed, t0.Add(goalRetryBase))
	if step.Kind != GoalGate || step.Action != GateRetry || !step.Push || !step.FailedTurn {
		t.Fatalf("step after the backoff = %+v", step)
	}
	applied := GoalApplied(failed, step, t0)
	if applied.GoalMode.Failures != 1 || applied.GoalMode.Pushes != 1 {
		t.Fatalf("goal = %+v", applied.GoalMode)
	}
	if step = NextGoalStep(applied, t0.Add(goalRetryBase)); step.Kind != GoalWait {
		t.Fatalf("second failure retried without a longer wait: %+v", step)
	}
	if got := goalRetryDelay(0); got != goalRetryBase {
		t.Fatalf("delay(0) = %s", got)
	}
	if got := goalRetryDelay(2); got != 4*goalRetryBase {
		t.Fatalf("delay(2) = %s", got)
	}
	if got := goalRetryDelay(40); got != goalRetryMax {
		t.Fatalf("delay(40) = %s", got)
	}
	// A turn that completes ends the streak.
	retried, _, err := Apply(applied, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
	if err != nil {
		t.Fatal(err)
	}
	done, _, _ := Apply(retried, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "ok"}}}, t0)
	if done.GoalMode.Failures != 0 {
		t.Fatalf("failures after a completed turn = %d", done.GoalMode.Failures)
	}
}

func TestNextGoalStepPushesAStalledPipeline(t *testing.T) {
	// A turn is owed and nothing runs: the interrupted gate, then its retry.
	p := goalBuilding(t, StagePlan, StageBuild)
	if p.PendingTurn == "" {
		t.Fatal("no turn owed after the plan was approved")
	}
	if step := NextGoalStep(p, t0.Add(GoalStallGrace-time.Second)); step.Kind != GoalWait {
		t.Fatalf("step inside the grace = %+v", step)
	}
	if step := NextGoalStep(p, t0.Add(GoalStallGrace)); step.Kind != GoalInterrupt {
		t.Fatalf("step on an owed turn = %+v", step)
	}
	interrupted, ok := Interrupt(p, later)
	if !ok {
		t.Fatal("Interrupt found nothing")
	}
	if step := NextGoalStep(interrupted, later); step.Kind != GoalGate || step.Action != GateRetry || !step.Push {
		t.Fatalf("step at the interrupted gate = %+v", step)
	}

	// Nothing owed, nothing decided (a reply without its block, twice).
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2}, t0)
	p, act, _ := Apply(p, Event{Kind: EventTurnCompleted, Turn: 3}, t0)
	if act.Kind != ActionNone || p.PendingTurn != "" {
		t.Fatalf("after two replies without a block: %+v pending=%q", act, p.PendingTurn)
	}
	step := NextGoalStep(p, later)
	if step.Kind != GoalNudge || !step.Push || !strings.Contains(step.Prompt, "build stage") {
		t.Fatalf("step on an idle stage = %+v", step)
	}
	nudged := GoalApplied(p, step, later)
	if nudged.PendingTurn != step.Prompt || nudged.GoalMode.Pushes != 1 || nudged.GoalMode.Decisions != 0 {
		t.Fatalf("after the nudge: pending=%q goal=%+v", nudged.PendingTurn, nudged.GoalMode)
	}
	spent := p.clone()
	spent.GoalMode.Pushes = spent.GoalMode.MaxPushes
	if step = NextGoalStep(spent, later); step.Kind != GoalEnd || step.Reason != GoalEndExhausted {
		t.Fatalf("idle step with the budget spent = %+v", step)
	}

	// A pipeline nothing ran on yet is started with its goal, free of charge.
	fresh := StartGoal(New("s1", "ship it", t0), 0, t0)
	if step = NextGoalStep(fresh, later); step.Kind != GoalNudge || step.Push || step.Prompt != "ship it" {
		t.Fatalf("first step = %+v", step)
	}
	fresh.Kickoff = "ship it, in detail"
	if step = NextGoalStep(fresh, later); step.Prompt != "ship it, in detail" {
		t.Fatalf("first step with a kickoff = %+v", step)
	}
	if plain := GoalApplied(New("s1", "g", t0), step, later); plain.GoalMode != nil {
		t.Fatal("GoalApplied touched a pipeline without goal mode")
	}
}
