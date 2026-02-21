package tarsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
