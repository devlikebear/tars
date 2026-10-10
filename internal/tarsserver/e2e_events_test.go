//go:build e2e

package tarsserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRegisterE2EEventsRoute_PublishesToStream is the e2e-only publish
// path's own coverage, built only with `-tags e2e` alongside the route it
// tests (#1192). A POST reaches subscribers of the real broker with
// whatever notificationEvent fields it was given (here a companion
// expression/line/session_id), and never a store/desktop-notify side
// effect since it calls broker.publish directly, not dispatcher.Emit.
func TestRegisterE2EEventsRoute_PublishesToStream(t *testing.T) {
	mux := http.NewServeMux()
	broker := newEventBroker()
	registerE2EEventsRoute(mux, e2eRouteDeps{Broker: broker})

	_, ch, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	body := []byte(`{"type":"notification","category":"companion","expression":"greeting","message":"welcome back","session_id":"sess_e2e"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/e2e/events", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	select {
	case evt := <-ch:
		if evt.Expression != "greeting" || evt.Category != "companion" || evt.SessionID != "sess_e2e" {
			t.Fatalf("unexpected event published: %+v", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the posted event to reach the subscriber")
	}
}

// TestRegisterE2EEventsRoute_RejectsNonPost matches requireMethod's usual
// contract elsewhere in this package.
func TestRegisterE2EEventsRoute_RejectsNonPost(t *testing.T) {
	mux := http.NewServeMux()
	broker := newEventBroker()
	registerE2EEventsRoute(mux, e2eRouteDeps{Broker: broker})

	req := httptest.NewRequest(http.MethodGet, "/v1/e2e/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
