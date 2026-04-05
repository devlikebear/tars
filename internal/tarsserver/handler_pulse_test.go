package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/pulse"
	"github.com/rs/zerolog"
)

type fakePulseRuntime struct {
	snap   pulse.Snapshot
	onRun  func(ctx context.Context) pulse.TickOutcome
	runCnt int
}

func (f *fakePulseRuntime) Snapshot() pulse.Snapshot { return f.snap }
func (f *fakePulseRuntime) RunOnce(ctx context.Context) pulse.TickOutcome {
	f.runCnt++
	if f.onRun != nil {
		return f.onRun(ctx)
	}
	return pulse.TickOutcome{}
}

func serveAndHit(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPulseHandler_StatusReturnsSnapshot(t *testing.T) {
	runtime := &fakePulseRuntime{
		snap: pulse.Snapshot{TotalTicks: 5, TotalNotifies: 2},
	}
	h := newPulseAPIHandler(runtime, pulseConfigView{Enabled: true}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodGet, "/v1/pulse/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got pulse.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TotalTicks != 5 || got.TotalNotifies != 2 {
		t.Errorf("snapshot mismatch: %+v", got)
	}
}

func TestPulseHandler_StatusNilRuntimeReturnsEmpty(t *testing.T) {
	h := newPulseAPIHandler(nil, pulseConfigView{}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodGet, "/v1/pulse/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPulseHandler_StatusRejectsPost(t *testing.T) {
	h := newPulseAPIHandler(&fakePulseRuntime{}, pulseConfigView{}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodPost, "/v1/pulse/status")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestPulseHandler_RunOnceTriggersRuntime(t *testing.T) {
	called := false
	runtime := &fakePulseRuntime{
		onRun: func(ctx context.Context) pulse.TickOutcome {
			called = true
			return pulse.TickOutcome{Skipped: true, SkipReason: "test"}
		},
	}
	h := newPulseAPIHandler(runtime, pulseConfigView{}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodPost, "/v1/pulse/run-once")
	if !called || runtime.runCnt != 1 {
		t.Error("RunOnce was not invoked")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "test") {
		t.Errorf("response missing skip reason: %s", rec.Body.String())
	}
}

func TestPulseHandler_RunOnceNilRuntimeReturns503(t *testing.T) {
	h := newPulseAPIHandler(nil, pulseConfigView{}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodPost, "/v1/pulse/run-once")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestPulseHandler_RunOnceRejectsGet(t *testing.T) {
	h := newPulseAPIHandler(&fakePulseRuntime{}, pulseConfigView{}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodGet, "/v1/pulse/run-once")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestPulseHandler_ConfigReturnsView(t *testing.T) {
	cfg := pulseConfigView{
		Enabled:          true,
		IntervalSeconds:  60,
		TimeoutSeconds:   120,
		ActiveHours:      "00:00-24:00",
		Timezone:         "Local",
		MinSeverity:      "warn",
		AllowedAutofixes: []string{"compress_old_logs"},
	}
	h := newPulseAPIHandler(&fakePulseRuntime{}, cfg, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodGet, "/v1/pulse/config")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got pulseConfigView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.IntervalSeconds != 60 || got.MinSeverity != "warn" || len(got.AllowedAutofixes) != 1 {
		t.Errorf("unexpected config view: %+v", got)
	}
}

func TestPulseHandler_ConfigRejectsPost(t *testing.T) {
	h := newPulseAPIHandler(&fakePulseRuntime{}, pulseConfigView{}, zerolog.Nop())
	rec := serveAndHit(t, h, http.MethodPost, "/v1/pulse/config")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
