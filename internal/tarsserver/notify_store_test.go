package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestNotificationStore_AppendAndRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.json")
	store, err := newNotificationStore(path, 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}

	for i := 0; i < 1005; i++ {
		evt, err := store.append(newNotificationEvent("cron", "info", "title", "msg"))
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		if evt.ID != int64(i+1) {
			t.Fatalf("expected id=%d, got %d", i+1, evt.ID)
		}
	}

	if _, err := store.markRead("user", 1000); err != nil {
		t.Fatalf("markRead: %v", err)
	}

	restored, err := newNotificationStore(path, 1000)
	if err != nil {
		t.Fatalf("restore newNotificationStore: %v", err)
	}
	snapshot, err := restored.history("user", 1000)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(snapshot.Items) != 1000 {
		t.Fatalf("expected 1000 items, got %d", len(snapshot.Items))
	}
	if snapshot.Items[0].ID != 6 {
		t.Fatalf("expected oldest id=6 after prune, got %d", snapshot.Items[0].ID)
	}
	if snapshot.LastID != 1005 {
		t.Fatalf("expected last_id=1005, got %d", snapshot.LastID)
	}
	if snapshot.ReadCursor != 1000 {
		t.Fatalf("expected read_cursor=1000, got %d", snapshot.ReadCursor)
	}
	if snapshot.UnreadCount != 5 {
		t.Fatalf("expected unread_count=5, got %d", snapshot.UnreadCount)
	}
}

func TestNotificationStore_ReadCursorByRole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.json")
	store, err := newNotificationStore(path, 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := store.append(newNotificationEvent("heartbeat", "info", "hb", "ok")); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	if _, err := store.markRead("user", 2); err != nil {
		t.Fatalf("markRead user: %v", err)
	}

	user, err := store.history("user", 100)
	if err != nil {
		t.Fatalf("history user: %v", err)
	}
	if user.UnreadCount != 1 {
		t.Fatalf("expected user unread=1, got %d", user.UnreadCount)
	}

	admin, err := store.history("admin", 100)
	if err != nil {
		t.Fatalf("history admin: %v", err)
	}
	if admin.UnreadCount != 3 {
		t.Fatalf("expected admin unread=3, got %d", admin.UnreadCount)
	}
}

func TestNotificationStore_HistoryUnreadCountUsesAllRetainedItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.json")
	store, err := newNotificationStore(path, 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}

	for i := 0; i < 4; i++ {
		if _, err := store.append(newNotificationEvent("cron", "info", "title", "msg")); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if _, err := store.markRead("user", 1); err != nil {
		t.Fatalf("markRead: %v", err)
	}

	view, err := store.history("user", 1)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(view.Items) != 1 {
		t.Fatalf("expected paged items=1, got %d", len(view.Items))
	}
	if view.UnreadCount != 3 {
		t.Fatalf("expected unread_count to include all retained items, got %d", view.UnreadCount)
	}
}

func TestEventsAPI_HistoryAndRead(t *testing.T) {
	store, err := newNotificationStore(filepath.Join(t.TempDir(), "notifications.json"), 1000)
	if err != nil {
		t.Fatalf("newNotificationStore: %v", err)
	}
	broker := newEventBroker()
	dispatcher := newNotificationDispatcher(broker, nil, false, zerolog.New(io.Discard))
	dispatcher.store = store
	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "info", "event-1", "hello"))
	dispatcher.Emit(context.Background(), newNotificationEvent("cron", "error", "event-2", "boom"))

	h := newEventsAPIHandler(broker, store, zerolog.New(io.Discard))

	recHistory := httptest.NewRecorder()
	reqHistory := httptest.NewRequest(http.MethodGet, "/v1/events/history?limit=1", nil)
	reqHistory.Header.Set("Tars-Debug-Auth-Role", "user")
	h.ServeHTTP(recHistory, reqHistory)
	if recHistory.Code != http.StatusOK {
		t.Fatalf("history expected 200, got %d body=%s", recHistory.Code, recHistory.Body.String())
	}
	var historyPayload struct {
		Items       []notificationEvent `json:"items"`
		UnreadCount int                 `json:"unread_count"`
		ReadCursor  int64               `json:"read_cursor"`
		LastID      int64               `json:"last_id"`
	}
	if err := json.Unmarshal(recHistory.Body.Bytes(), &historyPayload); err != nil {
		t.Fatalf("decode history payload: %v", err)
	}
	if len(historyPayload.Items) != 1 || historyPayload.Items[0].ID != 2 {
		t.Fatalf("unexpected history items: %+v", historyPayload.Items)
	}
	if historyPayload.UnreadCount != 2 || historyPayload.ReadCursor != 0 || historyPayload.LastID != 2 {
		t.Fatalf("unexpected history counters: %+v", historyPayload)
	}

	readBody, _ := json.Marshal(map[string]any{"last_id": 1})
	recRead := httptest.NewRecorder()
	reqRead := httptest.NewRequest(http.MethodPost, "/v1/events/read", bytes.NewReader(readBody))
	reqRead.Header.Set("Tars-Debug-Auth-Role", "user")
	h.ServeHTTP(recRead, reqRead)
	if recRead.Code != http.StatusOK {
		t.Fatalf("read expected 200, got %d body=%s", recRead.Code, recRead.Body.String())
	}
	var readPayload struct {
		Acknowledged bool  `json:"acknowledged"`
		ReadCursor   int64 `json:"read_cursor"`
		UnreadCount  int   `json:"unread_count"`
	}
	if err := json.Unmarshal(recRead.Body.Bytes(), &readPayload); err != nil {
		t.Fatalf("decode read payload: %v", err)
	}
	if !readPayload.Acknowledged || readPayload.ReadCursor != 1 || readPayload.UnreadCount != 1 {
		t.Fatalf("unexpected read payload: %+v", readPayload)
	}

	recAdmin := httptest.NewRecorder()
	reqAdmin := httptest.NewRequest(http.MethodGet, "/v1/events/history", nil)
	reqAdmin.Header.Set("Tars-Debug-Auth-Role", "admin")
	h.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusOK {
		t.Fatalf("admin history expected 200, got %d body=%s", recAdmin.Code, recAdmin.Body.String())
	}
	var adminPayload struct {
		UnreadCount int `json:"unread_count"`
	}
	if err := json.Unmarshal(recAdmin.Body.Bytes(), &adminPayload); err != nil {
		t.Fatalf("decode admin history payload: %v", err)
	}
	if adminPayload.UnreadCount != 2 {
		t.Fatalf("expected admin unread=2, got %d", adminPayload.UnreadCount)
	}
}
