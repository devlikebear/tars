package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
	"github.com/rs/zerolog"
)

func TestConsolePresenceHandlerRecordsAndRejectsWrongMethod(t *testing.T) {
	store := initiative.OpenPresenceStore(filepath.Join(t.TempDir(), "presence.json"))
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	h := newConsolePresenceAPIHandler(store, func() time.Time { return now }, zerolog.Nop())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/console/presence", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want 204", rec.Code)
	}
	st := store.Current()
	if !st.LastAt.Equal(now) || !st.ArrivedAt.Equal(now) {
		t.Fatalf("state = %+v, want recorded at %v", st, now)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/v1/console/presence", nil))
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", rec2.Code)
	}
}

// TestConsolePresenceHandlerAcceptsNoBody checks that the endpoint never
// expects (or reads) a request body — the server's own clock is the only
// timestamp used, and the console sends no other detail.
func TestConsolePresenceHandlerAcceptsNoBody(t *testing.T) {
	store := initiative.OpenPresenceStore(filepath.Join(t.TempDir(), "presence.json"))
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	h := newConsolePresenceAPIHandler(store, func() time.Time { return now }, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/console/presence", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestConsolePresenceHandlerNilStoreStillAnswers(t *testing.T) {
	h := newConsolePresenceAPIHandler(nil, nil, zerolog.Nop())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/console/presence", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 even with no store wired (setup-only mode)", rec.Code)
	}
}
