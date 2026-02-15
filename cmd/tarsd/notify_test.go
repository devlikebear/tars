package main

import (
	"context"
	"encoding/json"
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

func TestNotificationDispatcher_UsesDesktopNotifyWithoutSubscribers(t *testing.T) {
	broker := newEventBroker()
	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, fake, true, zerolog.New(io.Discard))

	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "info", "Cron done", "check inbox done"))

	if len(fake.calls) != 1 {
		t.Fatalf("expected desktop notify call when no subscribers, got %d", len(fake.calls))
	}
}

func TestNotificationDispatcher_SkipsDesktopNotifyWithSubscribers(t *testing.T) {
	broker := newEventBroker()
	_, _, unsubscribe := broker.subscribe()
	defer unsubscribe()

	fake := &fakeDesktopNotifier{}
	dispatcher := newNotificationDispatcher(broker, fake, true, zerolog.New(io.Discard))
	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "info", "Cron done", "check inbox done"))

	if len(fake.calls) != 0 {
		t.Fatalf("expected desktop notify to be skipped when subscribers exist, got %d", len(fake.calls))
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
