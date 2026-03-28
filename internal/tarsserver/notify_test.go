package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestEventStreamHandler_FiltersByProjectID(t *testing.T) {
	broker := newEventBroker()
	handler := newEventStreamHandler(broker, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream?project_id=proj-a", nil)
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

	match := newNotificationEvent("cron", "info", "event A", "job complete")
	match.ProjectID = "proj-a"
	other := newNotificationEvent("cron", "info", "event B", "job complete")
	other.ProjectID = "proj-b"
	broker.publish(match)
	broker.publish(other)

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event A") {
		t.Fatalf("expected matching project event in stream body, got %q", body)
	}
	if strings.Contains(body, "event B") {
		t.Fatalf("did not expect non-matching project event in stream body, got %q", body)
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
