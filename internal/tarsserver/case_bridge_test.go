package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/casebridge"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type caseBridgeFixture struct {
	bridge   *caseBridge
	handler  http.Handler
	store    *session.Store
	pipeline session.Session
	busy     session.Session
	now      time.Time
}

func newCaseBridgeFixture(t *testing.T) *caseBridgeFixture {
	t.Helper()
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	now := time.Date(2026, 10, 18, 21, 0, 0, 0, time.UTC)

	pipeline := plannedFocusSession(t, store)
	busy, err := store.Create("Refactor")
	if err != nil {
		t.Fatal(err)
	}
	activity := newChatActivity(store, nil)
	activity.now = func() time.Time { return now.Add(-3 * time.Minute) }
	end := activity.begin(busy.ID)
	t.Cleanup(end)
	activity.observe(busy.ID, map[string]any{"type": "permission_request", "request_id": "r1", "tool_name": "exec"})
	activity.queued = func() ([]chatQueuedApproval, error) {
		return []chatQueuedApproval{{ApprovalID: "a1", SessionID: busy.ID, Session: "Nightly", Source: "cron", ToolName: "write_file", RequestedAt: now.Add(-time.Hour)}}, nil
	}

	failedAt := now.Add(-30 * time.Minute)
	oldAt := now.Add(-48 * time.Hour)
	sources := caseBridgeSources{
		cronJobs: func() ([]cron.Job, error) {
			return []cron.Job{
				{ID: "c1", Name: "Backup", LastRunAt: &failedAt, LastRunError: "exit status 1"},
				{ID: "c2", Name: "Old", LastRunAt: &oldAt, LastRunError: "too old"},
				{ID: "c3", Name: "Fine", LastRunAt: &failedAt},
			}, nil
		},
		usage: func(period string) (float64, bool) {
			if period == "today" {
				return 1.5, true
			}
			return 9.25, true
		},
		pulse: func() pulse.Snapshot {
			return pulse.Snapshot{Recent: []pulse.TickOutcome{
				{Signals: []pulse.Signal{{Kind: "old", Severity: pulse.SeverityCritical, Summary: "an older tick"}}},
				{Signals: []pulse.Signal{{Kind: "disk", Severity: pulse.SeverityWarn, Summary: "disk 91% full"}, {Kind: "noise", Severity: pulse.SeverityInfo, Summary: "fine"}}},
				{Skipped: true},
			}}
		},
	}
	b := newCaseBridge(workspace, sources, zerolog.Nop())
	b.now = func() time.Time { return now }
	t.Cleanup(b.Close)
	b.sessions = store
	b.activity = activity
	b.cancels = newChatCancelRegistry()
	return &caseBridgeFixture{bridge: b, handler: newCaseBridgeHandler(b), store: store, pipeline: pipeline, busy: busy, now: now}
}

func caseRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func readCaseAnswer(t *testing.T, h http.Handler) caseAnswer {
	t.Helper()
	rec := caseRequest(t, h, http.MethodGet, "/v1/case/answer", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("answer = %d %s", rec.Code, rec.Body.String())
	}
	var out caseAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func waitCaseIdle(t *testing.T, b *caseBridge) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		busy := b.busy
		b.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the Bridge turn never ended")
}

func TestCaseBridgeAnswersFromTheStatusInItsTurnOnly(t *testing.T) {
	f := newCaseBridgeFixture(t)
	type call struct{ session, question, consoleContext, own, other string }
	var mu sync.Mutex
	var calls []call
	release := make(chan struct{})
	f.bridge.runTurn = func(_ context.Context, sessionID, question, consoleContext string) error {
		mu.Lock()
		calls = append(calls, call{sessionID, question, consoleContext, f.bridge.turnContext(sessionID), f.bridge.turnContext(f.busy.ID)})
		mu.Unlock()
		<-release
		transcript := f.store.TranscriptPath(sessionID)
		_ = session.AppendMessage(transcript, session.Message{Role: "user", Content: question, Timestamp: f.now})
		reply := "One pipeline waits at G1 and a cron job failed.\n<case-cards>[" +
			`{"kind":"focus","id":"` + f.pipeline.ID + `","title":"Plan","severity":"block"},` +
			`{"kind":"focus","id":"invented","title":"Nope"},` +
			`{"kind":"cron","id":"c1","title":"Backup","severity":"block"}]</case-cards>`
		_ = session.AppendMessage(transcript, session.Message{Role: "assistant", Content: reply, Timestamp: f.now})
		return nil
	}

	rec := caseRequest(t, f.handler, http.MethodPost, "/v1/case/ask", `{"question":"Anything stuck?","scope":"all","view":{"route":"focus","session_id":"`+f.pipeline.ID+`","title":"Plan"}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ask = %d %s", rec.Code, rec.Body.String())
	}
	var accepted struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &accepted)
	caseID := accepted.SessionID
	if caseID == "" {
		t.Fatal("no session id")
	}

	// A second question while the first runs is refused.
	if rec := caseRequest(t, f.handler, http.MethodPost, "/v1/case/ask", `{"question":"and now?"}`); rec.Code != http.StatusConflict {
		t.Fatalf("overlapping ask = %d", rec.Code)
	}
	if a := readCaseAnswer(t, f.handler); !a.Busy || a.Answer != "" || a.Question != "Anything stuck?" {
		t.Fatalf("answer while busy = %+v", a)
	}
	close(release)
	waitCaseIdle(t, f.bridge)

	mu.Lock()
	got := calls
	mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("turns = %d", len(got))
	}
	c := got[0]
	if c.session != caseID || c.question != "Anything stuck?" {
		t.Fatalf("turn = %+v", c)
	}
	if !strings.HasPrefix(c.consoleContext, casebridge.Instructions) || !strings.Contains(c.consoleContext, "["+f.pipeline.ID+"]") {
		t.Fatalf("console context = %q", c.consoleContext)
	}
	for _, want := range []string{
		"<tars-status>",
		"[focus:" + f.pipeline.ID + "]",
		`approval of exec in "Refactor"`,
		`unattended run from cron waits for approval of write_file in "Nightly"`,
		`a turn in "Refactor", running for 3 minutes [session:` + f.busy.ID + "]",
		`cron job "Backup" failed 30 minutes ago: exit status 1 [cron:c1]`,
		"warn disk: disk 91% full [pulse]",
		"Usage: today $1.50, this week $9.25 [usage]",
	} {
		if !strings.Contains(c.own, want) {
			t.Errorf("turn context is missing %q\n%s", want, c.own)
		}
	}
	for _, unwanted := range []string{"Old", "Fine", "an older tick", "noise", "[session:" + caseID + "]"} {
		if strings.Contains(c.own, unwanted) {
			t.Errorf("turn context has %q\n%s", unwanted, c.own)
		}
	}
	if c.other != "" {
		t.Fatalf("another session's turn got the status: %q", c.other)
	}
	if after := f.bridge.turnContext(caseID); after != "" {
		t.Fatalf("turn context after the question: %q", after)
	}

	sess, err := f.store.Get(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Kind != "worker" || !sess.Hidden || sess.PermissionMode != chatPermissionModePlan || sess.TierPin != caseBridgeTier || sess.Isolation != session.IsolationOff {
		t.Fatalf("CASE session = %+v", sess)
	}

	a := readCaseAnswer(t, f.handler)
	if a.Busy || a.SessionID != caseID || a.Answer != "One pipeline waits at G1 and a cron job failed." || a.Error != "" {
		t.Fatalf("answer = %+v", a)
	}
	if len(a.Cards) != 2 || a.Cards[0].Href != "/console/focus/"+f.pipeline.ID || a.Cards[1].Href != "/console/cron" {
		t.Fatalf("cards = %+v", a.Cards)
	}
}

func TestCaseBridgeSessionIsReusedForADayThenReplaced(t *testing.T) {
	f := newCaseBridgeFixture(t)
	f.bridge.runTurn = func(context.Context, string, string, string) error { return nil }
	ask := func() string {
		t.Helper()
		id, status, err := f.bridge.ask("q", "", casebridge.View{}, "")
		if err != nil {
			t.Fatalf("ask = %d %v", status, err)
		}
		waitCaseIdle(t, f.bridge)
		return id
	}
	first := ask()
	if again := ask(); again != first {
		t.Fatalf("same day made a new session: %s then %s", first, again)
	}
	raw, err := os.ReadFile(f.bridge.statePath())
	if err != nil || !strings.Contains(string(raw), `"day":"2026-10-18"`) {
		t.Fatalf("state = %s %v", raw, err)
	}
	f.bridge.now = func() time.Time { return f.now.Add(24 * time.Hour) }
	next := ask()
	if next == first {
		t.Fatal("a new day reused yesterday's session")
	}
	if err := f.store.Delete(next); err != nil {
		t.Fatal(err)
	}
	if replaced := ask(); replaced == next {
		t.Fatal("a deleted session was reused")
	}
}

func TestCaseBridgeRequestsAndFailures(t *testing.T) {
	unbound := newCaseBridge(t.TempDir(), caseBridgeSources{}, zerolog.Nop())
	defer unbound.Close()
	h := newCaseBridgeHandler(unbound)
	if rec := caseRequest(t, h, http.MethodPost, "/v1/case/ask", `{"question":"hi"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unbound ask = %d", rec.Code)
	}
	if a := readCaseAnswer(t, h); a.SessionID != "" || a.Cards == nil {
		t.Fatalf("unbound answer = %+v", a)
	}

	f := newCaseBridgeFixture(t)
	for _, tc := range []struct {
		method, body string
		want         int
	}{
		{http.MethodGet, "", http.StatusMethodNotAllowed},
		{http.MethodPost, `{"question":"  "}`, http.StatusBadRequest},
		{http.MethodPost, `{not json`, http.StatusBadRequest},
	} {
		if rec := caseRequest(t, f.handler, tc.method, "/v1/case/ask", tc.body); rec.Code != tc.want {
			t.Errorf("%s %q = %d, want %d", tc.method, tc.body, rec.Code, tc.want)
		}
	}
	if rec := caseRequest(t, f.handler, http.MethodPost, "/v1/case/answer", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST answer = %d", rec.Code)
	}

	// A turn that fails leaves an error and no answer.
	f.bridge.runTurn = func(context.Context, string, string, string) error { return errors.New("provider down") }
	if _, status, err := f.bridge.ask("q", casebridge.ScopeScreen, casebridge.View{}, ""); err != nil {
		t.Fatalf("ask = %d %v", status, err)
	}
	waitCaseIdle(t, f.bridge)
	if a := readCaseAnswer(t, f.handler); a.Error == "" || a.Answer != "" {
		t.Fatalf("failed answer = %+v", a)
	}

	// A broken session store refuses the question instead of running it.
	f.bridge.mu.Lock()
	f.bridge.workspaceDir = string([]byte{0})
	f.bridge.mu.Unlock()
	f.bridge.now = func() time.Time { return f.now.Add(48 * time.Hour) }
	if _, status, err := f.bridge.ask("q", "", casebridge.View{}, ""); err == nil || status != http.StatusInternalServerError {
		t.Fatalf("ask with an unwritable state = %d %v", status, err)
	}

	// After Close no question starts.
	f.bridge.Close()
	if _, status, err := f.bridge.ask("q", "", casebridge.View{}, ""); err == nil || status != http.StatusServiceUnavailable {
		t.Fatalf("ask after close = %d %v", status, err)
	}

	var nilBridge *caseBridge
	nilBridge.bind(chatHandlerDeps{})
	nilBridge.Close()
	if nilBridge.turnContext("x") != "" || nilBridge.answer().Cards == nil {
		t.Fatal("a nil Bridge must be inert")
	}
}

func TestCaseBridgeBindAndReadOnlyRefusal(t *testing.T) {
	b := newCaseBridge(t.TempDir(), caseBridgeSources{}, zerolog.Nop())
	defer b.Close()
	store := session.NewStore(t.TempDir())
	b.bind(chatHandlerDeps{store: store, chatActivity: newChatActivity(store, nil), cancelRegistry: newChatCancelRegistry()})
	if b.runTurn == nil || b.sessions != store {
		t.Fatal("bind did not connect the turn path")
	}
	if msg := unattendedReadOnlySources[caseBridgeSource]; msg != caseBridgeReadOnlyMessage {
		t.Fatalf("Bridge refusal = %q", msg)
	}
	if msg := unattendedReadOnlySources[focusSecondOpinionSource]; msg != unattendedReadOnlyMessage {
		t.Fatalf("second opinion refusal = %q", msg)
	}
}

func TestLatestPulseSignalsAndUsage(t *testing.T) {
	if got := latestPulseSignals(pulse.Snapshot{}); got != nil {
		t.Fatalf("no ticks = %+v", got)
	}
	if got := latestPulseSignals(pulse.Snapshot{Recent: []pulse.TickOutcome{{Skipped: true}}}); got != nil {
		t.Fatalf("only skipped = %+v", got)
	}
	got := latestPulseSignals(pulse.Snapshot{Recent: []pulse.TickOutcome{{Signals: []pulse.Signal{{Kind: "x", Severity: pulse.SeverityCritical, Summary: "boom"}}}}})
	if len(got) != 1 || got[0].Severity != pulse.SeverityCritical.String() {
		t.Fatalf("signals = %+v", got)
	}

	if caseBridgeUsage(nil) != nil {
		t.Fatal("no tracker should mean no usage source")
	}
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spend := caseBridgeUsage(tracker)
	if cost, ok := spend("today"); !ok || cost != 0 {
		t.Fatalf("empty tracker today = %v %v", cost, ok)
	}
	if _, ok := spend("not-a-period"); ok {
		t.Fatal("a bad period should not read as known")
	}
}

func TestLastAssistantReply(t *testing.T) {
	path := t.TempDir() + "/t.jsonl"
	if got := lastAssistantReply(path); got != "" {
		t.Fatalf("missing transcript = %q", got)
	}
	for _, m := range []session.Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "first"},
		{Role: "user", Content: "two"},
		{Role: "assistant", Content: "  "},
	} {
		_ = session.AppendMessage(path, m)
	}
	if got := lastAssistantReply(path); got != "" {
		t.Fatalf("a question with no reply yet answered with %q", got)
	}
	_ = session.AppendMessage(path, session.Message{Role: "assistant", Content: " second "})
	if got := lastAssistantReply(path); got != "second" {
		t.Fatalf("reply = %q", got)
	}
}
