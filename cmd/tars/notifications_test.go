package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotificationCenterFilterAndClear(t *testing.T) {
	center := newNotificationCenter(4)
	center.add(notificationMessage{Type: "notification", Category: "cron", Severity: "info", Title: "cron-1"})
	center.add(notificationMessage{Type: "notification", Category: "error", Severity: "error", Title: "err-1"})

	if err := center.setFilter("error"); err != nil {
		t.Fatalf("setFilter: %v", err)
	}
	items := center.filtered()
	if len(items) != 1 || items[0].Title != "err-1" {
		t.Fatalf("unexpected filtered items: %+v", items)
	}

	center.clear()
	if got := len(center.filtered()); got != 0 {
		t.Fatalf("expected empty notifications after clear, got %d", got)
	}
}

func TestEventStreamClientConsumeOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"notification\",\"category\":\"cron\",\"severity\":\"info\",\"title\":\"tick\",\"message\":\"done\",\"timestamp\":\"2026-02-18T12:00:00Z\"}\n\n")
	}))
	defer server.Close()

	client := eventStreamClient{serverURL: server.URL}
	received := make([]notificationMessage, 0, 1)
	err := client.consumeOnce(context.Background(), func(msg notificationMessage) {
		received = append(received, msg)
	})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if len(received) != 1 || received[0].Title != "tick" {
		t.Fatalf("unexpected received events: %+v", received)
	}
}
