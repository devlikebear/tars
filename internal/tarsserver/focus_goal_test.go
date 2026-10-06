package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

const focusGoalPlanReply = "Plan below.\n<focus-plan>" +
	`{"goal":"g","tasks":[{"title":"t1","done":"d1"}],"stages":["plan","build"],"verify":["make test"]}` +
	"</focus-plan>"

// goalDriver is a driver whose goal watcher ticks fast and whose clock
// jumps an hour on every reading, so a pipeline counts as stalled (and a
// failed turn's backoff as over) the moment nothing runs on it.
func goalDriver(t *testing.T, reply func(p focuspipeline.Pipeline, n int, prompt string) string, verifier *fakeVerifier) (*focusDriver, *fakeFocusTurns, *session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	sess := focusSession(t, store, "ship it")
	d := newFocusDriver(zerolog.Nop())
	t.Cleanup(func() { d.Close(context.Background()) })
	d.idlePoll = 5 * time.Millisecond
	d.goalTick = 5 * time.Millisecond
	var hours atomic.Int64
	d.now = func() time.Time { return time.Now().Add(time.Duration(hours.Add(1)) * time.Hour) }
	d.sessions = store
	d.feeds = newChatTurnFeeds()
	d.activity = newChatActivity(store, nil)
	d.cancels = newChatCancelRegistry()
	turns := &fakeFocusTurns{t: t, store: store, driver: d}
	turns.reply = func(n int, prompt string) string { return reply(pipelineOf(t, store, sess.ID), n, prompt) }
	d.runTurn = turns.run
	d.verify = verifier.verify
	return d, turns, store, sess.ID
}

func startGoal(t *testing.T, d *focusDriver, store *session.Store, id string, maxPushes int) {
	t.Helper()
	p, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.StartGoal(p, maxPushes, time.Now()), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	d.goalStarted(id, p)
}

func stageReply(p focuspipeline.Pipeline, _ int, _ string) string {
	if p.Current == focuspipeline.StagePlan {
		return focusGoalPlanReply
	}
	return focusReport("done", true)
}

func TestFocusGoalRunsToTheEnd(t *testing.T) {
	d, turns, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	var mu sync.Mutex
	var audited []string
	var notices []notificationEvent
	d.audit = func(e ops.AutomationAuditEntry) {
		mu.Lock()
		audited = append(audited, e.Result)
		mu.Unlock()
	}
	d.notify = func(_ context.Context, e notificationEvent) {
		mu.Lock()
		notices = append(notices, e)
		mu.Unlock()
	}
	if err := store.SetPermissionMode(id, chatPermissionModeManual); err != nil {
		t.Fatal(err)
	}
	startGoal(t, d, store, id, 0)
	if sess, _ := store.Get(id); sess.PermissionMode != chatPermissionModeAuto {
		t.Fatalf("permission mode in goal mode = %q", sess.PermissionMode)
	}

	waitFor(t, "goal mode to end", func() bool { return !pipelineOf(t, store, id).GoalActive() })
	p := pipelineOf(t, store, id)
	if !focuspipeline.Finished(p) || p.GoalMode.EndReason != focuspipeline.GoalEndFinished || p.GoalMode.Pushes != 0 || p.GoalMode.Decisions != 1 {
		t.Fatalf("finished=%v goal=%+v", focuspipeline.Finished(p), p.GoalMode)
	}
	// Nobody sent anything: the goal itself was the first turn, the plan
	// was approved, the build ran.
	prompts := turns.seen()
	if len(prompts) != 2 || prompts[0] != "ship it" || !strings.Contains(prompts[1], "Plan approved") {
		t.Fatalf("prompts = %q", prompts)
	}
	if sess, _ := store.Get(id); sess.PermissionMode != chatPermissionModeManual {
		t.Fatalf("permission mode after goal mode = %q", sess.PermissionMode)
	}
	if tasks, err := store.GetTasks(id); err != nil || len(tasks.Tasks) != 1 {
		t.Fatalf("session tasks of the approved plan = %+v %v", tasks, err)
	}
	waitFor(t, "the announcement", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(notices) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	if len(audited) < 3 || audited[0] != "started" || audited[len(audited)-1] != "ended" {
		t.Fatalf("audit = %v", audited)
	}
	if len(notices) != 1 || notices[0].Title != "Goal reached" || notices[0].SessionID != id {
		t.Fatalf("notices = %+v", notices)
	}
	d.mu.Lock()
	watched := d.goals[id]
	d.mu.Unlock()
	if watched {
		t.Fatal("an ended goal is still watched")
	}
}

func TestFocusGoalRetriesFailedTurnsAndAnswersQuestions(t *testing.T) {
	asked := false
	reply := func(p focuspipeline.Pipeline, _ int, prompt string) string {
		switch {
		case p.Current == focuspipeline.StagePlan:
			return focusGoalPlanReply
		case !asked:
			asked = true
			return "hm\n<focus-report>{\"summary\":\"which\",\"decisions\":[{\"id\":\"d1\",\"question\":\"A or B?\",\"options\":[\"A\",\"B\"]}]}</focus-report>"
		}
		return focusReport("done", true)
	}
	// The first verification fails twice in a row with the same output:
	// the build blocks, and goal mode retries it.
	verifier := &fakeVerifier{result: func(n int, command string) focuspipeline.VerificationResult {
		if n <= 2 {
			return focuspipeline.VerificationResult{Command: command, ExitCode: 1, Excerpt: "boom"}
		}
		return focuspipeline.VerificationResult{Command: command, Passed: true}
	}}
	d, turns, store, id := goalDriver(t, reply, verifier)
	turns.fail = func(n int) error {
		if n == 2 {
			return errors.New("provider timeout")
		}
		return nil
	}
	startGoal(t, d, store, id, 0)

	waitFor(t, "goal mode to end", func() bool { return !pipelineOf(t, store, id).GoalActive() })
	p := pipelineOf(t, store, id)
	if !focuspipeline.Finished(p) || p.GoalMode.EndReason != focuspipeline.GoalEndFinished {
		t.Fatalf("finished=%v goal=%+v", focuspipeline.Finished(p), p.GoalMode)
	}
	// One push for the failed turn, one for the blocked build.
	if p.GoalMode.Pushes != 2 {
		t.Fatalf("pushes = %d (prompts %q)", p.GoalMode.Pushes, turns.seen())
	}
	joined := strings.Join(turns.seen(), "\n---\n")
	if !strings.Contains(joined, "Decide yourself") || !strings.Contains(joined, "Try once more") {
		t.Fatalf("prompts = %s", joined)
	}
	for _, c := range p.Cards {
		if (c.Kind == focuspipeline.CardGate || c.Kind == focuspipeline.CardDecision) && c.State != focuspipeline.CardDecided {
			t.Fatalf("card left undecided: %+v", c)
		}
	}
}

func TestFocusGoalStopsWhenTheBudgetIsSpent(t *testing.T) {
	verifier := &fakeVerifier{result: func(_ int, command string) focuspipeline.VerificationResult {
		return focuspipeline.VerificationResult{Command: command, ExitCode: 1, Excerpt: "boom"}
	}}
	d, _, store, id := goalDriver(t, stageReply, verifier)
	var mu sync.Mutex
	var titles []string
	d.notify = func(_ context.Context, e notificationEvent) {
		mu.Lock()
		titles = append(titles, e.Title)
		mu.Unlock()
	}
	startGoal(t, d, store, id, 2)

	waitFor(t, "goal mode to end", func() bool { return !pipelineOf(t, store, id).GoalActive() })
	waitDriverIdle(t, d, id)
	p := pipelineOf(t, store, id)
	if focuspipeline.Finished(p) || p.GoalMode.EndReason != focuspipeline.GoalEndExhausted || p.GoalMode.Pushes != 2 || p.OpenGate != focuspipeline.GateBlocked {
		t.Fatalf("goal=%+v gate=%q", p.GoalMode, p.OpenGate)
	}
	if last := p.Cards[len(p.Cards)-1]; last.Title != focuspipeline.NoticeGoalEnded {
		t.Fatalf("last card = %+v", last)
	}
	waitFor(t, "the announcement", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(titles) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	if len(titles) != 1 || titles[0] != "Goal mode stopped" {
		t.Fatalf("notices = %v", titles)
	}
}

func TestFocusGoalResumesAfterARestart(t *testing.T) {
	d, turns, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	// The previous server died with the plan approved and the build turn owed.
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p = focuspipeline.StartGoal(p, 0, time.Now())
		p, _, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 1, Blocks: focuspipeline.ParseBlocks(focusGoalPlanReply)}, time.Now())
		if err != nil {
			return p, err
		}
		p, _, err = focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventGate, Gate: focuspipeline.GatePlan, Action: focuspipeline.GateApprove}, time.Now())
		return p, err
	}); err != nil {
		t.Fatal(err)
	}
	if n := interruptFocusPipelines(store, time.Now(), zerolog.Nop()); n != 1 {
		t.Fatalf("interrupted = %d", n)
	}
	if n := d.resumeGoals(); n != 1 {
		t.Fatalf("resumed = %d", n)
	}
	waitFor(t, "goal mode to end", func() bool { return !pipelineOf(t, store, id).GoalActive() })
	p := pipelineOf(t, store, id)
	if !focuspipeline.Finished(p) || p.GoalMode.Pushes != 1 {
		t.Fatalf("finished=%v goal=%+v", focuspipeline.Finished(p), p.GoalMode)
	}
	if prompts := turns.seen(); len(prompts) != 1 || !strings.Contains(prompts[0], "Plan approved") {
		t.Fatalf("prompts = %q", prompts)
	}
}

func TestFocusGoalEndsOnCancel(t *testing.T) {
	d, turns, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	turns.block = func(n int) bool { return n == 1 }
	startGoal(t, d, store, id, 0)
	waitFor(t, "the first turn", func() bool { return len(turns.seen()) == 1 })

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/cancel", func(w http.ResponseWriter, r *http.Request) {
		handleChatCancel(w, r, d.cancels, d, time.Second)
	})
	if rec := focusRequest(t, mux, http.MethodPost, "/v1/chat/cancel?session_id="+id, "", false); rec.Code != http.StatusOK {
		t.Fatalf("cancel = %d %s", rec.Code, rec.Body.String())
	}
	p := pipelineOf(t, store, id)
	if p.GoalActive() || p.GoalMode.EndReason != focuspipeline.GoalEndCancelled {
		t.Fatalf("goal after cancel = %+v", p.GoalMode)
	}
	// Nothing pushes the cancelled turn again.
	time.Sleep(50 * time.Millisecond)
	if n := len(turns.seen()); n != 1 {
		t.Fatalf("turns after cancel = %d", n)
	}
}

func TestFocusGoalAPI(t *testing.T) {
	d, turns, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	turns.block = func(int) bool { return true }
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	path := "/v1/focus/pipelines/" + id + "/goal"

	if rec := focusRequest(t, h, http.MethodPost, path, `{"enabled":true}`, false); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin enable = %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/nope/goal", `{"enabled":true}`, true); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown pipeline = %d", rec.Code)
	}
	var resp struct {
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	rec := focusRequest(t, h, http.MethodPost, path, `{"enabled":true,"max_pushes":3}`, true)
	decodeInto(t, rec, &resp)
	if rec.Code != http.StatusOK || !resp.Pipeline.GoalActive() || resp.Pipeline.GoalMode.MaxPushes != 3 || !resp.Pipeline.GoalMode.PermissionSet {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
	if sess, _ := store.Get(id); sess.PermissionMode != chatPermissionModeAuto {
		t.Fatalf("permission mode = %q", sess.PermissionMode)
	}
	var list []focusListItem
	decodeInto(t, focusRequest(t, h, http.MethodGet, "/v1/focus/pipelines", "", false), &list)
	if len(list) != 1 || !list[0].GoalMode {
		t.Fatalf("list = %+v", list)
	}

	// Anyone who can use the console may turn it off.
	rec = focusRequest(t, h, http.MethodPost, path, `{"enabled":false}`, false)
	decodeInto(t, rec, &resp)
	if rec.Code != http.StatusOK || resp.Pipeline.GoalActive() || resp.Pipeline.GoalMode.EndReason != focuspipeline.GoalEndDisabled {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body.String())
	}
	if sess, _ := store.Get(id); sess.PermissionMode != "" {
		t.Fatalf("permission mode after disable = %q", sess.PermissionMode)
	}
	if rec = focusRequest(t, h, http.MethodPost, path, `{"enabled":false}`, false); rec.Code != http.StatusOK {
		t.Fatalf("disable twice = %d", rec.Code)
	}

	// A stopped pipeline takes no goal.
	d.cancel(id)
	if rec = focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/stop", "", false); rec.Code != http.StatusOK {
		t.Fatalf("stop = %d %s", rec.Code, rec.Body.String())
	}
	if rec = focusRequest(t, h, http.MethodPost, path, `{"enabled":true}`, true); rec.Code != http.StatusConflict {
		t.Fatalf("enable on a stopped pipeline = %d", rec.Code)
	}
}

func TestFocusGoalLeavesTheUsersPermissionModeAlone(t *testing.T) {
	d, turns, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	turns.block = func(int) bool { return true }
	startGoal(t, d, store, id, 0)
	// The developer picks a mode while goal mode runs: it stays.
	if err := store.SetPermissionMode(id, chatPermissionModePlan); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.endGoal(id, focuspipeline.GoalEndDisabled); !ok {
		t.Fatal("goal mode was not on")
	}
	if sess, _ := store.Get(id); sess.PermissionMode != chatPermissionModePlan {
		t.Fatalf("permission mode = %q", sess.PermissionMode)
	}
	if _, ok := d.endGoal(id, focuspipeline.GoalEndDisabled); ok {
		t.Fatal("ended goal mode twice")
	}
	var nilDriver *focusDriver
	if _, ok := nilDriver.endGoal(id, focuspipeline.GoalEndDisabled); ok {
		t.Fatal("a nil driver ended goal mode")
	}
	nilDriver.watchGoal(id)
	if n := nilDriver.resumeGoals(); n != 0 {
		t.Fatalf("nil driver resumed %d", n)
	}
}

func TestFocusTemplatesAPI(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	dir := filepath.Join(f.store.WorkspaceDir(), focuspipeline.TemplateDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	blog := "name: Blog\nstages:\n  - id: plan\n  - id: write\n    kind: build\n    label: Write\n"
	if err := os.WriteFile(filepath.Join(dir, "blog.yaml"), []byte(blog), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("name: Bad\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var listed struct {
		Templates   []focuspipeline.Template           `json:"templates"`
		Diagnostics []focuspipeline.TemplateDiagnostic `json:"diagnostics"`
	}
	rec := focusRequest(t, h, http.MethodGet, "/v1/focus/templates", "", false)
	decodeInto(t, rec, &listed)
	if rec.Code != http.StatusOK || len(listed.Templates) != 4 || listed.Templates[0].ID != focuspipeline.DevTemplateID || listed.Templates[3].ID != "blog" {
		t.Fatalf("templates = %d %s", rec.Code, rec.Body.String())
	}
	if len(listed.Diagnostics) != 1 || listed.Diagnostics[0].Source != "bad.yaml" {
		t.Fatalf("diagnostics = %+v", listed.Diagnostics)
	}

	cwd := jsonString(f.repo)
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+cwd+`,"template":"nope"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown template = %d", rec.Code)
	}
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+cwd+`,"template":"blog","kind":"release"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("release with a template = %d", rec.Code)
	}
	var created struct {
		SessionID string                 `json:"session_id"`
		Pipeline  focuspipeline.Pipeline `json:"pipeline"`
	}
	rec = focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"a post","cwd":`+cwd+`,"template":"blog","goal_mode":true,"goal_max_pushes":4}`, true)
	decodeInto(t, rec, &created)
	p := created.Pipeline
	if rec.Code != http.StatusCreated || p.Template != "blog" || len(p.Stages) != 2 || p.Stages[1].ID != "write" || p.Stages[1].KindOf() != focuspipeline.StageBuild {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if !p.GoalActive() || p.GoalMode.MaxPushes != 4 {
		t.Fatalf("goal mode of the created pipeline = %+v", p.GoalMode)
	}
	var list []focusListItem
	decodeInto(t, focusRequest(t, h, http.MethodGet, "/v1/focus/pipelines", "", false), &list)
	if len(list) != 1 || list[0].Template != "blog" || !list[0].GoalMode || list[0].CurrentLabel != "" {
		t.Fatalf("list = %+v", list)
	}
}
