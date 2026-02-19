package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestEventStreamClientConsumeOnce_HTTPErrorReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/stream" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "unauthorized",
			"message": "unauthorized",
		})
	}))
	defer server.Close()

	client := eventStreamClient{serverURL: server.URL}
	err := client.consumeOnce(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *apiHTTPError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected apiHTTPError, got %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected status=401, got %d", apiErr.Status)
	}
	if strings.TrimSpace(apiErr.Code) != "unauthorized" {
		t.Fatalf("expected code=unauthorized, got %q", apiErr.Code)
	}
}

func TestEventStreamClientConsume_StopsRetryOnUnauthorized(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/stream" {
			http.NotFound(w, r)
			return
		}
		requests++
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "unauthorized",
			"message": "unauthorized",
		})
	}))
	defer server.Close()

	client := eventStreamClient{serverURL: server.URL}
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		client.consume(ctx, nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("consume should stop quickly on unauthorized")
	}
	if requests != 1 {
		t.Fatalf("expected single request on unauthorized, got %d", requests)
	}
}
