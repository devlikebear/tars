package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type fakeDesktopNotifier struct {
	calls []notificationEvent
}

func (n *fakeDesktopNotifier) Notify(_ context.Context, evt notificationEvent) error {
	n.calls = append(n.calls, evt)
	return nil
}

type flakyDesktopNotifier struct {
	calls int
}

func (n *flakyDesktopNotifier) Notify(_ context.Context, _ notificationEvent) error {
	n.calls++
	if n.calls == 1 {
		return errors.New("temporary notify error")
	}
	return nil
}

func TestNotificationDispatcher_UsesDesktopNotifyWithoutSubscribers(t *testing.T) {
	broker := newEventBroker()
	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, fake, true, zerolog.New(io.Discard))

	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "info", "Cron done", "check inbox done"))

	if len(fake.calls) != 1 {
		t.Fatalf("expected desktop notify call when no subscribers, got %d", len(fake.calls))
	}
}

func TestNotificationDispatcher_CronStillNotifiesWithSubscribers(t *testing.T) {
	broker := newEventBroker()
	_, _, unsubscribe := broker.subscribe()
	defer unsubscribe()

	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, fake, true, zerolog.New(io.Discard))
	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "info", "Cron done", "check inbox done"))

	if len(fake.calls) != 1 {
		t.Fatalf("expected cron desktop notify even with subscribers, got %d", len(fake.calls))
	}
}

func TestNotificationDispatcher_NonCronSkipsDesktopNotifyWithSubscribers(t *testing.T) {
	broker := newEventBroker()
	_, _, unsubscribe := broker.subscribe()
	defer unsubscribe()

	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, fake, true, zerolog.New(io.Discard))
	dispatcher.Emit(context.Background(), newNotificationEvent("heartbeat", "info", "Heartbeat", "ok"))

	if len(fake.calls) != 0 {
		t.Fatalf("expected non-cron desktop notify to be skipped when subscribers exist, got %d", len(fake.calls))
	}
}

func TestNotificationDispatcher_RetriesDesktopNotifyOnFailure(t *testing.T) {
	broker := newEventBroker()
	flaky := &flakyDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, flaky, true, zerolog.New(io.Discard))

	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "info", "Cron done", "check inbox done"))

	if flaky.calls != 2 {
		t.Fatalf("expected one retry for failed desktop notify, got %d calls", flaky.calls)
	}
}

func TestNotificationDispatcher_SuppressesDesktopNotifyForCoalescedPulse(t *testing.T) {
	store, err := newNotificationStore(t.TempDir()+"/notifications.json", 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}
	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(nil, fake, true, zerolog.New(io.Discard))
	dispatcher.store = store

	first := newNotificationEvent("pulse", "warn", "Chat sessions need attention", "3 sessions are stalled")
	first.Timestamp = "2026-05-08T13:00:00Z"
	dispatcher.Emit(context.Background(), first)
	duplicate := newNotificationEvent("pulse", "warn", "Chat sessions need attention", "4 sessions are stalled")
	duplicate.Timestamp = "2026-05-08T13:01:00Z"
	dispatcher.Emit(context.Background(), duplicate)

	if len(fake.calls) != 1 {
		t.Fatalf("expected duplicate pulse desktop notify to be suppressed, got %d calls", len(fake.calls))
	}
	view, err := store.history("user", 100)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(view.Items) != 1 || view.Items[0].Occurrences != 2 {
		t.Fatalf("expected grouped pulse notification, got %+v", view.Items)
	}
}

func TestEventStreamHandler_StreamsPublishedNotification(t *testing.T) {
	broker := newEventBroker()
	handler := newEventStreamHandler(broker, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)

	broker.publish(newNotificationEvent("cron", "info", "Cron done", "job complete"))
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "\"type\":\"notification\"") {
		t.Fatalf("expected notification event in SSE body, got %q", body)
	}
	if !strings.Contains(body, "Cron done") {
		t.Fatalf("expected event title in SSE body, got %q", body)
	}

	var statusCode = rec.Result().StatusCode
	if statusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", statusCode)
	}
}

func TestEventStreamHandler_BroadcastsPublishedNotifications(t *testing.T) {
	broker := newEventBroker()
	handler := newEventStreamHandler(broker, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)

	broker.publish(newNotificationEvent("cron", "info", "event A", "job complete"))
	broker.publish(newNotificationEvent("cron", "info", "event B", "job complete"))

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event A") || !strings.Contains(body, "event B") {
		t.Fatalf("expected both events in stream body, got %q", body)
	}
}

func TestNotificationEvent_JSONShape(t *testing.T) {
	evt := newNotificationEvent("heartbeat", "info", "Heartbeat", "ok")
	raw, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "\"type\":\"notification\"") {
		t.Fatalf("unexpected event payload: %s", text)
	}
}

// TestNotificationEvent_ExpressionOmittedByDefault locks in that most
// events (job_id/open_path's existing pattern) never grow an "expression"
// key just because the field exists on the struct (#1192).
func TestNotificationEvent_ExpressionOmittedByDefault(t *testing.T) {
	evt := newNotificationEvent("cron", "info", "Cron done", "job complete")
	raw, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if strings.Contains(string(raw), "expression") {
		t.Fatalf("expected no expression key when unset, got %s", raw)
	}
}

// TestNotificationEvent_ExpressionSerializes is the companion wire format
// (#1192): category companion carries expression, an optional message line
// and an optional session_id, all through the existing field set.
func TestNotificationEvent_ExpressionSerializes(t *testing.T) {
	evt := newNotificationEvent(companionEventCategory, "info", "", "ready when you are")
	evt.Expression = "greeting"
	evt.SessionID = "sess_123"
	raw, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var decoded notificationEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if decoded.Expression != "greeting" {
		t.Fatalf("expected expression greeting, got %q", decoded.Expression)
	}
	if decoded.Category != companionEventCategory {
		t.Fatalf("expected category companion, got %q", decoded.Category)
	}
	if decoded.SessionID != "sess_123" {
		t.Fatalf("expected session_id to round-trip, got %q", decoded.SessionID)
	}
	if decoded.Message != "ready when you are" {
		t.Fatalf("expected message to round-trip, got %q", decoded.Message)
	}
}

// TestNotificationDispatcher_CompanionEventSkipsStoreAndDesktopNotify is
// the core of #1192's server change: a companion event reaches live
// subscribers only — it is not a general alert, so it must not land in
// /v1/events/history, count toward unread, or fire a desktop notification
// (even with zero subscribers, where every other category would).
func TestNotificationDispatcher_CompanionEventSkipsStoreAndDesktopNotify(t *testing.T) {
	store, err := newNotificationStore(t.TempDir()+"/notifications.json", 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}
	broker := newEventBroker()
	_, ch, unsubscribe := broker.subscribe()
	defer unsubscribe()

	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, fake, true, zerolog.New(io.Discard))
	dispatcher.store = store

	evt := newNotificationEvent(companionEventCategory, "info", "", "ready when you are")
	evt.Expression = "greeting"
	evt.SessionID = "sess_123"
	dispatcher.Emit(context.Background(), evt)

	select {
	case received := <-ch:
		if received.Expression != "greeting" {
			t.Fatalf("expected subscriber to receive expression, got %q", received.Expression)
		}
		if received.Category != companionEventCategory {
			t.Fatalf("expected subscriber to receive companion category, got %q", received.Category)
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscriber to receive the companion event")
	}

	if len(fake.calls) != 0 {
		t.Fatalf("expected no desktop notify for a companion event, got %d calls", len(fake.calls))
	}
	view, err := store.history("user", 100)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(view.Items) != 0 {
		t.Fatalf("expected companion event to skip the notification store, got %+v", view.Items)
	}
}

// TestNotificationDispatcher_CompanionEventCategoryIsCaseInsensitive
// guards the EqualFold check against a caller that cases the category
// differently than the companionEventCategory constant.
func TestNotificationDispatcher_CompanionEventCategoryIsCaseInsensitive(t *testing.T) {
	store, err := newNotificationStore(t.TempDir()+"/notifications.json", 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}
	broker := newEventBroker()
	dispatcher := newNotificationDispatcher(broker, &fakeDesktopNotifier{}, true, zerolog.New(io.Discard))
	dispatcher.store = store

	evt := newNotificationEvent("Companion", "info", "", "hi")
	dispatcher.Emit(context.Background(), evt)

	view, err := store.history("user", 100)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(view.Items) != 0 {
		t.Fatalf("expected a differently-cased companion category to still skip the store, got %+v", view.Items)
	}
}

func TestBuildTerminalNotifierArgs_IncludesOpenPath(t *testing.T) {
	evt := newNotificationEvent("cron", "info", "Cron completed", "episode updated")
	evt.JobID = "job_demo"
	evt.OpenPath = "/tmp/cron.md"

	args := buildTerminalNotifierArgs(evt)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-group tars-cron-job-demo") {
		t.Fatalf("expected stable group in args, got %+v", args)
	}
	if !strings.Contains(joined, "-execute open '/tmp/cron.md'") {
		t.Fatalf("expected file open command in args, got %+v", args)
	}
	if strings.Contains(joined, "-sender com.apple.Terminal") {
		t.Fatalf("did not expect sender when click action is configured, got %+v", args)
	}
}

func TestBuildTerminalNotifierArgs_UsesSenderWithoutClickAction(t *testing.T) {
	evt := newNotificationEvent("cron", "info", "Cron completed", "episode updated")
	args := buildTerminalNotifierArgs(evt)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-sender com.apple.Terminal") {
		t.Fatalf("expected sender without click action, got %+v", args)
	}
}

func TestCommandNotifier_RunsConfiguredCommandThroughSystemShell(t *testing.T) {
	notifier := newCommandNotifier("test \"$TARS_NOTIFY_TITLE\" = Cron", zerolog.New(io.Discard))
	err := notifier.Notify(context.Background(), newNotificationEvent("cron", "info", "Cron", "done"))
	if err != nil {
		t.Fatalf("notify command: %v", err)
	}
}

func TestCommandNotifier_NotifyAutoUsesTerminalNotifierPath(t *testing.T) {
	prependFakeExecutable(t, "terminal-notifier", "#!/bin/sh\nexit 0\n")

	notifier := newCommandNotifier("", zerolog.New(io.Discard)).(*commandNotifier)
	err := notifier.notifyAutoForGOOS(context.Background(), newNotificationEvent("cron", "info", "Cron", "done"), "darwin")
	if err != nil {
		t.Fatalf("notify auto darwin terminal-notifier: %v", err)
	}
}

func TestCommandNotifier_NotifyAutoFallsBackToOsascriptPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	scriptPath := writeExecutable(t, dir, "osascript", "#!/bin/sh\nexit 0\n")
	original := notificationAppleScriptPath
	notificationAppleScriptPath = scriptPath
	t.Cleanup(func() { notificationAppleScriptPath = original })

	notifier := newCommandNotifier("", zerolog.New(io.Discard)).(*commandNotifier)
	err := notifier.notifyAutoForGOOS(context.Background(), newNotificationEvent("cron", "info", "Cron", "done"), "darwin")
	if err != nil {
		t.Fatalf("notify auto darwin osascript: %v", err)
	}
}

func TestCommandNotifier_NotifyAutoUsesNotifySendPath(t *testing.T) {
	prependFakeExecutable(t, "notify-send", "#!/bin/sh\nexit 0\n")

	notifier := newCommandNotifier("", zerolog.New(io.Discard)).(*commandNotifier)
	err := notifier.notifyAutoForGOOS(context.Background(), newNotificationEvent("cron", "info", "Cron", "done"), "linux")
	if err != nil {
		t.Fatalf("notify auto linux: %v", err)
	}
}

func prependFakeExecutable(t *testing.T, name string, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := writeExecutable(t, dir, name, content)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func writeExecutable(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}
