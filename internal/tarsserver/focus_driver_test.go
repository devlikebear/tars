package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func focusReport(summary string, tasksDone bool) string {
	done := "false"
	if tasksDone {
		done = "true"
	}
	return "ok\n<focus-report>{\"summary\":\"" + summary + "\",\"tasks_done\":" + done + "}</focus-report>"
}

// buildingFocusSession makes a session whose plan is approved: build is
// the current stage.
func buildingFocusSession(t *testing.T, store *session.Store) session.Session {
	t.Helper()
	sess := plannedFocusSession(t, store)
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, _, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventGate, Gate: focuspipeline.GatePlan, Action: focuspipeline.GateApprove}, time.Now())
		return next, err
	}); err != nil {
		t.Fatal(err)
	}
	return sess
}

// fakeFocusTurns plays a driver's turns: each prompt goes through the real
// guidance, transcript and post-turn hook, answered by reply.
type fakeFocusTurns struct {
	t      *testing.T
	store  *session.Store
	driver *focusDriver
	reply  func(n int, prompt string) string
	// block makes turn n wait for its context, or for gate when set.
	block func(n int) bool
	gate  chan struct{}
	// fail makes turn n return an error, as a provider failure would.
	fail func(n int) error

	mu      sync.Mutex
	prompts []string
	active  int32
	maxSeen int32
}

func (f *fakeFocusTurns) run(ctx context.Context, sessionID, prompt string) error {
	now := atomic.AddInt32(&f.active, 1)
	defer atomic.AddInt32(&f.active, -1)
	for {
		seen := atomic.LoadInt32(&f.maxSeen)
		if now <= seen || atomic.CompareAndSwapInt32(&f.maxSeen, seen, now) {
			break
		}
	}
	f.mu.Lock()
	f.prompts = append(f.prompts, prompt)
	n := len(f.prompts)
	f.mu.Unlock()
	if f.fail != nil {
		if err := f.fail(n); err != nil {
			return err
		}
	}
	if f.block != nil && f.block(n) {
		if f.gate == nil {
			<-ctx.Done()
			return ctx.Err()
		}
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	message, mark := appendFocusGuidanceAt(prompt, f.store, sessionID, focusQuestionGateFrom(ctx), zerolog.Nop())
	transcript := f.store.TranscriptPath(sessionID)
	if err := session.AppendMessage(transcript, session.Message{Role: "user", Content: message, Timestamp: time.Now()}); err != nil {
		return err
	}
	reply := f.reply(n, prompt)
	if err := session.AppendMessage(transcript, session.Message{Role: "assistant", Content: reply, Timestamp: time.Now()}); err != nil {
		return err
	}
	if _, act, ok := focusAfterTurn(f.store, sessionID, transcript, reply, mark, time.Now(), zerolog.Nop()); ok {
		f.driver.afterTurn(ctx, sessionID, act, "")
	}
	return nil
}

func (f *fakeFocusTurns) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

// fakeVerifier answers each command run with result(n, command).
type fakeVerifier struct {
	result func(n int, command string) focuspipeline.VerificationResult
	block  bool
	calls  atomic.Int32
}

func (v *fakeVerifier) verify(ctx context.Context, _ string, command string) (focuspipeline.VerificationResult, error) {
	n := int(v.calls.Add(1))
	if v.block {
		<-ctx.Done()
		return focuspipeline.VerificationResult{}, ctx.Err()
	}
	return v.result(n, command), nil
}

func passAll(int, string) focuspipeline.VerificationResult {
	return focuspipeline.VerificationResult{Command: "make test", Passed: true}
}

func testFocusDriver(t *testing.T, reply func(int, string) string, verifier *fakeVerifier) (*focusDriver, *fakeFocusTurns, *session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	sess := buildingFocusSession(t, store)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.idlePoll = 5 * time.Millisecond
	d.sessions = store
	d.feeds = newChatTurnFeeds()
	d.activity = newChatActivity(store, nil)
	d.cancels = newChatCancelRegistry()
	turns := &fakeFocusTurns{t: t, store: store, driver: d, reply: reply}
	d.runTurn = turns.run
	d.verify = verifier.verify
	return d, turns, store, sess.ID
}

func waitDriverIdle(t *testing.T, d *focusDriver, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for d.running(sessionID) {
		if time.Now().After(deadline) {
			t.Fatal("driver run never ended")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func pipelineOf(t *testing.T, store *session.Store, id string) focuspipeline.Pipeline {
	t.Helper()
	p, ok, err := focusStoreFor(store).Get(id)
	if err != nil || !ok {
		t.Fatalf("pipeline: %v %v", ok, err)
	}
	return p
}

func sendTurn(prompt string) focuspipeline.Action {
	return focuspipeline.Action{Kind: focuspipeline.ActionSendTurn, Prompt: prompt}
}

func TestFocusDriverPassAdvancesTheStage(t *testing.T) {
	d, turns, store, id := testFocusDriver(t, func(int, string) string { return focusReport("all done", true) }, &fakeVerifier{result: passAll})
	d.start(id, sendTurn("Plan approved. Start the build stage with task 1."), "")
	waitDriverIdle(t, d, id)

	p := pipelineOf(t, store, id)
	// The fixture plan's stages are plan, build, pr, merge.
	if p.Current != focuspipeline.StagePR {
		t.Fatalf("current = %s", p.Current)
	}
	prompts := turns.seen()
	if len(prompts) != 2 || !strings.Contains(prompts[1], "Start the pr stage") {
		t.Fatalf("prompts = %q", prompts)
	}
}

func TestFocusDriverFailureRetriesWithTheExcerpt(t *testing.T) {
	verifier := &fakeVerifier{result: func(n int, command string) focuspipeline.VerificationResult {
		if n == 1 {
			return focuspipeline.VerificationResult{Command: command, ExitCode: 1, Excerpt: "--- FAIL: TestGreet"}
		}
		return focuspipeline.VerificationResult{Command: command, Passed: true}
	}}
	d, turns, store, id := testFocusDriver(t, func(int, string) string { return focusReport("done", true) }, verifier)
	d.start(id, sendTurn("Plan approved. Start the build stage with task 1."), "")
	waitDriverIdle(t, d, id)

	prompts := turns.seen()
	if len(prompts) != 3 || !strings.Contains(prompts[1], "--- FAIL: TestGreet") || !strings.Contains(prompts[2], "pr stage") {
		t.Fatalf("prompts = %q", prompts)
	}
	p := pipelineOf(t, store, id)
	var failure *focuspipeline.Card
	for i := range p.Cards {
		if p.Cards[i].Kind == focuspipeline.CardFailure {
			failure = &p.Cards[i]
		}
	}
	if failure == nil || failure.Turn != 1 || p.Current != focuspipeline.StagePR {
		t.Fatalf("pipeline = %+v", p)
	}
}

func TestFocusDriverBlocksOnRepeatedFailure(t *testing.T) {
	verifier := &fakeVerifier{result: func(_ int, command string) focuspipeline.VerificationResult {
		return focuspipeline.VerificationResult{Command: command, ExitCode: 2, Excerpt: "same failure"}
	}}
	d, turns, store, id := testFocusDriver(t, func(int, string) string { return focusReport("tried", false) }, verifier)
	d.start(id, sendTurn("go"), "")
	waitDriverIdle(t, d, id)

	p := pipelineOf(t, store, id)
	if p.OpenGate != focuspipeline.GateBlocked || len(turns.seen()) != 2 {
		t.Fatalf("gate = %q prompts = %d", p.OpenGate, len(turns.seen()))
	}
}

func TestFocusDriverBlocksAtTheLimit(t *testing.T) {
	verifier := &fakeVerifier{result: func(n int, command string) focuspipeline.VerificationResult {
		return focuspipeline.VerificationResult{Command: command, ExitCode: 2, Excerpt: strings.Repeat("x", n)}
	}}
	d, turns, store, id := testFocusDriver(t, func(int, string) string { return focusReport("tried", false) }, verifier)
	d.start(id, sendTurn("go"), "")
	waitDriverIdle(t, d, id)

	p := pipelineOf(t, store, id)
	build, _ := p.Stage(focuspipeline.StageBuild)
	if p.OpenGate != focuspipeline.GateBlocked || build.Iteration != build.Limit || len(turns.seen()) != build.Limit {
		t.Fatalf("gate = %q build = %+v prompts = %d", p.OpenGate, build, len(turns.seen()))
	}
}

func TestFocusDriverCancel(t *testing.T) {
	t.Run("during a turn", func(t *testing.T) {
		d, turns, store, id := testFocusDriver(t, func(int, string) string { return focusReport("x", false) }, &fakeVerifier{result: passAll})
		turns.block = func(int) bool { return true }
		d.start(id, sendTurn("go"), "")
		waitFor(t, "the turn", func() bool { return len(turns.seen()) == 1 })
		if !d.cancel(id) {
			t.Fatal("no run to cancel")
		}
		if d.running(id) || len(turns.seen()) != 1 {
			t.Fatalf("run still going: prompts %d", len(turns.seen()))
		}
		if p := pipelineOf(t, store, id); p.Current != focuspipeline.StageBuild || len(p.Cards) != 1 {
			t.Fatalf("a cancelled run changes nothing: %+v", p)
		}
	})
	t.Run("during verification via the chat cancel registry", func(t *testing.T) {
		verifier := &fakeVerifier{block: true}
		d, turns, store, id := testFocusDriver(t, func(int, string) string { return focusReport("x", true) }, verifier)
		d.start(id, sendTurn("go"), "")
		waitFor(t, "verification", func() bool { return verifier.calls.Load() == 1 })
		running := d.activity.snapshot().Running
		if len(running) != 1 || running[0].SessionID != id || d.feeds.get(id) == nil {
			t.Fatal("verification shows as a running turn with a feed")
		}
		if _, ok := d.cancels.Cancel(id); !ok { // what POST /v1/chat/cancel does first
			t.Fatal("verification is not registered for cancel")
		}
		waitDriverIdle(t, d, id)
		p := pipelineOf(t, store, id)
		if p.Current != focuspipeline.StageBuild || !p.AwaitingVerification || len(turns.seen()) != 1 {
			t.Fatalf("pipeline = %+v", p)
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		d, turns, _, id := testFocusDriver(t, func(int, string) string { return "" }, &fakeVerifier{result: passAll})
		turns.block = func(int) bool { return true }
		d.start(id, sendTurn("go"), "")
		waitFor(t, "the turn", func() bool { return len(turns.seen()) == 1 })
		d.Close(context.Background())
		if d.running(id) {
			t.Fatal("Close leaves a run")
		}
		d.start(id, sendTurn("again"), "")
		if d.running(id) {
			t.Fatal("a closed driver starts nothing")
		}
	})
}

// asking pauses the loop after each turn (a decision), so queued actions
// run strictly in order.
func asking(int, string) string {
	return "ok\n<focus-report>{\"summary\":\"q\",\"decisions\":[{\"id\":\"d1\",\"question\":\"which?\",\"options\":[\"a\"]}]}</focus-report>"
}

func TestFocusDriverOneRunPerSession(t *testing.T) {
	d, turns, _, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	turns.gate = make(chan struct{})
	turns.block = func(n int) bool { return n == 1 }
	d.start(id, sendTurn("first"), "")
	waitFor(t, "first turn", func() bool { return len(turns.seen()) == 1 })
	// Actions arriving while the run is busy join it; they never replace
	// what it owes and never run beside it.
	d.start(id, sendTurn("second"), "")
	d.start(id, sendTurn("third"), "")
	close(turns.gate)
	waitDriverIdle(t, d, id)
	if max := atomic.LoadInt32(&turns.maxSeen); max != 1 {
		t.Fatalf("%d turns ran at once", max)
	}
	if prompts := turns.seen(); len(prompts) != 3 || prompts[0] != "first" || prompts[1] != "second" || prompts[2] != "third" {
		t.Fatalf("prompts = %q", prompts)
	}
}

// f3: a decision answered while a person's turn runs is still sent after
// that turn's own hook queues its verification.
func TestFocusDriverKeepsAnOwedTurnWhenAPersonsTurnEnds(t *testing.T) {
	d, turns, _, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	_, release, _ := d.cancels.Claim(id) // the person's turn runs
	d.start(id, sendTurn("Which flag? → --b"), "")
	d.afterTurn(context.Background(), id, focuspipeline.Action{Kind: focuspipeline.ActionRunVerification}, "")
	release()
	waitDriverIdle(t, d, id)
	if prompts := turns.seen(); len(prompts) != 1 || prompts[0] != "Which flag? → --b" {
		t.Fatalf("the owed answer was dropped: %q", prompts)
	}
}

// R3: a pipeline stopped while a step waited gets nothing more.
func TestFocusDriverSkipsStepsOfAStoppedPipeline(t *testing.T) {
	verifier := &fakeVerifier{result: passAll}
	d, turns, store, id := testFocusDriver(t, asking, verifier)
	_, release, _ := d.cancels.Claim(id)
	d.start(id, sendTurn("fix it"), "")
	d.start(id, focuspipeline.Action{Kind: focuspipeline.ActionRunVerification}, "")
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.AwaitingVerification = true
		next, _, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventStop}, time.Now())
		return next, err
	}); err != nil {
		t.Fatal(err)
	}
	release()
	waitDriverIdle(t, d, id)
	if len(turns.seen()) != 0 || verifier.calls.Load() != 0 {
		t.Fatalf("a stopped pipeline got turns %q and %d verifications", turns.seen(), verifier.calls.Load())
	}
	if p := pipelineOf(t, store, id); p.AwaitingVerification || p.PendingTurn != "" {
		t.Fatalf("stop left work pending: %+v", p)
	}
}

// R1: a step that finds its session claimed (a person's turn in its
// prepare window) waits and tries again instead of running beside it.
func TestFocusDriverRetriesWhenTheSessionIsClaimed(t *testing.T) {
	d, turns, _, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	calls := 0
	inner := d.runTurn
	d.runTurn = func(ctx context.Context, sessionID, message string) error {
		calls++
		if calls == 1 {
			return errChatTurnBusy
		}
		return inner(ctx, sessionID, message)
	}
	d.start(id, sendTurn("go"), "")
	waitDriverIdle(t, d, id)
	if calls != 2 || len(turns.seen()) != 1 {
		t.Fatalf("calls = %d prompts = %q", calls, turns.seen())
	}
}

// f5: a failed server turn raises the resumable blocked gate.
func TestFocusDriverFailedTurnRaisesTheGate(t *testing.T) {
	d, turns, store, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	turns.fail = func(int) error { return errors.New("cli timed out: no output for 15m0s") }
	// The owed turn, as an approval leaves it.
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.PendingTurn = "Plan approved. Start the build stage with task 1."
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	d.start(id, sendTurn("Plan approved. Start the build stage with task 1."), "")
	waitDriverIdle(t, d, id)
	p := pipelineOf(t, store, id)
	card := p.Cards[len(p.Cards)-1]
	if p.OpenGate != focuspipeline.GateBlocked || card.Title != focuspipeline.TurnFailedTitle || !strings.Contains(string(card.Payload), "no output for 15m0s") {
		t.Fatalf("gate = %q card = %+v", p.OpenGate, card)
	}
}

// R7/f9: Close returns by the shutdown deadline even when a turn ignores
// its cancel.
func TestFocusDriverCloseIsBounded(t *testing.T) {
	d, _, _, id := testFocusDriver(t, asking, &fakeVerifier{result: passAll})
	stuck := make(chan struct{})
	defer close(stuck)
	d.runTurn = func(context.Context, string, string) error { <-stuck; return nil }
	d.start(id, sendTurn("go"), "")
	waitFor(t, "the run", func() bool { return d.running(id) })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	d.Close(ctx)
	if time.Since(start) > 2*time.Second {
		t.Fatal("Close ignored the shutdown deadline")
	}
}

// f4: a pipeline the last server left mid-step waits at the interrupted
// gate; retry resumes the cut-off step.
func TestInterruptFocusPipelinesAtStartup(t *testing.T) {
	store := session.NewStore(t.TempDir())
	verifying := buildingFocusSession(t, store)
	if _, _, err := focusStoreFor(store).Update(verifying.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, _, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 2, Blocks: focuspipeline.ParseBlocks(focusReport("t1", false))}, time.Now())
		return next, err
	}); err != nil {
		t.Fatal(err)
	}
	idle := plannedFocusSession(t, store) // its gate is open: nothing was cut off
	if n := interruptFocusPipelines(store, time.Now(), zerolog.Nop()); n != 1 {
		t.Fatalf("interrupted = %d", n)
	}
	p := pipelineOf(t, store, verifying.ID)
	if p.OpenGate != focuspipeline.GateBlocked || p.AwaitingVerification || p.Cards[len(p.Cards)-1].Title != focuspipeline.InterruptedTitle {
		t.Fatalf("pipeline = %+v", p)
	}
	if q := pipelineOf(t, store, idle.ID); q.OpenGate != focuspipeline.GatePlan {
		t.Fatalf("idle pipeline changed: %q", q.OpenGate)
	}
}

func TestFocusDriverWaitsForARunningTurn(t *testing.T) {
	d, turns, _, id := testFocusDriver(t, func(int, string) string { return focusReport("x", false) }, &fakeVerifier{result: passAll})
	turns.block = func(int) bool { return true }
	_, unregister, _ := d.cancels.Claim(id) // a person's turn is still finishing
	d.start(id, sendTurn("go"), "")
	time.Sleep(30 * time.Millisecond)
	if len(turns.seen()) != 0 {
		t.Fatal("the driver started a turn while another ran")
	}
	unregister()
	waitFor(t, "the driver's turn", func() bool { return len(turns.seen()) == 1 })
	d.cancel(id)
}

func TestChatCancelRegistryClaims(t *testing.T) {
	r := newChatCancelRegistry()
	first, release, ok := r.Claim("s")
	if !ok || !r.Running("s") {
		t.Fatal("first claim")
	}
	if _, _, ok := r.Claim("s"); ok {
		t.Fatal("a second turn on the session must be refused")
	}
	// A cancel before the turn has its context fires once it has one, and
	// the claim stays until the turn releases it.
	ended, cancelled := r.Cancel("s")
	if !cancelled {
		t.Fatal("cancel found no turn")
	}
	if !r.Running("s") {
		t.Fatal("the claim stays until the cancelled turn ends")
	}
	fired := false
	first.setCancel(func() { fired = true })
	if !fired {
		t.Fatal("an early cancel fires when the cancel func arrives")
	}
	release()
	if r.Running("s") {
		t.Fatal("released")
	}
	select {
	case <-ended:
	default:
		t.Fatal("a cancel's ended channel closes when the turn releases the session")
	}
	_, again, ok := r.Claim("s")
	if !ok {
		t.Fatal("a released session can be claimed")
	}
	release() // an old release never ends a newer claim
	if !r.Running("s") {
		t.Fatal("old release removed a newer claim")
	}
	again()
	if _, _, ok := r.Claim(""); !ok {
		t.Fatal("an empty id claims nothing")
	}
	var nilRegistry *chatCancelRegistry
	if _, nilCancelled := nilRegistry.Cancel("s"); nilCancelled {
		t.Fatal("nil registry cancels nothing")
	}
	if _, _, ok := nilRegistry.Claim("s"); !ok {
		t.Fatal("nil registry")
	}
}

// focusChatHarness is the chat handler with a focus driver whose verifier
// is fake: turns are real (mock LLM).
func focusChatHarness(t *testing.T, client llm.Client, verifier *fakeVerifier) (http.Handler, *focusDriver, *session.Store) {
	t.Helper()
	_, store, root := testChatDeps(t, client)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.idlePoll = 5 * time.Millisecond
	tooling := defaultChatToolingOptions()
	tooling.Focus = d
	h := newChatAPIHandlerWithRuntimeConfig(root, store, client, nil, zerolog.Nop(), 2, nil, "", tooling)
	d.verify = verifier.verify
	return h, d, store
}

func TestFocusHumanTurnStartsTheDriver(t *testing.T) {
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: focusReport("done", true)}}}
	h, d, store := focusChatHarness(t, client, &fakeVerifier{result: passAll})
	sess := buildingFocusSession(t, store)

	rec := focusRequest(t, h, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"build it"}`, false)
	if !strings.Contains(rec.Body.String(), `"type":"pipeline"`) {
		t.Fatalf("no pipeline event: %s", rec.Body.String())
	}
	waitFor(t, "the server turn", func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return client.callCount == 2
	})
	waitDriverIdle(t, d, sess.ID)
	client.mu.Lock()
	second := client.seenMessages[1][len(client.seenMessages[1])-1].Content
	client.mu.Unlock()
	if !strings.Contains(second, "Start the pr stage") || !strings.Contains(second, "current stage: pr") {
		t.Fatalf("server turn = %q", second)
	}
	if p := pipelineOf(t, store, sess.ID); p.Current != focuspipeline.StagePR {
		t.Fatalf("current = %s", p.Current)
	}
}

func TestFocusHumanTurnPreemptsTheDriver(t *testing.T) {
	verifier := &fakeVerifier{block: true}
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: focusReport("done", true)}}}
	h, d, store := focusChatHarness(t, client, verifier)
	sess := buildingFocusSession(t, store)

	focusRequest(t, h, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"build it"}`, false)
	waitFor(t, "verification", func() bool { return verifier.calls.Load() == 1 })

	// A person types while verification runs: it stops, the turn runs, and
	// that turn's hook starts verification again.
	focusRequest(t, h, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"also rename it"}`, false)
	waitFor(t, "verification after the person's turn", func() bool { return verifier.calls.Load() == 2 })
	client.mu.Lock()
	calls := client.callCount
	client.mu.Unlock()
	if calls != 2 {
		t.Fatalf("llm calls = %d", calls)
	}
	d.cancel(sess.ID)
}

func TestFocusChatCancelStopsTheDriver(t *testing.T) {
	verifier := &fakeVerifier{block: true}
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: focusReport("done", true)}}}
	h, d, store := focusChatHarness(t, client, verifier)
	sess := buildingFocusSession(t, store)
	focusRequest(t, h, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"build it"}`, false)
	waitFor(t, "verification", func() bool { return verifier.calls.Load() == 1 })

	rec := focusRequest(t, h, http.MethodPost, "/v1/chat/cancel?session_id="+sess.ID, "", false)
	if rec.Code != http.StatusOK || d.running(sess.ID) {
		t.Fatalf("cancel = %d running = %v", rec.Code, d.running(sess.ID))
	}
}

func TestFocusGateStartsTheNextTurnServerSide(t *testing.T) {
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: focusReport("started", false)}}}
	verifier := &fakeVerifier{block: true}
	_, d, store := focusChatHarness(t, client, verifier)
	api := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	sess := plannedFocusSession(t, store)

	rec := approvePlanVia(t, api, sess.ID)
	var resp struct {
		NextPrompt string `json:"next_prompt"`
	}
	decodeInto(t, rec, &resp)
	if rec.Code != http.StatusOK || !strings.Contains(resp.NextPrompt, "Plan approved") {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, "the build turn", func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return client.callCount == 1
	})
	client.mu.Lock()
	first := client.seenMessages[0][len(client.seenMessages[0])-1].Content
	client.mu.Unlock()
	if !strings.HasPrefix(first, resp.NextPrompt) || !strings.Contains(first, "current stage: build") {
		t.Fatalf("server turn = %q", first)
	}
	// The build turn reported: verification runs next, on the server.
	waitFor(t, "verification", func() bool { return verifier.calls.Load() == 1 })

	// Stop ends the run.
	if rec := focusRequest(t, api, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/stop", "", false); rec.Code != http.StatusOK {
		t.Fatalf("stop = %d %s", rec.Code, rec.Body.String())
	}
	if d.running(sess.ID) {
		t.Fatal("stop leaves the driver running")
	}
}
