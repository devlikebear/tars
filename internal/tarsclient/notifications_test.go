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

func TestNotifySync_NotificationCenterDedupesByIDAndTracksUnread(t *testing.T) {
	center := newNotificationCenter(4)
	center.setReadCursor(0)
	center.add(notificationMessage{ID: 1, Type: "notification", Category: "cron", Title: "a"})
	center.add(notificationMessage{ID: 1, Type: "notification", Category: "cron", Title: "a-dup"})
	center.add(notificationMessage{ID: 2, Type: "notification", Category: "cron", Title: "b"})

	items := center.filtered()
	if len(items) != 2 {
		t.Fatalf("expected deduped length=2, got %d (%+v)", len(items), items)
	}
	if center.unreadCount() != 2 {
		t.Fatalf("expected unread=2, got %d", center.unreadCount())
	}

	center.setReadCursor(2)
	if center.unreadCount() != 0 {
		t.Fatalf("expected unread=0 after read cursor advance, got %d", center.unreadCount())
	}
}
