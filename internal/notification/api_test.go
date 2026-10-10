package notification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "notifications.json"), 0)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestNewStore_RequiresPath(t *testing.T) {
	if _, err := NewStore("  ", 10); err == nil {
		t.Fatal("a store without a path was created")
	}
}

// SetStore is how the server attaches the history store: every event the
// dispatcher emits afterwards is recorded.
func TestDispatcher_SetStoreRecordsEmittedEvents(t *testing.T) {
	store := newTestStore(t)
	dispatcher := NewDispatcher(NewBroker(), nil, false, zerolog.New(io.Discard))
	dispatcher.Emit(context.Background(), NewEvent("cron", "info", "before", "not recorded"))
	dispatcher.SetStore(store)
	dispatcher.Emit(context.Background(), NewEvent("cron", "info", "after", "recorded"))

	view, err := store.history("user", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(view.Items) != 1 || view.Items[0].Title != "after" {
		t.Fatalf("recorded events = %+v", view.Items)
	}
}

func TestEventsAPI_ErrorResponses(t *testing.T) {
	logger := zerolog.New(io.Discard)
	withStore := NewEventsAPIHandler(NewBroker(), newTestStore(t), logger)
	withoutStore := NewEventsAPIHandler(NewBroker(), nil, logger)

	for _, tc := range []struct {
		name    string
		handler http.Handler
		method  string
		target  string
		body    string
		want    int
		wantIn  string
	}{
		{"history without a store", withoutStore, http.MethodGet, "/v1/events/history", "", http.StatusServiceUnavailable, "notification store is not configured"},
		{"read without a store", withoutStore, http.MethodPost, "/v1/events/read", `{"last_id":1}`, http.StatusServiceUnavailable, "notification store is not configured"},
		{"history with a word for a limit", withStore, http.MethodGet, "/v1/events/history?limit=many", "", http.StatusBadRequest, "limit must be a positive integer"},
		{"history with a zero limit", withStore, http.MethodGet, "/v1/events/history?limit=0", "", http.StatusBadRequest, "limit must be a positive integer"},
		{"history with the wrong method", withStore, http.MethodPost, "/v1/events/history", "", http.StatusMethodNotAllowed, "method not allowed"},
		{"read with the wrong method", withStore, http.MethodGet, "/v1/events/read", "", http.StatusMethodNotAllowed, "method not allowed"},
		{"read with a body that is not JSON", withStore, http.MethodPost, "/v1/events/read", "not json", http.StatusBadRequest, "invalid request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body)))
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.wantIn) {
				t.Fatalf("got %d %q, want %d containing %q", rec.Code, rec.Body.String(), tc.want, tc.wantIn)
			}
		})
	}
}

// A pulse notification whose timestamp cannot be read is stored as its own
// entry: without a time there is no window to coalesce it into.
func TestStore_PulseNotificationWithoutATimeIsNotCoalesced(t *testing.T) {
	store := newTestStore(t)
	first := NewEvent("pulse", "warning", "Disk is filling up", "82% used")
	if _, err := store.append(first); err != nil {
		t.Fatalf("append: %v", err)
	}
	undated := NewEvent("pulse", "warning", "Disk is filling up", "83% used")
	undated.Timestamp = "not a time"
	result, err := store.append(undated)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if result.Coalesced {
		t.Fatal("an event without a readable time was coalesced")
	}
	view, err := store.history("user", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(view.Items) != 2 {
		t.Fatalf("stored events = %d, want 2", len(view.Items))
	}
}
