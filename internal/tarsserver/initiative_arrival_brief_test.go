package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// The Bridge's facts become a brief snapshot: pipelines with whether they
// wait, chat and queued approvals, and every cron job's last-run failure
// (not only the last 24 hours, so a recovery is seen as one).
func TestCaseBridgeBriefSnapshot(t *testing.T) {
	f := newCaseBridgeFixture(t)
	snap := f.bridge.briefSnapshot(f.now)
	if !snap.At.Equal(f.now) {
		t.Fatalf("at = %v", snap.At)
	}
	p, ok := snap.Pipelines[f.pipeline.ID]
	if !ok || !p.Waiting || p.Finished || p.Gate == "" || p.Title == "" {
		t.Fatalf("pipeline = %+v (ok %v)", p, ok)
	}
	if len(snap.ApprovalSessions) != 1 || snap.ApprovalSessions[0] != f.busy.ID || snap.QueuedApprovals != 1 {
		t.Fatalf("approvals = %v queued %d", snap.ApprovalSessions, snap.QueuedApprovals)
	}
	if !snap.Cron["c1"].Failed || !snap.Cron["c2"].Failed || snap.Cron["c3"].Failed || snap.Cron["c3"].Name != "Fine" {
		t.Fatalf("cron = %+v", snap.Cron)
	}

	var nilBridge *caseBridge
	if got := nilBridge.briefSnapshot(f.now); !got.At.Equal(f.now) || len(got.Pipelines) != 0 {
		t.Fatalf("nil bridge = %+v", got)
	}
}

// arrivedBriefStore is a store frozen at an arrival after which one
// pipeline started waiting at G2.
func arrivedBriefStore(t *testing.T, now time.Time) *initiative.BriefStore {
	t.Helper()
	state := initiative.StatusSnapshot{Pipelines: map[string]initiative.PipelinePoint{"p1": {Title: "Fix login", Stage: "build"}}}
	store := initiative.OpenBriefStore(filepath.Join(t.TempDir(), "snapshot.json"), func(time.Time) initiative.StatusSnapshot {
		out := state
		out.Pipelines = map[string]initiative.PipelinePoint{}
		for k, v := range state.Pipelines {
			out.Pipelines[k] = v
		}
		return out
	})
	p := initiative.RecordPresence(initiative.PresenceState{}, now.Add(-5*time.Hour))
	if err := store.Seen(p, now.Add(-5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	state.Pipelines["p1"] = initiative.PipelinePoint{Title: "Fix login", Stage: "review", Gate: "triage", Waiting: true}
	p = initiative.RecordPresence(p, now)
	if err := store.Seen(p, now); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestInitiativeSpeakerAttachesBriefAndCheckInButtons(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	speak := func(t *testing.T, intent initiative.Intent, brief *initiative.BriefStore, buttons bool) notificationEvent {
		t.Helper()
		store := session.NewStore(t.TempDir())
		main, err := store.EnsureMain()
		if err != nil {
			t.Fatal(err)
		}
		var events []notificationEvent
		speaker := &initiativeSpeaker{
			mainSessionID:  main.ID,
			store:          store,
			composer:       &initiative.SpeechComposer{Client: &speakStubClient{resp: speakTextResponse("Hello again.")}},
			location:       time.UTC,
			notify:         func(_ context.Context, evt notificationEvent) { events = append(events, evt) },
			logger:         zerolog.Nop(),
			claim:          alwaysClaims,
			brief:          brief,
			checkInButtons: buttons,
		}
		out, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: intent, EntryID: "entry-1", Now: now})
		if err != nil || !out.Delivered || len(events) != 1 {
			t.Fatalf("outcome = %+v err %v events %d", out, err, len(events))
		}
		msgs, _ := session.ReadMessages(store.TranscriptPath(main.ID))
		if len(msgs) != 1 || msgs[0].Content != "Hello again." {
			t.Fatalf("transcript = %+v, want the composed words only", msgs)
		}
		return events[0]
	}

	t.Run("greet carries the brief in both languages", func(t *testing.T) {
		evt := speak(t, initiative.IntentGreet, arrivedBriefStore(t, now), true)
		if len(evt.Brief) != 1 || evt.Actions != nil || evt.EntryID != "" {
			t.Fatalf("event = %+v", evt)
		}
		line := evt.Brief[0]
		if line.Kind != "waiting" || line.Severity != "block" || line.Href != "/console/focus/p1" ||
			line.Text["en"] != `"Fix login" is waiting at G2 (review)` || line.Text["ko"] != `"Fix login" — G2(리뷰)에서 기다려요` {
			t.Fatalf("line = %+v", line)
		}
	})
	t.Run("greet without a brief store is the plain greeting", func(t *testing.T) {
		if evt := speak(t, initiative.IntentGreet, nil, true); evt.Brief != nil {
			t.Fatalf("event = %+v", evt)
		}
	})
	t.Run("check-in carries the buttons and its entry", func(t *testing.T) {
		evt := speak(t, initiative.IntentCheckIn, arrivedBriefStore(t, now), true)
		if strings.Join(evt.Actions, ",") != "ok,snooze,quiet_today" || evt.EntryID != "entry-1" || evt.Brief != nil {
			t.Fatalf("event = %+v", evt)
		}
	})
	t.Run("check-in buttons off", func(t *testing.T) {
		if evt := speak(t, initiative.IntentCheckIn, nil, false); evt.Actions != nil || evt.EntryID != "" {
			t.Fatalf("event = %+v", evt)
		}
	})
}

func TestConsolePresenceFeedsTheBriefStore(t *testing.T) {
	dir := t.TempDir()
	presence := initiative.OpenPresenceStore(filepath.Join(dir, "presence.json"))
	brief := initiative.OpenBriefStore(filepath.Join(dir, "snapshot.json"), func(time.Time) initiative.StatusSnapshot {
		return initiative.StatusSnapshot{Pipelines: map[string]initiative.PipelinePoint{"p": {Title: "x"}}}
	})
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	h := newConsolePresenceAPIHandler(presence, brief, func() time.Time { return now }, zerolog.Nop())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/console/presence", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	if err != nil || !strings.Contains(string(raw), `"seen"`) {
		t.Fatalf("snapshot.json = %s, %v", raw, err)
	}
}

func TestBuildInitiativeRuntimeArrivalBriefGating(t *testing.T) {
	source := func(now time.Time) initiative.StatusSnapshot { return initiative.StatusSnapshot{At: now} }
	for _, tc := range []struct {
		name  string
		ic    config.InitiativeConfig
		src   func(time.Time) initiative.StatusSnapshot
		brief bool
	}{
		{"live, on", config.InitiativeConfig{Enabled: true, Mode: "live", ArrivalBrief: true}, source, true},
		{"live, off", config.InitiativeConfig{Enabled: true, Mode: "live"}, source, false},
		{"shadow", config.InitiativeConfig{Enabled: true, Mode: "shadow", ArrivalBrief: true}, source, false},
		{"no source", config.InitiativeConfig{Enabled: true, Mode: "live", ArrivalBrief: true}, nil, false},
		{"initiative off", config.InitiativeConfig{Mode: "live", ArrivalBrief: true}, source, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup := buildInitiativeRuntime(initiativeSetupInputs{Config: config.Config{Initiative: tc.ic}, WorkspaceDir: t.TempDir(), BriefSource: tc.src, Logger: zerolog.Nop()})
			if (setup.Brief != nil) != tc.brief {
				t.Fatalf("brief = %v, want %v", setup.Brief != nil, tc.brief)
			}
		})
	}
	// With a speaker (live remind needs none of a speak backend), the
	// speaker gets the store and the check-in buttons flag.
	setup := buildInitiativeRuntime(initiativeSetupInputs{
		Config:       config.Config{Initiative: config.InitiativeConfig{Enabled: true, Mode: "live", ArrivalBrief: true, CheckInButtons: true, ConfirmMemory: true}},
		WorkspaceDir: t.TempDir(), BriefSource: source, Logger: zerolog.Nop(),
	})
	if setup.Speaker == nil || setup.Speaker.brief != setup.Brief || !setup.Speaker.checkInButtons {
		t.Fatalf("speaker = %+v", setup.Speaker)
	}
}

func TestInitiativeCheckInAnswerRoute(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for _, m := range []session.Message{
		{Role: "assistant", Content: "How is it going?", Timestamp: now, Initiative: &session.MessageInitiative{Intent: "check_in", EntryID: "e-check"}},
		{Role: "assistant", Content: "Welcome back", Timestamp: now, Initiative: &session.MessageInitiative{Intent: "greet", EntryID: "e-greet"}},
	} {
		if err := session.AppendMessage(store.TranscriptPath(main.ID), m); err != nil {
			t.Fatal(err)
		}
	}
	runtime := initiative.NewRuntime(initiative.Config{Enabled: true, Mode: initiative.ModeLive, Location: time.UTC}, initiative.Dependencies{
		Ledger: initiative.OpenLedger(filepath.Join(workspace, "ledger.jsonl"), 0),
		Logger: zerolog.Nop(),
		Now:    func() time.Time { return now },
	})
	deps := initiativeMemoryAnswerDeps{WorkspaceDir: workspace, Store: store, MainSessionID: main.ID}
	h := newInitiativeAPIHandlerWithMemory(runtime, nil, nil, deps)
	post := func(h http.Handler, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/initiative/checkin/answer", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/checkin/answer", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", rec.Code)
	}
	for body, want := range map[string]int{
		`{"action":"ok"}`:                               http.StatusBadRequest,
		`{"entry_id":"e-check","action":"later"}`:       http.StatusBadRequest,
		`{"entry_id":"e-greet","action":"snooze"}`:      http.StatusNotFound,
		`{"entry_id":"missing","action":"snooze"}`:      http.StatusNotFound,
		`{"entry_id":"e-check","action":"SNOOZE"}`:      http.StatusOK,
		`{"entry_id":"e-check","action":"quiet_today"}`: http.StatusOK,
	} {
		if rec := post(h, body); rec.Code != want {
			t.Fatalf("%s = %d %s, want %d", body, rec.Code, rec.Body.String(), want)
		}
	}
	rec = post(h, `{"entry_id":"e-check","action":"snooze"}`)
	var out struct {
		EntryID string     `json:"entry_id"`
		Action  string     `json:"action"`
		Until   *time.Time `json:"until"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.EntryID != "e-check" || out.Action != "snooze" || out.Until == nil || !out.Until.Equal(now.Add(initiative.CheckInSnooze)) {
		t.Fatalf("answer = %s (%v)", rec.Body.String(), err)
	}
	snap := runtime.Snapshot()
	if !snap.Outcomes.StoppedToday || snap.CheckInSnoozedUntil == nil {
		t.Fatalf("snapshot = %+v", snap)
	}

	// Initiative off: the route answers 503 rather than recording anything.
	if rec := post(newInitiativeAPIHandlerWithMemory(nil, nil, nil, deps), `{"entry_id":"e-check","action":"ok"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no runtime = %d", rec.Code)
	}
	// No main session to look in: 404.
	if rec := post(newInitiativeAPIHandlerWithMemory(runtime, nil, nil, initiativeMemoryAnswerDeps{}), `{"entry_id":"e-check","action":"ok"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("no store = %d", rec.Code)
	}
}
