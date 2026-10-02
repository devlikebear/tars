package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// fakeProber answers probes from a script: answer(n) for the n-th probe.
type fakeProber struct {
	mu      sync.Mutex
	answer  func(n int) focuspipeline.PRProbe
	calls   int
	dirs    []string
	numbers []int
}

func (f *fakeProber) probe(_ context.Context, dir string, number int) focuspipeline.PRProbe {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.dirs = append(f.dirs, dir)
	f.numbers = append(f.numbers, number)
	answer := f.answer
	f.mu.Unlock()
	return answer(n)
}

func (f *fakeProber) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeProber) set(answer func(int) focuspipeline.PRProbe) {
	f.mu.Lock()
	f.answer = answer
	f.mu.Unlock()
}

type recordedFinish struct {
	mu      sync.Mutex
	actions []string
}

func (r *recordedFinish) finish(_ context.Context, _ string, action string) error {
	r.mu.Lock()
	r.actions = append(r.actions, action)
	r.mu.Unlock()
	return nil
}

func (r *recordedFinish) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.actions...)
}

func always(probe focuspipeline.PRProbe) func(int) focuspipeline.PRProbe {
	return func(int) focuspipeline.PRProbe { return probe }
}

func foundPR(state string, checks ...focuspipeline.PRCheck) focuspipeline.PRProbe {
	return focuspipeline.PRProbe{Status: focuspipeline.ProbeFound, Number: 7, URL: "https://example.test/pr/7", State: state, Checks: checks, HeadOID: "h1"}
}

// applyFocus applies events to a session's pipeline through the store.
func applyFocus(t *testing.T, store *session.Store, id string, events ...focuspipeline.Event) focuspipeline.Pipeline {
	t.Helper()
	p, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		for _, ev := range events {
			next, _, err := focuspipeline.Apply(p, ev, time.Now())
			if err != nil {
				return p, err
			}
			p = next
		}
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// openingPRSession moves a session's pipeline to the pr stage with G3
// approved: the PR is awaited.
func openingPRSession(t *testing.T, store *session.Store, id string) {
	t.Helper()
	prStageSession(t, store, id)
	applyFocus(t, store, id,
		focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 3, Blocks: focuspipeline.Blocks{PR: &focuspipeline.PRDraft{Title: "feat: x", Body: "b"}}},
		focuspipeline.Event{Kind: focuspipeline.EventGate, Gate: focuspipeline.GatePR, Action: focuspipeline.GateApprove},
		// The open turn ran.
		focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 4, Blocks: focuspipeline.Blocks{Report: &focuspipeline.Report{Summary: "opened"}}},
	)
}

// prStageSession moves a session's pipeline to the pr stage, round 1,
// with every stage planned.
func prStageSession(t *testing.T, store *session.Store, id string) {
	t.Helper()
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.Plan.Stages = append([]focuspipeline.StageID(nil), focuspipeline.StageOrder...)
		for i := range p.Stages {
			switch p.Stages[i].ID {
			case focuspipeline.StagePlan, focuspipeline.StageBuild, focuspipeline.StageReview:
				p.Stages[i].Status = focuspipeline.StatusDone
			case focuspipeline.StagePR:
				p.Stages[i].Status, p.Stages[i].Iteration, p.Stages[i].Limit = focuspipeline.StatusActive, 1, 3
			default:
				p.Stages[i].Status = focuspipeline.StatusPending
			}
		}
		p.Current = focuspipeline.StagePR
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testPRDriver(t *testing.T, answer func(int) focuspipeline.PRProbe) (*focusDriver, *session.Store, string, *fakeProber, *recordedFinish) {
	t.Helper()
	d, _, store, id := testFocusDriver(t, func(int, string) string { return focusReport("ok", false) }, &fakeVerifier{result: passAll})
	prober := &fakeProber{answer: answer}
	finished := &recordedFinish{}
	d.prPoll = 5 * time.Millisecond
	d.probe = prober.probe
	d.finishWorktree = finished.finish
	d.localBranch = func(context.Context, string) string { return "" }
	d.discardCheck = func(context.Context, string, string) string { return "" }
	if err := store.SetWorkDirs(id, []string{t.TempDir()}, ""); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := store.SetWorktree(id, &session.SessionWorktree{Path: wt, Dir: wt, Branch: "tars/session-" + id, SourceDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	openingPRSession(t, store, id)
	return d, store, id, prober, finished
}

// openThen answers an open PR first, then later(n) — a PR is found before
// it can be merged or closed (J1).
func openThen(later focuspipeline.PRProbe) func(int) focuspipeline.PRProbe {
	return func(n int) focuspipeline.PRProbe {
		if n == 1 {
			return foundPR(focuspipeline.PRStateOpen, focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckPending})
		}
		return later
	}
}

func findingCount(p focuspipeline.Pipeline) int {
	n := 0
	for _, c := range p.Cards {
		if c.Kind == focuspipeline.CardFinding {
			n++
		}
	}
	return n
}

func TestFocusPRPollFoundFailingCheckOnce(t *testing.T) {
	failing := focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckFail, StartedAt: "t1"}
	d, store, id, prober, _ := testPRDriver(t, always(foundPR(focuspipeline.PRStateOpen, failing)))
	d.watchPR(id)
	waitFor(t, "repeated polls", func() bool { return prober.count() >= 5 })
	p := pipelineOf(t, store, id)
	if p.Current != focuspipeline.StagePRReview || p.PR == nil || p.PR.Number != 7 {
		t.Fatalf("current %s pr %+v", p.Current, p.PR)
	}
	if n := findingCount(p); n != 1 {
		t.Fatalf("findings = %d", n)
	}
	if !d.polling(id) {
		t.Fatal("pr_review keeps polling")
	}
}

func TestFocusPRPollGreenOpensG4AndKeepsPolling(t *testing.T) {
	green := foundPR(focuspipeline.PRStateOpen, focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckPass})
	d, store, id, prober, finished := testPRDriver(t, always(green))
	d.watchPR(id)
	waitFor(t, "the merge gate", func() bool { return pipelineOf(t, store, id).OpenGate == focuspipeline.GateMerge })
	// #1087: G4 shows one head's facts, so the poller keeps watching it.
	calls := prober.count()
	waitFor(t, "probes while G4 is open", func() bool { return prober.count() >= calls+2 })
	if p := pipelineOf(t, store, id); p.OpenGate != focuspipeline.GateMerge || !d.polling(id) {
		t.Fatalf("gate %q polling %v", p.OpenGate, d.polling(id))
	}
	// Approving G4 probes once more (same head: approved), starts the
	// merge turn and the poll; MERGED ends the pipeline and discards the
	// worktree.
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	before := prober.count()
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/gates/merge", `{"action":"approve"}`, true); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	if prober.count() <= before {
		t.Fatal("approving G4 did not probe")
	}
	prober.set(always(foundPR(focuspipeline.PRStateMerged)))
	waitFor(t, "the pipeline to finish", func() bool { return focuspipeline.Finished(pipelineOf(t, store, id)) })
	waitDriverIdle(t, d, id)
	waitFor(t, "the worktree to be discarded", func() bool {
		seen := finished.seen()
		return len(seen) == 1 && seen[0] == "discard"
	})
}

// #1087: a push while G4 is open closes it and returns to pr_review, which
// reopens G4 on the new head once its checks pass.
func TestFocusPRPollMovedHeadClosesG4(t *testing.T) {
	pass := focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckPass}
	d, store, id, prober, _ := testPRDriver(t, always(foundPR(focuspipeline.PRStateOpen, pass)))
	d.watchPR(id)
	waitFor(t, "the merge gate", func() bool { return pipelineOf(t, store, id).OpenGate == focuspipeline.GateMerge })
	pending := foundPR(focuspipeline.PRStateOpen, focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckPending})
	pending.HeadOID = "h2"
	prober.set(always(pending))
	waitFor(t, "pr_review on the new head", func() bool {
		p := pipelineOf(t, store, id)
		return p.Current == focuspipeline.StagePRReview && p.OpenGate == focuspipeline.GateNone && p.PR.HeadOID == "h2"
	})
	green := foundPR(focuspipeline.PRStateOpen, pass)
	green.HeadOID = "h2"
	prober.set(always(green))
	waitFor(t, "G4 on the new head", func() bool {
		p := pipelineOf(t, store, id)
		return p.OpenGate == focuspipeline.GateMerge && p.PR.HeadOID == "h2"
	})
}

// #1087: approving G4 probes first; a moved head refuses with 409 and the
// current pipeline, and no merge turn runs.
func TestFocusG4ApproveRefusedWhenTheHeadMoved(t *testing.T) {
	d, store, id, prober, _ := testPRDriver(t, always(foundPR(focuspipeline.PRStateOpen, focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckPass})))
	var turns atomic.Int32
	d.runTurn = func(context.Context, string, string) error { turns.Add(1); return nil }
	// G4 opens from one probe, without a poller racing the approval.
	applyFocus(t, store, id, focuspipeline.Event{Kind: focuspipeline.EventPRProbe, Probe: func() *focuspipeline.PRProbe {
		p := prober.probe(context.Background(), "", 0)
		return &p
	}()})
	if p := pipelineOf(t, store, id); p.OpenGate != focuspipeline.GateMerge {
		t.Fatalf("gate = %q", p.OpenGate)
	}
	moved := foundPR(focuspipeline.PRStateOpen, focuspipeline.PRCheck{Name: "test", State: focuspipeline.CheckPending})
	moved.HeadOID = "h2"
	prober.set(always(moved))
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/gates/merge", `{"action":"approve"}`, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error    string                 `json:"error"`
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, "head moved") || resp.Pipeline.Current != focuspipeline.StagePRReview || resp.Pipeline.OpenGate != focuspipeline.GateNone {
		t.Fatalf("resp = %s", rec.Body.String())
	}
	if prober.numbers[len(prober.numbers)-1] != 7 {
		t.Fatalf("approve probed %v, want the pinned PR", prober.numbers)
	}
	waitDriverIdle(t, d, id)
	if n := turns.Load(); n != 0 {
		t.Fatalf("merge turns = %d", n)
	}
	if p := pipelineOf(t, store, id); p.PRWait != "" || p.PendingTurn != "" {
		t.Fatalf("wait %q pending %q", p.PRWait, p.PendingTurn)
	}
}

func TestFocusPRPollMergedDiscards(t *testing.T) {
	d, store, id, prober, finished := testPRDriver(t, openThen(foundPR(focuspipeline.PRStateMerged)))
	d.watchPR(id)
	waitFor(t, "the pipeline to finish", func() bool { return focuspipeline.Finished(pipelineOf(t, store, id)) })
	// Found once by branch, then probed by its pinned number.
	prober.mu.Lock()
	numbers := append([]int(nil), prober.numbers...)
	prober.mu.Unlock()
	if len(numbers) < 2 || numbers[0] != 0 || numbers[1] != 7 {
		t.Fatalf("probe numbers = %v", numbers)
	}
	waitFor(t, "the worktree end to be recorded", func() bool {
		end := pipelineOf(t, store, id).WorktreeEnd
		return end != nil && end.Action == "discard"
	})
	waitFor(t, "the worktree to be discarded", func() bool {
		seen := finished.seen()
		return len(seen) == 1 && seen[0] == "discard"
	})
	waitFor(t, "the poller to stop", func() bool { return !d.polling(id) })
	time.Sleep(20 * time.Millisecond)
	if seen := finished.seen(); len(seen) != 1 {
		t.Fatalf("finished twice: %v", seen)
	}
}

func TestFocusPRPollUnavailableOneNotice(t *testing.T) {
	d, store, id, prober, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeUnavailable, Error: "gh not found"}))
	d.watchPR(id)
	waitFor(t, "repeated polls", func() bool { return prober.count() >= 5 })
	notices := 0
	for _, c := range pipelineOf(t, store, id).Cards {
		if c.Kind == focuspipeline.CardNotice && c.Title == focuspipeline.NoticeGHUnavailable {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("notices = %d", notices)
	}
}

func TestFocusPRPollManualFinishKeepsTheWorktree(t *testing.T) {
	d, store, id, _, finished := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeUnavailable}))
	applyFocus(t, store, id,
		focuspipeline.Event{Kind: focuspipeline.EventAdvance, Stage: focuspipeline.StagePR},
		focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 5, Blocks: focuspipeline.Blocks{Report: &focuspipeline.Report{Summary: "checked"}, Findings: []focuspipeline.Finding{}}},
		focuspipeline.Event{Kind: focuspipeline.EventAdvance, Stage: focuspipeline.StagePRReview},
		focuspipeline.Event{Kind: focuspipeline.EventGate, Gate: focuspipeline.GateMerge, Action: focuspipeline.GateApprove},
		focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 6, Blocks: focuspipeline.Blocks{Report: &focuspipeline.Report{Summary: "merged"}}},
	)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/advance", `{"stage":"merge"}`, true); rec.Code != http.StatusOK {
		t.Fatalf("advance: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, "the worktree to be kept", func() bool {
		seen := finished.seen()
		return len(seen) == 1 && seen[0] == "keep"
	})
}

func TestFocusPRPollWaitsForTheTurn(t *testing.T) {
	d, _, id, prober, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	_, release, ok := d.cancels.Claim(id)
	if !ok {
		t.Fatal("claim")
	}
	d.watchPR(id)
	time.Sleep(40 * time.Millisecond)
	if prober.count() != 0 {
		t.Fatal("probed while a turn held the session")
	}
	release()
	waitFor(t, "a probe after the turn", func() bool { return prober.count() > 0 })
}

func TestFocusPRPollStopsOnStopAndClose(t *testing.T) {
	d, store, id, prober, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	d.watchPR(id)
	d.watchPR(id) // one poller per session
	waitFor(t, "a probe", func() bool { return prober.count() > 0 })
	applyFocus(t, store, id, focuspipeline.Event{Kind: focuspipeline.EventStop})
	waitFor(t, "the poller to stop", func() bool { return !d.polling(id) })

	d2, _, id2, prober2, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	d2.watchPR(id2)
	waitFor(t, "a probe", func() bool { return prober2.count() > 0 })
	d2.Close(context.Background())
	if d2.polling(id2) {
		t.Fatal("poller survived Close")
	}
}

func TestResumeFocusPRPolls(t *testing.T) {
	d, _, id, prober, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	if n := d.resumePRPolls(); n != 1 {
		t.Fatalf("resumed = %d", n)
	}
	waitFor(t, "a probe", func() bool { return prober.count() > 0 })
	if !d.polling(id) {
		t.Fatal("not polling")
	}
	prober.mu.Lock()
	dir := prober.dirs[0]
	prober.mu.Unlock()
	if dir == "" {
		t.Fatal("probed without the session folder")
	}
}

func TestFocusG3ApproveWithEdits(t *testing.T) {
	d, store, id, _, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	// Back to drafting (no PR awaited): a new draft opens G3.
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.PRWait, p.PRDraft = "", nil
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	applyFocus(t, store, id, focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 5, Blocks: focuspipeline.Blocks{PR: &focuspipeline.PRDraft{Title: "draft", Body: "b"}}})
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/gates/pr", `{"action":"approve","pr":{"title":"edited","body":"new body"}}`, true); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	p := pipelineOf(t, store, id)
	if p.PRDraft == nil || p.PRDraft.Title != "edited" || p.PRDraft.Body != "new body" || p.PRWait != focuspipeline.PRWaitOpen {
		t.Fatalf("draft %+v wait %q", p.PRDraft, p.PRWait)
	}
	// A second approve is stale: 409.
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/gates/pr", `{"action":"approve"}`, true); rec.Code != http.StatusConflict {
		t.Fatalf("stale approve: %d", rec.Code)
	}
}

func TestFocusPRPollOldMergedPRNeverFinishes(t *testing.T) {
	// J1: gh pr view by branch returns an old merged PR of a reused branch
	// name; it must not finish the pipeline nor discard the worktree.
	old := foundPR(focuspipeline.PRStateMerged)
	old.Number = 3
	d, store, id, prober, finished := testPRDriver(t, always(old))
	d.watchPR(id)
	waitFor(t, "repeated polls", func() bool { return prober.count() >= 4 })
	p := pipelineOf(t, store, id)
	if focuspipeline.Finished(p) || p.PR != nil || p.Current != focuspipeline.StagePR || p.PRProbes < 3 {
		t.Fatalf("finished %v pr %+v current %s probes %d", focuspipeline.Finished(p), p.PR, p.Current, p.PRProbes)
	}
	if seen := finished.seen(); len(seen) != 0 {
		t.Fatalf("worktree finished: %v", seen)
	}
}

func TestFocusPRPollMergedKeepsUnsafeWorktree(t *testing.T) {
	// J2: merged, but the worktree holds work the PR does not.
	d, store, id, _, finished := testPRDriver(t, openThen(foundPR(focuspipeline.PRStateMerged)))
	d.discardCheck = func(context.Context, string, string) string { return "the worktree has uncommitted changes" }
	d.watchPR(id)
	waitFor(t, "the worktree end", func() bool { return pipelineOf(t, store, id).WorktreeEnd != nil })
	end := pipelineOf(t, store, id).WorktreeEnd
	if end.Action != "keep" || end.Reason != "the worktree has uncommitted changes" {
		t.Fatalf("end = %+v", end)
	}
	if seen := finished.seen(); len(seen) != 1 || seen[0] != "keep" {
		t.Fatalf("finished = %v", seen)
	}
}

func TestFocusWorktreeLeftWhileATurnRuns(t *testing.T) {
	d, store, id, _, finished := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	_, release, ok := d.cancels.Claim(id)
	if !ok {
		t.Fatal("claim")
	}
	defer release()
	p := pipelineOf(t, store, id)
	if end := d.endWorktree(id, p); end.Action != "left" || len(finished.seen()) != 0 {
		t.Fatalf("end %+v finished %v", end, finished.seen())
	}
}

func TestFocusAdvanceRefusedWhileATurnRuns(t *testing.T) {
	// J2: the manual merge pass ends the worktree; never under a turn.
	d, store, id, _, finished := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeUnavailable}))
	_, release, ok := d.cancels.Claim(id)
	if !ok {
		t.Fatal("claim")
	}
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/advance", `{"stage":"pr"}`, true)
	if rec.Code != http.StatusConflict || pipelineOf(t, store, id).Current != focuspipeline.StagePR {
		t.Fatalf("advance while running: %d %s", rec.Code, rec.Body.String())
	}
	release()
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/advance", `{"stage":"pr"}`, true); rec.Code != http.StatusOK {
		t.Fatalf("advance after: %d", rec.Code)
	}
	if len(finished.seen()) != 0 {
		t.Fatal("finished a worktree on a pass that did not end the pipeline")
	}
}

func TestFocusWorktreeKeptWithoutADiscardCheck(t *testing.T) {
	// c23: a driver without bindPR keeps the worktree instead of panicking.
	d, store, id, _, finished := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}))
	d.discardCheck = nil
	d.localBranch = nil
	p := pipelineOf(t, store, id)
	p.PR = &focuspipeline.PRInfo{Number: 7, State: focuspipeline.PRStateMerged, HeadOID: "h"}
	end := d.endWorktree(id, p)
	if end.Action != "keep" || end.Reason != "the worktree could not be checked" || len(finished.seen()) != 1 || finished.seen()[0] != "keep" {
		t.Fatalf("end %+v finished %v", end, finished.seen())
	}
}

func TestFocusAdvanceRefusedWhileATurnRunsEvenUnreadable(t *testing.T) {
	// c24: the refusal no longer depends on re-reading the pipeline.
	d, store, id, _, _ := testPRDriver(t, always(focuspipeline.PRProbe{Status: focuspipeline.ProbeUnavailable}))
	_, release, ok := d.cancels.Claim(id)
	if !ok {
		t.Fatal("claim")
	}
	defer release()
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/advance", `{"stage":"pr"}`, true)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "a turn is running") {
		t.Fatalf("advance: %d %s", rec.Code, rec.Body.String())
	}
	if pipelineOf(t, store, id).Current != focuspipeline.StagePR {
		t.Fatal("passed under a running turn")
	}
}
