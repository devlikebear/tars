package tarsserver

import (
	"context"
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
	// block makes turn n wait for its context.
	block func(n int) bool

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
	if f.block != nil && f.block(n) {
		<-ctx.Done()
		return ctx.Err()
	}
	message, mark := appendFocusGuidance(prompt, f.store, sessionID, zerolog.Nop())
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
	t.Cleanup(d.Close)
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
		if !d.cancels.Cancel(id) { // what POST /v1/chat/cancel does first
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
		d.Close()
		if d.running(id) {
			t.Fatal("Close leaves a run")
		}
		d.start(id, sendTurn("again"), "")
		if d.running(id) {
			t.Fatal("a closed driver starts nothing")
		}
	})
}

func TestFocusDriverOneRunPerSession(t *testing.T) {
	d, turns, _, id := testFocusDriver(t, func(int, string) string { return focusReport("x", false) }, &fakeVerifier{result: passAll})
	turns.block = func(n int) bool { return n < 3 }
	d.start(id, sendTurn("first"), "")
	waitFor(t, "first turn", func() bool { return len(turns.seen()) == 1 })
	d.start(id, sendTurn("second"), "")
	waitFor(t, "second turn", func() bool { return len(turns.seen()) == 2 })
	d.start(id, sendTurn("third"), "")
	waitFor(t, "third turn", func() bool { return len(turns.seen()) >= 3 })
	d.cancel(id)
	if max := atomic.LoadInt32(&turns.maxSeen); max != 1 {
		t.Fatalf("%d turns ran at once", max)
	}
	if prompts := turns.seen(); prompts[0] != "first" || prompts[1] != "second" || prompts[2] != "third" {
		t.Fatalf("prompts = %q", prompts)
	}
}

func TestFocusDriverWaitsForARunningTurn(t *testing.T) {
	d, turns, _, id := testFocusDriver(t, func(int, string) string { return focusReport("x", false) }, &fakeVerifier{result: passAll})
	turns.block = func(int) bool { return true }
	unregister := d.cancels.Register(id, func() {}) // a person's turn is still finishing
	d.start(id, sendTurn("go"), "")
	time.Sleep(30 * time.Millisecond)
	if len(turns.seen()) != 0 {
		t.Fatal("the driver started a turn while another ran")
	}
	unregister()
	waitFor(t, "the driver's turn", func() bool { return len(turns.seen()) == 1 })
	d.cancel(id)
}

func TestChatCancelRegistryUnregistersOnlyItsOwnEntry(t *testing.T) {
	r := newChatCancelRegistry()
	first := r.Register("s", func() {})
	second := r.Register("s", func() {})
	first()
	if !r.Running("s") {
		t.Fatal("an older registration removed a newer one")
	}
	second()
	if r.Running("s") {
		t.Fatal("registration not removed")
	}
}

// focusChatHarness is the chat handler with a focus driver whose verifier
// is fake: turns are real (mock LLM).
func focusChatHarness(t *testing.T, client llm.Client, verifier *fakeVerifier) (http.Handler, *focusDriver, *session.Store) {
	t.Helper()
	_, store, root := testChatDeps(t, client)
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(d.Close)
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
