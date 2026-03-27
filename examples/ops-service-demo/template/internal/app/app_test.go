package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthReflectsFailureModeAndLogsSyntheticProbe(t *testing.T) {
	var logs bytes.Buffer
	now := func() time.Time { return time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC) }
	svc := New(&logs, now)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthy service, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"failure_mode":"none"`) {
		t.Fatalf("expected healthy body, got %s", rec.Body.String())
	}

	if err := svc.SetFailureMode(FailureModeTimeout); err != nil {
		t.Fatalf("set failure mode: %v", err)
	}
	svc.RunSyntheticCheck()

	rec = httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for failure mode, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"failure_mode":"timeout"`) {
		t.Fatalf("expected timeout body, got %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), `"event":"synthetic_probe_failed"`) {
		t.Fatalf("expected synthetic failure log, got %s", logs.String())
	}
}

func TestAdminFailureEndpointsSwitchModes(t *testing.T) {
	var logs bytes.Buffer
	svc := New(&logs, func() time.Time { return time.Unix(0, 0).UTC() })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/failure?mode=http500", nil)
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when setting failure mode, got %d", rec.Code)
	}
	if got := svc.FailureMode(); got != FailureModeHTTP500 {
		t.Fatalf("expected http500 mode, got %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/failure/clear", nil)
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when clearing failure mode, got %d", rec.Code)
	}
	if got := svc.FailureMode(); got != FailureModeNone {
		t.Fatalf("expected cleared mode, got %q", got)
	}
}
