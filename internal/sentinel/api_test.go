package sentinel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newRunningSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(probe.Close)
	cmd, args, env := helperProcess("sleep")
	s := NewSupervisor(Options{
		TargetCommand:      cmd,
		TargetArgs:         args,
		TargetEnv:          env,
		Autostart:          true,
		ProbeURL:           probe.URL,
		ProbeInterval:      25 * time.Millisecond,
		ProbeTimeout:       200 * time.Millisecond,
		ProbeFailThreshold: 3,
		ProbeStartGrace:    750 * time.Millisecond,
		RestartMaxAttempts: 3,
		RestartBackoff:     10 * time.Millisecond,
		RestartBackoffMax:  20 * time.Millisecond,
		RestartCooldown:    250 * time.Millisecond,
		EventBufferSize:    128,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	t.Cleanup(func() {
		closeSupervisor(t, s)
	})
	waitForCondition(t, 2*time.Second, func() bool {
		st := s.Status()
		return st.TargetPID > 0 && st.SupervisionState == StateRunning
	}, "supervisor running for api test")
	return s
}

func TestAPIHandler_StatusAndEvents(t *testing.T) {
	s := newRunningSupervisor(t)
	waitForCondition(t, 2*time.Second, func() bool {
		return s.Status().LastProbeDurationMS > 0
	}, "probe duration telemetry populated")
	h := NewAPIHandler(s)

	statusRec := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodGet, "/v1/sentinel/status", nil)
	h.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var statusPayload Status
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusPayload); err != nil {
		t.Fatalf("decode status payload: %v", err)
	}
	if !statusPayload.Enabled {
		t.Fatalf("expected enabled=true, payload=%+v", statusPayload)
	}
	if statusPayload.TargetPID == 0 {
		t.Fatalf("expected target pid in status payload: %+v", statusPayload)
	}
	if strings.TrimSpace(statusPayload.StartGraceUntil) == "" {
		t.Fatalf("expected start_grace_until telemetry, payload=%+v", statusPayload)
	}
	if statusPayload.LastProbeDurationMS <= 0 {
		t.Fatalf("expected last_probe_duration_ms telemetry, payload=%+v", statusPayload)
	}

	eventsRec := httptest.NewRecorder()
	eventsReq := httptest.NewRequest(http.MethodGet, "/v1/sentinel/events?limit=2", nil)
	h.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected events 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var eventsPayload struct {
		Count  int     `json:"count"`
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &eventsPayload); err != nil {
		t.Fatalf("decode events payload: %v", err)
	}
	if eventsPayload.Count < 1 || len(eventsPayload.Events) < 1 {
		t.Fatalf("expected events payload, got %+v", eventsPayload)
	}
}

func TestAPIHandler_ControlEndpoints(t *testing.T) {
	s := newRunningSupervisor(t)
	h := NewAPIHandler(s)

	pauseRec := httptest.NewRecorder()
	pauseReq := httptest.NewRequest(http.MethodPost, "/v1/sentinel/pause", nil)
	h.ServeHTTP(pauseRec, pauseReq)
	if pauseRec.Code != http.StatusOK {
		t.Fatalf("expected pause 200, got %d body=%s", pauseRec.Code, pauseRec.Body.String())
	}
	if s.Status().SupervisionState != StatePaused {
		t.Fatalf("expected paused state after /pause, got %+v", s.Status())
	}

	resumeRec := httptest.NewRecorder()
	resumeReq := httptest.NewRequest(http.MethodPost, "/v1/sentinel/resume", nil)
	h.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("expected resume 200, got %d body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	waitForCondition(t, 2*time.Second, func() bool {
		st := s.Status()
		return st.SupervisionState == StateRunning
	}, "resume returns to running")

	restartRec := httptest.NewRecorder()
	restartReq := httptest.NewRequest(http.MethodPost, "/v1/sentinel/restart", nil)
	h.ServeHTTP(restartRec, restartReq)
	if restartRec.Code != http.StatusOK {
		t.Fatalf("expected restart 200, got %d body=%s", restartRec.Code, restartRec.Body.String())
	}
	waitForCondition(t, 2*time.Second, func() bool {
		st := s.Status()
		return st.TargetPID > 0 && st.SupervisionState == StateRunning
	}, "restart keeps running state")
}

func TestAPIHandler_ValidationAndMethodGuards(t *testing.T) {
	h := NewAPIHandler(nil)

	missingRec := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodGet, "/v1/sentinel/status", nil)
	h.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when supervisor missing, got %d", missingRec.Code)
	}

	s := newRunningSupervisor(t)
	h = NewAPIHandler(s)

	methodRec := httptest.NewRecorder()
	methodReq := httptest.NewRequest(http.MethodGet, "/v1/sentinel/restart", nil)
	h.ServeHTTP(methodRec, methodReq)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on restart GET, got %d", methodRec.Code)
	}

	limitRec := httptest.NewRecorder()
	limitReq := httptest.NewRequest(http.MethodGet, "/v1/sentinel/events?limit=abc", nil)
	h.ServeHTTP(limitRec, limitReq)
	if limitRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid limit, got %d", limitRec.Code)
	}
}
