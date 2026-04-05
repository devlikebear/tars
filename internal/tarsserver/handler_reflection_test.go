package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/reflection"
	"github.com/rs/zerolog"
)

type fakeReflectionRuntime struct {
	snap   reflection.Snapshot
	onRun  func(ctx context.Context) reflection.RunSummary
	runCnt int
}

func (f *fakeReflectionRuntime) Snapshot() reflection.Snapshot { return f.snap }
func (f *fakeReflectionRuntime) RunOnce(ctx context.Context) reflection.RunSummary {
	f.runCnt++
	if f.onRun != nil {
		return f.onRun(ctx)
	}
	return reflection.RunSummary{Success: true}
}

func hit(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReflectionHandler_StatusReturnsSnapshot(t *testing.T) {
	rt := &fakeReflectionRuntime{snap: reflection.Snapshot{TotalRuns: 7, TotalSuccesses: 5}}
	h := newReflectionAPIHandler(rt, reflectionConfigView{Enabled: true}, zerolog.Nop())
	rec := hit(t, h, http.MethodGet, "/v1/reflection/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var snap reflection.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.TotalRuns != 7 || snap.TotalSuccesses != 5 {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestReflectionHandler_StatusNilRuntime(t *testing.T) {
	h := newReflectionAPIHandler(nil, reflectionConfigView{}, zerolog.Nop())
	rec := hit(t, h, http.MethodGet, "/v1/reflection/status")
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestReflectionHandler_RunOnce(t *testing.T) {
	called := false
	rt := &fakeReflectionRuntime{onRun: func(ctx context.Context) reflection.RunSummary {
		called = true
		return reflection.RunSummary{Success: true, Results: []reflection.JobResult{{Name: "memory", Success: true}}}
	}}
	h := newReflectionAPIHandler(rt, reflectionConfigView{}, zerolog.Nop())
	rec := hit(t, h, http.MethodPost, "/v1/reflection/run-once")
	if !called || rt.runCnt != 1 {
		t.Error("RunOnce not invoked")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestReflectionHandler_RunOnceNilRuntime(t *testing.T) {
	h := newReflectionAPIHandler(nil, reflectionConfigView{}, zerolog.Nop())
	rec := hit(t, h, http.MethodPost, "/v1/reflection/run-once")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestReflectionHandler_MethodNotAllowed(t *testing.T) {
	h := newReflectionAPIHandler(&fakeReflectionRuntime{}, reflectionConfigView{}, zerolog.Nop())
	for _, c := range []struct {
		path   string
		method string
	}{
		{"/v1/reflection/status", http.MethodPost},
		{"/v1/reflection/run-once", http.MethodGet},
		{"/v1/reflection/config", http.MethodPost},
	} {
		rec := hit(t, h, c.method, c.path)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", c.method, c.path, rec.Code)
		}
	}
}

func TestReflectionHandler_ConfigView(t *testing.T) {
	cfg := reflectionConfigView{
		Enabled:             true,
		SleepWindow:         "02:00-05:00",
		Timezone:            "UTC",
		TickIntervalSeconds: 300,
	}
	h := newReflectionAPIHandler(&fakeReflectionRuntime{}, cfg, zerolog.Nop())
	rec := hit(t, h, http.MethodGet, "/v1/reflection/config")
	var got reflectionConfigView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SleepWindow != "02:00-05:00" || got.TickIntervalSeconds != 300 {
		t.Errorf("config view = %+v", got)
	}
}
