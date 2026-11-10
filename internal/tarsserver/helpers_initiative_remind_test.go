package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func reminderJob(t *testing.T, id, text string) cron.Job {
	t.Helper()
	payload, err := cron.MergePayloadMeta(nil, cron.PayloadMeta{TaskType: "reminder", ReminderMessage: text})
	if err != nil {
		t.Fatal(err)
	}
	return cron.Job{ID: id, Name: text, Schedule: "at:2026-11-10T09:00:00Z", SessionTarget: "main", Payload: payload}
}

func noLLMRunner(t *testing.T) agentRuntimePromptRunner {
	return func(context.Context, string, string, []string, string, *agentruntime.ProviderOverride) (string, error) {
		t.Fatal("a reminder must not reach the llm runner")
		return "", nil
	}
}

// TestCronReminderRoutesToInitiativeQueueAndFallsBack covers the remind
// round trip (tars#1001): with remind active a main-session reminder skips
// cron's [REMINDER] note and external send and lands in the queue; once it
// waits past ReminderGrace, Due hands it back to exactly that delivery.
func TestCronReminderRoutesToInitiativeQueueAndFallsBack(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	queue := newInitiativeReminderQueue(filepath.Join(root, "_shared", "initiative", "reminders.json"), nil, zerolog.Nop())
	var sent []string
	runner := newCronJobRunner(root, store, noLLMRunner(t), zerolog.Nop(), nil, mainSession.ID, 0, nil,
		func(_ context.Context, _ cron.Job, text string) error { sent = append(sent, text); return nil },
		queue, true)

	if _, err := runner(context.Background(), reminderJob(t, "job_1", "약 먹기")); err != nil {
		t.Fatal(err)
	}
	msgs, _ := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if len(msgs) != 0 || len(sent) != 0 {
		t.Fatalf("cron delivered a routed reminder: msgs=%+v sent=%v", msgs, sent)
	}
	if queue.Len() != 1 {
		t.Fatalf("queue len = %d, want 1", queue.Len())
	}

	// A fresh queue on the same file sees it (restart safety).
	reread := newInitiativeReminderQueue(queue.path, nil, zerolog.Nop())
	now := time.Now()
	due := reread.Due(context.Background(), now)
	if due == nil || due.Text != "약 먹기" {
		t.Fatalf("due = %+v, want the queued reminder", due)
	}
	if got := queue.Due(context.Background(), now.Add(-time.Hour)); got != nil {
		t.Fatalf("a reminder is not due before its time: %+v", got)
	}

	// Past the grace: cron's own delivery takes it back.
	if got := queue.Due(context.Background(), now.Add(initiative.ReminderGrace+time.Minute)); got != nil {
		t.Fatalf("an overdue reminder must not stay due: %+v", got)
	}
	msgs, _ = session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "[REMINDER]") || len(sent) != 1 {
		t.Fatalf("fallback delivery: msgs=%+v sent=%v", msgs, sent)
	}
	if queue.Len() != 0 {
		t.Fatalf("queue len after fallback = %d", queue.Len())
	}
	if _, err := os.Stat(queue.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty queue should remove its file: %v", err)
	}
}

func TestCronReminderStaysWithCronWhenRemindInactive(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	queue := newInitiativeReminderQueue(filepath.Join(root, "reminders.json"), nil, zerolog.Nop())
	if err := queue.Enqueue(cronReminderHandoff{Job: reminderJob(t, "old", "left over"), TargetSessionID: mainSession.ID, Text: "left over", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var sent []string
	runner := newCronJobRunner(root, store, noLLMRunner(t), zerolog.Nop(), nil, mainSession.ID, 0, nil,
		func(_ context.Context, _ cron.Job, text string) error { sent = append(sent, text); return nil },
		queue, false)
	if _, err := runner(context.Background(), reminderJob(t, "job_1", "stretch")); err != nil {
		t.Fatal(err)
	}
	queue.FlushAll(context.Background())
	msgs, _ := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if len(msgs) != 2 || len(sent) != 2 || queue.Len() != 0 {
		t.Fatalf("msgs=%d sent=%v queue=%d, want both reminders delivered by cron", len(msgs), sent, queue.Len())
	}
}

func TestInitiativeRemindRouterOnlyTakesMainSessionReminders(t *testing.T) {
	queue := newInitiativeReminderQueue(filepath.Join(t.TempDir(), "reminders.json"), nil, zerolog.Nop())
	route := initiativeRemindRouter(queue, "main-id")
	base := reminderJob(t, "j", "x")
	bound := base
	bound.SessionID = "other"
	silent := base
	silent.DeliveryMode = "none"
	cases := []struct {
		name string
		h    cronReminderHandoff
		want bool
	}{
		{"main target", cronReminderHandoff{Job: base, TargetSessionID: "main-id"}, true},
		{"no session target", cronReminderHandoff{Job: base}, true},
		{"another session", cronReminderHandoff{Job: base, TargetSessionID: "other"}, false},
		{"bound to a session", cronReminderHandoff{Job: bound, TargetSessionID: "main-id"}, false},
		{"delivery none", cronReminderHandoff{Job: silent, TargetSessionID: "main-id"}, false},
	}
	for _, c := range cases {
		if got := route(context.Background(), c.h); got != c.want {
			t.Fatalf("%s: route = %v, want %v", c.name, got, c.want)
		}
	}
	if initiativeRemindRouter(nil, "main-id") != nil {
		t.Fatal("nil queue must give no router")
	}
}

func TestInitiativeReminderQueueWithoutFallbackDropsOverdue(t *testing.T) {
	queue := newInitiativeReminderQueue(filepath.Join(t.TempDir(), "reminders.json"), nil, zerolog.Nop())
	if err := queue.Enqueue(cronReminderHandoff{Job: reminderJob(t, "j", "x"), At: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if got := queue.Due(context.Background(), time.Now()); got != nil || queue.Len() != 0 {
		t.Fatalf("due=%+v len=%d", got, queue.Len())
	}
}

func TestInitiativeReminderQueueIgnoresCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reminders.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := newInitiativeReminderQueue(path, nil, zerolog.Nop())
	if queue.Len() != 0 {
		t.Fatalf("len = %d", queue.Len())
	}
}

func subjectSpeaker(t *testing.T, client *speakStubClient) (*initiativeSpeaker, *session.Store, string, *[]notificationEvent) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	var events []notificationEvent
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID, store: store, location: time.UTC, logger: zerolog.Nop(), claim: alwaysClaims,
		notify: func(_ context.Context, evt notificationEvent) { events = append(events, evt) },
	}
	if client != nil {
		speaker.composer = &initiative.SpeechComposer{Client: client}
	}
	return speaker, store, main.ID, &events
}

func TestInitiativeSpeakerRemindWithoutSendsTextUsesFixedWords(t *testing.T) {
	client := &speakStubClient{resp: speakTextResponse("should not be used")}
	speaker, store, mainID, events := subjectSpeaker(t, client)
	speaker.reminders = newInitiativeReminderQueue(filepath.Join(t.TempDir(), "reminders.json"), nil, zerolog.Nop())
	if err := speaker.reminders.Enqueue(cronReminderHandoff{Job: reminderJob(t, "j", "약 먹기"), Text: "약 먹기", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	subject := speaker.reminders.Due(context.Background(), time.Now())
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentRemind, EntryID: "e1", Now: time.Now(), Subject: subject, Channel: initiative.ChannelConsole,
	})
	if err != nil || !outcome.Delivered || outcome.Composed {
		t.Fatalf("outcome = %+v err=%v, want delivered without compose", outcome, err)
	}
	if client.messages != nil {
		t.Fatal("composer was called without SendsText")
	}
	msgs, _ := session.ReadMessages(store.TranscriptPath(mainID))
	if len(msgs) != 1 || msgs[0].Content != "알려 드릴 시간이에요: 약 먹기" || msgs[0].Initiative.Intent != "remind" || msgs[0].Initiative.Ref != "" {
		t.Fatalf("message = %+v", msgs)
	}
	if speaker.reminders.Len() != 0 {
		t.Fatal("a delivered reminder must leave the queue")
	}
	if len(*events) != 1 || (*events)[0].Expression != "alert" {
		t.Fatalf("events = %+v", *events)
	}
}

func TestInitiativeSpeakerConfirmMemoryComposesWithSubjectAndRef(t *testing.T) {
	client := &speakStubClient{resp: speakTextResponse("짧은 답을 좋아하신다고 기억해 둘까요?")}
	speaker, store, mainID, _ := subjectSpeaker(t, client)
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentConfirmMemory, EntryID: "e2", Now: time.Now(), SendsText: true,
		Subject: &initiative.Subject{ID: "cand-1", Text: "prefers short answers"}, Channel: initiative.ChannelConsole,
	})
	if err != nil || !outcome.Delivered || !outcome.Composed {
		t.Fatalf("outcome = %+v err=%v", outcome, err)
	}
	if !strings.Contains(client.messages[1].Content, "prefers short answers") {
		t.Fatalf("compose prompt lacks the subject: %s", client.messages[1].Content)
	}
	msgs, _ := session.ReadMessages(store.TranscriptPath(mainID))
	if len(msgs) != 1 || msgs[0].Initiative.Ref != "cand-1" || msgs[0].Initiative.Intent != "confirm_memory" {
		t.Fatalf("message = %+v", msgs)
	}
}

func TestInitiativeSpeakerWithoutComposer(t *testing.T) {
	speaker, _, _, _ := subjectSpeaker(t, nil)
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet, Now: time.Now()})
	if err != nil || outcome.Composed || outcome.Delivered || outcome.Reason != "speak_unavailable" {
		t.Fatalf("greet without composer = %+v err=%v", outcome, err)
	}
	outcome, err = speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentConfirmMemory, Now: time.Now(), SendsText: true,
		Subject: &initiative.Subject{ID: "c", Text: "likes tea"},
	})
	if err != nil || !outcome.Delivered || outcome.Composed {
		t.Fatalf("confirm_memory without composer = %+v err=%v, want fixed words", outcome, err)
	}
	outcome, _ = speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentRemind, Now: time.Now(), Subject: &initiative.Subject{ID: "r", Text: " "},
	})
	if outcome.Reason != "speak_unavailable" {
		t.Fatalf("empty subject = %+v", outcome)
	}
}

func TestMemoryConfirmSubjectPicksNewestPendingPreference(t *testing.T) {
	got := memoryConfirmSubject([]memory.MemoryCandidate{
		{ID: "a", Status: memory.MemoryCandidateStatusApproved, Category: "preference", Summary: "done"},
		{ID: "b", Status: memory.MemoryCandidateStatusPending, Category: "fact", Summary: "not a preference"},
		{ID: "c", Status: memory.MemoryCandidateStatusPending, Category: "Preference", Summary: " likes tea "},
		{ID: "d", Status: memory.MemoryCandidateStatusPending, Category: "preference", Summary: "older"},
	})
	if got == nil || got.ID != "c" || got.Text != "likes tea" {
		t.Fatalf("subject = %+v", got)
	}
	if memoryConfirmSubject(nil) != nil {
		t.Fatal("no candidates must give no subject")
	}
}

func TestSessionObserverReadsReminderAndMemoryCandidate(t *testing.T) {
	now := time.Now()
	obs, err := newSessionInitiativeObserver(sessionObserverDeps{
		Reminder: func(context.Context, time.Time) *initiative.Subject { return &initiative.Subject{ID: "r"} },
		MemoryCandidates: func() ([]memory.MemoryCandidate, error) {
			return []memory.MemoryCandidate{{ID: "c", Status: memory.MemoryCandidateStatusPending, Category: "preference", Summary: "s"}}, nil
		},
	}).Observe(context.Background(), now)
	if err != nil || obs.Reminder == nil || obs.MemoryCandidate == nil || obs.MemoryCandidate.ID != "c" {
		t.Fatalf("obs = %+v err=%v", obs, err)
	}
	obs, _ = newSessionInitiativeObserver(sessionObserverDeps{
		MemoryCandidates: func() ([]memory.MemoryCandidate, error) { return nil, errors.New("broken") },
	}).Observe(context.Background(), now)
	if obs.MemoryCandidate != nil {
		t.Fatal("a broken inbox must read as nothing to ask")
	}
}

func TestBuildInitiativeRuntimeWiresRemindAndConfirmMemory(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	queue := newInitiativeReminderQueue(filepath.Join(root, "reminders.json"), nil, zerolog.Nop())
	cfg := config.Config{}
	cfg.Initiative = config.InitiativeConfig{Enabled: true, Mode: "live", Remind: true, ConfirmMemory: true}
	setup := buildInitiativeRuntime(initiativeSetupInputs{
		Config: cfg, WorkspaceDir: root, SessionStore: store, MainSessionID: main.ID, Reminders: queue, Logger: zerolog.Nop(),
	})
	if !setup.RemindActive || setup.Speaker == nil || setup.Speaker.reminders != queue || setup.Speaker.composer != nil {
		t.Fatalf("setup = %+v speaker=%+v", setup, setup.Speaker)
	}
	snap := setup.Runtime.Snapshot()
	if !snap.Remind.Enabled || !snap.ConfirmMemory.Enabled {
		t.Fatalf("snapshot = %+v / %+v", snap.Remind, snap.ConfirmMemory)
	}

	cfg.Initiative.Mode = "shadow"
	setup = buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: root, SessionStore: store, MainSessionID: main.ID, Reminders: queue, Logger: zerolog.Nop()})
	if setup.RemindActive || setup.Speaker != nil {
		t.Fatalf("shadow mode must not route reminders or build a speaker: %+v", setup)
	}
}

// TestInitiativeMemoryAnswerRoute drives the console's answer buttons:
// status read, approve (saved through the memory review), a second answer
// (409), unknown entry (404), bad action (400), and the route surviving
// with initiative off.
func TestInitiativeMemoryAnswerRoute(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	cand, added, err := memory.AppendInboxCandidateIfNew(context.Background(), root, nil, memory.MemoryCandidate{Category: "preference", Summary: "prefers short answers"})
	if err != nil || !added {
		t.Fatalf("append candidate: %v added=%v", err, added)
	}
	if err := session.AppendMessage(store.TranscriptPath(main.ID), session.Message{
		Role: "assistant", Content: "Should I remember this?", Timestamp: time.Now(),
		Initiative: &session.MessageInitiative{Intent: "confirm_memory", EntryID: "init_q", Ref: cand.ID},
	}); err != nil {
		t.Fatal(err)
	}
	h := newInitiativeAPIHandlerWithMemory(nil, nil, nil, initiativeMemoryAnswerDeps{WorkspaceDir: root, Store: store, MainSessionID: main.ID})
	do := func(method, target, body string) (int, map[string]string) {
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	if code, out := do(http.MethodGet, "/v1/initiative/memory/answer?entry_id=init_q", ""); code != 200 || out["status"] != "pending" {
		t.Fatalf("GET = %d %v", code, out)
	}
	if code, _ := do(http.MethodPost, "/v1/initiative/memory/answer", `{"entry_id":"init_q","action":"maybe"}`); code != 400 {
		t.Fatalf("bad action = %d", code)
	}
	if code, out := do(http.MethodPost, "/v1/initiative/memory/answer", `{"entry_id":"init_q","action":"approve"}`); code != 200 || out["status"] != "approved" {
		t.Fatalf("approve = %d %v", code, out)
	}
	if code, out := do(http.MethodPost, "/v1/initiative/memory/answer", `{"entry_id":"init_q","action":"reject"}`); code != http.StatusConflict || out["status"] != "approved" {
		t.Fatalf("second answer = %d %v", code, out)
	}
	if code, _ := do(http.MethodGet, "/v1/initiative/memory/answer?entry_id=nope", ""); code != 404 {
		t.Fatalf("unknown entry = %d", code)
	}
	if code, _ := do(http.MethodGet, "/v1/initiative/memory/answer", ""); code != 400 {
		t.Fatalf("missing entry = %d", code)
	}
	if code, _ := do(http.MethodDelete, "/v1/initiative/memory/answer", ""); code != http.StatusMethodNotAllowed {
		t.Fatalf("delete = %d", code)
	}
	if status, err := memoryCandidateStatus(root, "gone"); err != nil || status != "missing" {
		t.Fatalf("missing candidate status = %q %v", status, err)
	}
	if _, err := confirmMemoryCandidateID(initiativeMemoryAnswerDeps{}, "x"); err == nil {
		t.Fatal("no main session must not resolve")
	}

	disabled := buildInitiativeRuntime(initiativeSetupInputs{WorkspaceDir: root, SessionStore: store, MainSessionID: main.ID})
	rec := httptest.NewRecorder()
	disabled.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/memory/answer?entry_id=init_q", nil))
	if rec.Code != 200 {
		t.Fatalf("answer route with initiative off = %d", rec.Code)
	}
}
