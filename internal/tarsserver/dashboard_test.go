package tarsserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestProjectDashboardHandler_ProjectStreamEmitsProjectEvents(t *testing.T) {
	broker := newProjectDashboardBroker()
	handler := newProjectDashboardHandler(nil, nil, broker, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/ui/projects/demo/stream", nil)
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

	broker.publish(newProjectDashboardEvent("demo", "activity"))
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"project_id\":\"demo\"") {
		t.Fatalf("expected stream body to include project id, got %q", body)
	}
	if !strings.Contains(body, "\"kind\":\"activity\"") {
		t.Fatalf("expected stream body to include event kind, got %q", body)
	}
}

func TestProjectDashboardHandler_NonStreamPathNotFound(t *testing.T) {
	handler := newProjectDashboardHandler(nil, nil, newProjectDashboardBroker(), zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/projects/demo", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-stream dashboard path, got %d body=%q", rec.Code, rec.Body.String())
	}
}
